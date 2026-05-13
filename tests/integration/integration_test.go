//go:build integration

package integration

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	evmBuilder "github.com/jowency-me/bridge-tx-builder/builder/evm"
	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/jowency-me/bridge-tx-builder/provider/mock"
	"github.com/jowency-me/bridge-tx-builder/router"
	evmSim "github.com/jowency-me/bridge-tx-builder/simulator/evm"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCrossChainSwap_EVM_FullFlow exercises the complete router pipeline with a
// deterministic mock provider and a real public Ethereum mainnet RPC node.
func TestCrossChainSwap_EVM_FullFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	fromAddr := crypto.PubkeyToAddress(key.PublicKey).Hex()

	ethClient, selectedRPC := dialMainnetEthereumRPC(t, ctx)
	defer ethClient.Close()

	tipCap, err := ethClient.SuggestGasTipCap(ctx)
	require.NoError(t, err)
	if tipCap.Sign() == 0 {
		tipCap = big.NewInt(2_000_000_000)
	}
	header, err := ethClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, header.BaseFee)
	feeCap := new(big.Int).Mul(header.BaseFee, big.NewInt(2))
	feeCap.Add(feeCap, tipCap)

	usdc := domain.Token{
		Symbol:   "USDC",
		Address:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		Decimals: 6,
		ChainID:  domain.ChainEthereum,
	}
	quote := &domain.Quote{
		ID:          "integration-mainnet-evm-usdc-transfer",
		FromToken:   usdc,
		ToToken:     usdc,
		FromAmount:  decimal.NewFromInt(1),
		ToAmount:    decimal.NewFromInt(1),
		MinAmount:   decimal.NewFromInt(1),
		Slippage:    0.005,
		Provider:    "mock-mainnet",
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          usdc.Address,
		TxData:      evmTransferCalldata("0x1111111111111111111111111111111111111111", big.NewInt(1)),
		TxValue:     decimal.Zero,
		EstimateGas: 300000,
		GasTipCap:   decimal.NewFromBigInt(tipCap, 0),
		GasFeeCap:   decimal.NewFromBigInt(feeCap, 0),
	}

	r := router.New()
	r.RegisterProvider(mock.NewFixedProvider("mock-mainnet", quote))
	r.RegisterBuilder(evmBuilder.NewBuilder(1))
	simulator, err := evmSim.NewSimulator(selectedRPC)
	require.NoError(t, err)
	r.RegisterSimulator(domain.ChainEthereum, simulator)

	req := domain.QuoteRequest{
		FromToken: usdc,
		ToToken:   usdc,
		Amount:    decimal.NewFromInt(1),
		Slippage:  0.005,
		FromAddr:  fromAddr,
		ToAddr:    fromAddr,
	}

	providers, err := r.FindProviders(ctx, req)
	require.NoError(t, err)
	require.Equal(t, []string{"mock-mainnet"}, providers)

	selected, err := r.SelectBest(ctx, req, router.StrategyBestAmount)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.NotEmpty(t, selected.TxData)
	require.NotEmpty(t, selected.To)

	tx, err := r.BuildTransaction(ctx, *selected, fromAddr, crypto.FromECDSA(key))
	require.NoError(t, err)
	require.NotNil(t, tx)
	assert.Equal(t, domain.ChainEthereum, tx.ChainID)
	assert.Equal(t, fromAddr, tx.From)
	assert.NotEmpty(t, tx.Data)

	res, err := r.Simulate(ctx, tx)
	require.NoError(t, err)
	require.NotNil(t, res)
	if res.Success {
		assert.NotZero(t, res.GasUsed)
	} else {
		assert.NotEmpty(t, res.RevertReason)
	}
	t.Logf("mainnet simulation success=%v gasUsed=%d revert=%q", res.Success, res.GasUsed, res.RevertReason)
}

func evmTransferCalldata(to string, amount *big.Int) []byte {
	data := make([]byte, 4+32+32)
	copy(data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb})
	copy(data[16:36], common.HexToAddress(to).Bytes())
	copy(data[68-len(amount.Bytes()):68], amount.Bytes())
	return data
}

func dialMainnetEthereumRPC(t *testing.T, ctx context.Context) (*ethclient.Client, string) {
	t.Helper()

	var lastErr error
	for _, rpcURL := range publicEthereumRPCURLs {
		client, err := ethclient.DialContext(ctx, rpcURL)
		if err != nil {
			lastErr = err
			continue
		}
		networkID, err := client.NetworkID(ctx)
		if err != nil {
			lastErr = err
			client.Close()
			continue
		}
		if networkID.Int64() != 1 {
			lastErr = assert.AnError
			client.Close()
			continue
		}
		return client, rpcURL
	}
	require.NoError(t, lastErr, "no public Ethereum mainnet RPC endpoint reachable")
	return nil, ""
}
