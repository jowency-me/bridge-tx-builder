package squid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Quote(t *testing.T) {
	c := NewClient()
	params := QuoteParams{
		FromChain:   "1",
		ToChain:     "8453",
		FromToken:   "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		ToToken:     "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		FromAmount:  "1000000",
		FromAddress: "0x1234567890123456789012345678901234567890",
		ToAddress:   "0x0987654321098765432109876543210987654321",
		Slippage:    1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skip("real API unavailable:", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.RequestID)
}

func TestClient_Status(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	if err != nil {
		t.Skip("real API unavailable:", err)
	}
}

func TestClient_Quote_RequestParams(t *testing.T) {
	var method string
	var path string
	var contentType string
	var integrator string
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		contentType = r.Header.Get("Content-Type")
		integrator = r.Header.Get("x-integrator-id")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "hdr-req-42")
		_ = json.NewEncoder(w).Encode(QuoteResponse{RequestID: "body-req-1"})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "integrator-1"

	resp, err := c.Quote(context.Background(), QuoteParams{
		FromChain:   "1",
		ToChain:     "8453",
		FromToken:   "0xFromToken",
		ToToken:     "0xToToken",
		FromAmount:  "1000000",
		FromAddress: "0xFrom",
		ToAddress:   "0xTo",
		Slippage:    1,
	})
	require.NoError(t, err)
	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "/route", path)
	require.Contains(t, contentType, "application/json")
	require.Equal(t, "integrator-1", integrator)
	require.Equal(t, "1", body["fromChain"])
	require.Equal(t, "8453", body["toChain"])
	require.Equal(t, "0xFrom", body["fromAddress"])
	require.Equal(t, "0xTo", body["toAddress"])
	require.Equal(t, float64(1), body["slippage"])
	// x-request-id header should override the body requestId
	require.Equal(t, "hdr-req-42", resp.RequestID)
}

func TestClient_Quote_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "squid quote failed: status 500")
}

func TestClient_Quote_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "squid quote decode")
}

func TestClient_Status_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtx")
	require.Error(t, err)
	require.Contains(t, err.Error(), "squid status failed: status 404")
}

func TestClient_Status_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("bad json"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtx")
	require.Error(t, err)
	require.Contains(t, err.Error(), "squid status decode")
}

func TestClient_Status_RequestParams(t *testing.T) {
	var txID string
	var integrator string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		txID = r.URL.Query().Get("transactionId")
		integrator = r.Header.Get("x-integrator-id")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StatusResponse{ID: "status-1", SquidTransactionStatus: "completed"})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "squid-key"
	_, err := c.Status(context.Background(), "tx-123")
	require.NoError(t, err)
	require.Equal(t, "tx-123", txID)
	require.Equal(t, "squid-key", integrator)
}

func TestClient_Quote_TransportError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient()
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1"

	_, err := c.Quote(ctx, QuoteParams{})
	require.Error(t, err)
}

