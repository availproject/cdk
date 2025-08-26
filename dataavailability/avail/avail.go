package avail

import (
	"context"

	"github.com/0xPolygon/cdk/etherman"
	"github.com/availproject/cdk-avail-da-server/lib/avail"
	"github.com/ethereum/go-ethereum/common"
)

type Backend struct {
	*avail.AvailBackend
}

func New(l1RPCURL string, contractAddr common.Address, config avail.Config) (*Backend, error) {
	backend, err := avail.New(l1RPCURL, contractAddr, config)
	if err != nil {
		return nil, err
	}
	return &Backend{backend}, nil
}

func (a *Backend) PostSequenceElderberry(ctx context.Context, batchesData [][]byte) ([]byte, error) {
	return a.AvailBackend.PostSequence(ctx, batchesData)
}

func (a *Backend) PostSequenceBanana(ctx context.Context, sequence etherman.SequenceBanana) ([]byte, error) {
	batchesData := make([][]byte, 0, len(sequence.Batches))
	for _, batch := range sequence.Batches {
		batchesData = append(batchesData, batch.L2Data)
	}
	return a.AvailBackend.PostSequence(ctx, batchesData)
}

// You can directly forward GetSequence if you want, since AvailBackend already has it
func (a *Backend) GetSequence(ctx context.Context, batchHashes []common.Hash, dataAvailabilityMessage []byte) ([][]byte, error) {
	return a.AvailBackend.GetSequence(ctx, batchHashes, dataAvailabilityMessage)
}
