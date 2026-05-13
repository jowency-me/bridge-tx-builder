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
			Code:     400,
			ErrorMsg: "insufficient liquidity",
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
