package avail

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/0xPolygon/cdk/log"
	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	gsrpc_types "github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

func TestAvailDASubmitData(t *testing.T) {
	randomBytes, err := GenerateRandomBytes(32) // Change the size as needed
	if err != nil {
		log.Fatalf("Error generating random bytes:", err)
	}

	var config Config
	err = config.GetConfig("./avail-config.json")
	if err != nil {
		log.Fatalf("cannot get config: %+v", err)
	}

	log.Info("AvailDAInfo: Config: ", config)

	api, err := gsrpc.NewSubstrateAPI(config.WsApiUrl)
	if err != nil {
		log.Fatalf("cannot get ws api: %+v", err)
	}

	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		log.Fatalf("cannot get metadata: %+v", err)
	}

	appId := 0

	// if app id is greater than 0 then it must be created before submitting data
	if config.AppID != 0 {
		appId = config.AppID
	}

	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		log.Fatalf("cannot get block hash: %+v", err)
	}

	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		log.Fatalf("cannot get runtime version: %+v", err)
	}

	keyringPair, err := signature.KeyringPairFromSecret(config.Seed, 42)
	if err != nil {
		log.Fatalf("cannot create keypair: %+v", err)
	}
	key, err := gsrpc_types.CreateStorageKey(meta, "System", "Account", keyringPair.PublicKey)
	if err != nil {
		fmt.Errorf("AvailDAError: ⚠️ cannot create storage key, %w. %w", err, ErrAvailDAClientInit)
	}
	log.Infof("AvailDAInfo: 🔑 Using KeyringPair with address", keyringPair.Address)

	availBackend := AvailBackend{api, nil, "", "", meta, appId, genesisHash, rv, keyringPair, key, config.Timeout}
	blockHash, nonce, err := availBackend.submitData(randomBytes)

	log.Infof("AvailDA Result: BlockHash %+v, nonce %+v", blockHash, nonce)
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
