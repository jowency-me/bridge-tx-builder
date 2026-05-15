package thorchain

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
			InboundAddress:    "bc1qt9723ak9t7lu7a97lt9kelq4gnrlmyvk4yhzwr",
			ExpectedAmountOut: "2035299208",
			Memo:              "=:ETH.ETH:0x86d526d6624AbC0178cF7296cD538Ecc080A95F1:0/1/0",
			Expiry:            time.Now().Add(10 * time.Minute).Unix(),
			SlippageBps:       50,
			Fees: FeesInfo{
				Total:       "2092072",
				Liquidity:   "2037232",
				SlippageBps: 9,
			},
		},
	}

	p := &Provider{client: mock}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000000000000000000000000000000000000000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.005,
		FromAddr:  "bc1qFrom",
		ToAddr:    "0xTo",
	}

	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.NotEmpty(t, quote.ID)
	assert.Equal(t, int64(2_035_299_208), quote.ToAmount.IntPart())
	assert.Equal(t, "thorchain", quote.Provider)
	assert.Equal(t, 2, len(quote.Route))
	assert.Equal(t, "bc1qt9723ak9t7lu7a97lt9kelq4gnrlmyvk4yhzwr", quote.To)
	assert.Equal(t, int64(2_092_072), quote.EstimateFee.IntPart())
	// M-05: Memo should be encoded as TxData
	assert.Equal(t, []byte("=:ETH.ETH:0x86d526d6624AbC0178cF7296cD538Ecc080A95F1:0/1/0"), quote.TxData)
}

func TestProvider_Quote_ERC20Approval(t *testing.T) {
	// ERC-20 deposit: ApprovalAddress should be set to InboundAddress
	// when fromToken is not native ETH or zero address.
	// Verified against THORChain docs: vault requires token approval for ERC-20 deposits.
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			InboundAddress:    "0xVaultAddress",
			ExpectedAmountOut: "500000000",
			Memo:              "=:BTC.BTC:bc1qTo:0/1/0",
			Expiry:            time.Now().Add(10 * time.Minute).Unix(),
			SlippageBps:       50,
			Fees: FeesInfo{
				Total: "5000000",
			},
		},
	}

	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "BTC", Address: "0x0000000000000000000000000000000000000000", Decimals: 8, ChainID: "bitcoin"},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "bc1qTo",
	}

	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "0xVaultAddress", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
	assert.Equal(t, int64(1_000_000), quote.AllowanceNeeded.IntPart())
}
func TestProvider_Quote_HTTPError(t *testing.T) {
	mock := &mockClient{quoteErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000000000000000000000000000000000000000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.005,
		FromAddr:  "bc1qFrom",
		ToAddr:    "0xTo",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
}

func TestProvider_Status_Success(t *testing.T) {
	mock := &mockClient{
		statusResp: &StatusResponse{
			Tx: StatusTxDetail{
				ID:    "0xTxID",
				Chain: "ETH",
			},
			Stages: StatusStages{
				InboundObserved:            InboundObservedStage{Completed: true},
				InboundConfirmationCounted: ConfirmCountedStage{Completed: true},
				InboundFinalised:           StageCompleted{Completed: true},
				SwapStatus:                 SwapStatusStage{Pending: false},
				SwapFinalised:              StageCompleted{Completed: true},
			},
		},
	}

	p := &Provider{client: mock}
	ctx := context.Background()
	status, err := p.Status(ctx, "tx-123")
	require.NoError(t, err)
	assert.Equal(t, "tx-123", status.TxID)
	assert.Equal(t, "completed", status.State)
	assert.Equal(t, "0xTxID", status.SrcChainTx)
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, "thorchain", p.Name())
}

func TestProvider_Status_Error(t *testing.T) {
	mock := &mockClient{statusErr: assert.AnError}
	p := &Provider{client: mock}
	ctx := context.Background()

	_, err := p.Status(ctx, "0xTx")
	require.Error(t, err)
}

