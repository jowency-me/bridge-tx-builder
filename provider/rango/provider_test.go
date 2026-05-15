package rango

import (
	"context"
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

func (m *mockClient) Status(ctx context.Context, txID string, statusParams ...StatusParams) (*StatusResponse, error) {
	return m.statusResp, m.statusErr
}

func TestProvider_Quote_Success(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			RequestID:  "rg-123",
			ResultType: "OK",
			Route: Route{
				From:            TokenInfo{Symbol: "USDC", Address: "0xA", Blockchain: "ETH", Decimals: 6},
				To:              TokenInfo{Symbol: "USDT", Address: "0xB", Blockchain: "BASE", Decimals: 6},
				OutputAmount:    "999000",
				OutputAmountMin: "990000",
				Path: []QuotePath{
					{
						From:        TokenInfo{Symbol: "USDC", Address: "0xA", Blockchain: "ETH", Decimals: 6},
						To:          TokenInfo{Symbol: "USDT", Address: "0xB", Blockchain: "BASE", Decimals: 6},
						Swapper:     SwapperMeta{ID: "1inch", Title: "1inch"},
						SwapperType: "DEX",
					},
				},
				Fee: []SwapFee{
					{Amount: "1000", ExpenseType: "NETWORK_FEE"},
				},
				EstimatedTimeInSeconds: 120,
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
	assert.Equal(t, "rango", quote.Provider)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, int64(990_000), quote.MinAmount.IntPart())
	assert.Equal(t, "rg-123", quote.ID)
	assert.Equal(t, 1, len(quote.Route))
	assert.Equal(t, "1inch", quote.Route[0].Protocol)
	assert.Equal(t, "swap", quote.Route[0].Action)
	assert.Equal(t, int64(1000), quote.EstimateFee.IntPart())
	// Token mapping from API response
	assert.Equal(t, "USDC", quote.FromToken.Symbol)
	assert.Equal(t, "USDT", quote.ToToken.Symbol)
}

func TestProvider_Quote_SlippageBasedMinAmount(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			RequestID:  "rg-456",
			ResultType: "OK",
			Route: Route{
				From:         TokenInfo{Symbol: "USDC", Address: "0xA", Blockchain: "ETH", Decimals: 6},
				To:           TokenInfo{Symbol: "USDT", Address: "0xB", Blockchain: "BASE", Decimals: 6},
				OutputAmount: "1000000",
				Path: []QuotePath{
					{
						From:    TokenInfo{Symbol: "USDC", Address: "0xA", Blockchain: "ETH", Decimals: 6},
						To:      TokenInfo{Symbol: "USDT", Address: "0xB", Blockchain: "BASE", Decimals: 6},
						Swapper: SwapperMeta{ID: "1inch"},
					},
				},
				EstimatedTimeInSeconds: 60,
			},
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	// 1% slippage — no OutputAmountMin, so fallback to computed
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.01,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	expected := decimal.NewFromInt(990_000) // 1000000 * (1 - 0.01)
	assert.True(t, quote.MinAmount.Equal(expected), "expected %s got %s", expected, quote.MinAmount)
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider("key")
	assert.Equal(t, "rango", p.Name())
}

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			Status: "success",
			BridgeData: &BridgeData{
				SrcTxHash:  "0xSrc",
				DestTxHash: "0xDst",
			},
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "rg-123")
	require.NoError(t, err)
	assert.Equal(t, "success", status.State)
	assert.Equal(t, "0xSrc", status.SrcChainTx)
	assert.Equal(t, "0xDst", status.DstChainTx)
}

func TestProvider_Status_Error(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "rg-123")
	require.Error(t, err)
}

func TestProvider_Status_NilResponse(t *testing.T) {
	p := &Provider{client: &mockClient{statusResp: nil}}
	status, err := p.Status(context.Background(), "rg-123")
	require.NoError(t, err)
	assert.Equal(t, "unknown", status.State)
	assert.Equal(t, "rg-123", status.TxID)
}

func TestProvider_Quote_UnsupportedChain(t *testing.T) {
	mock := &mockClient{quoteResp: &QuoteResponse{}}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: "unsupported-chain"},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported from chain")
}

func TestProvider_Quote_NilResponse(t *testing.T) {
	mock := &mockClient{quoteResp: nil}
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
