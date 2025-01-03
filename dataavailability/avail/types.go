package avail

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type MerkleProofInput struct {
	DataRootProof [][32]byte `abi:"dataRootProof"`
	LeafProof     [][32]byte `abi:"leafProof"`
	RangeHash     [32]byte   `abi:"rangeHash"`
	DataRootIndex *big.Int   `abi:"dataRootIndex"`
	BlobRoot      [32]byte   `abi:"blobRoot"`
	BridgeRoot    [32]byte   `abi:"bridgeRoot"`
	Leaf          [32]byte   `abi:"leaf"`
	LeafIndex     *big.Int   `abi:"leafIndex"`
}

type BridgeAPIResponse struct {
	BlobRoot           common.Hash   `json:"blobRoot"`
	BlockHash          common.Hash   `json:"blockHash"`
	BridgeRoot         common.Hash   `json:"bridgeRoot"`
	DataRoot           common.Hash   `json:"dataRoot"`
	DataRootIndex      *big.Int      `json:"dataRootIndex"`
	DataRootCommitment common.Hash   `json:"dataRootCommitment"`
	DataRootProof      []common.Hash `json:"dataRootProof"`
	Leaf               common.Hash   `json:"leaf"`
	LeafIndex          *big.Int      `json:"leafIndex"`
	LeafProof          []common.Hash `json:"leafProof"`
	RangeHash          common.Hash   `json:"rangeHash"`
}

type AccountNextIndexRPCResponse struct {
	Result uint `json:"result"`
}

type DataProofRPCResponse struct {
	Result DataProof `json:"result"`
}

type DataProof struct {
	Root           string   `json:"root"`
	Proof          []string `json:"proof"`
	NumberOfLeaves uint     `json:"numberOfLeaves"`
	LeafIndex      uint     `json:"leafIndex"`
	Leaf           string   `json:"leaf"`
}

var unit8Type = abi.Type{T: abi.UintTy, Size: 8}
var byte32Type = abi.Type{T: abi.FixedBytesTy, Size: 32}
var uint32Type = abi.Type{Size: 32, T: abi.UintTy}
var stringType = abi.Type{T: abi.StringTy}
var byte32ArrayType = abi.Type{T: abi.SliceTy, Elem: &abi.Type{T: abi.FixedBytesTy, Size: 32}}
var uint64Type = abi.Type{Size: 64, T: abi.UintTy}

var byteArrayType = abi.Type{T: abi.SliceTy, Elem: &abi.Type{T: abi.BytesTy}} // Type for bytes[]
var merkleProofInputType = abi.Type{T: abi.TupleTy, TupleType: reflect.TypeOf(MerkleProofInput{}), TupleElems: []*abi.Type{&byte32ArrayType, &byte32ArrayType, &byte32Type, &uint64Type, &byte32Type, &byte32Type, &byte32Type, &uint64Type}, TupleRawNames: []string{"dataRootProof", "leafProof", "rangeHash", "dataRootIndex", "blobRoot", "bridgeRoot", "leaf", "leafIndex"}}

var byteArrayArguments = abi.Arguments{{Type: byteArrayType}}
var merkleProofInputArguments = abi.Arguments{
	{Type: merkleProofInputType},
}

func (m *MerkleProofInput) EnodeToBinary() ([]byte, error) {
	return merkleProofInputArguments.Pack(m)
}

func (m *MerkleProofInput) DecodeFromBinary(data []byte) error {
	unpackedData, err := merkleProofInputArguments.Unpack(data)
	if err != nil {
		return fmt.Errorf("unable to convert the data bytes to merkleProofInput. error:%w", err)
	}

	m = unpackedData[0].(*MerkleProofInput)
	return nil
}
