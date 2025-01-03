package avail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/0xPolygon/cdk/etherman"
	"github.com/0xPolygon/cdk/log"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/0xPolygon/cdk/dataavailability/avail/availattestation"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	gsrpc_types "github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

const (
	AvailMessageHeaderFlag byte = 0x0a
	BridgeApiTimeout            = time.Duration(1200)
	BridgeApiWaitInterval       = time.Duration(420)
	BridgeApiRetryCount         = 10
	VectorXTimeout              = time.Duration(10000)
)

var (
	ErrAvailDAClientInit          = errors.New("unable to initialize to connect with AvailDA")
	ErrBatchSubmitToAvailDAFailed = errors.New("unable to submit batch to AvailDA")
	ErrWrongAvailDAPointer        = errors.New("unable to retrieve batch, wrong blobPointer")
)

type AvailBackend struct {
	api                 *gsrpc.SubstrateAPI
	attestationContract *availattestation.Availattestation
	httpApi             string
	bridgeApi           string
	meta                *gsrpc_types.Metadata
	appId               int
	genesisHash         gsrpc_types.Hash
	rv                  *gsrpc_types.RuntimeVersion
	keyringPair         signature.KeyringPair
	timeout             int
}

func New(l1RPCURL string, availattestationContractAddress common.Address) (*AvailBackend, error) {
	var config Config
	err := config.GetConfig("/app/avail-config.json")
	if err != nil {
		log.Fatalf("cannot get config: %+v", err)
		return nil, err
	}

	ethClient, err := ethclient.Dial(l1RPCURL)
	if err != nil {
		log.Errorf("error connecting to %s: %+v", l1RPCURL, err)
		return nil, err
	}

	log.Infof("AvailDAInfo: 📜 Attestation contract address: %v", availattestationContractAddress)
	attestationContract, err := availattestation.NewAvailattestation(availattestationContractAddress, ethClient)

	if err != nil {
		return nil, err
	}

	api, err := gsrpc.NewSubstrateAPI(config.WsApiUrl)
	if err != nil {
		log.Fatalf("cannot get ws api: %+v", err)
		return nil, err
	}

	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		log.Fatalf("cannot get metadata: %+v", err)
		return nil, err
	}

	appId := 0

	// if app id is greater than 0 then it must be created before submitting data
	if config.AppID != 0 {
		appId = config.AppID
	}

	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		log.Fatalf("cannot get block hash: %+v", err)
		return nil, err
	}

	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		log.Fatalf("cannot get runtime version: %+v", err)
		return nil, err
	}

	keyringPair, err := signature.KeyringPairFromSecret(config.Seed, 42)
	if err != nil {
		log.Fatalf("cannot create keypair: %+v", err)
		return nil, err
	}
	log.Infof("AvailDAInfo: 🔑 Using KeyringPair with address %v", keyringPair.Address)

	return &AvailBackend{
		attestationContract: attestationContract,
		api:                 api,
		httpApi:             config.HttpApiUrl,
		bridgeApi:           config.BridgeApiUrl,
		meta:                meta,
		appId:               appId,
		genesisHash:         genesisHash,
		rv:                  rv,
		keyringPair:         keyringPair,
		timeout:             config.Timeout,
	}, nil
}

func (a *AvailBackend) Init() error {
	return nil
}

func (a *AvailBackend) PostSequenceElderberry(ctx context.Context, batchesData [][]byte) ([]byte, error) {
	return a.PostSequence(ctx, batchesData)
}

func (a *AvailBackend) PostSequenceBanana(ctx context.Context, sequence etherman.SequenceBanana) ([]byte, error) {
	sequenceBatches := make([][]byte, 0, len(sequence.Batches))
	for _, batch := range sequence.Batches {
		sequenceBatches = append(sequenceBatches, batch.L2Data)
	}
	return a.PostSequence(ctx, sequenceBatches)
}

func (a *AvailBackend) PostSequence(ctx context.Context, batchesData [][]byte) ([]byte, error) {
	sequence, err := byteArrayArguments.Pack(batchesData)
	if err != nil {
		return nil, fmt.Errorf("cannot pack data:%w", err)
	}

	log.Infof("AvailDAInfo: ⚡️ Prepared data for Avail:%d bytes", len(sequence))

	blockHash, nonce, err := a.submitData(sequence)
	txIndex, err := getExtrinsicIndex(a.api, blockHash, a.keyringPair.Address, nonce)
	if err != nil {
		return nil, fmt.Errorf("cannot get tx index:%+v", err)
	}

	var input BridgeAPIResponse
	waitTime := BridgeApiWaitInterval * time.Second
	retryCount := BridgeApiRetryCount
	for retryCount > 0 {
		log.Infof("AvailDAInfo: Bridge API URL: %v", fmt.Sprintf("%s/eth/proof/%#x?index=%d", a.bridgeApi, blockHash, txIndex))
		resp, err := http.Get(fmt.Sprintf("%s/eth/proof/%#x?index=%d", a.bridgeApi, blockHash, txIndex))
		if err != nil {
			log.Infof("⏳ Attestation proof RPC errored, waiting...")
		} else {
			if resp.StatusCode == 200 {
				log.Infof("✅ Attestation proof received")
				data, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("cannot read body:%v", err)
				}
				err = json.Unmarshal(data, &input)
				if err != nil {
					return nil, fmt.Errorf("cannot unmarshal data:%v", err)
				}
				break
			}
			defer resp.Body.Close()
		}
		retryCount--
		time.Sleep(waitTime)
	}
	log.Infof("AvailDAInfo: 🔗 Attestation proof received: %+v", input)

	var dataRootProof [][32]byte
	for _, hash := range input.DataRootProof {
		dataRootProof = append(dataRootProof, hash)
	}
	var leafProof [][32]byte
	for _, hash := range input.LeafProof {
		leafProof = append(leafProof, hash)
	}
	merkleProofInput := &MerkleProofInput{
		DataRootProof: dataRootProof,
		LeafProof:     leafProof,
		RangeHash:     input.RangeHash,
		DataRootIndex: input.DataRootIndex,
		BlobRoot:      input.BlobRoot,
		BridgeRoot:    input.BridgeRoot,
		Leaf:          input.Leaf,
		LeafIndex:     input.LeafIndex,
	}
	log.Infof("AvailDAInfo: 🔗 Merkle proof input: %+v", merkleProofInput)
	ret, err := merkleProofInput.EnodeToBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot encode data:%v", err)
	}
	return ret, nil
}

