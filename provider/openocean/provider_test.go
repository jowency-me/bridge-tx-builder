package openocean

import (
	"context"
	"encoding/json"
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
			Code: 200,
			Data: &QuoteData{
				To:           "0xRouter",
				Data:         "0xdeadbeef",
				Value:        "0",
				OutAmount:    "999000",
				EstimatedGas: json.Number("200000"),
				InToken:      TokenDetail{Symbol: "USDC", Address: "0xA", Decimals: 6},
				OutToken:     TokenDetail{Symbol: "USDT", Address: "0xB", Decimals: 6},
				InAmount:     "1000000",
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
	assert.Equal(t, "oo-999000", quote.ID)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, "openocean", quote.Provider)
	assert.Equal(t, uint64(200_000), quote.EstimateGas)
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

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, "openocean", p.Name())
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

func TestProvider_Status_Error(t *testing.T) {
	p := &Provider{client: &mockClient{}}
	ctx := context.Background()
	_, err := p.Status(ctx, "0xtxid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}

func TestProvider_Quote_UnsupportedChain(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: "9999"},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported from chain")
}

func TestProvider_Quote_UnsupportedToChain(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: "9999"},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported to chain")
}

func TestMapQuote_InvalidOutAmount(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Code: 200,
			Data: &QuoteData{
				To:           "0xRouter",
				Data:         "0xdeadbeef",
				Value:        "0",
				OutAmount:    "not_a_number",
				EstimatedGas: json.Number("200000"),
				InToken:      TokenDetail{Symbol: "USDC", Address: "0xA", Decimals: 6},
				OutToken:     TokenDetail{Symbol: "USDT", Address: "0xB", Decimals: 6},
				InAmount:     "1000000",
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
	assert.True(t, quote.ToAmount.IsZero())
}

func TestMapQuote_NilData(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{Code: 200, Data: nil},
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
	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty data in response")
}

func TestMapQuote_NonZeroValue(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Code: 200,
			Data: &QuoteData{
				To:           "0xRouter",
				Data:         "0x",
				Value:        "1000000000000000000",
				OutAmount:    "999000",
				EstimatedGas: json.Number("200000"),
				InToken:      TokenDetail{Symbol: "ETH", Address: "0xE", Decimals: 18},
				OutToken:     TokenDetail{Symbol: "USDC", Address: "0xA", Decimals: 6},
				InAmount:     "1000000000000000000",
			},
		},
	}
	p := &Provider{client: mock}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0xE", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000_000_000_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.True(t, quote.TxValue.GreaterThan(decimal.Zero))
}

func TestMapQuote_DataNo0xPrefix(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Code: 200,
			Data: &QuoteData{
				To:           "0xRouter",
				Data:         "deadbeef",
				Value:        "0",
				OutAmount:    "999000",
				EstimatedGas: json.Number("200000"),
				InToken:      TokenDetail{Symbol: "USDC", Address: "0xA", Decimals: 6},
				OutToken:     TokenDetail{Symbol: "USDT", Address: "0xB", Decimals: 6},
				InAmount:     "1000000",
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
	assert.Empty(t, quote.TxData)
}

func TestProvider_Quote_SlippageBasedMinAmount(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Code: 200,
			Data: &QuoteData{
				To:           "0xRouter",
				Data:         "0xdeadbeef",
				Value:        "0",
				OutAmount:    "1000000",
				EstimatedGas: json.Number("200000"),
				InToken:      TokenDetail{Symbol: "USDC", Address: "0xA", Decimals: 6},
				OutToken:     TokenDetail{Symbol: "USDT", Address: "0xB", Decimals: 6},
				InAmount:     "1000000",
			},
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	// 1% slippage
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
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

func TestProvider_Quote_ApprovalAddress(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Code: 200,
			Data: &QuoteData{
				To:           "0xRouter",
				Data:         "0xdeadbeef",
				Value:        "0",
				OutAmount:    "999000",
				EstimatedGas: json.Number("200000"),
				InToken:      TokenDetail{Symbol: "USDC", Address: "0xA", Decimals: 6},
				OutToken:     TokenDetail{Symbol: "USDT", Address: "0xB", Decimals: 6},
				InAmount:     "1000000",
			},
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
	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "0xRouter", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
	assert.Equal(t, int64(1_000_000), quote.AllowanceNeeded.IntPart())
}

func TestProvider_Quote_ApprovalSkippedForNativeETH(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Code: 200,
			Data: &QuoteData{
				To:           "0xRouter",
				Data:         "0x",
				Value:        "1000000000000000000",
				OutAmount:    "999000",
				EstimatedGas: json.Number("200000"),
				InToken:      TokenDetail{Symbol: "ETH", Address: "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18},
				OutToken:     TokenDetail{Symbol: "USDC", Address: "0xA", Decimals: 6},
				InAmount:     "1000000000000000000",
			},
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000_000_000_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.Empty(t, quote.ApprovalAddress)
	assert.Nil(t, quote.AllowanceNeeded)
}

func TestMapQuote_InvalidHexData(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Code: 200,
			Data: &QuoteData{
				To:           "0xRouter",
				Data:         "0xzz",
				Value:        "0",
				OutAmount:    "999000",
				EstimatedGas: json.Number("200000"),
				InToken:      TokenDetail{Symbol: "USDC", Address: "0xA", Decimals: 6},
				OutToken:     TokenDetail{Symbol: "USDT", Address: "0xB", Decimals: 6},
				InAmount:     "1000000",
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
	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tx data")
}
