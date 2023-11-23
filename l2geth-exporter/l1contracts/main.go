package l1contracts

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tenderly/optimism/l2geth-exporter/bindings"
)

// CTC interacts with the BVM Canonical Transaction Chain contract
type CTC struct {
	Address common.Address
	Client  *ethclient.Client
}

// SCC interacts with the BVM State Commitment Chain contract
type SCC struct {
	Address common.Address
	Client  *ethclient.Client
}

func (ctc *SCC) GetTotalElements(ctx context.Context) (*big.Int, error) {

	contract, err := bindings.NewCanonicalTransactionChainCaller(ctc.Address, ctc.Client)
	if err != nil {
		return nil, err
	}

	totalElements, err := contract.GetTotalElements(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, err
	}

	return totalElements, nil

}

func (ctc *CTC) GetTotalElements(ctx context.Context) (*big.Int, error) {

	contract, err := bindings.NewCanonicalTransactionChainCaller(ctc.Address, ctc.Client)
	if err != nil {
		return nil, err
	}

	totalElements, err := contract.GetTotalElements(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, err
	}

	return totalElements, nil

}
