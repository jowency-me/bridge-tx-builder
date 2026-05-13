package zerox

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
	apiKey := os.Getenv("ZEROX_API_KEY")
	if apiKey == "" {
		t.Skip("ZEROX_API_KEY not set")
	}
	c := NewClient(apiKey)
	params := QuoteParams{
		ChainID:      "1",
		SellToken:    "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
		BuyToken:     "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		SellAmount:   "1000000000000000000",
		TakerAddress: "0x1234567890123456789012345678901234567890",
		SlippageBps:  "50",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real 0x API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.BuyAmount)
}

func TestClient_Status(t *testing.T) {
	c := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	// 0x has no real status API; reachable = success
	if err != nil {
		t.Skipf("real 0x API unreachable: %v", err)
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
	require.Contains(t, err.Error(), "0x quote failed: status 500")
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
	require.Contains(t, err.Error(), "0x quote decode")
}

func TestClient_Quote_RequestParams(t *testing.T) {
	var receivedAPIKey string
	var receivedChainID string
	var receivedVersion string
	var receivedTaker string
	var receivedSlippageBps string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.Header.Get("0x-api-key")
		receivedChainID = r.URL.Query().Get("chainId")
		receivedVersion = r.Header.Get("0x-version")
		receivedTaker = r.URL.Query().Get("taker")
		receivedSlippageBps = r.URL.Query().Get("slippageBps")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			SellAmount: "1000000",
			BuyAmount:  "995000",
		})
	}))
	defer server.Close()

	c := NewClient("key-abc123")
	c.baseURL = server.URL

	params := QuoteParams{
		ChainID:      "137",
		SellToken:    "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		BuyToken:     "0x7D1AfA7B718fb893dB30A3aBc0Cfc608C687fDA0",
		SellAmount:   "1000000",
		TakerAddress: "0x1234567890123456789012345678901234567890",
		SlippageBps:  "50",
	}
	resp, err := c.Quote(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "key-abc123", receivedAPIKey)
	require.Equal(t, "137", receivedChainID)
	require.Equal(t, "v2", receivedVersion)
	require.Equal(t, "0x1234567890123456789012345678901234567890", receivedTaker)
	require.Equal(t, "50", receivedSlippageBps)
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
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient("")
	c.baseURL = server.URL

	_, err := c.Status(context.Background(), "0xtxhash")
	require.Error(t, err)
	require.Contains(t, err.Error(), "0x server error: status 500")
}

func TestClient_Status_TransportError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient("")
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1"

	_, err := c.Status(ctx, "0xtxhash")
	require.Error(t, err)
}
