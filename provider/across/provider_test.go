package across

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	quoteResp  *QuoteResponse
	quoteErr   error
	statusResp *StatusResponse
	statusErr  error
}

func (m *mockClient) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	return m.quoteResp, m.quoteErr
}

func (m *mockClient) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	return m.statusResp, m.statusErr
}

func TestProvider_Quote_Success(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			CrossSwapType: "zip",
			AmountType:    "exactInput",
			Checks: Checks{
				Allowance: AllowanceCheck{Token: "0xA", Spender: "0xSpender", Actual: "0", Expected: "1000000"},
				Balance:   BalanceCheck{Token: "0xA", Actual: "5000000", Expected: "1000000"},
			},
			Steps: Steps{
				Bridge: BridgeStep{InputAmount: "1000000", OutputAmount: "995000", Provider: "across"},
			},
			InputAmount:          "1000000",
			ExpectedOutputAmount: "995000",
			MinOutputAmount:      "990000",
			ExpectedFillTime:     4,
			SwapTx: TxInfo{
				Ecosystem: "evm", SimulationSuccess: true, ChainID: 1,
				To: "0xSpokeTarget", Data: "0xdeadbeef", Value: "0", Gas: "250000",
			},
			QuoteExpiryTimestamp: 1234567890,
			ID:                   "quote-abc123",
		},
	}

	p := &Provider{client: mock}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "across", quote.Provider)
	assert.Equal(t, int64(1_000_000), quote.FromAmount.IntPart())
	assert.Equal(t, int64(995_000), quote.ToAmount.IntPart())
	assert.Equal(t, int64(990_000), quote.MinAmount.IntPart())
	assert.Equal(t, "0xSpokeTarget", quote.To)
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, quote.TxData)
	assert.Equal(t, uint64(250_000), quote.EstimateGas)
	assert.True(t, quote.TxValue.IsZero())
	assert.Equal(t, 1, len(quote.Route))
	assert.Equal(t, "bridge", quote.Route[0].Action)
	assert.Equal(t, domain.ChainEthereum, quote.Route[0].ChainID)
	assert.Equal(t, 0.005, quote.Slippage)
	assert.Equal(t, "0xSpender", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
	assert.Equal(t, int64(1_000_000), quote.AllowanceNeeded.IntPart())
	assert.Equal(t, domain.ChainEthereum, quote.FromToken.ChainID)
	assert.Equal(t, domain.ChainBase, quote.ToToken.ChainID)
}

func TestProvider_Quote_HTTPError(t *testing.T) {
	mock := &mockClient{quoteErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, "across", p.Name())
}

func TestNewProvider_WithBaseURL(t *testing.T) {
	p := NewProvider(WithBaseURL("https://custom.example.com"))
	require.NotNil(t, p)
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, "https://custom.example.com", c.baseURL)
}

func TestNewProvider_WithHTTPClient(t *testing.T) {
	p := NewProvider(WithHTTPClient(http.DefaultClient))
	require.NotNil(t, p)
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, http.DefaultClient, c.client)
}

func TestNewProvider_WithAPIKeyAndIntegratorID(t *testing.T) {
	p := NewProvider(WithAPIKey("key"), WithIntegratorID("integrator"))
	require.NotNil(t, p)
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, "key", c.apiKey)
	assert.Equal(t, "integrator", c.integratorID)
}

func TestMapQuote_NilResponse(t *testing.T) {
	_, err := mapQuote(nil, domain.QuoteRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty quote response")
}

func TestMapQuote_FallbackToInputAmount(t *testing.T) {
	qr := &QuoteResponse{
		InputAmount: "1000000",
		Checks: Checks{
			Allowance: AllowanceCheck{Expected: "-1"},
		},
	}
	quote, err := mapQuote(qr, domain.QuoteRequest{Amount: decimal.NewFromInt(1000)})
	require.NoError(t, err)
	// toAmt falls back to fromAmt when zero/negative
	assert.True(t, quote.ToAmount.Equal(decimal.NewFromInt(1_000_000)))
}

func TestProvider_Status_Error(t *testing.T) {
	p := NewProvider()
	_, err := p.Status(context.Background(), "0xTx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}

func TestMapQuote_SwapTxData(t *testing.T) {
	qr := &QuoteResponse{
		InputAmount:          "1000000",
		ExpectedOutputAmount: "990000",
		MinOutputAmount:      "985000",
		SwapTx:               TxInfo{To: "0xSpoke", Data: "0xdeadbeef", Value: "123", Gas: "210000"},
	}
	quote, err := mapQuote(qr, domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	})
	require.NoError(t, err)
	assert.Equal(t, "0xSpoke", quote.To)
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, quote.TxData)
	assert.Equal(t, int64(123), quote.TxValue.IntPart())
	assert.Equal(t, uint64(210000), quote.EstimateGas)
}

func TestMapQuote_InvalidSwapTxData(t *testing.T) {
	_, err := mapQuote(&QuoteResponse{SwapTx: TxInfo{Data: "0xzz"}}, domain.QuoteRequest{Amount: decimal.NewFromInt(1)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tx data")
}
