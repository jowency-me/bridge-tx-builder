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
		Slippage:    "1",
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
	var body map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		contentType = r.Header.Get("Content-Type")
		integrator = r.Header.Get("x-integrator-id")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{RequestID: "req-1"})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "integrator-1"

	_, err := c.Quote(context.Background(), QuoteParams{
		FromChain:   "1",
		ToChain:     "8453",
		FromToken:   "0xFromToken",
		ToToken:     "0xToToken",
		FromAmount:  "1000000",
		FromAddress: "0xFrom",
		ToAddress:   "0xTo",
		Slippage:    "1",
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
		_ = json.NewEncoder(w).Encode(StatusResponse{ID: "status-1", Status: "completed"})
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
