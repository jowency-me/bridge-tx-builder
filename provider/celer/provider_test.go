package celer

import (
	"context"
	"net/http"
	"net/http/httptest"
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
			EqValueTokenAmt:   "999000",
			PercFee:           "500",
			BaseFee:           "500",
			SlippageTolerance: 5000,
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
	assert.Equal(t, "celer", quote.Provider)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, int64(500), quote.EstimateFee.IntPart())
	assert.Equal(t, 1, len(quote.Route))
	assert.Equal(t, "bridge", quote.Route[0].Action)
	// Verify slippage from response (5000 / 1e6 = 0.005)
	assert.Equal(t, 0.005, quote.Slippage)
	// Verify MinAmount is computed (0.995 * ToAmount)
	assert.True(t, quote.MinAmount.IsPositive())
	assert.True(t, quote.MinAmount.LessThan(quote.ToAmount))
	// Verify token chain IDs preserved
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
	assert.Equal(t, "celer", p.Name())
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

func TestProvider_Quote_UnsupportedFromChain(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainSolana},
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

func TestProvider_Quote_UnsupportedToChain(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainSolana},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported to chain")
}

func TestProvider_Quote_EmptyQuoteResponse(t *testing.T) {
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
	assert.Contains(t, err.Error(), "empty quote response")
}

func TestProvider_Quote_InvalidToAmount(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			EqValueTokenAmt:   "not-a-number",
			PercFee:           "500",
			SlippageTolerance: 5000,
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
	assert.True(t, quote.ToAmount.IsZero())
}

func TestProvider_Quote_EmptyPercFee(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			EqValueTokenAmt:   "999000",
			PercFee:           "",
			SlippageTolerance: 5000,
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
	assert.True(t, quote.EstimateFee.IsZero())
}

func TestProvider_Quote_InvalidPercFee(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			EqValueTokenAmt:   "999000",
			PercFee:           "not-a-number",
			SlippageTolerance: 5000,
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
	assert.True(t, quote.EstimateFee.IsZero())
}

func TestProvider_Quote_ZeroSlippageTolerance(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			EqValueTokenAmt:   "999000",
			PercFee:           "500",
			SlippageTolerance: 0,
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()
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
	assert.Equal(t, 0.01, quote.Slippage)
}

func TestProvider_Quote_NonZeroSlippageTolerance(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			EqValueTokenAmt:   "999000",
			PercFee:           "500",
			SlippageTolerance: 10000,
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
	assert.Equal(t, 0.01, quote.Slippage)
}

func TestProvider_Status(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()
	_, err := p.Status(ctx, "0x123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}

func TestProvider_Quote_RequestValidationError(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
}

// TestProvider_Quote_SlippageSentToAPI verifies that slippage is correctly
// converted from decimal (0.005) to millionths (5000) and sent to the API.
// This is a regression test for C-01: slippage was never passed to the API.
func TestProvider_Quote_SlippageSentToAPI(t *testing.T) {
	var receivedSlippage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSlippage = r.URL.Query().Get("slippage_tolerance")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"eq_value_token_amt":"999000","perc_fee":"500","base_fee":"100","slippage_tolerance":5000}`))
	}))
	defer srv.Close()

	p := NewProvider(WithBaseURL(srv.URL))
	ctx := context.Background()

	// 0.5% slippage = 0.005 in decimal = 5000 in millionths (1e6)
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
	require.NotNil(t, quote)
	assert.Equal(t, "5000", receivedSlippage, "slippage_tolerance should be 5000 (0.005 * 1e6) for 0.5%%")
}

// TestProvider_Quote_SlippageOnePercent verifies 1% slippage = 10000 millionths.
func TestProvider_Quote_SlippageOnePercent(t *testing.T) {
	var receivedSlippage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSlippage = r.URL.Query().Get("slippage_tolerance")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"eq_value_token_amt":"999000","perc_fee":"500","base_fee":"100","slippage_tolerance":10000}`))
	}))
	defer srv.Close()

	p := NewProvider(WithBaseURL(srv.URL))
	ctx := context.Background()

	// 1% slippage = 0.01 in decimal = 10000 in millionths (1e6)
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
	require.NotNil(t, quote)
	assert.Equal(t, "10000", receivedSlippage, "slippage_tolerance should be 10000 (0.01 * 1e6) for 1%%")
}

func TestProvider_Quote_SlippageBasedMinAmount(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			EqValueTokenAmt:   "1000000",
			PercFee:           "0.001",
			SlippageTolerance: 50000,
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.05,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	// SlippageTolerance=50000 means 0.05 fraction, so MinAmount = 1000000 * (1 - 0.05) = 950000
	expected := decimal.NewFromInt(950_000)
	assert.True(t, quote.MinAmount.Equal(expected), "expected %s got %s", expected, quote.MinAmount)
}