func TestToThorchainAsset_Native(t *testing.T) {
	asset := toThorchainAsset("ETH", domain.Token{Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000"})
	assert.Equal(t, "ETH.ETH", asset)
}

func TestToThorchainAsset_ZeroAddress(t *testing.T) {
	asset := toThorchainAsset("ETH", domain.Token{Symbol: "WETH", Address: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"})
	assert.Equal(t, "ETH.ETH", asset)
}

func TestToThorchainAsset_BNB(t *testing.T) {
	asset := toThorchainAsset("BSC", domain.Token{Symbol: "BNB", Address: "0xA"})
	assert.Equal(t, "BSC.BSC", asset)
}

func TestConvertTo1e8_InvalidDecimals(t *testing.T) {
	res := convertTo1e8(decimal.NewFromInt(1_000_000), -1)
	assert.NotEmpty(t, res)
	res2 := convertTo1e8(decimal.NewFromInt(1_000_000), 20)
	assert.NotEmpty(t, res2)
}

func TestMapQuote_Nil(t *testing.T) {
	_, err := mapQuote(nil, domain.QuoteRequest{}, "ETH", "BASE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty quote response")
}

func TestProvider_Quote_UnsupportedChain(t *testing.T) {
	mock := &mockClient{quoteResp: &QuoteResponse{}}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainCosmos},
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

func TestToThorchainAsset_BTCNative(t *testing.T) {
	// Test BTC native asset (chain code matches symbol)
	asset := toThorchainAsset("BTC", domain.Token{Symbol: "BTC", Address: "0xA"})
	assert.Equal(t, "BTC.BTC", asset)
}

func TestToThorchainAsset_ERC20(t *testing.T) {
	// Test ERC20 token
	asset := toThorchainAsset("ETH", domain.Token{Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"})
	assert.Equal(t, "ETH.USDC-0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", asset)
}

func TestMapStatus_NilResponse(t *testing.T) {
	status := mapStatus(nil, "tx-123")
	assert.Equal(t, "tx-123", status.TxID)
	assert.Equal(t, "unknown", status.State)
}

func TestMapStatus_NoTxStatus_UsesStages(t *testing.T) {
	sr := &StatusResponse{
		Tx: StatusTxDetail{
			ID:    "0xSrc",
			Chain: "ETH",
		},
		Stages: StatusStages{
			InboundFinalised: StageCompleted{Completed: true},
			SwapStatus:       SwapStatusStage{Pending: false},
			SwapFinalised:    StageCompleted{Completed: true},
		},
	}
	status := mapStatus(sr, "tx-123")
	assert.Equal(t, "completed", status.State)
}

func TestMapStatus_CompletedStages(t *testing.T) {
	sr := &StatusResponse{
		Tx: StatusTxDetail{
			ID:    "0xSrc",
			Chain: "ETH",
		},
		Stages: StatusStages{
			InboundFinalised: StageCompleted{Completed: true},
			SwapFinalised:    StageCompleted{Completed: true},
			SwapStatus:       SwapStatusStage{Pending: false},
		},
	}
	status := mapStatus(sr, "tx-456")
	assert.Equal(t, "completed", status.State)
}

func TestMapStatus_PendingState(t *testing.T) {
	sr := &StatusResponse{
		Tx: StatusTxDetail{
			ID:    "0xSrc",
			Chain: "ETH",
		},
		Stages: StatusStages{},
	}
	status := mapStatus(sr, "tx-789")
	assert.Equal(t, "pending", status.State)
}

func TestProvider_Quote_UnsupportedToChain(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	ctx := context.Background()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0xA", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "ATOM", Address: "0xB", Decimals: 6, ChainID: domain.ChainCosmos},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xFrom",
		ToAddr:    "cosmos1To",
	}

	_, err := p.Quote(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported to chain")
}

func TestMapQuote_FeesParseError(t *testing.T) {
	qr := &QuoteResponse{
		InboundAddress:    "bc1qt9723ak9t7lu7a97lt9kelq4gnrlmyvk4yhzwr",
		ExpectedAmountOut: "1000000",
		Memo:              "=:ETH.ETH:0x86d526d6624AbC0178cF7296cD538Ecc080A95F1:0/1/0",
		Expiry:            time.Now().Add(10 * time.Minute).Unix(),
		SlippageBps:       50,
		Fees: FeesInfo{
			Total: "invalid", // invalid, will fail parse
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000000000000000000000000000000000000000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req, "BTC", "ETH")
	require.NoError(t, err)
	// feeAmt should be 0 when parse fails, not error
	assert.Equal(t, "0", quote.EstimateFee.String())
}

func TestMapQuote_MinAmountNegative(t *testing.T) {
	// When fee > toAmt, minAmount becomes negative, fallback to 0.995 * toAmt
	qr := &QuoteResponse{
		InboundAddress:    "bc1qt9723",
		ExpectedAmountOut: "100000", // small amount
		Memo:              "=:ETH",
		Expiry:            time.Now().Add(10 * time.Minute).Unix(),
		SlippageBps:       50,
		Fees: FeesInfo{
			Total: "200000", // fee > toAmt, so minAmount negative
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req, "BTC", "ETH")
	require.NoError(t, err)
	// Should fallback to 0.995 * toAmt
	expectedMin := decimal.NewFromFloat(99500.0)
	assert.True(t, quote.MinAmount.Equal(expectedMin) || quote.MinAmount.Equal(decimal.NewFromFloat(99500.0)))
}

func TestMapQuote_ExpiryZero(t *testing.T) {
	qr := &QuoteResponse{
		InboundAddress:    "bc1qt9723",
		ExpectedAmountOut: "1000000",
		Memo:              "=:ETH",
		Expiry:            0, // zero expiry
		SlippageBps:       50,
		Fees: FeesInfo{
			Total: "1000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req, "BTC", "ETH")
	require.NoError(t, err)
	// Deadline should be ~10 minutes from now when expiry is 0
	assert.True(t, quote.Deadline.After(time.Now()))
	assert.True(t, quote.Deadline.Before(time.Now().Add(11*time.Minute)))
}

func TestConvertTo1e8_Valid(t *testing.T) {
	// 100_000_000 sats with 8 decimals → factor=1e8 → amountInUnits=1 → *1e8 = 100000000
	res := convertTo1e8(decimal.NewFromInt(100_000_000), 8)
	assert.Equal(t, "100000000", res)
}

func TestConvertTo1e8_18Decimals(t *testing.T) {
	// 1e18 wei with 18 decimals → factor=1e18 → amountInUnits=1 → *1e8 = 100000000
	res := convertTo1e8(decimal.NewFromInt(1_000_000_000_000_000_000), 18)
	assert.Equal(t, "100000000", res)
}

func TestMapQuote_SwapRouteFromChain(t *testing.T) {
	qr := &QuoteResponse{
		InboundAddress:    "bc1qt9723",
		ExpectedAmountOut: "1000000",
		Memo:              "=:ETH",
		Expiry:            time.Now().Add(10 * time.Minute).Unix(),
		SlippageBps:       50,
		Fees: FeesInfo{
			Total: "1000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req, "BTC", "ETH")
	require.NoError(t, err)
	require.Len(t, quote.Route, 2)
	assert.Equal(t, domain.ChainBitcoin, quote.Route[0].ChainID)
	assert.Equal(t, "swap", quote.Route[0].Action)
	assert.Equal(t, domain.ChainEthereum, quote.Route[1].ChainID)
	assert.Equal(t, "bridge", quote.Route[1].Action)
}

func TestToThorchainAsset_SOLNative(t *testing.T) {
	// SOL native (Solana chain)
	asset := toThorchainAsset("SOL", domain.Token{Symbol: "SOL", Address: "0xA"})
	assert.Equal(t, "SOL.SOL", asset)
}

func TestToThorchainAsset_BNBChainNative(t *testing.T) {
	// BSC native BNB
	asset := toThorchainAsset("BSC", domain.Token{Symbol: "BNB", Address: "0xA"})
	assert.Equal(t, "BSC.BSC", asset)
}

func TestToThorchainAsset_AVAXNative(t *testing.T) {
	// AVAX native
	asset := toThorchainAsset("AVAX", domain.Token{Symbol: "AVAX", Address: "0xA"})
	assert.Equal(t, "AVAX.AVAX", asset)
}

func TestToThorchainAsset_TRXNative(t *testing.T) {
	// TRON native
	asset := toThorchainAsset("TRX", domain.Token{Symbol: "TRX", Address: "0xA"})
	assert.Equal(t, "TRX.TRX", asset)
}

func TestToThorchainAsset_ERC20MixedCase(t *testing.T) {
	// ERC20 token - address is used as-is (not lowercased in the output string)
	asset := toThorchainAsset("ETH", domain.Token{Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"})
	assert.Contains(t, asset, "ETH.USDC-0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
}

func TestMapQuote_ToAmountParseError(t *testing.T) {
	// When ExpectedAmountOut is invalid, toAmt becomes zero
	qr := &QuoteResponse{
		InboundAddress:    "bc1qt9723",
		ExpectedAmountOut: "not-a-number",
		Memo:              "=:ETH",
		Expiry:            time.Now().Add(10 * time.Minute).Unix(),
		SlippageBps:       50,
		Fees: FeesInfo{
			Total: "1000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req, "BTC", "ETH")
	require.NoError(t, err)
	assert.True(t, quote.ToAmount.IsZero())
}

func TestMapQuote_SlippageBpsZero(t *testing.T) {
	// When SlippageBps is 0, use req.Slippage
	qr := &QuoteResponse{
		InboundAddress:    "bc1qt9723",
		ExpectedAmountOut: "1000000",
		Memo:              "=:ETH",
		Expiry:            time.Now().Add(10 * time.Minute).Unix(),
		SlippageBps:       0, // zero slippage
		Fees: FeesInfo{
			Total: "1000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.007, // 0.7%
	}
	quote, err := mapQuote(qr, req, "BTC", "ETH")
	require.NoError(t, err)
	assert.Equal(t, 0.007, quote.Slippage)
}

func TestConvertTo1e8_ZeroDecimals(t *testing.T) {
	// Zero decimals
	res := convertTo1e8(decimal.NewFromInt(100), 0)
	assert.NotEmpty(t, res)
}

func TestConvertTo1e8_Decimals17(t *testing.T) {
	// 17 decimals (within range)
	res := convertTo1e8(decimal.NewFromInt(1_000_000_000_000_000_000), 17)
	assert.NotEmpty(t, res)
}

func TestConvertTo1e8_Decimals19_Clamped(t *testing.T) {
	// 19 decimals > 18, clamped to 18
	res := convertTo1e8(decimal.NewFromInt(1_000_000_000_000_000_000), 19)
	assert.NotEmpty(t, res)
}

func TestMapQuote_MemoEncodedAsTxData(t *testing.T) {
	// M-05: Memo should be encoded as TxData for EVM deposits
	qr := &QuoteResponse{
		InboundAddress:    "0xInboundVault",
		ExpectedAmountOut: "500000000",
		Memo:              "=:ETH.ETH:0x86d526d6624AbC0178cF7296cD538Ecc080A95F1:0/1/0",
		Expiry:            time.Now().Add(10 * time.Minute).Unix(),
		SlippageBps:       50,
		Fees: FeesInfo{
			Total: "5000000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "BTC", Address: "0x0000000000000000000000000000000000000000", Decimals: 8, ChainID: "bitcoin"},
		Amount:    decimal.NewFromInt(1_000_000_000_000_000_000),
		Slippage:  0.01,
	}
	quote, err := mapQuote(qr, req, "ETH", "BTC")
	require.NoError(t, err)
	assert.Equal(t, []byte("=:ETH.ETH:0x86d526d6624AbC0178cF7296cD538Ecc080A95F1:0/1/0"), quote.TxData)
	assert.Equal(t, "0xInboundVault", quote.To)
}

func TestMapQuote_EmptyMemoNoTxData(t *testing.T) {
	// When memo is empty, TxData should be nil
	qr := &QuoteResponse{
		InboundAddress:    "bc1qt9723",
		ExpectedAmountOut: "1000000",
		Memo:              "",
		Expiry:            time.Now().Add(10 * time.Minute).Unix(),
		SlippageBps:       50,
		Fees: FeesInfo{
			Total: "1000",
		},
	}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "BTC", Address: "0x0000", Decimals: 8, ChainID: "bitcoin"},
		ToToken:   domain.Token{Symbol: "ETH", Address: "0x0000", Decimals: 18, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(100_000_000),
		Slippage:  0.005,
	}
	quote, err := mapQuote(qr, req, "BTC", "ETH")
	require.NoError(t, err)
	assert.Nil(t, quote.TxData)
}

func TestProvider_Quote_WithValidMock(t *testing.T) {
	// Test that Quote returns correct ID format
	mock := &mockClient{
		quoteResp: &QuoteResponse{
			InboundAddress:    "bc1qt9723",
			ExpectedAmountOut: "500000000",
			Memo:              "swaps:ETH.ETH",
			Expiry:            time.Now().Add(20 * time.Minute).Unix(),
			SlippageBps:       100,
			Fees: FeesInfo{
				Total: "5000000",
			},
		},
	}
	p := &Provider{client: mock}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "BTC", Address: "0x0000000000000000000000000000000000000000", Decimals: 8, ChainID: "bitcoin"},
		Amount:    decimal.NewFromInt(1_000_000_000_000_000_000),
		Slippage:  0.01,
		FromAddr:  "0xFrom",
		ToAddr:    "bc1qTo",
	}

	quote, err := p.Quote(ctx, req)
	require.NoError(t, err)
	// ID should contain "thorchain-"
	assert.Contains(t, quote.ID, "thorchain-")
	// Slippage from response (100 bps = 1%)
	assert.InDelta(t, 0.01, quote.Slippage, 0.0001)
}
