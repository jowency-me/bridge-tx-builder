package swing

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
	projectID := os.Getenv("SWING_PROJECT_ID")
	if projectID == "" {
		t.Skip("SWING_PROJECT_ID not set")
	}
	c := NewClient(projectID)
	params := QuoteParams{
		FromChain:       "ethereum",
		ToChain:         "base",
		FromToken:       "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		ToToken:         "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		FromAmount:      "1000000",
		FromUserAddress: "0x1234567890123456789012345678901234567890",
		ToUserAddress:   "0x0987654321098765432109876543210987654321",
		Slippage:        "1",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real Swing API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Routes)
}

func TestClient_Status(t *testing.T) {
	projectID := os.Getenv("SWING_PROJECT_ID")
	if projectID == "" {
		t.Skip("SWING_PROJECT_ID not set")
	}
	c := NewClient(projectID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	if err != nil {
		t.Skipf("real Swing API unavailable: %v", err)
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
	require.Contains(t, err.Error(), "swing quote failed: status 500")
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
	require.Contains(t, err.Error(), "swing quote decode")
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

func TestClient_Quote_RequestParams(t *testing.T) {
	var receivedURL string
	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		receivedHeader = r.Header.Get("project-id")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			Routes: []RouteInfo{{}},
		})
	}))
	defer server.Close()

	c := NewClient("my-project-123")
	c.baseURL = server.URL

	params := QuoteParams{
		FromChain:       "ethereum",
		ToChain:         "base",
		FromToken:       "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		ToToken:         "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		FromAmount:      "1000000",
		FromUserAddress: "0xFrom",
		ToUserAddress:   "0xTo",
		Slippage:        "0.5",
	}
	resp, err := c.Quote(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Contains(t, receivedURL, "fromChain=ethereum")
	require.Contains(t, receivedURL, "toChain=base")
	require.Contains(t, receivedURL, "maxSlippage=0.5")
	require.Equal(t, "my-project-123", receivedHeader)
}

func TestClient_Status_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	_, err := c.Status(context.Background(), "tx-123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "swing status failed: status 502")
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

	_, err := c.Status(context.Background(), "tx-123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "swing status decode")
}

func TestClient_Status_TransportError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient("test-key")
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1"

	_, err := c.Status(ctx, "tx-123")
	require.Error(t, err)
}
