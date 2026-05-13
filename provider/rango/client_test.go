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

func TestClient_Status_TransportError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := NewClient("test-key")
	c.client = &http.Client{Timeout: 1 * time.Second}
	c.baseURL = "http://127.0.0.1:1"

	_, err := c.Status(ctx, "rg-123")
	require.Error(t, err)
}
