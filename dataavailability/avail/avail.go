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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/vedhavyas/go-subkey/v2"

	"github.com/0xPolygon/cdk/dataavailability/avail/availattestation"
	s3_storage_service "github.com/0xPolygon/cdk/dataavailability/avail/s3StorageService"
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
	sdk     avail_sdk.SDK
	acc     subkey.KeyPair
	address string
	appId   int

	httpApi string

	// AvailDA bridge
	bridgeEnabled       bool
	bridgeApi           string
	attestationContract *availattestation.Availattestation
	bridgeTimeout       int

	// S3 Fallback service
	fallbackS3Service *s3_storage_service.S3StorageService
}

func New(l1RPCURL string, availattestationContractAddress common.Address, config Config) (*AvailBackend, error) {

	log.Infof("AvailDAInfo:ℹ️ AvailDA config: ws-api-url:%+v, http-api-url: %+v, app-id: %+v, bridge-enabled:%+v, bridge-api-url: %+v, bridge-timeout: %+v, ", config.WsApiUrl, config.HttpApiUrl, config.AppID, config.BridgeEnabled, config.BridgeApiUrl, config.BridgeTimeout)
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

	var fallbackS3Service *s3_storage_service.S3StorageService
	if config.FallbackS3ServiceConfig.Enable {
		log.Infof("AvailDAInfo:ℹ️ Fallback S3 config: s3-bucket: %+v, region: %+v, object-prefix: %+v, secret-key: %+v, access-key: %+v, ", config.FallbackS3ServiceConfig.Bucket, config.FallbackS3ServiceConfig.Region, config.FallbackS3ServiceConfig.ObjectPrefix, config.FallbackS3ServiceConfig.SecretKey, config.FallbackS3ServiceConfig.AccessKey)
		fallbackS3Service, err = s3_storage_service.NewS3StorageService(config.FallbackS3ServiceConfig)
		if err != nil {
			return nil, fmt.Errorf("AvailDAError: unable to intialize s3 storage service for fallback, %w. %w", err, ErrAvailDAClientInit)
		}
	}

	log.Infof("AvailDAInfo: 🔑 Using KeyringPair with address %v", acc.SS58Address(AvailNetworkID))
	log.Infof("AvailDAInfo:✌️ Avail backend client is created successfully")

	return &AvailBackend{
		sdk:     sdk,
		acc:     acc,
		address: acc.SS58Address(AvailNetworkID),
		appId:   appId,
		httpApi: config.HttpApiUrl,

		bridgeEnabled:       config.BridgeEnabled,
		attestationContract: attestationContract,
		bridgeApi:           config.BridgeApiUrl,
		bridgeTimeout:       config.BridgeTimeout,

		fallbackS3Service: fallbackS3Service,
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

	var resp []byte
	var dataCommitment common.Hash
	if a.bridgeEnabled {
		var input *BridgeAPIResponse
		waitTime := time.Duration(a.bridgeTimeout) * time.Second
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
		resp, err = merkleProofInput.EnodeToBinary()
		if err != nil {
			return nil, fmt.Errorf("cannot encode data:%v", err)
		}
		dataCommitment = merkleProofInput.Leaf
	} else {
		dataCommitment = crypto.Keccak256Hash(sequence)
		var blobPointer BlobPointer = BlobPointer{BLOBPOINTER_VERSION0, txDetails.BlockNumber, txDetails.TxIndex, dataCommitment}
		resp, err = blobPointer.MarshalToBinary()
		if err != nil {
			return nil, fmt.Errorf("cannot encide blobPointer: %v", err)
		}

	}

	// fallback
	if a.fallbackS3Service != nil {
		err := a.fallbackS3Service.Put(ctx, sequence, 0, dataCommitment)
		if err != nil {
			log.Error("AvailDAError: failed to put data on s3 storage service: %w", err)
		} else {
			log.Info("AvailInfo: ✅  Succesfully posted data from Avail S3 using fallbackS3Service")
		}
	}

	return resp, nil
}

func (a *AvailBackend) GetSequence(ctx context.Context, batchHashes []common.Hash, dataAvailabilityMessage []byte) ([][]byte, error) {

	var resp [][]byte
	var blobData []byte
	var blockNumber uint32
	var index uint32
	var indexType IndexType
	var dataCommitment common.Hash

	if a.bridgeEnabled {
		var inp *MerkleProofInput
		inp.DecodeFromBinary(dataAvailabilityMessage)
		attestationData, err := a.attestationContract.Attestations(nil, inp.Leaf)
		if err != nil {
			return nil, fmt.Errorf("cannot get attestation data from contract:%v", err)
		}
		blockNumber = attestationData.BlockNumber
		index = uint32(attestationData.LeafIndex.Uint64())
		indexType = LeafIndex
		dataCommitment = inp.Leaf
	} else {
		var blobPointer BlobPointer
		blobPointer.UnmarshalFromBinary(dataAvailabilityMessage)
		blockNumber = blobPointer.BlockHeight
		index = blobPointer.ExtrinsicIndex
		indexType = TxIndex
		dataCommitment = blobPointer.BlobDataKeccak265H
	}

	if a.fallbackS3Service != nil {
		var err error
		blobData, err = a.fallbackS3Service.GetByHash(ctx, dataCommitment)
		if err != nil {
			log.Info("AvailInfo: ❌  failed to read data from fallback s3 storage, err: %w", err)
		} else {
			log.Info("AvailInfo: ✅  Succesfully fetched data from Avail S3 using fallbackS3Service")
		}
	}

	if len(blobData) == 0 || blobData == nil {
		var err error
		blobData, err = a.getData(blockNumber, index, indexType)
		if err != nil {
			log.Warn("AvailDAError: unable to read data from AvailDA & Fallback s3 storage")
			return nil, fmt.Errorf("cannot get data from block:%v", err)
		}
		log.Infof("AvailDAInfo: ✅ Successfully able to retreive the data from AvailDA")
	}

	unpackedData, err := byteArrayArguments.Unpack(blobData)
	if err != nil {
		return nil, fmt.Errorf("cannot decode data:%v", err)
	}
	var ok bool
	resp, ok = unpackedData[0].([][]byte)
	if !ok {
		return nil, fmt.Errorf("cannot parse data")
	}

	return resp, nil
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

type IndexType string

const (
	LeafIndex IndexType = "leaf"
	TxIndex   IndexType = "tx"
)

func (a *AvailBackend) getData(blockNumber uint32, index uint32, indexType IndexType) ([]byte, error) {
	blockHash, err := a.sdk.Client.BlockHash(blockNumber)
	if err != nil {
		return nil, fmt.Errorf("❎ Cannot get block hash: %w", err)
	}

	block, err := avail_sdk.NewBlock(a.sdk.Client, blockHash)
	if err != nil {
		return nil, fmt.Errorf("❎ Cannot get block: %w", err)
	}

	var blob avail_sdk.DataSubmission

	switch indexType {
	case LeafIndex:
		blobs := block.DataSubmissions(avail_sdk.Filter{})
		if int(index) >= len(blobs) {
			return nil, fmt.Errorf("❎ Unable to retrieve blob at index %d from block %d", index, blockNumber)
		}
		blob = blobs[index]

	case TxIndex:
		blobs := block.DataSubmissions(avail_sdk.Filter{}.WTxIndex(index))
		if len(blobs) == 0 {
			return nil, fmt.Errorf("❎ No blobs found for transaction index %d in block %d", index, blockNumber)
		}
		blob = blobs[0]

	default:
		return nil, fmt.Errorf("❎ Invalid index type: %v", indexType)
	}

	signerAddress, err := primitives.NewAccountIdFromMultiAddress(blob.TxSigner)
	if err != nil {
		log.Warn("AvailDAWarn:‼️ Unable to extract the signer address for the blob")
	}

	log.Info("AvailDAInfo: ✅ Tx batch retrieved from Avail chain",
		" signer: ", signerAddress.ToHuman(),
		", appID: ", blob.AppId,
		", extrinsicHash: ", blob.TxHash,
	)

	return blob.Data, nil
}
