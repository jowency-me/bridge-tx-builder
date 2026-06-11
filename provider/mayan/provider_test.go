package mayan

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jowency-me/bridge-tx-builder/domain"
	hexutil "github.com/jowency-me/bridge-tx-builder/utils/hex"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	views    []QuoteView
	quoteErr error
	build    *BuildResponse
	buildErr error

	lastBuildQuote  json.RawMessage
	lastBuildParams BuildParams
}

func (m *mockClient) Quote(ctx context.Context, params QuoteParams) ([]QuoteView, error) {
	return m.views, m.quoteErr
}

func (m *mockClient) Build(ctx context.Context, quote json.RawMessage, params BuildParams) (*BuildResponse, error) {
	m.lastBuildQuote = quote
	m.lastBuildParams = params
	return m.build, m.buildErr
}

func evmBuild(to, data, value string, chainID int) *BuildResponse {
	br := &BuildResponse{Success: true}
	br.Transaction.ChainCategory = "evm"
	br.Transaction.QuoteType = "MCTP"
	br.Transaction.Transaction = json.RawMessage(
		`{"to":"` + to + `","data":"` + data + `","value":"` + value + `","chainId":` + itoa(chainID) + `}`)
	return br
}

func svmBuild(b64 string) *BuildResponse {
	br := &BuildResponse{Success: true}
	br.Transaction.ChainCategory = "svm"
	br.Transaction.QuoteType = "SWIFT"
	raw, _ := json.Marshal(b64)
	br.Transaction.Transaction = raw
	return br
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func evmReq() domain.QuoteRequest {
	return domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(10_000_000),
		Slippage:  0.005,
		FromAddr:  "0xSwapper", ToAddr: "0xDest",
	}
}

func TestProvider_Name(t *testing.T) {
	assert.Equal(t, "mayan", NewProvider().Name())
}

func TestNewProvider_WithBaseURLAndAPIKey(t *testing.T) {
	p := NewProvider(WithBaseURL("https://self-hosted:3000/"), WithAPIKey("k"), WithHTTPClient(http.DefaultClient))
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, "https://self-hosted:3000", c.baseURL)
	assert.Equal(t, "k", c.apiKey)
	assert.Equal(t, http.DefaultClient, c.client)
}

func TestProvider_Status_NotSupported(t *testing.T) {
	_, err := NewProvider().Status(context.Background(), "0xTx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}

func TestProvider_Quote_Success_EVM(t *testing.T) {
	mock := &mockClient{
		views: []QuoteView{
			{Type: "SWIFT", ExpectedAmountOutBaseUnits: "9919289", MinAmountOutBaseUnits: "9914329", Deadline64: "1781175819", Raw: json.RawMessage(`{"type":"SWIFT"}`)},
			{Type: "MCTP", ExpectedAmountOutBaseUnits: "9995444", MinAmountOutBaseUnits: "9990000", Deadline64: "1781178383", Raw: json.RawMessage(`{"type":"MCTP"}`)},
		},
		build: evmBuild("0xForwarder", "0xa11b1198deadbeef", "0", 1),
	}
	p := &Provider{client: mock}
	quote, err := p.Quote(context.Background(), evmReq())
	require.NoError(t, err)
	assert.Equal(t, "mayan", quote.Provider)
	// MCTP (9995444) beats SWIFT (9919289): selectBestQuote picks the higher output.
	assert.Equal(t, int64(9_995_444), quote.ToAmount.IntPart())
	assert.Equal(t, int64(9_990_000), quote.MinAmount.IntPart())
	assert.Equal(t, "0xForwarder", quote.To)
	expected, _ := hexutil.Decode("a11b1198deadbeef")
	assert.Equal(t, expected, quote.TxData)
	assert.True(t, quote.TxValue.IsZero())
	assert.Equal(t, "0xForwarder", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
	assert.Equal(t, "bridge", quote.Route[0].Action)
	// The chosen (MCTP) raw quote must be the one echoed to /build, with signerChainId=1 (Ethereum).
	assert.JSONEq(t, `{"type":"MCTP"}`, string(mock.lastBuildQuote))
	assert.Equal(t, 1, mock.lastBuildParams.SignerChainID)
	assert.Equal(t, "0xSwapper", mock.lastBuildParams.SwapperAddress)
}

func TestProvider_Quote_NativeInput_NoApproval(t *testing.T) {
	mock := &mockClient{
		views: []QuoteView{{Type: "MCTP", ExpectedAmountOutBaseUnits: "9995444", MinAmountOutBaseUnits: "9990000", Raw: json.RawMessage(`{}`)}},
		build: evmBuild("0xForwarder", "0xdeadbeef", "1000000000000000000", 1),
	}
	p := &Provider{client: mock}
	quote, err := p.Quote(context.Background(), evmReq())
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(1_000_000_000_000_000_000).Equal(quote.TxValue))
	assert.Empty(t, quote.ApprovalAddress, "native input (non-zero value) needs no ERC-20 approval")
	assert.Nil(t, quote.AllowanceNeeded)
}

func TestProvider_Quote_Success_Solana(t *testing.T) {
	mock := &mockClient{
		views: []QuoteView{{Type: "SWIFT", ExpectedAmountOutBaseUnits: "995000", MinAmountOutBaseUnits: "990025", Raw: json.RawMessage(`{}`)}},
		build: svmBuild("AQIDBA=="),
	}
	p := &Provider{client: mock}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "EPjF", Decimals: 6, ChainID: domain.ChainSolana},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000), Slippage: 0.005,
		FromAddr: "SolanaPubkey", ToAddr: "0xDest",
	}
	quote, err := p.Quote(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, quote.TxData)
	assert.True(t, quote.TxValue.IsZero())
	assert.Empty(t, quote.ApprovalAddress, "Solana has no ERC-20 approval")
}

