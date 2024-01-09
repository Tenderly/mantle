package go_tests

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	addressDict "github.com/tenderly/mantle/go-test/contracts/L1/deployment/AddressDictator.sol"
	l1bridge "github.com/tenderly/mantle/go-test/contracts/L1/messaging/L1StandardBridge.sol"

	"github.com/stretchr/testify/require"
	"testing"
)

func TestMix(t *testing.T) {
	l1Client, err := ethclient.Dial("https://eth-goerli.g.alchemy.com/v2/821_LFssCCQnEG3mHnP7tSrc87IQKsUp")
	require.NoError(t, err)
	require.NotNil(t, l1Client)

	// query eth erc20 token
	l1Bridge, err := l1bridge.NewL1StandardBridge(common.HexToAddress("0xfc9dc9e4f9a5e6a03b268485395517236c2a0f0a"), l1Client)
	require.NoError(t, err)

	l1mantle, err := l1Bridge.L1MantleAddress(&bind.CallOpts{})
	require.NoError(t, err)
	t.Log(l1mantle.Hex())

	l2bridge, err := l1Bridge.L2TokenBridge(&bind.CallOpts{})
	require.NoError(t, err)
	t.Log(l2bridge.Hex())
}

func TestL1TokenBridge(t *testing.T) {
	l1Client, err := ethclient.Dial(l1url)
	require.NoError(t, err)
	require.NotNil(t, l1Client)

	// query eth erc20 token
	addrDict, err := addressDict.NewAddressDictator(common.HexToAddress("0x5FbDB2315678afecb367f032d93F642f64180aa3"), l1Client)
	require.NoError(t, err)

	ret, err := addrDict.GetNamedAddresses(&bind.CallOpts{})
	require.NoError(t, err)
	t.Log(ret)
}
