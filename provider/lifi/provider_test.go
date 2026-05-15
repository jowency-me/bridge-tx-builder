package lifi

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
			ID:         "liq-123",
			FromAmount: "1000000",
			ToAmount:   "999000",
			Estimate: Estimate{
				ToAmountMin:     "995000",
				ApprovalAddress: "0xRouter",
				GasCosts: []GasCost{
					{Estimate: "50000"},
				},
			},
			Action: Action{
				FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
				ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
				FromChainID: 1,
				ToChainID:   8453,
			},
			IncludedSteps: []Step{
				{Type: "swap", Tool: "1inch"},
				{Type: "cross", Tool: "across"},
			},
			TransactionRequest: TxRequest{
				To:       "0xRouter",
				Data:     "0xdeadbeef",
				Value:    "0",
				GasLimit: "200000",
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
	assert.Equal(t, "liq-123", quote.ID)
	assert.Equal(t, int64(999_000), quote.ToAmount.IntPart())
	assert.Equal(t, int64(995_000), quote.MinAmount.IntPart())
	assert.Equal(t, "lifi", quote.Provider)
	assert.Equal(t, 2, len(quote.Route))
	assert.Equal(t, uint64(50_000), quote.EstimateGas)

	// Verify approval fields: when Estimate.ApprovalAddress is set,
	// AllowanceNeeded should be set to fromAmount.
	// Real LI.FI API returns approvalAddress for ERC-20 tokens (verified 2026-05-15).
	assert.Equal(t, "0xRouter", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
	assert.Equal(t, int64(1_000_000), quote.AllowanceNeeded.IntPart())
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
			ID:         "liq-123",
			FromAmount: "1000000",
			ToAmount:   "999000",
			Estimate: Estimate{
				ToAmountMin: "995000",
				GasCosts: []GasCost{
					{Estimate: "not-a-number"},
				},
			},
			Action: Action{
				FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
				ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
				FromChainID: 1,
				ToChainID:   8453,
			},
			TransactionRequest: TxRequest{
				To:       "0xRouter",
				Data:     "0xdeadbeef",
				Value:    "0",
				GasLimit: "200000",
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
	p := NewProvider("test-key")
	assert.Equal(t, "lifi", p.Name())
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

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			Status:           "DONE",
			Substatus:        "COMPLETED",
			SubstatusMessage: "Transaction completed",
			BridgeExplorer:   "https://explorer.li.fi/tx/0xabc123",
			TxHistoryURL:     "https://li.fi/history/0xabc123",
			TokenAmountIn:    "1000000",
			TokenAmountOut:   "995000",
			Sending: TxInfo{
				TxHash:  "0xabc123",
				ChainID: 1,
			},
			Receiving: TxInfo{
				TxHash:  "0xdef456",
				ChainID: 8453,
			},
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "0xtxhash")
	require.NoError(t, err)
	require.Equal(t, "completed", status.State)
	require.Equal(t, "0xabc123", status.SrcChainTx)
	require.Equal(t, "0xdef456", status.DstChainTx)
}

func TestProvider_Status_HTTPError(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "0xtxhash")
	require.Error(t, err)
}

func TestProvider_Status_NilResponse(t *testing.T) {
	mock := &mockClient{statusResp: nil}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "0xtxhash")
	require.NoError(t, err)
	require.Equal(t, "unknown", status.State)
}

func TestProvider_Status_EmptyStatus(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			Status: "",
			Sending: TxInfo{
				TxHash:  "0xabc123",
				ChainID: 1,
			},
		},
	}
	p := &Provider{client: mock}
	ctx := context.Background()

	status, err := p.Status(ctx, "0xtxhash")
	require.NoError(t, err)
	require.Equal(t, "unknown", status.State)
}

func TestMapQuote_NilResponse(t *testing.T) {
	_, err := mapQuote(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty quote response")
}

func TestMapQuote_DataWithout0xPrefix(t *testing.T) {
	// mapQuote only decodes when data starts with "0x"; without prefix it leaves txData nil
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "deadbeef",
			Value:    "0",
			GasLimit: "200000",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.Nil(t, quote.TxData)
}

func TestMapQuote_NilGasCostsFallsBackToGasLimit(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    nil,
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "0",
			GasLimit: "300000",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.Equal(t, uint64(300000), quote.EstimateGas)
}

func TestMapQuote_NilGasCostsFallsBackToHexGasLimit(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    nil,
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "0x0",
			GasLimit: "0x447b4",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.Equal(t, uint64(0x447b4), quote.EstimateGas)
}

func TestMapQuote_NonZeroTxValue(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "ETH", Address: "0xA", Decimals: 18},
			ToToken:     TokenInfo{Symbol: "WETH", Address: "0xB", Decimals: 18},
			FromChainID: 1,
			ToChainID:   1,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "1500000000000000000",
			GasLimit: "200000",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.True(t, quote.TxValue.GreaterThan(decimal.Zero))
}

func TestMapQuote_HexTxValue(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "ETH", Address: "0xA", Decimals: 18},
			ToToken:     TokenInfo{Symbol: "WETH", Address: "0xB", Decimals: 18},
			FromChainID: 1,
			ToChainID:   1,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "0x14d1120d7b160000",
			GasLimit: "200000",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.True(t, quote.TxValue.GreaterThan(decimal.Zero))
	// 0x14d1120d7b160000 = 1500000000000000000 (1.5 ETH in wei)
	require.Equal(t, "1500000000000000000", quote.TxValue.String())
}

func TestMapQuote_ZeroTxValue(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "0",
			GasLimit: "200000",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.True(t, quote.TxValue.IsZero())
}

func TestMapQuote_InvalidFromAmount(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "not-a-number",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "0",
			GasLimit: "200000",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.True(t, quote.FromAmount.IsZero())
}

func TestMapQuote_InvalidToAmount(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "not-a-number",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "0",
			GasLimit: "200000",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.True(t, quote.ToAmount.IsZero())
}

func TestMapQuote_InvalidMinAmount(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "not-a-number",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "0",
			GasLimit: "200000",
		},
	}
	quote, err := mapQuote(qr)
	require.NoError(t, err)
	require.True(t, quote.MinAmount.IsZero())
}

func TestMapQuote_InvalidGasLimit(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0x",
			Value:    "0",
			GasLimit: "not-a-number",
		},
	}
	_, err := mapQuote(qr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse gas limit")
}

func TestMapQuote_InvalidTxData(t *testing.T) {
	qr := &QuoteResponse{
		ID:         "liq-123",
		FromAmount: "1000000",
		ToAmount:   "999000",
		Estimate: Estimate{
			ToAmountMin: "995000",
			GasCosts:    []GasCost{},
		},
		Action: Action{
			FromToken:   TokenInfo{Symbol: "USDC", Address: "0xA", Decimals: 6},
			ToToken:     TokenInfo{Symbol: "USDT", Address: "0xB", Decimals: 6},
			FromChainID: 1,
			ToChainID:   8453,
		},
		IncludedSteps: []Step{},
		TransactionRequest: TxRequest{
			To:       "0xRouter",
			Data:     "0xzz",
			Value:    "0",
			GasLimit: "200000",
		},
	}
	_, err := mapQuote(qr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid tx data")
}
