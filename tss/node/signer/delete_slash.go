package signer

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/tenderly/mantle/l2geth/log"
	"github.com/tenderly/mantle/tss/slash"

	tss "github.com/tenderly/mantle/tss/common"
)

func (p *Processor) deleteSlashing() {
	queryTicker := time.NewTicker(p.taskInterval)
	for {
		signingInfos := p.nodeStore.ListSlashingInfo()
		for _, si := range signingInfos {
			log.Info("start to handleSlashing", "address", si.Address.String(), "batch_index", si.BatchIndex, "slash_type", si.SlashType, "election id", si.ElectionId)
			p.handleSlashing(si)
		}
		select {
		case <-p.stopChan:
			return
		case <-queryTicker.C:
		}
	}
}

func (p *Processor) handleSlashing(si slash.SlashingInfo) {
	currentBlockNumber, err := p.l1Client.BlockNumber(context.Background())
	if err != nil {
		log.Error("failed to query block number", "err", err)
		return
	}
	found, err := p.tssStakingSlashingCaller.GetSlashRecord(&bind.CallOpts{BlockNumber: new(big.Int).SetUint64(currentBlockNumber - uint64(p.l1ConfirmBlocks))}, new(big.Int).SetUint64(si.BatchIndex), si.Address)
	if err != nil {
		log.Error("failed to GetSlashRecord", "err", err)
		return
	}
	if found { // is submitted to ethereum
		p.nodeStore.RemoveSlashingInfo(si.Address, si.BatchIndex)
		return
	}

	unJailMembers, err := p.tssGroupManagerCaller.GetTssGroupUnJailMembers(&bind.CallOpts{BlockNumber: new(big.Int).SetUint64(currentBlockNumber - uint64(p.l1ConfirmBlocks))})
	if err != nil {
		log.Error("failed to GetTssGroupUnJailMembers", "err", err)
		return
	}
	if !tss.IsAddrExist(unJailMembers, si.Address) {
		log.Warn("can not slash the address are not unJailed", "address", si.Address.String())
		p.nodeStore.RemoveSlashingInfo(si.Address, si.BatchIndex)
		return
	}

	currentTssInfo, err := p.tssQueryService.QueryActiveInfo()
	if err != nil {
		log.Error("failed to query active tss info", "err", err)
		return
	}

	if si.ElectionId != currentTssInfo.ElectionId {
		log.Error("the election which this node supposed to be slashed is expired, ignore the slash",
			"node", si.Address.String(), "electionId", si.ElectionId, "batch index", si.BatchIndex)
		p.nodeStore.RemoveSlashingInfo(si.Address, si.BatchIndex)
		return
	}
}
