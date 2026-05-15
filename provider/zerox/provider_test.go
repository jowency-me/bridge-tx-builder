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
	quoteResp       *QuoteResponse
	quoteErr        error
	lastQuoteParams QuoteParams
	statusResp      *StatusResponse
	statusErr       error
}

func (m *mockClient) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	m.lastQuoteParams = params
	return m.quoteResp, m.quoteErr
}

func (m *mockClient) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	return m.statusResp, m.statusErr
}

func TestProvider_Quote_Success(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			BuyAmount:  "999000",
			SellAmount: "1000000",
			Route: RouteData{
				Fills: []RouteFill{
					{Source: "Uniswap", Proportion: "0.5"},
					{Source: "SushiSwap", Proportion: "0.5"},
				},
			},
			Fees: FeeData{ZeroExFee: &ZeroExFee{Amount: "500", Token: "0xB", Type: "volume"}},
			Transaction: TxData{
				To:   "0xRouter",
				Data: "0xdeadbeef",
				Gas:  "200000",
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
	assert.Equal(t, int64(1_000_000), quote.FromAmount.IntPart())
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, "0x", quote.Provider)
	assert.Equal(t, 2, len(quote.Route))
	assert.Equal(t, uint64(200_000), quote.EstimateGas)
	assert.True(t, quote.EstimateFee.Equal(decimal.NewFromInt(500)))
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
			BuyAmount:  "999000",
			SellAmount: "1000000",
			Transaction: TxData{
				Gas: "also-bad",
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

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse gas")
}

func TestProvider_Quote_SlippageMultipliedBy100(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			BuyAmount:  "999000",
			SellAmount: "1000000",
			Transaction: TxData{
				Gas: "150000",
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
	assert.Equal(t, "50", mock.lastQuoteParams.SlippageBps)
	assert.Equal(t, 0.005, quote.Slippage)
}

func TestProvider_Status_Success(t *testing.T) {
	// 0x does not provide a transaction status API.
	// Status() returns an error indicating status tracking is not supported.
	p := &Provider{client: &mockClient{}}
	ctx := context.Background()

	_, err := p.Status(ctx, "0xTx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}

func TestProvider_Status_Error(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "0xTx")
	require.Error(t, err)
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider("test-key")
	assert.Equal(t, "0x", p.Name())
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

func TestProvider_Quote_InvalidHexData(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			BuyAmount:  "999000",
			SellAmount: "1000000",
			Transaction: TxData{
				Data: "0xzz",
				Gas:  "150000",
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

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tx data")
}

func TestProvider_Quote_CrossChainNotSupported(t *testing.T) {
	mock := &mockClient{quoteResp: &QuoteResponse{}}
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
	assert.Contains(t, err.Error(), "cross-chain swaps are not supported")
}

func TestProvider_Quote_UnsupportedFromChain(t *testing.T) {
	mock := &mockClient{quoteResp: &QuoteResponse{}}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: "unsupported"},
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
	mock := &mockClient{quoteResp: &QuoteResponse{}}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: "unsupported"},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported to chain")
}

func TestMapQuote_Nil(t *testing.T) {
	_, err := mapQuote(nil, domain.QuoteRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty quote response")
}

func TestMapQuote_MinAmountFallback(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:   "0xRouter",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	// When minBuyAmount is empty, fallback to 0.995 * buyAmount
	assert.Equal(t, int64(994_005), quote.MinAmount.IntPart())
}

func TestMapQuote_MinBuyAmount(t *testing.T) {
	// When minBuyAmount is provided by the API, use it directly
	qr := &QuoteResponse{
		SellAmount:   "1000000000000000000",
		BuyAmount:    "995000000",
		MinBuyAmount: "990025000",
		Transaction: TxData{
			To:   "0xRouter",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000_000_000_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.Equal(t, int64(990_025_000), quote.MinAmount.IntPart())
}

func TestMapQuote_TransactionData(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:       "0xRouter",
			Data:     "0xcafebabe",
			Value:    "1000",
			Gas:      "200000",
			GasPrice: "10",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.Equal(t, "0xRouter", quote.To)
	assert.Equal(t, []byte{0xca, 0xfe, 0xba, 0xbe}, quote.TxData)
	assert.True(t, quote.TxValue.Equal(decimal.NewFromInt(1000)))
	assert.Equal(t, uint64(200_000), quote.EstimateGas)
}

func TestMapQuote_InvalidSellAmount(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "invalid",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:   "0xRouter",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000), quote.FromAmount.IntPart())
}

func TestMapQuote_InvalidBuyAmount(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "invalid",
		Transaction: TxData{
			To:   "0xRouter",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.True(t, quote.ToAmount.IsZero())
}

func TestMapQuote_InvalidFeeAmount(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:   "0xRouter",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
		Fees: FeeData{ZeroExFee: &ZeroExFee{Amount: "bad-fee", Token: "0xB", Type: "volume"}},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.True(t, quote.EstimateFee.IsZero())
}

func TestMapQuote_NilZeroExFee(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:   "0xRouter",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
		Fees: FeeData{ZeroExFee: nil},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.True(t, quote.EstimateFee.IsZero())
}

func TestMapQuote_EmptyFillsFallback(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:   "0xRouter",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
		Route: RouteData{Fills: []RouteFill{}},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.Equal(t, 1, len(quote.Route))
	assert.Equal(t, "0x", quote.Route[0].Protocol)
	assert.Equal(t, "swap", quote.Route[0].Action)
}

func TestMapQuote_ZeroProportionFills(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:   "0xRouter",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
		Route: RouteData{
			Fills: []RouteFill{
				{Source: "Uniswap", Proportion: "0"},
				{Source: "SushiSwap", Proportion: "0"},
			},
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.Equal(t, 1, len(quote.Route))
	assert.Equal(t, "0x", quote.Route[0].Protocol)
}

func TestMapQuote_ApprovalAddress(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:   "0xAllowanceHolder",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.Equal(t, "0xAllowanceHolder", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
	assert.True(t, quote.AllowanceNeeded.Equal(decimal.NewFromInt(1_000_000)))
}

func TestMapQuote_ApprovalSkippedForNativeToken(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		Transaction: TxData{
			To:   "0xAllowanceHolder",
			Data: "0xdeadbeef",
			Gas:  "200000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	assert.Empty(t, quote.ApprovalAddress)
	assert.Nil(t, quote.AllowanceNeeded)
}

func TestProvider_Status_NilResponse(t *testing.T) {
	p := &Provider{client: &mockClient{}}
	ctx := context.Background()

	_, err := p.Status(ctx, "0xTx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}
