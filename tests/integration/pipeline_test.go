//go:build integration

package integration

import (
	"context"
	"math/big"
	"testing"
	"time"

	evmBuilder "github.com/jowency-me/bridge-tx-builder/builder/evm"
	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/jowency-me/bridge-tx-builder/provider/across"
	"github.com/jowency-me/bridge-tx-builder/provider/celer"
	"github.com/jowency-me/bridge-tx-builder/provider/debridge"
	"github.com/jowency-me/bridge-tx-builder/provider/hop"
	"github.com/jowency-me/bridge-tx-builder/provider/lifi"
	"github.com/jowency-me/bridge-tx-builder/provider/mock"
	"github.com/jowency-me/bridge-tx-builder/provider/openocean"
	"github.com/jowency-me/bridge-tx-builder/router"
	evmSim "github.com/jowency-me/bridge-tx-builder/simulator/evm"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPipeline_QuoteBuildSimulate exercises the complete Quote→Build→Simulate
// flow using a mock provider and a real Ethereum mainnet RPC node.
func TestPipeline_QuoteBuildSimulate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	privKey, fromAddr := generateKey(t)
	ethClient, selectedRPC := dialEthereumRPC(t, ctx)
	defer ethClient.Close()

	tipCap, feeCap := evmGasParams(t, ctx, ethClient)

	quote := &domain.Quote{
		ID:          "pipeline-evm-usdc-transfer",
		FromToken:   usdcEthereum,
		ToToken:     usdcEthereum,
		FromAmount:  decimal.NewFromInt(1_000_000),
		ToAmount:    decimal.NewFromInt(999_000),
		MinAmount:   decimal.NewFromInt(995_000),
		Slippage:    0.005,
		Provider:    "mock-pipeline",
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          usdcEthereum.Address,
		TxData:      evmTransferCalldata("0x1111111111111111111111111111111111111111", big.NewInt(1)),
		TxValue:     decimal.Zero,
		EstimateGas: 300000,
		GasTipCap:   decimal.NewFromBigInt(tipCap, 0),
		GasFeeCap:   decimal.NewFromBigInt(feeCap, 0),
	}

	r := router.New()
	r.RegisterProvider(mock.NewFixedProvider("mock-pipeline", quote))
	r.RegisterBuilder(evmBuilder.NewBuilder(1))
	simulator, err := evmSim.NewSimulator(selectedRPC)
	require.NoError(t, err)
	r.RegisterSimulator(domain.ChainEthereum, simulator)

	req := crossChainQuoteRequest(fromAddr)

	// Step 1: FindProviders
	providers, err := r.FindProviders(ctx, req)
	require.NoError(t, err)
	require.Equal(t, []string{"mock-pipeline"}, providers)

	// Step 2: SelectBest
	selected, err := r.SelectBest(ctx, req, router.StrategyBestAmount)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "pipeline-evm-usdc-transfer", selected.ID)
	assert.Equal(t, "mock-pipeline", selected.Provider)
	assert.True(t, selected.FromAmount.Equal(decimal.NewFromInt(1_000_000)))
	assert.True(t, selected.ToAmount.Equal(decimal.NewFromInt(999_000)))
	assert.True(t, selected.MinAmount.Equal(decimal.NewFromInt(995_000)))
	assert.NotEmpty(t, selected.TxData)
	assert.Equal(t, usdcEthereum.Address, selected.To)
	assert.Nil(t, selected.AllowanceNeeded)
	assert.Empty(t, selected.ApprovalAddress)

	// Step 3: EnsureApproval (no approval needed for this quote)
	approval, err := r.EnsureApproval(ctx, *selected)
	require.NoError(t, err)
	assert.Nil(t, approval, "no approval needed when ApprovalAddress is empty")

	// Step 4: BuildTransaction
	tx, err := r.BuildTransaction(ctx, *selected, fromAddr, privKey)
	require.NoError(t, err)
	require.NotNil(t, tx)
	assert.Equal(t, domain.ChainEthereum, tx.ChainID)
	assert.Equal(t, fromAddr, tx.From)
	assert.Equal(t, usdcEthereum.Address, tx.To)
	assert.NotEmpty(t, tx.Data)
	assert.Equal(t, uint64(300000), tx.Gas)

	// Step 5: Simulate
	res, err := r.Simulate(ctx, tx)
	require.NoError(t, err)
	require.NotNil(t, res)
	if res.Success {
		assert.NotZero(t, res.GasUsed)
	} else {
		assert.NotEmpty(t, res.RevertReason)
	}
	t.Logf("simulation: success=%v gasUsed=%d revert=%q", res.Success, res.GasUsed, res.RevertReason)
}

