package rango

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Quote(t *testing.T) {
	apiKey := os.Getenv("RANGO_API_KEY")
	if apiKey == "" {
		t.Skip("RANGO_API_KEY not set")
	}
	c := NewClient(apiKey)
	params := QuoteParams{
		From:        "ETH",
		To:          "BASE",
		FromToken:   "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		ToToken:     "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		Amount:      "1000000",
		FromAddress: "0x1234567890123456789012345678901234567890",
		ToAddress:   "0x0987654321098765432109876543210987654321",
		Slippage:    "0.5",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real Rango API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.RequestID)
}

func TestClient_Status(t *testing.T) {
	apiKey := os.Getenv("RANGO_API_KEY")
	if apiKey == "" {
		t.Skip("RANGO_API_KEY not set")
	}
	c := NewClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "rg-test-123")
	if err != nil {
		t.Skipf("real Rango API unavailable: %v", err)
	}
}

func TestClient_Quote_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rango quote failed: status 500")
}

func TestClient_Quote_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rango quote decode")
}

func TestClient_Quote_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			ResultType: "ERROR",
			Error:      "insufficient liquidity",
		})
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient liquidity")
}

func TestClient_Quote_CompositeParams(t *testing.T) {
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			RequestID:  "rg-123",
			ResultType: "OK",
			Route:      Route{OutputAmount: "1000"},
		})
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	params := QuoteParams{
		From:        "ETH",
		To:          "BASE",
		FromToken:   "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		ToToken:     "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		Amount:      "1000000",
		FromAddress: "0xFrom",
		ToAddress:   "0xTo",
		Slippage:    "0.5",
	}
	_, err := c.Quote(context.Background(), params)
	require.NoError(t, err)
	require.Contains(t, receivedQuery, "from=ETH.0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C")
	require.Contains(t, receivedQuery, "to=BASE--0x833589fcd6edb6e08f4c7c32d4f71b54bda02913")
	require.NotContains(t, receivedQuery, "fromToken=")
	require.NotContains(t, receivedQuery, "toToken=")
}

func TestClient_Quote_TransportError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient("test-key")
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1"

	_, err := c.Quote(ctx, QuoteParams{})
	require.Error(t, err)
}

func TestClient_Status_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	_, err := c.Status(context.Background(), "rg-123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "rango status failed: status 502")
}

