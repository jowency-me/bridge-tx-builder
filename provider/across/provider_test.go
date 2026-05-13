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
			TotalRelayFee: FeeBreakdown{
				Pct:   "0.001",
				Total: "1000",
			},
			RelayerFee: FeeBreakdown{
				Pct:   "0.0005",
				Total: "500",
			},
			LpFee: FeeBreakdown{
				Pct:   "0.0005",
				Total: "500",
			},
			Timestamp:           "1234567890",
			IsAmountTooLow:      false,
			QuoteBlock:          "12345678",
			SpokePoolAddress:    "0xSpokePool",
			ExpectedFillTimeSec: 4,
			CapitalCostFeePct:   "0",
			RelayFeeFullPct:     "0.001",
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
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, "0xSpokePool", quote.To)
	assert.Equal(t, 1, len(quote.Route))
	assert.Equal(t, "bridge", quote.Route[0].Action)
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
}

func TestNewProvider_WithHTTPClient(t *testing.T) {
	p := NewProvider(WithHTTPClient(http.DefaultClient))
	require.NotNil(t, p)
}

func TestNewProvider_WithAPIKeyAndIntegratorID(t *testing.T) {
	p := NewProvider(WithAPIKey("key"), WithIntegratorID("integrator"))
	require.NotNil(t, p)
}

func TestMapQuote_NilResponse(t *testing.T) {
	_, err := mapQuote(nil, domain.QuoteRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty quote response")
}

func TestMapQuote_NegativeFee(t *testing.T) {
	qr := &QuoteResponse{
		TotalRelayFee: FeeBreakdown{Total: "-100"},
		QuoteBlock:    "123",
	}
	_, err := mapQuote(qr, domain.QuoteRequest{Amount: decimal.NewFromInt(1000)})
	// fee is negative → toAmt stays zero → falls back to fromAmt
	require.NoError(t, err)
}

func TestMapQuote_ParseFloatError(t *testing.T) {
	qr := &QuoteResponse{
		TotalRelayFee:   FeeBreakdown{Total: "100"},
		RelayFeeFullPct: "not-a-float",
		QuoteBlock:      "123",
	}
	_, err := mapQuote(qr, domain.QuoteRequest{Amount: decimal.NewFromInt(1000)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse relay fee pct")
}

func TestProvider_Status_Error(t *testing.T) {
	p := NewProvider()
	_, err := p.Status(context.Background(), "0xTx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}

func TestMapQuote_SwapTxData(t *testing.T) {
	qr := &QuoteResponse{
		InputAmount:     "1000000",
		OutputAmount:    "990000",
		RelayFeeFullPct: "0.001",
		SwapTx:          TxInfo{To: "0xSpoke", Data: "0xdeadbeef", Value: "123", Gas: "210000"},
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
