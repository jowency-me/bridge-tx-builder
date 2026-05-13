package debridge

import (
	"context"
	"fmt"
	"net/http"
	"testing"

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
			OrderID:            "db-123",
			EstimateToAmount:   "999000",
			EstimateFromAmount: "1000000",
			Tx: TxInfo{
				To:       "0xRouter",
				Data:     "0xdeadbeef",
				Value:    "0",
				GasLimit: "180000",
			},
			TokenIn: TokenInfo{
				Symbol:   "USDC",
				Address:  "0xA",
				Decimals: 6,
				ChainID:  1,
			},
			TokenOut: TokenInfo{
				Symbol:   "USDT",
				Address:  "0xB",
				Decimals: 6,
				ChainID:  8453,
			},
		},
	}

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

	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "debridge", quote.Provider)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, uint64(180_000), quote.EstimateGas)
}

func TestProvider_Quote_InvalidGas(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Tx: TxInfo{
				To:       "0xRouter",
				Data:     "0xdeadbeef",
				Value:    "0",
				GasLimit: "not-a-number",
			},
			EstimateToAmount: "999000",
		},
	}
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
	assert.Contains(t, err.Error(), "parse gas limit")
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, "debridge", p.Name())
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

func TestNewProvider_WithAPIKey(t *testing.T) {
	p := NewProvider(WithAPIKey("test-key"))
	require.NotNil(t, p)
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, "test-key", c.apiKey)
}

// ─── mapQuote edge cases ────────────────────────────────────────────────────

func TestProvider_Quote_EmptyResponse(t *testing.T) {
	mock := &mockClient{quoteResp: nil, quoteErr: nil}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), validReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty quote response")
}

func TestProvider_Quote_InvalidFromAmount(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			OrderID:            "db-123",
			EstimateToAmount:   "999000",
			EstimateFromAmount: "not-a-number",
			Tx:                 TxInfo{To: "0xRouter", Data: "0x", Value: "0", GasLimit: "180000"},
			TokenIn:            TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 1},
			TokenOut:           TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: 8453},
		},
	}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), validReq())
	require.NoError(t, err) // should use zero on parse error
}

func TestProvider_Quote_InvalidToAmount(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			OrderID:            "db-123",
			EstimateToAmount:   "not-a-number",
			EstimateFromAmount: "1000000",
			Tx:                 TxInfo{To: "0xRouter", Data: "0x", Value: "0", GasLimit: "180000"},
			TokenIn:            TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 1},
			TokenOut:           TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: 8453},
		},
	}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), validReq())
	require.NoError(t, err) // should use zero on parse error
}

func TestProvider_Quote_ZeroGasLimit(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			OrderID:            "db-123",
			EstimateToAmount:   "999000",
			EstimateFromAmount: "1000000",
			Tx:                 TxInfo{To: "0xRouter", Data: "0x", Value: "0", GasLimit: "0"},
			TokenIn:            TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 1},
			TokenOut:           TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: 8453},
		},
	}
	p := &Provider{client: mock}
	quote, err := p.Quote(context.Background(), validReq())
	require.NoError(t, err)
	assert.Equal(t, uint64(0), quote.EstimateGas)
}

func TestProvider_Quote_TxValue(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			OrderID:            "db-123",
			EstimateToAmount:   "999000",
			EstimateFromAmount: "1000000",
			Tx:                 TxInfo{To: "0xRouter", Data: "0x", Value: "1500000000000000000", GasLimit: "180000"},
			TokenIn:            TokenInfo{Symbol: "ETH", Address: "0xE", Decimals: 18, ChainID: 1},
			TokenOut:           TokenInfo{Symbol: "WETH", Address: "0xC", Decimals: 18, ChainID: 8453},
		},
	}
	p := &Provider{client: mock}
	quote, err := p.Quote(context.Background(), validReqEth())
	require.NoError(t, err)
	assert.True(t, quote.TxValue.GreaterThan(decimal.Zero))
}

func TestProvider_Quote_ClientError(t *testing.T) {
	mock := &mockClient{quoteErr: fmt.Errorf("network error")}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), validReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
}

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			OrderID: "order-456",
			Status:  "COMPLETED",
		},
	}
	p := &Provider{client: mock}
	st, err := p.Status(context.Background(), "order-456")
	require.NoError(t, err)
	assert.Equal(t, "order-456", st.TxID)
	assert.Equal(t, "COMPLETED", st.State)
}

func TestProvider_Status_ClientError(t *testing.T) {
	mock := &mockClient{statusErr: fmt.Errorf("status error")}
	p := &Provider{client: mock}
	_, err := p.Status(context.Background(), "order-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status error")
}

// ─── mapToken ───────────────────────────────────────────────────────────────

func TestProvider_Quote_ChainIDMapping(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			OrderID:            "db-chain-map",
			EstimateToAmount:   "500000",
			EstimateFromAmount: "1000000",
			Tx:                 TxInfo{To: "0xRouter", Data: "0x", Value: "0", GasLimit: "180000"},
			TokenIn:            TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 56},
			TokenOut:           TokenInfo{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: 137},
		},
	}
	p := &Provider{client: mock}
	quote, err := p.Quote(context.Background(), validReq())
	require.NoError(t, err)
	assert.NotNil(t, quote.FromToken.ChainID)
	assert.NotNil(t, quote.ToToken.ChainID)
}

// ─── helpers ────────────────────────────────────────────────────────────────

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

func validReqEth() domain.QuoteRequest {
	return domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0xE", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "WETH", Address: "0xC", Decimals: 18, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
}