// TestPipeline_ApprovalFlow verifies the approval + build pipeline
// when the quote requires ERC-20 approval.
func TestPipeline_ApprovalFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, fromAddr := generateKey(t)

	allowance := decimal.NewFromInt(1_000_000)
	quote := &domain.Quote{
		ID:              "pipeline-approval-test",
		FromToken:       usdcEthereum,
		ToToken:         usdcBase,
		FromAmount:      decimal.NewFromInt(1_000_000),
		ToAmount:        decimal.NewFromInt(990_000),
		MinAmount:       decimal.NewFromInt(985_000),
		Slippage:        0.005,
		Provider:        "mock-approval",
		Deadline:        time.Now().Add(10 * time.Minute),
		To:              "0xRouterContract",
		TxData:          evmTransferCalldata("0x1111111111111111111111111111111111111111", big.NewInt(1)),
		TxValue:         decimal.Zero,
		EstimateGas:     250000,
		ApprovalAddress: "0xSpenderContract",
		AllowanceNeeded: &allowance,
	}

	r := router.New()
	r.RegisterProvider(mock.NewFixedProvider("mock-approval", quote))
	r.RegisterBuilder(evmBuilder.NewBuilder(1))

	req := crossChainQuoteRequest(fromAddr)

	selected, err := r.SelectBest(ctx, req, router.StrategyBestAmount)
	require.NoError(t, err)
	assert.Equal(t, "0xSpenderContract", selected.ApprovalAddress)
	require.NotNil(t, selected.AllowanceNeeded)
	assert.True(t, selected.AllowanceNeeded.Equal(decimal.NewFromInt(1_000_000)))

	// EnsureApproval should return an approval action
	approval, err := r.EnsureApproval(ctx, *selected)
	require.NoError(t, err)
	require.NotNil(t, approval)
	assert.Equal(t, usdcEthereum.Address, approval.TokenAddr)
	assert.Equal(t, "0xSpenderContract", approval.Spender)
	assert.Equal(t, int64(1_000_000), approval.Amount.Int64())
	assert.NotEmpty(t, approval.TxData)
	assert.Equal(t, usdcEthereum.Address, approval.TxTo)
}

// TestPipeline_MixedMockAndReal verifies router selection among multiple providers.
func TestPipeline_MixedMockAndReal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	req := crossChainQuoteRequest("0x1234567890123456789012345678901234567890")

	// High-quality mock provider that should always win
	mockQuote := &domain.Quote{
		ID:         "mock-best",
		FromToken:  req.FromToken,
		ToToken:    req.ToToken,
		FromAmount: req.Amount,
		ToAmount:   decimal.NewFromInt(999_999),
		MinAmount:  decimal.NewFromInt(995_000),
		Slippage:   req.Slippage,
		Provider:   "mock",
		Deadline:   time.Now().Add(10 * time.Minute),
	}

	r := router.New()
	r.RegisterProvider(&mockProvider{name: "mock", quote: mockQuote})

	// Add real providers that don't require API keys
	for _, p := range []domain.Provider{
		lifi.NewProvider(""),
		debridge.NewProvider(),
		openocean.NewProvider(),
		across.NewProvider(),
		hop.NewProvider(),
		celer.NewProvider(),
	} {
		r.RegisterProvider(p)
	}

	names, err := r.FindProviders(ctx, req)
	require.NoError(t, err)
	require.NotEmpty(t, names)
	t.Logf("available providers: %v", names)

	// Mock should win for best amount (999999 > any real quote after fees)
	best, err := r.SelectBest(ctx, req, router.StrategyBestAmount)
	require.NoError(t, err)
	require.NotNil(t, best)
	assert.Equal(t, "mock-best", best.ID)
	assert.Equal(t, int64(999_999), best.ToAmount.IntPart())
	assert.True(t, best.ToAmount.GreaterThan(decimal.Zero))
	assert.True(t, best.FromAmount.Equal(req.Amount))

	// Named strategy should select mock exactly
	named, err := r.SelectBest(ctx, req, router.StrategyNamed("mock"))
	require.NoError(t, err)
	assert.Equal(t, "mock-best", named.ID)

	// Lowest fee strategy should return a valid quote
	lowestFee, err := r.SelectBest(ctx, req, router.StrategyLowestFee)
	if err == nil && lowestFee != nil {
		assert.NotEmpty(t, lowestFee.ID)
		assert.True(t, lowestFee.EstimateFee.GreaterThanOrEqual(decimal.Zero))
		t.Logf("lowest fee: provider=%s fee=%s", lowestFee.Provider, lowestFee.EstimateFee.String())
	}
}
