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
			Success:    true,
			StatusCode: 200,
			Result: &QuoteResult{
				OriginChainID:      1,
				DestinationChainID: 8453,
				AutoRoute: &AutoRoute{
					UserOp:       "sign",
					OutputAmount: "999000",
					Output: OutputData{
						Amount:       "999000",
						MinAmountOut: "990000",
					},
					Slippage:      0.01,
					QuoteID:       "sock-123",
					EstimatedTime: 120,
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
	assert.Equal(t, 1, len(quote.Route))
	assert.Equal(t, "bridge", quote.Route[0].Action)
	assert.Equal(t, 0.01, quote.Slippage)
}

func TestProvider_Quote_NilResult(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{Success: true, StatusCode: 200},
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
	assert.Contains(t, err.Error(), "no route found")
	assert.Nil(t, quote)
}

func TestProvider_Quote_NilAutoRoute(t *testing.T) {
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			Success:    true,
			StatusCode: 200,
			Result:     &QuoteResult{},
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no route found")
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
			Result: []StatusResult{
				{
					OriginData: OriginData{
						TxHash: "0xSrc",
						Status: "COMPLETED",
					},
					DestinationData: DestinationData{
						TxHash: "0xDst",
						Status: "COMPLETED",
					},
				},
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
	mock := &mockClient{}
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

func TestMapQuote_ApprovalData(t *testing.T) {
	qr := &QuoteResponse{
		Success:    true,
		StatusCode: 200,
		Result: &QuoteResult{
			OriginChainID:      1,
			DestinationChainID: 8453,
			AutoRoute: &AutoRoute{
				OutputAmount: "999000",
				Output: OutputData{
					Amount:       "999000",
					MinAmountOut: "990000",
				},
				QuoteID:  "sock-456",
				Slippage: 0.01,
				ApprovalData: &ApprovalData{
					SpenderAddress: "0xSpender",
					Amount:         "1000000",
					TokenAddress:   "0xA",
				},
			},
		},
	}
	quote, err := mapQuote(qr, domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	})
	require.NoError(t, err)
	assert.Equal(t, "0xSpender", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
	assert.True(t, quote.AllowanceNeeded.Equal(decimal.NewFromInt(1_000_000)))
}

func TestMapQuote_NoApprovalData(t *testing.T) {
	qr := &QuoteResponse{
		Success:    true,
		StatusCode: 200,
		Result: &QuoteResult{
			OriginChainID:      1,
			DestinationChainID: 8453,
			AutoRoute: &AutoRoute{
				OutputAmount: "999000",
				Output: OutputData{
					Amount:       "999000",
					MinAmountOut: "990000",
				},
				QuoteID: "sock-789",
			},
		},
	}
	quote, err := mapQuote(qr, domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	})
	require.NoError(t, err)
	assert.Empty(t, quote.ApprovalAddress)
	assert.Nil(t, quote.AllowanceNeeded)
}

func TestMapQuote_InvalidOutputAmount(t *testing.T) {
	qr := &QuoteResponse{
		Success:    true,
		StatusCode: 200,
		Result: &QuoteResult{
			AutoRoute: &AutoRoute{
				OutputAmount: "not-a-number",
				QuoteID:      "sock-err",
			},
		},
	}
	quote, err := mapQuote(qr, domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	})
	require.NoError(t, err)
	assert.True(t, quote.ToAmount.IsZero())
}

func TestMapQuote_SlippageFromAPI(t *testing.T) {
	qr := &QuoteResponse{
		Success:    true,
		StatusCode: 200,
		Result: &QuoteResult{
			AutoRoute: &AutoRoute{
				OutputAmount: "999000",
				Slippage:     0.03,
				QuoteID:      "sock-slip",
			},
		},
	}
	quote, err := mapQuote(qr, domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	})
	require.NoError(t, err)
	assert.Equal(t, 0.03, quote.Slippage)
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
	assert.Contains(t, err.Error(), "no route found")
	assert.Nil(t, quote)
}

func TestMapQuote_MinAmountFromAPI(t *testing.T) {
	qr := &QuoteResponse{
		Success:    true,
		StatusCode: 200,
		Result: &QuoteResult{
			AutoRoute: &AutoRoute{
				OutputAmount: "1000000",
				Output: OutputData{
					Amount:       "1000000",
					MinAmountOut: "985000",
				},
				QuoteID: "sock-min",
			},
		},
	}
	quote, err := mapQuote(qr, domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(985000), quote.MinAmount.IntPart())
}