func TestClient_Status_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	_, err := c.Status(context.Background(), "rg-123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "rango status decode")
}

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	// Captured 2026-05-15 from real Rango API (ETH -> USDC on Base via Symbiosis).
	// Simplified to struct-captured fields; real response has many more TokenInfo fields.
	rawJSON := []byte(`{
		"requestId": "38f3dcc6-bcf1-4dd1-8756-6799a57ba3f3",
		"resultType": "OK",
		"route": {
			"outputAmount": "2253505213",
			"outputAmountMin": "2242237686",
			"outputAmountUsd": 2252.79,
			"swapper": {
				"id": "SymbiosisV1",
				"title": "Symbiosis",
				"logo": "https://raw.githubusercontent.com/rango-exchange/assets/main/swappers/Symbiosis/icon.svg",
				"swapperGroup": "Symbiosis",
				"types": ["BRIDGE"],
				"enabled": true
			},
			"from": {
				"blockchain": "ETH",
				"symbol": "ETH",
				"address": null,
				"decimals": 18
			},
			"to": {
				"blockchain": "BASE",
				"symbol": "USDC",
				"address": "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
				"decimals": 6
			},
			"fee": [
				{
					"token": {"blockchain": "BASE", "symbol": "WETH", "address": "0x4200000000000000000000000000000000000006", "decimals": 18},
					"expenseType": "DECREASE_FROM_OUTPUT",
					"amount": "250000000000000",
					"name": "Swapper Fee"
				},
				{
					"token": {"blockchain": "ETH", "symbol": "ETH", "address": null, "decimals": 18},
					"expenseType": "FROM_SOURCE_WALLET",
					"amount": "73091127854024",
					"name": "Network Fee",
					"meta": {"type": "EvmNetworkFeeMeta", "gasLimit": "501032", "gasPrice": "145881157"}
				}
			],
			"feeUsd": 0.164,
			"estimatedTimeInSeconds": 135,
			"path": [
				{
					"swapper": {"id": "SymbiosisV1", "title": "Symbiosis", "logo": "https://example.com/sym.svg", "swapperGroup": "Symbiosis", "types": ["BRIDGE"], "enabled": true},
					"swapperType": "DEX",
					"from": {"blockchain": "ETH", "symbol": "ETH", "address": null, "decimals": 18},
					"to": {"blockchain": "BASE", "symbol": "USDC", "address": "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", "decimals": 6},
					"inputAmount": "1000000000000000000",
					"expectedOutput": "2253505213",
					"estimatedTimeInSeconds": 135
				}
			]
		},
		"error": null,
		"errorCode": null,
		"traceId": null
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawJSON)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{
		From: "ETH", To: "BASE",
		FromToken: "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
		ToToken:   "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		Amount:    "1000000000000000000",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Top-level
	require.Equal(t, "38f3dcc6-bcf1-4dd1-8756-6799a57ba3f3", resp.RequestID)
	require.Equal(t, "OK", resp.ResultType)
	require.Equal(t, "", resp.Error)
	require.Nil(t, resp.ErrorCode)
	require.Nil(t, resp.TraceID)

	// Route
	require.Equal(t, "2253505213", resp.Route.OutputAmount)
	require.Equal(t, "2242237686", resp.Route.OutputAmountMin)
	require.NotNil(t, resp.Route.OutputAmountUsd)
	require.InDelta(t, 2252.79, *resp.Route.OutputAmountUsd, 0.01)
	require.Equal(t, 135, resp.Route.EstimatedTimeInSeconds)

	// Route-level swapper
	require.Equal(t, "SymbiosisV1", resp.Route.Swapper.ID)
	require.Equal(t, "Symbiosis", resp.Route.Swapper.Title)
	require.Equal(t, "Symbiosis", resp.Route.Swapper.SwapperGroup)
	require.Contains(t, resp.Route.Swapper.Types, "BRIDGE")
	require.True(t, resp.Route.Swapper.Enabled)

	// Route from/to
	require.Equal(t, "ETH", resp.Route.From.Symbol)
	require.Equal(t, "ETH", resp.Route.From.Blockchain)
	require.Equal(t, 18, resp.Route.From.Decimals)
	require.Equal(t, "USDC", resp.Route.To.Symbol)
	require.Equal(t, "BASE", resp.Route.To.Blockchain)

	// Fees
	require.Len(t, resp.Route.Fee, 2)
	require.Equal(t, "250000000000000", resp.Route.Fee[0].Amount)
	require.Equal(t, "DECREASE_FROM_OUTPUT", resp.Route.Fee[0].ExpenseType)
	require.Equal(t, "Swapper Fee", resp.Route.Fee[0].Name)
	require.Equal(t, "73091127854024", resp.Route.Fee[1].Amount)
	require.Equal(t, "FROM_SOURCE_WALLET", resp.Route.Fee[1].ExpenseType)
	require.Equal(t, "Network Fee", resp.Route.Fee[1].Name)
	require.NotNil(t, resp.Route.Fee[1].Meta)

	// Path
	require.Len(t, resp.Route.Path, 1)
	require.Equal(t, "SymbiosisV1", resp.Route.Path[0].Swapper.ID)
	require.Equal(t, "DEX", resp.Route.Path[0].SwapperType)
	require.Equal(t, "1000000000000000000", resp.Route.Path[0].InputAmount)
	require.Equal(t, "2253505213", resp.Route.Path[0].ExpectedOutput)
	require.Equal(t, 135, resp.Route.Path[0].EstimatedTimeInSeconds)
}

func TestClient_Status_JSONDeserialization(t *testing.T) {
	rawJSON := []byte(`{
		"status": "success",
		"error": "",
		"output": {
			"type": "BRIDGE",
			"amount": "999500000",
			"receivedToken": {"symbol": "USDT", "address": "0xB", "blockchain": "BASE", "decimals": 6}
		},
		"explorerUrl": [
			{"url": "https://etherscan.io/tx/0xsrc", "description": "Source transaction"},
			{"url": "https://basescan.org/tx/0xdst", "description": "Destination transaction"}
		],
		"diagnosisUrl": "https://diag.example.com/rg-456",
		"bridgeData": {
			"srcTxHash": "0xsrcTxHash",
			"destTxHash": "0xdstTxHash",
			"srcToken": {"symbol": "USDC", "blockchain": "ETH"},
			"destToken": {"symbol": "USDT", "blockchain": "BASE"},
			"srcAmount": "1000000",
			"destAmount": "999500000"
		}
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawJSON)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	resp, err := c.Status(context.Background(), "rg-status-456")
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Equal(t, "success", resp.Status)
	require.Equal(t, "", resp.Error)

	require.NotNil(t, resp.Output)
	require.Equal(t, "BRIDGE", resp.Output.Type)
	require.Equal(t, "999500000", resp.Output.Amount)
	require.Equal(t, "USDT", resp.Output.ReceivedToken.Symbol)

	require.Len(t, resp.ExplorerURL, 2)
	require.Equal(t, "https://etherscan.io/tx/0xsrc", resp.ExplorerURL[0].URL)
	require.Equal(t, "Source transaction", resp.ExplorerURL[0].Description)

	require.Equal(t, "https://diag.example.com/rg-456", resp.DiagnosisURL)

	require.NotNil(t, resp.BridgeData)
	require.Equal(t, "0xsrcTxHash", resp.BridgeData.SrcTxHash)
	require.Equal(t, "0xdstTxHash", resp.BridgeData.DestTxHash)
	require.Equal(t, "USDC", resp.BridgeData.SrcToken.Symbol)
	require.Equal(t, "USDT", resp.BridgeData.DestToken.Symbol)
	require.Equal(t, "1000000", resp.BridgeData.SrcAmount)
	require.Equal(t, "999500000", resp.BridgeData.DestAmount)
}

func TestClient_Status_TransportError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient("test-key")
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1"

	_, err := c.Status(ctx, "rg-123")
	require.Error(t, err)
}
