package types

import (
	"github.com/ethereum/go-ethereum/common"

	tss "github.com/tenderly/mantle/tss/common"
	"github.com/tenderly/mantle/tss/index"
	"github.com/tenderly/mantle/tss/slash"
)

type SignService interface {
	SignStateBatch(request tss.SignStateRequest) ([]byte, error)
	SignRollBack(request tss.SignStateRequest) ([]byte, error)
	SignTxBatch() error
}

type AdminService interface {
	ResetScanHeight(height uint64) error
	GetScannedHeight() (uint64, error)
	RemoveSlashingInfo(common.Address, uint64)
}

type TssQueryService interface {
	QueryActiveInfo() (*TssCommitteeInfo, error)
	QueryInactiveInfo() (*TssCommitteeInfo, error)
	QueryTssGroupMembers() (*TssCommitteeInfo, error)
}

type CPKStore interface {
	Insert(CpkData) error
	GetByElectionId(uint64) (CpkData, error)
}

type ManagerStore interface {
	CPKStore
	index.StateBatchStore
	index.ScanHeightStore
	slash.SlashingStore
}
