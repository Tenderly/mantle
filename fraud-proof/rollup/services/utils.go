package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	ethc "github.com/ethereum/go-ethereum/common"
	"github.com/tenderly/optimism/fraud-proof/bindings"
	"github.com/tenderly/optimism/fraud-proof/proof"
	"github.com/tenderly/optimism/l2geth/common"
	"github.com/tenderly/optimism/l2geth/core/types"
	"github.com/tenderly/optimism/l2geth/log"
)

func SubmitOneStepProof(
	challengeSession *bindings.ChallengeSession,
	proofBackend proof.Backend,
	ctx context.Context,
	state *proof.ExecutionState,
	challengedStepIndex *big.Int,
	prevChallengedSegmentStart *big.Int,
	prevChallengedSegmentLength *big.Int,
) error {
	log.Info("OSP GenerateProof...")
	osp, err := proof.GenerateProof(ctx, proofBackend, state, nil)
	if err != nil {
		log.Error("UNHANDELED: osp generation failed", "err", err)
		return err
	}
	log.Info("OSP GenerateProof success")
	log.Info("OSP BuildVerificationContext...")
	verificationContext, err := BuildVerificationContext(ctx, proofBackend, state)
	if err != nil {
		log.Error("UNHANDELED: osp build verification context failed", "err", err)
		return err
	}

	log.Info("OSP BuildVerificationContext success")
	log.Debug("OSP VerifyOneStepProof...")
	log.Debug("OSP verificationContext: ", "verificationContext", verificationContext)
	log.Debug("OSP VerifierType: ", "VerifierType", uint8(osp.VerifierType))
	log.Debug("OSP encode: ", "osp", osp.Encode())
	log.Debug("challengedStepIndex: ", "challengedStepIndex", challengedStepIndex)
	log.Debug("prevChallengedSegmentStart: ", "prevChallengedSegmentStart", prevChallengedSegmentStart)
	log.Debug("prevChallengedSegmentLength: ", "prevChallengedSegmentLength", prevChallengedSegmentLength)
	_, err = challengeSession.VerifyOneStepProof(
		*verificationContext,
		uint8(osp.VerifierType),
		osp.Encode(),
		challengedStepIndex,
		prevChallengedSegmentStart,
		prevChallengedSegmentLength,
	)
	if err != nil {
		log.Error("OSP verification failed")
		return err
	}
	log.Info("OSP VerifyOneStepProof submitted")
	return nil
}

// Responder -> startStateHash, endStateHash
func RespondBisection(
	b *BaseService,
	challengeSession *bindings.ChallengeSession,
	ev *bindings.ChallengeBisected,
	states []*proof.ExecutionState,
) error {
	var challengedStepIndex = new(big.Int)
	var bisection [3][32]byte
	var challengeIdx uint64
	var newStart uint64
	var newLen uint64 // New segment length

	// Get bisection info from event
	segStart := ev.ChallengedSegmentStart.Uint64()
	segLen := ev.ChallengedSegmentLength.Uint64()

	if segStart+segLen >= uint64(len(states)) {
		log.Error("RespondBisection out of range", "segStart", segStart, "segLen", segLen, "len(states)", len(states))
		return errors.New("RespondBisection out of range")
	}

	startState := states[segStart].Hash()
	midState := MidState(states, segStart, segLen)
	endState := states[segStart+segLen].Hash()
	if segLen >= 3 {
		if !bytes.Equal(midState[:], ev.MidState[:]) {
			newLen = MidLen(segLen)
			newStart = segStart
			bisection[0] = startState
			bisection[1] = MidState(states, newStart, newLen)
			bisection[2] = midState
			challengeIdx = 1
		} else {
			newLen = MidLen(segLen)
			newStart = segStart + MidLenWithMod(segLen)
			bisection[0] = midState
			bisection[1] = MidState(states, newStart, newLen)
			bisection[2] = endState
			challengeIdx = 2
		}
	} else if segLen <= 2 && segLen > 0 {
		var state *proof.ExecutionState
		if !bytes.Equal(startState[:], ev.StartState[:]) {
			log.Error("bisection find different start state")
			state = states[segStart]
			challengedStepIndex.SetUint64(0)
		} else if !bytes.Equal(midState[:], ev.MidState[:]) {
			state = states[segStart+segLen/2+segLen%2]
			challengedStepIndex.SetUint64(1)
		} else if !bytes.Equal(endState[:], ev.EndState[:]) {
			state = states[segStart+segLen]
			challengedStepIndex.SetUint64(2)
		} else {
			return errors.New("RespondBisection can't find state difference")
		}

		// We've reached one step
		err := SubmitOneStepProof(
			challengeSession,
			b.ProofBackend,
			b.Ctx,
			state,
			challengedStepIndex,
			ev.ChallengedSegmentStart,
			ev.ChallengedSegmentLength,
		)
		if err != nil {
			log.Error("UNHANDELED: osp failed", "err", err)
			return err
		}
		return nil
	} else {
		log.Error("RespondBisection segLen in event is illegal")
		return errors.New("RespondBisection segLen in event is illegal")
	}
	log.Info("BisectExecution", "bisection[0]", hex.EncodeToString(bisection[0][:]), "bisection[1]", hex.EncodeToString(bisection[1][:]), "bisection[2]", hex.EncodeToString(bisection[2][:]), "cidx", challengeIdx, "segStart", segStart, "segLen", segLen)
	_, err := challengeSession.BisectExecution(
		bisection,
		new(big.Int).SetUint64(challengeIdx),
		new(big.Int).SetUint64(newStart),
		new(big.Int).SetUint64(newLen),
		ev.ChallengedSegmentStart,
		ev.ChallengedSegmentLength,
	)
	if err != nil {
		log.Error("UNHANDELED: bisection excution failed", "err", err)
		return err
	}
	return nil
}