func (a *AvailBackend) GetSequence(ctx context.Context, batchHashes []common.Hash, dataAvailabilityMessage []byte) ([][]byte, error) {

	var inp *MerkleProofInput
	inp.DecodeFromBinary(dataAvailabilityMessage)
	attestationData, err := a.attestationContract.Attestations(nil, inp.Leaf)
	if err != nil {
		return nil, fmt.Errorf("cannot get attestation data from contract:%v", err)
	}
	blobData, err := a.getData(uint64(attestationData.BlockNumber), uint(attestationData.LeafIndex.Int64()))
	if err != nil {
		return nil, fmt.Errorf("cannot get data from block:%v", err)
	}

	unpackedData, err := byteArrayArguments.Unpack(blobData)
	ret, ok := unpackedData[0].([][]byte)
	if !ok {
		return nil, fmt.Errorf("cannot parse data")
	}
	if err != nil {
		return nil, fmt.Errorf("cannot decode data:%v", err)
	}

	return ret, nil
}

func (a *AvailBackend) submitData(sequence []byte) (gsrpc_types.Hash, gsrpc_types.UCompact, error) {
	log.Infof("AvailDAInfo: Account address for batch submission on Avail: %v\n", a.keyringPair.Address)
	newCall, err := gsrpc_types.NewCall(a.meta, "DataAvailability.submit_data", gsrpc_types.NewBytes(sequence))
	if err != nil {
		return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("cannot create new call:%w", err)
	}

	// Create the extrinsic
	ext := gsrpc_types.NewExtrinsic(newCall)

	nonce, err := getAccountNextIndex(a.httpApi, a.keyringPair.Address)
	if err != nil {
		return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("cannot get account next index:%w", err)
	}

	options := gsrpc_types.SignatureOptions{
		BlockHash:          a.genesisHash,
		Era:                gsrpc_types.ExtrinsicEra{IsMortalEra: false},
		GenesisHash:        a.genesisHash,
		Nonce:              nonce,
		SpecVersion:        a.rv.SpecVersion,
		Tip:                gsrpc_types.NewUCompactFromUInt(1000),
		AppID:              gsrpc_types.NewUCompactFromUInt(uint64(a.appId)),
		TransactionVersion: a.rv.TransactionVersion,
	}

	log.Infof("AvailDAInfo: Avail extrinsic options:%+v\n", options)

	err = ext.Sign(a.keyringPair, options)
	if err != nil {
		return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("cannot sign:%w", err)
	}

	// Send the extrinsic
	sub, err := a.api.RPC.Author.SubmitAndWatchExtrinsic(ext)
	if err != nil {
		return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("cannot submit extrinsic:%w", err)
	}

	defer sub.Unsubscribe()
	timeout := time.After(time.Duration(a.timeout) * time.Second)
	var blockHash gsrpc_types.Hash
out:
	for {
		select {
		case status := <-sub.Chan():
			if status.IsInBlock {
				log.Infof("AvailDAInfo: 📥 Submit data extrinsic included in block %v", status.AsInBlock.Hex())
			} else if status.IsFinalized {
				blockHash = status.AsFinalized
				log.Infof("AvailDAInfo: 📥  Submit data extrinsic included in finalized block", "blockHash", blockHash.Hex())
				break out
			} else if status.IsRetracted {
				log.Warn("AvailDAWarn: ✂️  AvailDA transaction got retracted from block", "blockHash", status.AsRetracted.Hex())
			} else if status.IsInvalid {
				return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("❌ Extrinsic invalid")
			}
		case <-timeout:
			return gsrpc_types.Hash{}, gsrpc_types.UCompact{}, fmt.Errorf("⌛️ Timeout of %d seconds reached without getting finalized status for extrinsic", a.timeout)
		}
	}

	log.Infof("AvailDAInfo: ✅ Data submitted by sequencer:%d bytes against AppID %v sent with hash %#x", len(sequence), a.appId, blockHash)
	return blockHash, nonce, nil
}

func (a *AvailBackend) getData(blockNumber uint64, index uint) ([]byte, error) {
	blockHash, err := a.api.RPC.Chain.GetBlockHash(uint64(blockNumber))
	if err != nil {
		return nil, fmt.Errorf("❎ Cannot get block hash:%w", err)
	}

	block, err := a.api.RPC.Chain.GetBlock(blockHash)
	if err != nil {
		return nil, fmt.Errorf("❎ Cannot get block:%w", err)
	}

	var idx uint = 0
	for _, ext := range block.Block.Extrinsics {
		if ext.Method.CallIndex.SectionIndex == 29 && ext.Method.CallIndex.MethodIndex == 1 {
			var availBlob []byte
			err = scale.NewDecoder(bytes.NewReader(ext.Method.Args)).Decode(&availBlob)
			if err != nil {
				return nil, fmt.Errorf("❎ Error while scale decoding blob: %w", err)
			}
			if idx == index {
				return availBlob, nil
			}
			idx++
		}
	}
	return nil, fmt.Errorf("❎ Cannot find data")
}
