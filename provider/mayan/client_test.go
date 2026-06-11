package mayan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveMayanQuoteResponse is the verbatim shape of a real GET /quote response
// captured from https://tx-builder.mayan.finance 2026-06-11 (USDC ETH->Base),
// trimmed to the fields the adapter reads. Amounts are floats with separate
// *BaseUnits strings; the envelope is {success, quotes:[...]}.
const liveMayanQuoteResponse = `{"success":true,"quotes":[{"type":"SWIFT","expectedAmountOut":9.919289,"expectedAmountOutBaseUnits":"9919289","minAmountOut":9.914329,"minAmountOutBaseUnits":"9914329","deadline64":"1781175819","gasless":false,"referrerBps":0,"fromChain":"ethereum","toChain":"base","fromToken":{"contract":"0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48","decimals":6},"toToken":{"contract":"0x833589fcd6edb6e08f4c7c32d4f71b54bda02913","decimals":6}},{"type":"MCTP","expectedAmountOut":9.995444,"expectedAmountOutBaseUnits":"9995444","minAmountOut":9.995444,"minAmountOutBaseUnits":"9995444","deadline64":"1781178383","gasless":false,"fromChain":"ethereum","toChain":"base","fromToken":{"contract":"0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48","decimals":6},"toToken":{"contract":"0x833589fcd6edb6e08f4c7c32d4f71b54bda02913","decimals":6}}]}`

// liveMayanBuildEVMResponse is the documented POST /build EVM response shape
// (github.com/mayan-finance/tx-builder): the inner transaction is an object.
const liveMayanBuildEVMResponse = `{"success":true,"transaction":{"chainCategory":"evm","quoteType":"MCTP","gasless":false,"transaction":{"to":"0x337685fdaB40D39bd02028545a4FfA7D287cC3E2","data":"0xa11b1198deadbeef","value":"0","chainId":1}}}`

func TestClient_Quote_RequestParams(t *testing.T) {
	var gotPath, gotAmount, gotFromChain, gotToChain, gotSwift, gotSlip, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAmount = r.URL.Query().Get("amountIn64")
		gotFromChain = r.URL.Query().Get("fromChain")
		gotToChain = r.URL.Query().Get("toChain")
		gotSwift = r.URL.Query().Get("swift")
		gotSlip = r.URL.Query().Get("slippageBps")
		gotKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(liveMayanQuoteResponse))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "test-key"
	views, err := c.Quote(context.Background(), QuoteParams{
		AmountIn64: "10000000", FromToken: "0xA", ToToken: "0xB",
		FromChain: "ethereum", ToChain: "base", SlippageBps: "50", Swift: true, MCTP: true,
	})
	require.NoError(t, err)
	require.Len(t, views, 2)
	assert.Equal(t, "/quote", gotPath)
	assert.Equal(t, "10000000", gotAmount)
	assert.Equal(t, "ethereum", gotFromChain)
	assert.Equal(t, "base", gotToChain)
	assert.Equal(t, "true", gotSwift)
	assert.Equal(t, "50", gotSlip)
	assert.Equal(t, "test-key", gotKey)
	assert.Equal(t, "SWIFT", views[0].Type)
	assert.Equal(t, "9919289", views[0].ExpectedAmountOutBaseUnits)
	assert.NotEmpty(t, views[0].Raw, "raw quote must be preserved for /build")
}

func TestClient_Quote_SlippageAutoDefault(t *testing.T) {
	var gotSlip string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSlip = r.URL.Query().Get("slippageBps")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"quotes":[]}`))
	}))
	defer server.Close()
	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{AmountIn64: "1"})
	require.NoError(t, err)
	assert.Equal(t, "auto", gotSlip, "empty SlippageBps must default to auto")
}

func TestClient_Quote_Unsuccessful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"error":"ROUTE_NOT_FOUND"}`))
	}))
	defer server.Close()
	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ROUTE_NOT_FOUND")
}

func TestClient_Build_EVM(t *testing.T) {
	var gotPath, gotKey string
	var gotBody buildRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-API-Key")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(liveMayanBuildEVMResponse))
	}))
	defer server.Close()
	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "k"
	resp, err := c.Build(context.Background(), json.RawMessage(`{"type":"MCTP"}`), BuildParams{
		SwapperAddress: "0xSwapper", DestinationAddress: "0xDest", SignerChainID: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "/build", gotPath)
	assert.Equal(t, "k", gotKey)
	assert.Equal(t, "0xSwapper", gotBody.Params.SwapperAddress)
	assert.Equal(t, "evm", resp.Transaction.ChainCategory)
	var tx EVMTx
	require.NoError(t, json.Unmarshal(resp.Transaction.Transaction, &tx))
	assert.Equal(t, "0x337685fdaB40D39bd02028545a4FfA7D287cC3E2", tx.To)
}

func TestClient_Build_Unsuccessful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"error":"API key required"}`))
	}))
	defer server.Close()
	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Build(context.Background(), json.RawMessage(`{}`), BuildParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key required")
}

// TestProvider_EndToEnd_LiveGolden runs the full Provider.Quote flow through the
// real *Client against an httptest server that serves the captured real /quote
// shape and the documented /build shape — proving the adapter handles the
// production payloads end to end without network access.
func TestProvider_EndToEnd_LiveGolden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/quote":
			_, _ = w.Write([]byte(liveMayanQuoteResponse))
		case "/build":
			_, _ = w.Write([]byte(liveMayanBuildEVMResponse))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewProvider(WithBaseURL(server.URL), WithAPIKey("k"))
	quote, err := p.Quote(context.Background(), evmReq())
	require.NoError(t, err)
	// MCTP (9995444) is the higher-output option and must be selected.
	assert.Equal(t, int64(9_995_444), quote.ToAmount.IntPart())
	assert.Equal(t, "0x337685fdaB40D39bd02028545a4FfA7D287cC3E2", quote.To)
	assert.NotEmpty(t, quote.TxData)
	assert.Equal(t, "0x337685fdaB40D39bd02028545a4FfA7D287cC3E2", quote.ApprovalAddress)
}

func TestClient_Quote_RealAPI(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	views, err := c.Quote(ctx, QuoteParams{
		AmountIn64: "10000000",
		FromToken:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		ToToken:    "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		FromChain:  "ethereum", ToChain: "base", SlippageBps: "auto",
		Swift: true, MCTP: true,
	})
	if err != nil {
		t.Skipf("real API unavailable: %v", err)
	}
	require.NotEmpty(t, views)
	assert.NotEmpty(t, views[0].ExpectedAmountOutBaseUnits)
}
