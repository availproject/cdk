package avail

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	gsrpc_types "github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/vedhavyas/go-subkey"
)

func getAccountNextIndex(httpApi string, address string) (gsrpc_types.UCompact, error) {
	resp, err := http.Post(httpApi, "application/json", strings.NewReader(fmt.Sprintf("{\"id\":1,\"jsonrpc\":\"2.0\",\"method\":\"system_accountNextIndex\",\"params\":[\"%v\"]}", address)))
	if err != nil {
		return gsrpc_types.NewUCompactFromUInt(0), fmt.Errorf("cannot post query request:%v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)

	if err != nil {
		return gsrpc_types.NewUCompactFromUInt(0), fmt.Errorf("cannot read body:%v", err)
	}

	var accountNextIndex AccountNextIndexRPCResponse
	json.Unmarshal(data, &accountNextIndex)

	return gsrpc_types.NewUCompactFromUInt(uint64(accountNextIndex.Result)), nil
}

func getExtrinsicIndex(api *gsrpc.SubstrateAPI, blockHash gsrpc_types.Hash, address string, nonce gsrpc_types.UCompact) (int, error) {
	// Fetching block based on block hash
	avail_blk, err := api.RPC.Chain.GetBlock(blockHash)
	if err != nil {
		return -1, fmt.Errorf("❌ cannot get block for hash:%v and getting error:%w", blockHash.Hex(), err)
	}

	// Extracting the required extrinsic according to the reference
	for i, ext := range avail_blk.Block.Extrinsics {
		// Extracting sender address for extrinsic
		ext_Addr := subkey.SS58Encode(ext.Signature.Signer.AsID.ToBytes(), 42)
		if ext_Addr == address && ext.Signature.Nonce.Int64() == nonce.Int64() {
			return i, nil
		}
	}
	return -1, fmt.Errorf("❌ unable to find any extrinsic in block %v, from address %v with nonce %v", blockHash, address, nonce)
}