func TestProvider_Quote_SelectsHighestOutput(t *testing.T) {
	mock := &mockClient{
		views: []QuoteView{
			{Type: "WH", ExpectedAmountOutBaseUnits: "980000", MinAmountOutBaseUnits: "975000", Raw: json.RawMessage(`{"type":"WH"}`)},
			{Type: "MCTP", ExpectedAmountOutBaseUnits: "999000", MinAmountOutBaseUnits: "990000", Raw: json.RawMessage(`{"type":"MCTP"}`)},
			{Type: "SWIFT", ExpectedAmountOutBaseUnits: "995000", MinAmountOutBaseUnits: "990025", Raw: json.RawMessage(`{"type":"SWIFT"}`)},
		},
		build: evmBuild("0xF", "0xab", "0", 8453),
	}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), evmReq())
	require.NoError(t, err)
	// MCTP has the highest output (999000); its raw quote must be sent to /build.
	assert.JSONEq(t, `{"type":"MCTP"}`, string(mock.lastBuildQuote))
}

func TestProvider_Quote_UnsupportedChain(t *testing.T) {
	p := &Provider{client: &mockClient{}}
	req := evmReq()
	req.FromToken.ChainID = domain.ChainBitcoin
	_, err := p.Quote(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported from chain")
}

func TestProvider_Quote_QuoteError(t *testing.T) {
	p := &Provider{client: &mockClient{quoteErr: assert.AnError}}
	_, err := p.Quote(context.Background(), evmReq())
	require.Error(t, err)
}

func TestProvider_Quote_BuildError(t *testing.T) {
	mock := &mockClient{
		views:    []QuoteView{{Type: "MCTP", ExpectedAmountOutBaseUnits: "999000", MinAmountOutBaseUnits: "990000", Raw: json.RawMessage(`{}`)}},
		buildErr: assert.AnError,
	}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), evmReq())
	require.Error(t, err)
}

func TestProvider_Quote_NoParseableRoute(t *testing.T) {
	mock := &mockClient{views: []QuoteView{{Type: "MCTP", ExpectedAmountOutBaseUnits: "notnum", Raw: json.RawMessage(`{}`)}}}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), evmReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no quote route")
}

func TestProvider_Quote_InvalidRequest(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	req := evmReq()
	req.Amount = decimal.Zero
	_, err := p.Quote(context.Background(), req)
	require.Error(t, err)
	assert.Nil(t, mock.views, "mock must not be called on invalid request")
}

func TestProvider_Quote_EmptyEVMTo(t *testing.T) {
	mock := &mockClient{
		views: []QuoteView{{Type: "MCTP", ExpectedAmountOutBaseUnits: "999000", MinAmountOutBaseUnits: "990000", Raw: json.RawMessage(`{}`)}},
		build: evmBuild("", "0xab", "0", 1),
	}
	p := &Provider{client: mock}
	_, err := p.Quote(context.Background(), evmReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty tx.to")
}
