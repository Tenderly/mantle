package services

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tenderly/mantle/fraud-proof/bindings"
	"github.com/tenderly/mantle/fraud-proof/proof"
	"github.com/tenderly/mantle/l2geth/core"
	"github.com/tenderly/mantle/l2geth/core/types"
	"github.com/tenderly/mantle/l2geth/log"
)

type BaseService struct {
	Config *Config

	Eth          Backend
	ProofBackend proof.Backend
	Chain        *core.BlockChain
	L1           *ethclient.Client
	TransactOpts *bind.TransactOpts
	Rollup       *bindings.RollupSession
	AssertionMap *bindings.AssertionMapCallerSession

	Ctx    context.Context
	Cancel context.CancelFunc
	Wg     sync.WaitGroup
}

func NewBaseService(eth Backend, proofBackend proof.Backend, cfg *Config, auth *bind.TransactOpts) (*BaseService, error) {
	ctx, cancel := context.WithCancel(context.Background())
	l1, err := ethclient.DialContext(ctx, cfg.L1Endpoint)
	if err != nil {
		cancel()
		return nil, err
	}
	callOpts := bind.CallOpts{
		Pending: true,
		Context: ctx,
	}
	transactOpts := bind.TransactOpts{
		From:    auth.From,
		Signer:  auth.Signer,
		Context: ctx,
	}
	rollup, err := bindings.NewRollup(common.Address(cfg.RollupAddr), l1)
	if err != nil {
		cancel()
		return nil, err
	}
	rollupSession := &bindings.RollupSession{
		Contract:     rollup,
		CallOpts:     callOpts,
		TransactOpts: transactOpts,
	}
	assertionMapAddr, err := rollupSession.Assertions()
	if err != nil {
		cancel()
		return nil, err
	}
	assertionMap, err := bindings.NewAssertionMapCaller(assertionMapAddr, l1)
	if err != nil {
		cancel()
		return nil, err
	}
	assertionMapSession := &bindings.AssertionMapCallerSession{
		Contract: assertionMap,
		CallOpts: callOpts,
	}
	b := &BaseService{
		Config:       cfg,
		Eth:          eth,
		ProofBackend: proofBackend,
		L1:           l1,
		TransactOpts: &transactOpts,
		Rollup:       rollupSession,
		AssertionMap: assertionMapSession,
		Ctx:          ctx,
		Cancel:       cancel,
	}
	if eth != nil {
		b.Chain = eth.BlockChain()
	}
	return b, nil
}

func (b *BaseService) Start(cleanL1, stake bool) *types.Block {
	// Check if we are at genesis
	// TODO: if not, sync from L1
	genesis := b.Eth.BlockChain().CurrentBlock()
	log.Info("Genesis root", "root", genesis.Root())

	if stake {
		// Initial staking
		// TODO: sync L1 staking status
		stakeOpts := b.Rollup.TransactOpts
		isStaked, err := b.Rollup.Contract.IsStaked(&bind.CallOpts{}, stakeOpts.From)
		if err != nil {
			log.Crit("Failed to query stake", "err", err)
		}
		if !isStaked {
			_, err = b.Rollup.Contract.Stake(&stakeOpts, new(big.Int).SetUint64(b.Config.StakeAmount), stakeOpts.From)
			if err != nil {
				log.Crit("Failed to stake", "from", stakeOpts.From.String(), "amount", b.Config.StakeAmount, "err", err)
			}
		}
	}
	return genesis
}
