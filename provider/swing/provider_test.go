package swing

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
			Routes: []RouteInfo{
				{
					Route: []RouteStep{
						{Bridge: "debridge", Steps: []string{"approve", "send"}, Name: "USDC", Part: 100},
					},
					Quote: QuoteDetail{
						Integration: "debridge",
						Type:        "swap",
						Amount:      "999000",
						Decimals:    6,
						Fees: []Fee{
							{Type: "bridge", Amount: "20867118", AmountUSD: "3.024"},
						},
					},
					Duration: 120,
					Gas:      "2770874189563960",
					GasUSD:   "8.123",
				},
			},
			FromToken: TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 1},
			FromChain: ChainInfo{ChainID: 1, Slug: "ethereum"},
			ToToken:   TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: 8453},
			ToChain:   ChainInfo{ChainID: 8453, Slug: "base"},
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
	assert.Equal(t, "debridge-swap", quote.ID)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, "swing", quote.Provider)
	assert.Equal(t, 1, len(quote.Route))
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

func TestProvider_Quote_EmptyRoutes(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{Routes: []RouteInfo{}},
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
	assert.Contains(t, err.Error(), "no routes found")
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider("")
	assert.Equal(t, "swing", p.Name())
}

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			Status:          "completed",
			TxID:            "sw-123",
			FromChainTxHash: "0xSrc",
			ToChainTxHash:   "0xDst",
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "sw-123")
	require.NoError(t, err)
	assert.Equal(t, "completed", status.State)
	assert.Equal(t, "0xSrc", status.SrcChainTx)
	assert.Equal(t, "0xDst", status.DstChainTx)
}

func TestProvider_Status_Error(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "sw-123")
	require.Error(t, err)
}

func TestProvider_Status_NilResponse(t *testing.T) {
	mock := &mockClient{statusResp: nil}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "sw-123")
	require.NoError(t, err)
	assert.Equal(t, "unknown", status.State)
}

func TestMapStatus_Nil(t *testing.T) {
	st := mapStatus(nil)
	assert.Equal(t, "unknown", st.State)
}

func TestMapStatus_EmptyState(t *testing.T) {
	st := mapStatus(&StatusResponse{})
	assert.Equal(t, "unknown", st.State)
}

func TestNewProvider_WithBaseURL(t *testing.T) {
	p := NewProvider("key", WithBaseURL("https://custom.example.com"))
	require.NotNil(t, p)
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, "https://custom.example.com", c.baseURL)
}

func TestNewProvider_WithHTTPClient(t *testing.T) {
	p := NewProvider("key", WithHTTPClient(http.DefaultClient))
	require.NotNil(t, p)
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, http.DefaultClient, c.client)
}
