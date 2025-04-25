package avail

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

var unit8Type = abi.Type{T: abi.UintTy, Size: 8}
var byte32Type = abi.Type{T: abi.FixedBytesTy, Size: 32}
var uint32Type = abi.Type{Size: 32, T: abi.UintTy}
var stringType = abi.Type{T: abi.StringTy}
var byte32ArrayType = abi.Type{T: abi.SliceTy, Elem: &abi.Type{T: abi.FixedBytesTy, Size: 32}}
var uint256Type = abi.Type{Size: 256, T: abi.UintTy}
var byteArrayType = abi.Type{T: abi.SliceTy, Elem: &abi.Type{T: abi.BytesTy}} // Type for bytes[]

var byteArrayArguments = abi.Arguments{{Type: byteArrayType}}

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

var merkleProofInputType = abi.Type{T: abi.TupleTy, TupleType: reflect.TypeOf(MerkleProofInput{}), TupleElems: []*abi.Type{&byte32ArrayType, &byte32ArrayType, &byte32Type, &uint256Type, &byte32Type, &byte32Type, &byte32Type, &uint256Type}, TupleRawNames: []string{"dataRootProof", "leafProof", "rangeHash", "dataRootIndex", "blobRoot", "bridgeRoot", "leaf", "leafIndex"}}
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

// BlobPointer version
const (
	BLOBPOINTER_VERSION0 = 0x00
	BLOBPOINTER_VERSION1 = 0x01
	BLOBPOINTER_VERSION2 = 0x02
	BLOBPOINTER_VERSION3 = 0x03
	BLOBPOINTER_VERSION4 = 0x04
)

// BlobPointer contains the reference to the data blob on Avail
type BlobPointer struct {
	Version            uint8
	BlockHeight        uint32      // Block height for avail chain in which data in being included
	ExtrinsicIndex     uint32      // extrinsic index in the block height
	BlobDataKeccak265H common.Hash // Keccak256(blobData) to verify the originality of proof (it will work as preimage of the commitment)
}

var blobPointerArguments = abi.Arguments{
	{Type: unit8Type}, {Type: uint32Type}, {Type: uint32Type}, {Type: byte32Type},
}

func (b *BlobPointer) MarshalToBinary() ([]byte, error) {
	packedData, err := blobPointerArguments.PackValues([]interface{}{b.Version, b.BlockHeight, b.ExtrinsicIndex, b.BlobDataKeccak265H})
	if err != nil {
		return []byte{}, fmt.Errorf("unable to covert the blobPointer into array of bytes and getting error:%w", err)
	}
	return packedData, nil
}

func (b *BlobPointer) UnmarshalFromBinary(data []byte) error {
	unpackedData, err := blobPointerArguments.UnpackValues(data)
	if err != nil {
		return fmt.Errorf("unable to covert the data bytes into blobPointer and getting error:%w", err)
	}
	b.Version = unpackedData[0].(uint8)
	b.BlockHeight = unpackedData[1].(uint32)
	b.ExtrinsicIndex = unpackedData[2].(uint32)
	b.BlobDataKeccak265H = unpackedData[3].([32]uint8)
	return nil
}

// Method to convert BlobPointer to string
func (bp *BlobPointer) String() string {
	return fmt.Sprintf(
		"BlockHeight: %d,  ExtrinsicIndex: %d,  BlobDataKeccak265H: %s",
		bp.BlockHeight,
		bp.ExtrinsicIndex,
		bp.BlobDataKeccak265H.Hex(),
	)
}
