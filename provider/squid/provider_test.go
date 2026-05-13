package squid

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
			RequestID: "sqd-123",
			Route: RouteData{
				Estimate: Estimate{
					FromAmount:  "1000000",
					ToAmount:    "999000",
					ToAmountMin: "995000",
					GasCosts: []GasCost{
						{Estimate: "50000"},
					},
					FeeCosts: []FeeCost{
						{Amount: "100"},
					},
					EstimatedRouteDuration: 300,
				},
				TransactionRequest: TransactionRequest{
					TargetAddress: "0xRouter",
					Data:          "0xdeadbeef",
					Value:         "0",
					GasLimit:      "200000",
				},
				Params: struct {
					FromToken TokenInfo `json:"fromToken"`
					ToToken   TokenInfo `json:"toToken"`
				}{
					FromToken: TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 1},
					ToToken:   TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: 8453},
				},
			},
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
	assert.Equal(t, "sqd-123", quote.ID)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, int64(995_000), quote.MinAmount.IntPart())
	assert.Equal(t, "squid", quote.Provider)
	assert.Equal(t, 2, len(quote.Route))
	assert.Equal(t, uint64(50_000), quote.EstimateGas)
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

func TestProvider_Quote_InvalidGas(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			RequestID: "sqd-123",
			Route: RouteData{
				Estimate: Estimate{
					FromAmount: "1000000",
					ToAmount:   "999000",
					GasCosts: []GasCost{
						{Estimate: "not-a-number"},
					},
				},
				TransactionRequest: TransactionRequest{
					TargetAddress: "0xRouter",
					Data:          "0xdeadbeef",
					Value:         "0",
					GasLimit:      "200000",
				},
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

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse gas cost")
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, "squid", p.Name())
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

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			ID:     "tx-456",
			Status: "completed",
			FromChain: &ChainTxInfo{
				TransactionID:  "0xsrc",
				TransactionURL: "https://etherscan.io/tx/0xsrc",
			},
			ToChain: &ChainTxInfo{
				TransactionID:  "0xdst",
				TransactionURL: "https://basescan.io/tx/0xdst",
			},
		},
	}
	p := &Provider{client: mock}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status, err := p.Status(ctx, "tx-456")
	require.NoError(t, err)
	assert.Equal(t, "tx-456", status.TxID)
	assert.Equal(t, "completed", status.State)
	assert.Equal(t, "0xsrc", status.SrcChainTx)
	assert.Equal(t, "0xdst", status.DstChainTx)
}

func TestProvider_Status_HTTPError(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "tx-err")
	require.Error(t, err)
}

func TestProvider_Status_NilChains(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			ID:     "tx-empty",
			Status: "pending",
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "tx-empty")
	require.NoError(t, err)
	assert.Equal(t, "tx-empty", status.TxID)
	assert.Equal(t, "pending", status.State)
	assert.Empty(t, status.SrcChainTx)
	assert.Empty(t, status.DstChainTx)
}

func TestProvider_Quote_UnsupportedFromChain(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0xA", Decimals: 18, ChainID: domain.ChainID("999999")},
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
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainID("999999")},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported to chain")
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
	assert.Contains(t, err.Error(), "empty quote response")
}

func TestProvider_Quote_EmptyGasCosts(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			RequestID: "sqd-empty-gas",
			Route: RouteData{
				Estimate: Estimate{
					FromAmount:  "1000000",
					ToAmount:    "999000",
					ToAmountMin: "995000",
					GasCosts:    []GasCost{},
					FeeCosts: []FeeCost{
						{Amount: "100"},
					},
					EstimatedRouteDuration: 300,
				},
				TransactionRequest: TransactionRequest{
					TargetAddress: "0xRouter",
					Data:          "0xdeadbeef",
					Value:         "0",
					GasLimit:      "200000",
				},
				Params: struct {
					FromToken TokenInfo `json:"fromToken"`
					ToToken   TokenInfo `json:"toToken"`
				}{
					FromToken: TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 1},
					ToToken:   TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: 8453},
				},
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
	assert.Equal(t, uint64(200_000), quote.EstimateGas)
}

func TestProvider_Quote_EmptyFeeCosts(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			RequestID: "sqd-empty-fee",
			Route: RouteData{
				Estimate: Estimate{
					FromAmount:  "1000000",
					ToAmount:    "999000",
					ToAmountMin: "995000",
					GasCosts: []GasCost{
						{Estimate: "50000"},
					},
					FeeCosts:               []FeeCost{},
					EstimatedRouteDuration: 300,
				},
				TransactionRequest: TransactionRequest{
					TargetAddress: "0xRouter",
					Data:          "0xdeadbeef",
					Value:         "0",
					GasLimit:      "200000",
				},
				Params: struct {
					FromToken TokenInfo `json:"fromToken"`
					ToToken   TokenInfo `json:"toToken"`
				}{
					FromToken: TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 1},
					ToToken:   TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: 8453},
				},
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
	assert.True(t, quote.EstimateFee.IsZero())
}

func TestProvider_Quote_NonZeroValue(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			RequestID: "sqd-value",
			Route: RouteData{
				Estimate: Estimate{
					FromAmount:  "1000000",
					ToAmount:    "999000",
					ToAmountMin: "995000",
					GasCosts: []GasCost{
						{Estimate: "50000"},
					},
					FeeCosts: []FeeCost{
						{Amount: "100"},
					},
					EstimatedRouteDuration: 300,
				},
				TransactionRequest: TransactionRequest{
					TargetAddress: "0xRouter",
					Data:          "0xdeadbeef",
					Value:         "1000000000000000000",
					GasLimit:      "200000",
				},
				Params: struct {
					FromToken TokenInfo `json:"fromToken"`
					ToToken   TokenInfo `json:"toToken"`
				}{
					FromToken: TokenInfo{Symbol: "ETH", Address: "0xE", Decimals: 18, ChainID: 1},
					ToToken:   TokenInfo{Symbol: "ETH", Address: "0xE", Decimals: 18, ChainID: 8453},
				},
			},
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0xE", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0xE", Decimals: 18, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "1000000000000000000", quote.TxValue.String())
}

func TestProvider_Quote_InvalidGasLimit(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			RequestID: "sqd-123",
			Route: RouteData{
				Estimate: Estimate{
					FromAmount: "1000000",
					ToAmount:   "999000",
					GasCosts:   []GasCost{},
				},
				TransactionRequest: TransactionRequest{
					TargetAddress: "0xRouter",
					Data:          "0xdeadbeef",
					Value:         "0",
					GasLimit:      "not-a-number",
				},
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

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse gas limit")
}

func TestProvider_Quote_EmptyGasCostEstimate(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			RequestID: "sqd-empty-est",
			Route: RouteData{
				Estimate: Estimate{
					FromAmount:  "1000000",
					ToAmount:    "999000",
					ToAmountMin: "995000",
					GasCosts: []GasCost{
						{Estimate: ""},
					},
					FeeCosts: []FeeCost{
						{Amount: "100"},
					},
					EstimatedRouteDuration: 300,
				},
				TransactionRequest: TransactionRequest{
					TargetAddress: "0xRouter",
					Data:          "0xdeadbeef",
					Value:         "0",
					GasLimit:      "200000",
				},
				Params: struct {
					FromToken TokenInfo `json:"fromToken"`
					ToToken   TokenInfo `json:"toToken"`
				}{
					FromToken: TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: 1},
					ToToken:   TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: 8453},
				},
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
	// Should fall back to GasLimit since GasCosts[0].Estimate is empty
	assert.Equal(t, uint64(200_000), quote.EstimateGas)
}
