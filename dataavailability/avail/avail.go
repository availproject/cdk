package avail

import (
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
	"github.com/vedhavyas/go-subkey"

	"github.com/0xPolygon/cdk/dataavailability/avail/availattestation"
	"github.com/availproject/avail-go-sdk/primitives"
	avail_sdk "github.com/availproject/avail-go-sdk/sdk"
)

const (
	AvailMessageHeaderFlag byte = 0x0a
	AvailNetworkID              = 42
	BridgeApiTimeout            = time.Duration(1200)
	AvailRPCTimeout             = time.Duration(120)
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
	sdk                 avail_sdk.SDK
	acc                 subkey.KeyPair
	address             string
	appId               int
	attestationContract *availattestation.Availattestation
	httpApi             string
	bridgeApi           string
	timeout             int
}

func New(l1RPCURL string, availattestationContractAddress common.Address, config Config) (*AvailBackend, error) {

	log.Infof("AvailDAInfo:ℹ️ AvailDA config: ws-api-url:%+v, http-api-url: %+v, bridge-api-url: %+v, app-id: %+v, timeout: %+v", config.WsApiUrl, config.HttpApiUrl, config.BridgeApiUrl, config.AppID, config.Timeout)
	ethClient, err := ethclient.Dial(l1RPCURL)
	if err != nil {
		log.Errorf("AvailDAError: ⚠️ error connecting to %s: %+v", l1RPCURL, err)
		return nil, err
	}

	log.Infof("AvailDAInfo: 📜 Attestation contract address: %v", availattestationContractAddress)
	attestationContract, err := availattestation.NewAvailattestation(availattestationContractAddress, ethClient)
	if err != nil {
		return nil, err
	}

	sdk, err := avail_sdk.NewSDK(config.HttpApiUrl)
	if err != nil {
		log.Errorf("AvailDAError: ⚠️ error connecting to %s: %+v", config.HttpApiUrl, err)
		return nil, err
	}

	appId := 0

	// if app id is greater than 0 then it must be created before submitting data
	if config.AppID != 0 {
		appId = config.AppID
	}

	acc, err := avail_sdk.Account.NewKeyPair(config.Seed)
	if err != nil {
		log.Errorf("AvailDAError: ⚠️ unable to generate keypair from given seed")
	}

	log.Infof("AvailDAInfo: 🔑 Using KeyringPair with address %v", acc.SS58Address(AvailNetworkID))

	return &AvailBackend{
		sdk:                 sdk,
		acc:                 acc,
		address:             acc.SS58Address(AvailNetworkID),
		appId:               appId,
		attestationContract: attestationContract,
		httpApi:             config.HttpApiUrl,
		bridgeApi:           config.BridgeApiUrl,
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

	log.Infof("AvailDAInfo: ⚡️ Prepared data for Avail: %d bytes", len(sequence))

	txDetails, err := a.submitData(sequence)
	if err != nil {
		return nil, fmt.Errorf("cannot submit data:%+v", err)
	}

	var input *BridgeAPIResponse
	waitTime := time.Duration(a.timeout) * time.Second
	retryCount := BridgeApiRetryCount
	for retryCount > 0 {
		log.Infof("AvailDAInfo: ℹ️ Bridge API URL: %v", fmt.Sprintf("%s/eth/proof/%s?index=%d", a.bridgeApi, txDetails.BlockHash.String(), txDetails.TxIndex))
		resp, err := http.Get(fmt.Sprintf("%s/eth/proof/%s?index=%d", a.bridgeApi, txDetails.BlockHash.String(), txDetails.TxIndex))
		if err == nil && resp.StatusCode == 200 {
			log.Infof("AvailDAInfo: ✅ Attestation proof received")
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("cannot read body:%v", err)
			}
			input = &BridgeAPIResponse{}
			err = json.Unmarshal(data, input)
			if err != nil {
				return nil, fmt.Errorf("cannot unmarshal data:%v", err)
			}
			break

		}
		log.Infof("AvailDAWarn: ⏳ Attestation proof RPC errored, response code: %v, retry count left: %v, retrying in %v", resp.StatusCode, retryCount, waitTime)

		defer resp.Body.Close()

		retryCount--
		time.Sleep(waitTime)
	}

	if input == nil {
		return nil, fmt.Errorf("didn't get any proof from bridge api:%+v", err)
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

func (a *AvailBackend) submitData(sequence []byte) (avail_sdk.TransactionDetails, error) {

	// Transaction will be signed, sent, and watched
	// If the transaction was dropped or never executed, the system will retry it
	// for 2 more times using the same nonce and app id.
	//
	// Waits for finalization to finalize the transaction.
	tx := a.sdk.Tx.DataAvailability.SubmitData(sequence)
	txDetails, err := tx.ExecuteAndWatchFinalization(a.acc, avail_sdk.NewTransactionOptions().WithAppId(uint32(a.appId)))
	if err != nil {
		return avail_sdk.TransactionDetails{}, fmt.Errorf("⚠️ extrinsic got rejected from avail chain, %w", err)
	}

	// Returns None if there was no way to determine the
	// success status of a transaction. Otherwise it returns
	// true or false.
	status := txDetails.IsSuccessful().UnsafeUnwrap()
	if status != true {
		return avail_sdk.TransactionDetails{}, fmt.Errorf("⚠️ extrinsic got failed while execution on avail chain, status: %v", status)
	}

	log.Info("AvailDAInfo: ✅  Tx batch is got included in Avail chain, ", "address: ", a.address, ", appID: ", a.appId, ", block_number: ", txDetails.BlockNumber, ", block_hash: ", txDetails.BlockHash, ", tx_index: ", txDetails.TxIndex)
	return txDetails, nil
}

func (a *AvailBackend) getData(blockNumber uint64, index uint) ([]byte, error) {

	blockHash, err := a.sdk.Client.BlockHash(uint32(blockNumber))
	if err != nil {
		return nil, fmt.Errorf("❎ Cannot get block hash:%w", err)
	}

	block, err := avail_sdk.NewBlock(a.sdk.Client, blockHash)
	if err != nil {
		return nil, fmt.Errorf("❎ Cannot get block:%w", err)
	}

	// All Block Blobs
	blobs := block.DataSubmissions(avail_sdk.Filter{})
	blob := blobs[index]

	signerAddress, err := primitives.NewAccountIdFromMultiAddress(blob.TxSigner)
	if err != nil {
		log.Warn("AvailDAWarn:‼️ unable to extract the signer address for the blob")
	}
	appId := blob.AppId
	log.Info("AvailDAInfo: ✅  Tx batch is got retreived from Avail chain, ", "signer: ", signerAddress.ToHuman(), ", appID: ", appId, ", extrinsicHash: ", blob.TxHash)

	return blob.Data, nil
}
