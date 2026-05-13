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
			Price:            "0.999",
			GuaranteedPrice:  "0.998",
			To:               "0xRouter",
			Data:             "0xdeadbeef",
			Value:            "0",
			Gas:              "200000",
			BuyAmount:        "999000",
			SellAmount:       "1000000",
			BuyTokenAddress:  "0xB",
			SellTokenAddress: "0xA",
			Sources: []SourceInfo{
				{Name: "Uniswap", Proportion: "0.5"},
				{Name: "SushiSwap", Proportion: "0.5"},
			},
			Fee: FeeInfo{FeeAmount: "500"},
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
			Gas: "not-a-number",
			Transaction: TxData{
				Gas: "also-bad",
			},
			BuyAmount:  "999000",
			SellAmount: "1000000",
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
			Gas:        "150000",
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005, // 0.5% → 50 bps
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}

	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "50", mock.lastQuoteParams.SlippageBps)
	assert.Equal(t, 0.005, quote.Slippage)
}

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{statusResp: &StatusResponse{Status: "reachable"}}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "0xTx")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "reachable", status.State)
	assert.Equal(t, "0xTx", status.TxID)
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
			Data:       "0xzz",
			Gas:        "150000",
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

func TestMapQuote_GuaranteedPrice(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount:      "1000000",
		BuyAmount:       "999000",
		GuaranteedPrice: "0.998",
		To:              "0xRouter",
		Data:            "0xdeadbeef",
		Gas:             "200000",
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	// minAmount = fromAmt * guaranteedPrice = 1000000 * 0.998 = 998000
	assert.Equal(t, int64(998_000), quote.MinAmount.IntPart())
}

func TestMapQuote_GuaranteedPriceZeroFallback(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount:      "1000000",
		BuyAmount:       "999000",
		GuaranteedPrice: "0",
		To:              "0xRouter",
		Data:            "0xdeadbeef",
		Gas:             "200000",
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	// fallback to 0.995 * toAmount = 994005
	assert.Equal(t, int64(994_005), quote.MinAmount.IntPart())
}

func TestMapQuote_TransactionFallback(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		To:         "0xOldRouter",
		Data:       "0xdeadbeef",
		Value:      "0",
		Gas:        "100000",
		Transaction: TxData{
			To:       "0xNewRouter",
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
	assert.Equal(t, "0xNewRouter", quote.To)
	assert.Equal(t, []byte{0xca, 0xfe, 0xba, 0xbe}, quote.TxData)
	assert.True(t, quote.TxValue.Equal(decimal.NewFromInt(1000)))
	assert.Equal(t, uint64(200_000), quote.EstimateGas)
}

func TestMapQuote_InvalidSellAmount(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "invalid",
		BuyAmount:  "999000",
		To:         "0xRouter",
		Data:       "0xdeadbeef",
		Gas:        "200000",
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	// fallback to req.Amount
	assert.Equal(t, int64(1_000_000), quote.FromAmount.IntPart())
}

func TestMapQuote_InvalidBuyAmount(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "invalid",
		To:         "0xRouter",
		Data:       "0xdeadbeef",
		Gas:        "200000",
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

func TestMapQuote_InvalidGuaranteedPrice(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount:      "1000000",
		BuyAmount:       "999000",
		GuaranteedPrice: "bad",
		To:              "0xRouter",
		Data:            "0xdeadbeef",
		Gas:             "200000",
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req)
	require.NoError(t, err)
	// fallback to 0.995 * toAmount
	assert.Equal(t, int64(994_005), quote.MinAmount.IntPart())
}

func TestMapQuote_InvalidFeeAmount(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		To:         "0xRouter",
		Data:       "0xdeadbeef",
		Gas:        "200000",
		Fee:        FeeInfo{FeeAmount: "bad-fee"},
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

func TestMapQuote_EmptySourcesFallback(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		To:         "0xRouter",
		Data:       "0xdeadbeef",
		Gas:        "200000",
		Sources:    []SourceInfo{},
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

func TestMapQuote_ZeroProportionSources(t *testing.T) {
	qr := &QuoteResponse{
		SellAmount: "1000000",
		BuyAmount:  "999000",
		To:         "0xRouter",
		Data:       "0xdeadbeef",
		Gas:        "200000",
		Sources: []SourceInfo{
			{Name: "Uniswap", Proportion: "0"},
			{Name: "SushiSwap", Proportion: "0"},
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

func TestProvider_Status_NilResponse(t *testing.T) {
	mock := &mockClient{statusResp: nil}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "0xTx")
	require.NoError(t, err)
	assert.Equal(t, "unknown", status.State)
}
