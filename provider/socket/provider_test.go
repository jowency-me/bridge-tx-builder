package socket

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
			Routes: []Route{
				{
					RouteID:      "sock-123",
					ToAmount:     "999000",
					TotalGasFees: "1.50",
					TotalFee:     "2.00",
					UserTxs: []UserTx{
						{
							TxType:   "swap",
							TxData:   "0xdeadbeef",
							TxTarget: "0xRouter",
							ChainID:  "1",
						},
						{
							TxType:   "fund-movr",
							TxData:   "0xcafebabe",
							TxTarget: "0xBridge",
							ChainID:  "8453",
						},
					},
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
	assert.Equal(t, "socket-sock-123", quote.ID)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, "socket", quote.Provider)
	assert.Equal(t, 2, len(quote.Route))
	assert.Equal(t, "swap", quote.Route[0].Action)
	assert.Equal(t, "bridge", quote.Route[1].Action)
	assert.Equal(t, int64(2), quote.EstimateFee.IntPart())
}

func TestProvider_Quote_EmptyRoutes(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{Routes: []Route{}},
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no routes found")
	assert.Nil(t, quote)
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

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			Success: true,
			Result: Result{
				SourceTxHash:      "0xSrc",
				DestinationTxHash: "0xDst",
				Status:            "completed",
			},
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "0xTx")
	require.NoError(t, err)
	assert.Equal(t, "completed", status.State)
	assert.Equal(t, "0xSrc", status.SrcChainTx)
	assert.Equal(t, "0xDst", status.DstChainTx)
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, "socket", p.Name())
}

func TestProvider_Status_Error(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "0xTx")
	require.Error(t, err)
}

func TestProvider_Quote_UnsupportedChain(t *testing.T) {
	mock := &mockClient{quoteResp: &QuoteResponse{Routes: []Route{{RouteID: "1"}}}}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainBitcoin},
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

func TestProvider_NewProvider_WithOptions(t *testing.T) {
	p := NewProvider(WithBaseURL("https://custom.example.com"), WithAPIKey("key"))
	require.NotNil(t, p)
	assert.Equal(t, "socket", p.Name())
}

func TestMapStatus_Nil(t *testing.T) {
	st := mapStatus(nil, "0xTx")
	assert.Equal(t, "unknown", st.State)
	assert.Equal(t, "0xTx", st.TxID)
}

func TestNewProvider_WithBaseURL(t *testing.T) {
	p := NewProvider(WithBaseURL("https://custom.example.com"))
	require.NotNil(t, p)
}

func TestNewProvider_WithHTTPClient(t *testing.T) {
	p := NewProvider(WithHTTPClient(http.DefaultClient))
	require.NotNil(t, p)
}

func TestNewProvider_WithAPIKey(t *testing.T) {
	p := NewProvider(WithAPIKey("test-key"))
	require.NotNil(t, p)
}

func TestMapQuote_NilResponse(t *testing.T) {
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "0xTo",
	}
	quote, err := mapQuote(nil, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty quote response")
	assert.Nil(t, quote)
}

func TestMapQuote_InvalidToAmountDecimal(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Routes: []Route{
				{
					RouteID:  "sock-123",
					ToAmount: "not-a-number",
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
	assert.True(t, quote.ToAmount.IsZero())
}

func TestMapQuote_EmptyUserTxs(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Routes: []Route{
				{
					RouteID:      "sock-123",
					ToAmount:     "999000",
					TotalGasFees: "1.50",
					UserTxs:      []UserTx{},
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
	assert.Empty(t, quote.Route)
	assert.Empty(t, quote.TxData)
	assert.Equal(t, "", quote.To)
}

func TestMapQuote_InvalidHexData(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Routes: []Route{
				{
					RouteID:  "sock-123",
					ToAmount: "999000",
					UserTxs: []UserTx{
						{
							TxType:   "swap",
							TxData:   "0xGGGGGGGG", // invalid hex
							TxTarget: "0xRouter",
							ChainID:  "1",
						},
					},
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
	assert.Contains(t, err.Error(), "invalid tx data")
}

func TestMapQuote_InvalidTotalFeeDecimal(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Routes: []Route{
				{
					RouteID:      "sock-123",
					ToAmount:     "999000",
					TotalFee:     "invalid",
					TotalGasFees: "",
					UserTxs:      []UserTx{},
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

func TestMapQuote_InvalidGasFeesDecimal(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Routes: []Route{
				{
					RouteID:      "sock-123",
					ToAmount:     "999000",
					TotalFee:     "",
					TotalGasFees: "also-invalid",
					UserTxs:      []UserTx{},
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
