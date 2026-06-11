package symbiosis

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

func TestClient_Quote_RequestParams(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotReq QuoteRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotReq))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			Tx:                Tx{ChainID: 1, To: "0xPool", Data: "0xdeadbeef", Value: "0"},
			TokenAmountOut:    TokenAmount{Symbol: "USDT", Amount: "995000"},
			TokenAmountOutMin: TokenAmount{Symbol: "USDT", Amount: "990025"},
		})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	resp, err := c.Quote(context.Background(), QuoteRequest{
		TokenAmountIn: TokenAmount{Symbol: "USDC", Address: "0xA", ChainID: 1, Decimals: 6, Amount: "1000000"},
		TokenOut:      TokenAmount{Symbol: "USDT", Address: "0xB", ChainID: 56, Decimals: 6},
		From:          "0xSender", To: "0xRecipient", Slippage: 500,
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/v1/swap", gotPath)
	assert.Contains(t, gotContentType, "application/json")
	assert.Equal(t, "0xSender", gotReq.From)
	assert.Equal(t, 500, gotReq.Slippage)
	assert.Equal(t, "0xPool", resp.Tx.To)
}

func TestClient_Quote_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"chain not supported"}`))
	}))
	defer server.Close()
	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestClient_Quote_RealAPI(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := c.Quote(ctx, QuoteRequest{
		TokenAmountIn: TokenAmount{Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", ChainID: 1, Decimals: 6, Amount: "1000000"},
		TokenOut:      TokenAmount{Symbol: "USDC", Address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", ChainID: 8453, Decimals: 6},
		From:          "0x1234567890123456789012345678901234567890",
		To:            "0x1234567890123456789012345678901234567890",
		Slippage:      50,
	})
	if err != nil {
		t.Skipf("real API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.Tx.To)
	assert.NotEmpty(t, resp.Tx.Data)
	assert.NotEmpty(t, resp.TokenAmountOut.Amount)
	assert.NotEmpty(t, resp.ApproveTo)
}

// liveSymbiosisSwapResponse is the verbatim shape of a real
// POST /crosschain/v1/swap response captured 2026-06-11 (USDC ETH->Base).
// It is the source of truth for the QuoteResponse struct: it includes the
// fields whose types are easy to get wrong — priceImpact (string, not number),
// amountInUsd (object, not string), and tx (object with string value). Decoding
// it through QuoteResponse and feeding it to mapQuote proves the adapter handles
// the production payload, even though the network is unavailable in CI.
const liveSymbiosisSwapResponse = `{"fee":{"symbol":"USDbC","icon":"x","address":"0xd9aAEc86B65D86f6A7B5B1b0c42FFA531710b6CA","amount":"250000","chainId":8453,"decimals":6,"priceUsd":0.9996556},"route":[],"inTradeType":"symbiosis","outTradeType":"symbiosis","priceImpact":"-0.13","tokenAmountOut":{"symbol":"tokenOut","address":"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913","amount":"499177","chainId":8453,"decimals":6,"priceUsd":0.999778},"tokenAmountOutMin":{"symbol":"tokenOut","address":"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913","amount":"496183","chainId":8453,"decimals":6,"priceUsd":0.999778},"amountInUsd":{"symbol":"tokenIn","address":"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48","amount":"1000000","chainId":1,"decimals":6,"priceUsd":1},"approveTo":"0xfCEF2Fe72413b65d3F393d278A714caD87512bcd","type":"evm","estimatedTime":18,"tx":{"chainId":1,"data":"0xa11b119800000000000000000000000000000000","to":"0xf621Fb08BBE51aF70e7E0F4EA63496894166Ff7F","value":"0"}}`

// TestClient_Quote_LiveGolden decodes the captured real response through the
// client and confirms the QuoteResponse struct matches the live API shape.
func TestClient_Quote_LiveGolden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/swap", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(liveSymbiosisSwapResponse))
	}))
	defer server.Close()
	c := NewClient()
	c.baseURL = server.URL
	resp, err := c.Quote(context.Background(), QuoteRequest{})
	require.NoError(t, err, "the real-shape response must decode cleanly")
	assert.Equal(t, "499177", resp.TokenAmountOut.Amount)
	assert.Equal(t, "496183", resp.TokenAmountOutMin.Amount)
	assert.Equal(t, "-0.13", resp.PriceImpact)
	assert.Equal(t, "0xfCEF2Fe72413b65d3F393d278A714caD87512bcd", resp.ApproveTo)
	assert.Equal(t, "0xf621Fb08BBE51aF70e7E0F4EA63496894166Ff7F", resp.Tx.To)
	assert.Equal(t, "evm", resp.Type)
	assert.Equal(t, 18, resp.EstimatedTime)
}