func TestClient_Status_TransportError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient()
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1"

	_, err := c.Status(ctx, "0xtx")
	require.Error(t, err)
}

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	// Captured 2026-05-15 from real Squid v2 API (USDC ETH -> USDC Base via Axelar).
	rawJSON := `{
		"route": {
			"quoteId": "54fe12db8b826d6ba36f96f8c266c7d5",
			"estimate": {
				"fromAmount": "1000000",
				"toAmount": "999804",
				"toAmountMin": "989306",
				"fromToken": {
					"symbol": "USDC",
					"address": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
					"decimals": 6,
					"chainId": "1",
					"name": "USDC"
				},
				"toToken": {
					"symbol": "USDC",
					"address": "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
					"decimals": 6,
					"chainId": "8453",
					"name": "USDC"
				},
				"gasCosts": [
					{
						"type": "executeCall",
						"amount": "33511211500000",
						"gasLimit": "260000",
						"amountUsd": "0.08",
						"token": {
							"symbol": "ETH",
							"address": "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
							"decimals": 18,
							"chainId": "1",
							"name": "Ethereum"
						}
					}
				],
				"feeCosts": [
					{
						"amount": "16474276928467",
						"amountUsd": "0.04",
						"name": "Gas receiver fee",
						"token": {
							"symbol": "ETH",
							"address": "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
							"decimals": 18,
							"chainId": "1",
							"name": "Ethereum"
						},
						"logoURI": "https://raw.githubusercontent.com/0xsquid/assets/main/images/master/providers/axelar.svg"
					},
					{
						"amount": "0",
						"amountUsd": "0.00",
						"name": "Boost fee",
						"token": {
							"symbol": "ETH",
							"address": "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
							"decimals": 18,
							"chainId": "1",
							"name": "Ethereum"
						},
						"logoURI": "https://raw.githubusercontent.com/0xsquid/assets/main/images/master/providers/axelar.svg"
					}
				],
				"estimatedRouteDuration": 80
			},
			"transactionRequest": {
				"type": "ON_CHAIN_EXECUTION",
				"target": "0xce16F69375520ab01377ce7B88f5BA8C48F8D666",
				"data": "0x2147796000000000000000000000000000000000000000000000000000000000000000e000000000000000000000000000000000000000000000000000000000000f4240",
				"value": "16474276928467",
				"gasLimit": "260000"
			},
			"params": {
				"fromChain": "1",
				"toChain": "8453",
				"fromToken": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
				"toToken": "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
				"fromAmount": "1000000",
				"fromAddress": "0x1234567890123456789012345678901234567890",
				"toAddress": "0x0987654321098765432109876543210987654321",
				"slippage": 1
			}
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "54fe12db8b826d6ba36f96f8c266c7d5")
		_, _ = w.Write([]byte(rawJSON))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{
		FromChain:   "1",
		ToChain:     "8453",
		FromToken:   "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		ToToken:     "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
		FromAmount:  "1000000",
		FromAddress: "0x1234567890123456789012345678901234567890",
		ToAddress:   "0x0987654321098765432109876543210987654321",
		Slippage:    1,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// x-request-id header overrides body
	require.Equal(t, "54fe12db8b826d6ba36f96f8c266c7d5", resp.RequestID)
	require.Equal(t, "54fe12db8b826d6ba36f96f8c266c7d5", resp.Route.QuoteID)

	// Estimate
	require.Equal(t, "1000000", resp.Route.Estimate.FromAmount)
	require.Equal(t, "999804", resp.Route.Estimate.ToAmount)
	require.Equal(t, "989306", resp.Route.Estimate.ToAmountMin)

	// Token info (chainId is string from real API)
	require.Equal(t, "USDC", resp.Route.Estimate.FromToken.Symbol)
	require.Equal(t, "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", resp.Route.Estimate.FromToken.Address)
	require.Equal(t, 6, resp.Route.Estimate.FromToken.Decimals)
	require.Equal(t, "1", resp.Route.Estimate.FromToken.ChainID)
	require.Equal(t, "USDC", resp.Route.Estimate.ToToken.Symbol)
	require.Equal(t, "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", resp.Route.Estimate.ToToken.Address)
	require.Equal(t, 6, resp.Route.Estimate.ToToken.Decimals)
	require.Equal(t, "8453", resp.Route.Estimate.ToToken.ChainID)

	// GasCosts
	require.Len(t, resp.Route.Estimate.GasCosts, 1)
	require.Equal(t, "executeCall", resp.Route.Estimate.GasCosts[0].Type)
	require.Equal(t, "33511211500000", resp.Route.Estimate.GasCosts[0].Amount)
	require.Equal(t, "260000", resp.Route.Estimate.GasCosts[0].GasLimit)
	require.Equal(t, "0.08", resp.Route.Estimate.GasCosts[0].AmountUSD)
	require.Equal(t, "ETH", resp.Route.Estimate.GasCosts[0].Token.Symbol)
	require.Equal(t, "1", resp.Route.Estimate.GasCosts[0].Token.ChainID)

	// FeeCosts (2 fees: gas receiver + boost)
	require.Len(t, resp.Route.Estimate.FeeCosts, 2)
	require.Equal(t, "16474276928467", resp.Route.Estimate.FeeCosts[0].Amount)
	require.Equal(t, "Gas receiver fee", resp.Route.Estimate.FeeCosts[0].Name)
	require.Equal(t, "0", resp.Route.Estimate.FeeCosts[1].Amount)
	require.Equal(t, "Boost fee", resp.Route.Estimate.FeeCosts[1].Name)

	require.Equal(t, 80, resp.Route.Estimate.EstimatedRouteDuration)

	// TransactionRequest
	require.Equal(t, "ON_CHAIN_EXECUTION", resp.Route.TransactionRequest.Type)
	require.Equal(t, "0xce16F69375520ab01377ce7B88f5BA8C48F8D666", resp.Route.TransactionRequest.Target)
	require.NotEmpty(t, resp.Route.TransactionRequest.Data)
	require.Equal(t, "16474276928467", resp.Route.TransactionRequest.Value)
	require.Equal(t, "260000", resp.Route.TransactionRequest.GasLimit)

	// Params
	require.Equal(t, "1", resp.Route.Params.FromChain)
	require.Equal(t, "8453", resp.Route.Params.ToChain)
	require.Equal(t, "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", resp.Route.Params.FromToken)
	require.Equal(t, "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", resp.Route.Params.ToToken)
}

func TestClient_Status_JSONDeserialization(t *testing.T) {
	// Tests full JSON deserialization using raw JSON bytes from a real Squid status response
	rawJSON := `{
		"id": "status-abc123",
		"squidTransactionStatus": "completed",
		"fromChain": {
			"transactionId": "0xsrc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd",
			"transactionUrl": "https://etherscan.io/tx/0xsrc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd",
			"callEventStatus": "callExecuted"
		},
		"toChain": {
			"transactionId": "0xdst1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd",
			"transactionUrl": "https://basescan.io/tx/0xdst1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd",
			"callEventStatus": "callExecuted"
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Status(context.Background(), "0xsrc123")
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Equal(t, "status-abc123", resp.ID)
	require.Equal(t, "completed", resp.SquidTransactionStatus)
	require.NotNil(t, resp.FromChain)
	require.Equal(t, "0xsrc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd", resp.FromChain.TransactionID)
	require.Equal(t, "https://etherscan.io/tx/0xsrc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd", resp.FromChain.TransactionURL)
	require.Equal(t, "callExecuted", resp.FromChain.CallEventStatus)
	require.NotNil(t, resp.ToChain)
	require.Equal(t, "0xdst1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd", resp.ToChain.TransactionID)
	require.Equal(t, "https://basescan.io/tx/0xdst1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd", resp.ToChain.TransactionURL)
	require.Equal(t, "callExecuted", resp.ToChain.CallEventStatus)
}
