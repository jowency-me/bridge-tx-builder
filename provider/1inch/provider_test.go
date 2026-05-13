package oneinch

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
			DstAmount: "999000",
			SrcAmount: "1000000",
			FromToken: TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:   TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			Tx: TxData{
				To:    "0xRouter",
				Data:  "0xdeadbeef",
				Value: "0",
				Gas:   200000,
			},
			Gas: 150000,
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
	assert.Equal(t, int64(1_000_000), quote.FromAmount.IntPart())
	assert.Equal(t, "1inch", quote.Provider)
	assert.Equal(t, 1, len(quote.Route))
	assert.Equal(t, uint64(150_000), quote.EstimateGas)
	assert.Equal(t, "0xRouter", quote.To)
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, quote.TxData)
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
	p := NewProvider("test-key")
	assert.Equal(t, "1inch", p.Name())
}

func TestProvider_WithHTTPClient(t *testing.T) {
	p := NewProvider("test-key", WithHTTPClient(http.DefaultClient))
	require.NotNil(t, p)
}

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{statusResp: &StatusResponse{Status: "reachable"}}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "0xTx")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "completed", status.State)
	assert.Equal(t, "0xTx", status.SrcChainTx)
}

func TestProvider_Status_Error(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "0xTx")
	require.Error(t, err)
}

func TestProvider_Quote_UnsupportedChain(t *testing.T) {
	mock := &mockClient{quoteResp: &QuoteResponse{}}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainSolana},
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

func TestProvider_Quote_NilResponse(t *testing.T) {
	mock := &mockClient{quoteResp: nil}
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

func TestProvider_NewProvider_WithOptions(t *testing.T) {
	p := NewProvider("test-key", WithBaseURL("https://custom.example.com"))
	require.NotNil(t, p)
	assert.Equal(t, "1inch", p.Name())
}

func TestMapQuote_Nil(t *testing.T) {
	_, err := mapQuote(nil, domain.QuoteRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty quote response")
}

func TestMapQuote_InvalidTxData(t *testing.T) {
	qr := &QuoteResponse{
		DstAmount: "1000",
		SrcAmount: "1000",
		Tx:        TxData{Data: "0xzz", To: "0xRouter"},
	}
	_, err := mapQuote(qr, domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tx data")
}

func TestMapQuote_NoHexPrefix(t *testing.T) {
	qr := &QuoteResponse{
		DstAmount: "1000",
		SrcAmount: "1000",
		Tx:        TxData{Data: "deadbeef", To: "0xRouter", Value: "100"},
	}
	q, err := mapQuote(qr, domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
	})
	require.NoError(t, err)
	assert.Nil(t, q.TxData)
}
