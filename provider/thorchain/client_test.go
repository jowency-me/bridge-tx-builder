package thorchain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Quote(t *testing.T) {
	c := NewClient()
	params := QuoteParams{
		FromAsset:   "ETH.ETH",
		ToAsset:     "BTC.BTC",
		Amount:      "100000000",
		Destination: "bc1qyl7wjm2ldfezgnjk2c78adqlk7dvtm8sd7gn0q",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skip("real API unavailable:", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.ExpectedAmountOut)
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

func TestClient_Quote_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "thorchain quote failed: status 500")
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
	require.Contains(t, err.Error(), "thorchain quote decode")
}

func TestClient_Quote_RequestParams(t *testing.T) {
	var fromAsset, toAsset, amount, destination string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		fromAsset = q.Get("from_asset")
		toAsset = q.Get("to_asset")
		amount = q.Get("amount")
		destination = q.Get("destination")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"expected_amount_out":"1000000","inbound_address":"bc1qtest","memo":"=:ETH.ETH:0xTo","expiry":1234567890,"slippage_bps":50}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	resp, err := c.Quote(context.Background(), QuoteParams{
		FromAsset:   "ETH.ETH",
		ToAsset:     "BTC.BTC",
		Amount:      "100000000",
		Destination: "bc1qdest",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "ETH.ETH", fromAsset)
	require.Equal(t, "BTC.BTC", toAsset)
	require.Equal(t, "100000000", amount)
	require.Equal(t, "bc1qdest", destination)
}

func TestClient_Quote_WithAPIKey(t *testing.T) {
	var apiKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey = r.Header.Get("x-client-id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"expected_amount_out":"1000000","inbound_address":"bc1qtest","memo":"=:ETH","expiry":1234567890}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "thor-key"
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.NoError(t, err)
	require.Equal(t, "thor-key", apiKey)
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
	require.Contains(t, err.Error(), "thorchain status failed: status 404")
}

func TestClient_Status_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtx")
	require.Error(t, err)
	require.Contains(t, err.Error(), "thorchain status decode")
}

func TestClient_Status_RequestParams(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tx":{"id":"tx-1","chain":"ETH","status":"done"},"stages":{"inbound_observed":{"completed":true}}}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "tx-123")
	require.NoError(t, err)
	require.Equal(t, "/thorchain/tx/tx-123", path)
}

func TestClient_Status_WithAPIKey(t *testing.T) {
	var apiKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey = r.Header.Get("x-client-id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tx":{"id":"tx-1","chain":"ETH","status":"done"}}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "thor-status-key"
	_, err := c.Status(context.Background(), "tx-123")
	require.NoError(t, err)
	require.Equal(t, "thor-status-key", apiKey)
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
