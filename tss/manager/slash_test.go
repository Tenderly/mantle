package manager

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/tenderly/optimism/l2geth/common/hexutil"
	tss "github.com/tenderly/optimism/tss/common"
	"github.com/tenderly/optimism/tss/index"
	"github.com/tenderly/optimism/tss/manager/store"
	"github.com/tenderly/optimism/tss/slash"
)

func TestLivenessDetect(t *testing.T) {
	storage, err := store.NewStorage("")
	require.NoError(t, err)

	slashing := slash.NewSlashing(storage, storage, 10)
	priK, err := crypto.GenerateKey()
	nodePublicKey := hexutil.Encode(crypto.CompressPubkey(&priK.PublicKey))
	address := crypto.PubkeyToAddress(priK.PublicKey)
	// index: 0 -> 5
	for i := 0; i < 6; i++ {
		err = storage.SetStateBatch(index.StateBatchInfo{
			BatchRoot:   [32]byte{byte(i)},
			ElectionId:  1,
			AbsentNodes: []string{nodePublicKey},
			BatchIndex:  uint64(i),
		})
		require.NoError(t, err)
		err = storage.IndexStateBatch(uint64(i), [32]byte{byte(i)})
		require.NoError(t, err)
		err = slashing.AfterStateBatchIndexed([32]byte{byte(i)})
		require.NoError(t, err)

		if i == 5 {
			found, slashingInfo := storage.GetSlashingInfo(address, uint64(i))
			require.True(t, found)
			require.EqualValues(t, address, slashingInfo.Address)
			require.EqualValues(t, i, slashingInfo.BatchIndex)
			require.EqualValues(t, 1, slashingInfo.ElectionId)
			require.EqualValues(t, tss.SlashTypeLiveness, slashingInfo.SlashType)

			storage.RemoveSlashingInfo(address, uint64(i))
		} else {
			found, _ := storage.GetSlashingInfo(address, uint64(i))
			require.False(t, found)
		}
	}

	found, signingInfo := storage.GetSigningInfo(address)
	require.True(t, found)
	require.EqualValues(t, 6, signingInfo.MissedBlocksCounter)

	// index: 6
	err = storage.SetStateBatch(index.StateBatchInfo{
		BatchRoot:   [32]byte{byte(6)},
		ElectionId:  2,
		AbsentNodes: []string{nodePublicKey},
		BatchIndex:  uint64(6),
	})
	require.NoError(t, err)
	err = slashing.AfterStateBatchIndexed([32]byte{byte(6)})
	require.NoError(t, err)

	found, signingInfo = storage.GetSigningInfo(address)
	require.True(t, found)
	require.EqualValues(t, 1, signingInfo.MissedBlocksCounter)

	// index: 7-15, absent from 7-10
	for i := 7; i <= 15; i++ {
		if i <= 10 {
			err = storage.SetStateBatch(index.StateBatchInfo{
				BatchRoot:   [32]byte{byte(i)},
				ElectionId:  2,
				AbsentNodes: []string{nodePublicKey},
				BatchIndex:  uint64(i),
			})
		} else {
			err = storage.SetStateBatch(index.StateBatchInfo{
				BatchRoot:    [32]byte{byte(i)},
				ElectionId:   2,
				WorkingNodes: []string{nodePublicKey},
				BatchIndex:   uint64(i),
			})
		}
		require.NoError(t, err)
		err = storage.IndexStateBatch(uint64(i), [32]byte{byte(i)})
		require.NoError(t, err)
		err = slashing.AfterStateBatchIndexed([32]byte{byte(i)})
		require.NoError(t, err)
	}
	found, signingInfo = storage.GetSigningInfo(address)
	require.True(t, found)
	require.EqualValues(t, 5, signingInfo.MissedBlocksCounter)
	require.False(t, storage.IsInSlashing(address))

	// index: 16-20
	for i := 16; i <= 20; i++ {
		err = storage.SetStateBatch(index.StateBatchInfo{
			BatchRoot:   [32]byte{byte(i)},
			ElectionId:  2,
			AbsentNodes: []string{nodePublicKey},
			BatchIndex:  uint64(i),
		})
		require.NoError(t, err)
		err = storage.IndexStateBatch(uint64(i), [32]byte{byte(i)})
		require.NoError(t, err)
		err = slashing.AfterStateBatchIndexed([32]byte{byte(i)})
		require.NoError(t, err)
	}
	found, signingInfo = storage.GetSigningInfo(address)
	require.True(t, found)

	require.EqualValues(t, 5, signingInfo.MissedBlocksCounter)
	require.False(t, storage.IsInSlashing(address))

	// index: 21
	err = storage.SetStateBatch(index.StateBatchInfo{
		BatchRoot:   [32]byte{byte(21)},
		ElectionId:  2,
		AbsentNodes: []string{nodePublicKey},
		BatchIndex:  uint64(21),
	})
	require.NoError(t, err)
	err = storage.IndexStateBatch(uint64(21), [32]byte{byte(21)})
	require.NoError(t, err)
	err = slashing.AfterStateBatchIndexed([32]byte{byte(21)})
	require.NoError(t, err)

	found, signingInfo = storage.GetSigningInfo(address)
	require.True(t, found)
	require.EqualValues(t, 6, signingInfo.MissedBlocksCounter)
	require.True(t, storage.IsInSlashing(address))
	found, slashingInfo := storage.GetSlashingInfo(address, 21)
	require.True(t, found)
	require.EqualValues(t, tss.SlashTypeLiveness, slashingInfo.SlashType)
}
