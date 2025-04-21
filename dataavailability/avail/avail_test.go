package avail

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/0xPolygon/cdk/log"
	avail_sdk "github.com/availproject/avail-go-sdk/sdk"
)

func TestAvailDASubmitData(t *testing.T) {
	randomBytes, err := GenerateRandomBytes(32) // Change the size as needed
	if err != nil {
		log.Fatalf("Error generating random bytes: ", err)
	}

	var config Config
	err = config.GetConfig("./avail-config.json")
	if err != nil {
		log.Fatalf("cannot get config: %+v", err)
	}

	log.Info("AvailDAInfo: Config: ", config)

	sdk, err := avail_sdk.NewSDK(config.HttpApiUrl)
	if err != nil {
		log.Fatalf("AvailDAError: ⚠️ error connecting to %s: %+v", config.HttpApiUrl, err)
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

	log.Infof("AvailDAInfo: 🔑 Using KeyringPair with address ", acc.SS58Address(AvailNetworkID))

	availBackend := AvailBackend{sdk, acc, acc.SS58Address(AvailNetworkID), appId, nil, config.HttpApiUrl, config.BridgeApiUrl, config.Timeout}
	txDetails, err := availBackend.submitData(randomBytes)
	if err != nil {
		log.Fatalf("cannot submit data:%+v", err)
	}
	log.Info("AvailDAInfo: ✅  Tx batch is got included in Avail chain, ", "address: ", acc.SS58Address(AvailNetworkID), ", appID: ", appId, ", block_number: ", txDetails.BlockNumber, ", block_hash: ", txDetails.BlockHash, ", tx_index: ", txDetails.TxIndex)
	log.Infof("/eth/proof/%s?index=%d", txDetails.BlockHash.String(), txDetails.TxIndex)
}

func TestAvailDAGetData(t *testing.T) {
	blockNumber := 1315421
	leafIndex := 3

	var config Config
	err := config.GetConfig("./avail-config.json")
	if err != nil {
		log.Fatalf("cannot get config: %+v", err)
	}

	log.Info("AvailDAInfo: Config: ", config)

	sdk, err := avail_sdk.NewSDK(config.HttpApiUrl)
	if err != nil {
		log.Fatalf("AvailDAError: ⚠️ error connecting to %s: %+v", config.HttpApiUrl, err)
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

	log.Infof("AvailDAInfo: 🔑 Using KeyringPair with address ", acc.SS58Address(AvailNetworkID))
	availBackend := AvailBackend{sdk, acc, acc.SS58Address(AvailNetworkID), appId, nil, config.HttpApiUrl, config.BridgeApiUrl, config.Timeout}
	data, err := availBackend.getData(uint64(blockNumber), uint(leafIndex))
	log.Info("AvailDAInfo: Data: ", data)
}

func TestAvailDAMerkleProofInputEncodeToBinary(t *testing.T) {
	merkleProofInput := MerkleProofInput{
		DataRootProof: [][32]byte{
			{173, 50, 40, 182, 118, 247, 211, 205, 66, 132, 165, 68, 63, 23, 241, 150, 43, 54, 228, 145, 179, 10, 64, 178, 64, 88, 73, 229, 151, 186, 95, 181},
			{81, 248, 78, 114, 121, 205, 246, 172, 184, 26, 247, 122, 236, 100, 246, 24, 247, 16, 41, 183, 217, 198, 211, 124, 3, 92, 55, 19, 78, 81, 122, 242},
			{235, 31, 119, 9, 81, 32, 9, 172, 32, 21, 208, 151, 17, 157, 133, 56, 163, 15, 209, 34, 211, 249, 46, 126, 7, 208, 52, 38, 127, 62, 101, 18},
		},
		LeafProof:     [][32]byte{}, // Empty
		RangeHash:     [32]byte{74, 2, 48, 0, 144, 77, 206, 176, 198, 200, 82, 99, 32, 54, 32, 242, 123, 95, 90, 254, 8, 215, 19, 15, 109, 54, 74, 220, 219, 130, 98, 102},
		DataRootIndex: big.NewInt(24),
		BlobRoot:      [32]byte{227, 61, 198, 14, 188, 123, 255, 156, 231, 33, 32, 207, 108, 158, 140, 142, 12, 196, 230, 137, 56, 41, 122, 229, 61, 0, 206, 169, 222, 15, 138, 196},
		BridgeRoot:    [32]byte{}, // Empty
		Leaf:          [32]byte{204, 60, 53, 158, 101, 234, 176, 102, 163, 244, 80, 224, 228, 98, 240, 37, 42, 215, 226, 82, 233, 176, 242, 242, 100, 92, 147, 96, 170, 10, 232, 43},
		LeafIndex:     big.NewInt(0),
	}

	encodedData, err := merkleProofInput.EnodeToBinary()
	if err != nil {
		t.Fatalf("Failed to encode MerkleProofInput: %v", err)
	}

	t.Logf("Encoded MerkleProofInput: %x", encodedData) // Logs binary output
}

// GenerateRandomBytes returns a random byte slice of the given size
func GenerateRandomBytes(size int) ([]byte, error) {
	b := make([]byte, size)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}