// MidLen middle index with ceil
func MidLen(segLen uint64) uint64 {
	return segLen / 2
}

// MidLenWithMod middle index with ceil
func MidLenWithMod(segLen uint64) uint64 {
	return segLen/2 + segLen%2
}

// MidState mid-states with floor index
func MidState(states []*proof.ExecutionState, segStart, segLen uint64) common.Hash {
	return states[segStart+MidLenWithMod(segLen)].Hash()
}

func BuildVerificationContext(ctx context.Context, proofBackend proof.Backend, state *proof.ExecutionState) (*bindings.VerificationContextContext, error) {
	var evmTx bindings.EVMTypesLibTransaction
	var tx *types.Transaction
	var header *types.Header
	var err error
	// get block
	if state != nil && state.Block != nil {
		header = state.Block.Header()
	} else {
		return nil, fmt.Errorf("get nil block from ExecutionState status")
	}
	// get transaction
	if state != nil && state.Block.Transactions() != nil {
		txs := state.Block.Transactions()
		if uint64(len(txs)) < state.TransactionIdx+1 {
			return nil, fmt.Errorf("get transaction index from ExecutionState out of range")
		}
		tx = state.Block.Transactions()[state.TransactionIdx]
	} else {
		return nil, fmt.Errorf("get nil transactions from ExecutionState status")
	}
	// build EVMTypesLibTransaction
	var txOrigin common.Address
	evmTx.Nonce = tx.Nonce()
	evmTx.GasPrice = tx.GasPrice()
	evmTx.To = ethc.Address(*tx.To())
	evmTx.Value = tx.Value()
	evmTx.Data = tx.Data()
	if tx.QueueOrigin() == types.QueueOriginSequencer {
		evmTx.V, evmTx.R, evmTx.S = tx.RawSignatureValues()
		signer := types.NewEIP155Signer(tx.ChainId())
		txOrigin, err = types.Sender(signer, tx)
		if err != nil {
			return nil, err
		}
	} else {
		evmTx.V = big.NewInt(0)
		evmTx.R = big.NewInt(0)
		evmTx.S = big.NewInt(0)
		txOrigin = common.BigToAddress(common.Big0)
	}
	return &bindings.VerificationContextContext{
		Coinbase:    ethc.Address(header.Coinbase),
		Timestamp:   new(big.Int).SetUint64(tx.L1Timestamp()),
		Number:      header.Number,
		Origin:      ethc.Address(txOrigin),
		Transaction: evmTx,
		InputRoot:   [32]byte{0},
		TxHash:      tx.Hash(),
	}, nil
}
