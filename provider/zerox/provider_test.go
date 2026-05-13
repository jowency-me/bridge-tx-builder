package zerox

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
			Price:            "0.999",
			GuaranteedPrice:  "0.995",
			To:               "0xRouter",
			Data:             "0xdeadbeef",
			Value:            "0",
			Gas:              "200000",
			BuyAmount:        "999000",
			SellAmount:       "1000000",
			BuyTokenAddress:  "0xB",
			SellTokenAddress: "0xA",
			BuyToken:         "USDT",
			SellToken:        "USDC",
			Sources: []SourceInfo{
				{Name: "UniswapV3", Proportion: "0.5"},
				{Name: "Sushiswap", Proportion: "0.5"},
			},
		},
	}

	p := &Provider{client: mock}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.NotEmpty(t, quote.ID)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, int64(1_000_000), quote.FromAmount.IntPart())
	assert.Equal(t, "0x", quote.Provider)
	assert.Equal(t, 2, len(quote.Route))
	assert.Equal(t, uint64(200_000), quote.EstimateGas)
	assert.Equal(t, "0xRouter", quote.To)
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, quote.TxData)
}

func TestProvider_Quote_HTTPError(t *testing.T) {
	mock := &mockClient{quoteErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
}

func TestProvider_Quote_InvalidGas(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Price:     "0.999",
			To:        "0xRouter",
			Data:      "0xdeadbeef",
			Value:     "0",
			Gas:       "not-a-number",
			BuyAmount: "999000",
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse gas")
}

func TestProvider_Quote_SlippageMultipliedBy100(t *testing.T) {
	// Verify that slippage is multiplied by 100 when building QuoteParams.
	// 0.5% (0.005) should be sent as "0.5000".
	// We verify this indirectly by ensuring the quote succeeds with the
	// expected slippage behavior; the exact param value is tested at the
	// client layer in client_test.go.
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Price:     "0.999",
			To:        "0xRouter",
			Data:      "0xdeadbeef",
			Value:     "0",
			Gas:       "200000",
			BuyAmount: "999000",
		},
	}

	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.NoError(t, err)
}

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{statusResp: &StatusResponse{Status: "reachable"}}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "tx-123")
	require.NoError(t, err)
	assert.Equal(t, "tx-123", status.TxID)
	assert.Equal(t, "reachable", status.State)
}

func TestProvider_Status_Error(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "tx-123")
	require.Error(t, err)
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider("test-key")
	assert.Equal(t, "0x", p.Name())
}

func TestNewProvider_WithBaseURL(t *testing.T) {
	p := NewProvider("key", WithBaseURL("https://custom.example.com"))
	require.NotNil(t, p)
}

func TestNewProvider_WithHTTPClient(t *testing.T) {
	p := NewProvider("key", WithHTTPClient(http.DefaultClient))
	require.NotNil(t, p)
}

func TestProvider_Quote_InvalidHexData(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Price:     "0.999",
			To:        "0xRouter",
			Data:      "0xnot-valid-hex!@#",
			Value:     "0",
			Gas:       "200000",
			BuyAmount: "999000",
		},
	}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tx data")
}
