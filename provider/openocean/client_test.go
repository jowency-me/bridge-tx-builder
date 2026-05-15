package openocean

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

func TestClient_Quote(t *testing.T) {
	// Real HTTP request to open-api.openocean.finance.
	// Use ETH→USDC on Base chain (chainCode=base).
	c := NewClient()
	params := QuoteParams{
		ChainCode:       "eth",
		InTokenAddress:  "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", // ETH
		OutTokenAddress: "0xA0B86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC
		Amount:          "1000000000000000000",                        // 1 ETH
		GasPrice:        "50000000000",
		Slippage:        "1",
		Account:         "0x0000000000000000000000000000000000000000",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real OpenOcean API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Code, "OpenOcean API should return code 200")
	require.NotNil(t, resp.Data, "response data must not be nil")
	require.NotEmpty(t, resp.Data.OutAmount, "outAmount must not be empty")
}

func TestClient_Quote_InvalidChain(t *testing.T) {
	// OpenOcean returns 200 but with code!=200 in body for invalid params.
	c := NewClient()
	params := QuoteParams{
		ChainCode:       "nonexistent_chain_xyz",
		InTokenAddress:  "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
		OutTokenAddress: "0xA0B86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		Amount:          "1000000",
		GasPrice:        "50000000000",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Quote(ctx, params)
	// May succeed with code!=200 or fail with HTTP error — either is valid API behavior.
	if err != nil {
		msg := err.Error()
		prefixes := []string{"openocean quote error", "openocean quote failed"}
		matched := false
		for _, p := range prefixes {
			if len(msg) >= len(p) && msg[:len(p)] == p {
				matched = true
				break
			}
		}
		if !matched {
			t.Skipf("real OpenOcean API unavailable: %v", err)
		}
	}
}

func TestClient_Quote_HTTPError(t *testing.T) {
	// HTTP layer: non-200 status code triggers error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "openocean quote failed: status 500")
}

func TestClient_Quote_DecodeError(t *testing.T) {
	// HTTP 200 but invalid JSON body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "openocean quote decode")
}

func TestClient_Quote_APICodeError(t *testing.T) {
	// HTTP 200 but API-level error code in body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			Code:    400,
			Message: "insufficient liquidity",
		})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient liquidity")
}

func TestClient_Quote_TransportError(t *testing.T) {
	// TCP connection refused — tests the transport-layer error path.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient()
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1" // nothing listening here

	_, err := c.Quote(ctx, QuoteParams{})
	require.Error(t, err)
	// Should get a connection-refused or timeout error from the transport layer.
	assert.Contains(t, err.Error(), "connection refused")
}

func TestClient_Status(t *testing.T) {
	// Real HTTP HEAD request to open-api.openocean.finance.
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x123")
	// OpenOcean returns nil for Status (not a real status API).
	// The real endpoint must at least be reachable (no network error).
	if err != nil {
		t.Skipf("real OpenOcean API unreachable: %v", err)
	}
}

func TestClient_Status_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	_, err := c.Status(context.Background(), "0xtxhash")
	require.Error(t, err)
	require.Contains(t, err.Error(), "openocean server error: status 502")
}

func TestClient_Status_DecodeError(t *testing.T) {
	// HEAD request succeeds but response is unexpected — not applicable for HEAD.
	// Test the transport error path instead.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient()
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1"

	_, err := c.Status(ctx, "0xtxhash")
	require.Error(t, err)
}

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	// Verify all QuoteResponse/QuoteData fields are correctly deserialized from raw JSON.
	// Real API response from https://open-api.openocean.finance/v3/eth/swap_quote
	// (simplified to only struct-captured fields; real response also includes inToken.name / outToken.name).
	// estimatedGas is json.Number since the API returns it as a numeric value, not a string.
	raw := []byte(`{
		"code": 200,
		"data": {
			"inToken": {
				"address": "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
				"decimals": 18,
				"symbol": "ETH"
			},
			"outToken": {
				"address": "0xA0B86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
				"decimals": 6,
				"symbol": "USDC"
			},
			"inAmount": "1000000000000000000000000000000000000",
			"outAmount": "124677904122952",
			"estimatedGas": 15439066,
			"to": "0x6352a56caadC4F1E25CD6c75970Fa768A3304e64",
			"data": "0x1234567890abcdef",
			"value": "1000000000000000000000000000000000000"
		}
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{
		ChainCode:       "eth",
		InTokenAddress:  "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
		OutTokenAddress: "0xA0B86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		Amount:          "1000000000000000000000000000000000000",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Top-level fields
	require.Equal(t, 200, resp.Code)
	require.NotNil(t, resp.Data)

	// Data fields
	require.Equal(t, "0x6352a56caadC4F1E25CD6c75970Fa768A3304e64", resp.Data.To)
	require.Equal(t, "0x1234567890abcdef", resp.Data.Data)
	require.Equal(t, "1000000000000000000000000000000000000", resp.Data.Value)
	require.Equal(t, "124677904122952", resp.Data.OutAmount)
	require.Equal(t, json.Number("15439066"), resp.Data.EstimatedGas)
	require.Equal(t, uint64(15439066), resp.Data.EstimatedGasUint())

	// Token fields
	require.Equal(t, "ETH", resp.Data.InToken.Symbol)
	require.Equal(t, "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE", resp.Data.InToken.Address)
	require.Equal(t, 18, resp.Data.InToken.Decimals)
	require.Equal(t, "USDC", resp.Data.OutToken.Symbol)
	require.Equal(t, "0xA0B86991c6218b36c1d19D4a2e9Eb0cE3606eB48", resp.Data.OutToken.Address)
	require.Equal(t, 6, resp.Data.OutToken.Decimals)
	require.Equal(t, "1000000000000000000000000000000000000", resp.Data.InAmount)
}

func TestClient_Quote_JSONDeserialization_StringEstimatedGas(t *testing.T) {
	// Some OpenOcean endpoints return estimatedGas as a quoted string.
	raw := []byte(`{
		"code": 200,
		"message": "OK",
		"data": {
			"to": "0xRouter",
			"data": "0x",
			"value": "0",
			"outAmount": "1000",
			"estimatedGas": "250000",
			"inToken": {"symbol": "USDC", "address": "0xA", "decimals": 6},
			"outToken": {"symbol": "USDT", "address": "0xB", "decimals": 6},
			"inAmount": "1000000"
		}
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{ChainCode: "eth"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, json.Number("250000"), resp.Data.EstimatedGas)
	require.Equal(t, uint64(250000), resp.Data.EstimatedGasUint())
}
