package router

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/jowency-me/bridge-tx-builder/provider/mock"
)

func TestRouter_FindProviders(t *testing.T) {
	ctx := context.Background()
	req := validReq()

	r := New()
	r.RegisterProvider(mock.NewFixedProvider("p1", mock.StaticQuote("q1", "p1")))
	r.RegisterProvider(mock.NewErrorProvider("p2", errors.New("down")))
	r.RegisterProvider(mock.NewFixedProvider("p3", mock.StaticQuote("q3", "p3")))

	list, err := r.FindProviders(ctx, req)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"p1", "p3"}, list)
}

func TestRouter_SelectBest_BestAmount(t *testing.T) {
	ctx := context.Background()
	req := validReq()

	qLow := mock.StaticQuote("q1", "p1")
	qLow.ToAmount = decimal.NewFromInt(900_000)
	qLow.EstimateFee = decimal.NewFromInt(10_000)

	qHigh := mock.StaticQuote("q2", "p2")
	qHigh.ToAmount = decimal.NewFromInt(950_000)
	qHigh.EstimateFee = decimal.NewFromInt(50_000)

	r := New()
	r.RegisterProvider(mock.NewFixedProvider("p1", qLow))
	r.RegisterProvider(mock.NewFixedProvider("p2", qHigh))

	quote, err := r.SelectBest(ctx, req, StrategyBestAmount)
	require.NoError(t, err)
	assert.Equal(t, "q2", quote.ID)
}

func TestRouter_SelectBest_LowestFee(t *testing.T) {
	ctx := context.Background()
	req := validReq()

	qExpensive := mock.StaticQuote("q1", "p1")
	qExpensive.ToAmount = decimal.NewFromInt(950_000)
	qExpensive.EstimateFee = decimal.NewFromInt(50_000)

	qCheap := mock.StaticQuote("q2", "p2")
	qCheap.ToAmount = decimal.NewFromInt(900_000)
	qCheap.EstimateFee = decimal.NewFromInt(5_000)

	r := New()
	r.RegisterProvider(mock.NewFixedProvider("p1", qExpensive))
	r.RegisterProvider(mock.NewFixedProvider("p2", qCheap))

	quote, err := r.SelectBest(ctx, req, StrategyLowestFee)
	require.NoError(t, err)
	assert.Equal(t, "q2", quote.ID)
}

func TestRouter_SelectBest_NamedProvider(t *testing.T) {
	ctx := context.Background()
	req := validReq()

	q1 := mock.StaticQuote("q1", "p1")
	q2 := mock.StaticQuote("q2", "p2")

	r := New()
	r.RegisterProvider(mock.NewFixedProvider("p1", q1))
	r.RegisterProvider(mock.NewFixedProvider("p2", q2))

	quote, err := r.SelectBest(ctx, req, StrategyNamed("p2"))
	require.NoError(t, err)
	assert.Equal(t, "q2", quote.ID)

	_, err = r.SelectBest(ctx, req, StrategyNamed("unknown"))
	require.Error(t, err)
}

func TestRouter_SelectBest_NoProviders(t *testing.T) {
	ctx := context.Background()
	req := validReq()

	r := New()
	r.RegisterProvider(mock.NewErrorProvider("p1", errors.New("down")))

	_, err := r.SelectBest(ctx, req, StrategyBestAmount)
	require.Error(t, err)
}

func TestRouter_BuildTransaction(t *testing.T) {
	quote := domain.Quote{
		ID:          "q1",
		Provider:    "p1",
		To:          "0x1111111111111111111111111111111111111111",
		TxData:      []byte{0xde, 0xad},
		TxValue:     decimal.Zero,
		EstimateGas: 200000,
		FromToken:   domain.Token{Symbol: "ETH", Address: "0xA", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:     domain.Token{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		FromAmount:  decimal.NewFromInt(1_000_000),
		ToAmount:    decimal.NewFromInt(999_000),
		MinAmount:   decimal.NewFromInt(995_000),
	}

	r := New()
	r.RegisterBuilder(&mockBuilder{cid: domain.ChainEthereum})

	tx, err := r.BuildTransaction(context.Background(), quote, "0xFrom", []byte("pk"))
	require.NoError(t, err)
	assert.Equal(t, domain.ChainEthereum, tx.ChainID)
}

func TestRouter_BuildTransaction_NoBuilder(t *testing.T) {
	quote := domain.Quote{
		ID:         "q1",
		Provider:   "p1",
		FromToken:  domain.Token{Symbol: "TRX", Address: "Txxx", Decimals: 6, ChainID: domain.ChainTron},
		ToToken:    domain.Token{Symbol: "USDT", Address: "Tyyy", Decimals: 6, ChainID: domain.ChainTron},
		FromAmount: decimal.NewFromInt(1_000_000),
		ToAmount:   decimal.NewFromInt(999_000),
		MinAmount:  decimal.NewFromInt(995_000),
	}

	r := New()
	_, err := r.BuildTransaction(context.Background(), quote, "from", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no builder registered for chain")
}

// mockBuilder is a test double for domain.ChainBuilder.
type mockBuilder struct {
	cid domain.ChainID
}

func (m *mockBuilder) ChainID() domain.ChainID { return m.cid }
func (m *mockBuilder) Build(_ context.Context, _ domain.Quote, from string, _ []byte) (*domain.Transaction, error) {
	return &domain.Transaction{ChainID: m.cid, From: from, Value: decimal.Zero}, nil
}

func TestRouter_RegisterSimulator_Nil(t *testing.T) {
	r := New()
	// Should not panic.
	r.RegisterSimulator(domain.ChainEthereum, nil)
	assert.Len(t, r.simulators, 0)
}

func TestRouter_Simulate_NilTx(t *testing.T) {
	r := New()
	_, err := r.Simulate(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction required")
}

func TestRouter_Simulate_NoSimulator(t *testing.T) {
	r := New()
	tx := &domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    "0x1234567890123456789012345678901234567890",
		To:      "0x0987654321098765432109876543210987654321",
		Value:   decimal.Zero,
	}
	_, err := r.Simulate(context.Background(), tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no simulator for chain")
}

func TestRouter_Simulate_Success(t *testing.T) {
	r := New()
	r.RegisterSimulator(domain.ChainEthereum, &mockSimulator{})

	tx := &domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    "0x1234567890123456789012345678901234567890",
		To:      "0x0987654321098765432109876543210987654321",
		Value:   decimal.Zero,
	}
	res, err := r.Simulate(context.Background(), tx)
	require.NoError(t, err)
	assert.True(t, res.Success)
}

// mockSimulator is a test double for domain.Simulator.
type mockSimulator struct{}

func (m *mockSimulator) Simulate(_ context.Context, tx *domain.Transaction) (*domain.SimulationResult, error) {
	return &domain.SimulationResult{Success: true, GasUsed: 21000}, nil
}

func validReq() domain.QuoteRequest {
	return domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
}
