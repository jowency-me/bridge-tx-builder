package socket

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Quote(t *testing.T) {
	apiKey := os.Getenv("SOCKET_API_KEY")
	if apiKey == "" {
		t.Skip("SOCKET_API_KEY not set")
	}
	c := NewClient()
	c.apiKey = apiKey
	params := QuoteParams{
		FromChainID:      "1",
		ToChainID:        "8453",
		FromTokenAddress: "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		ToTokenAddress:   "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		FromAmount:       "1000000",
		UserAddress:      "0x1234567890123456789012345678901234567890",
		Recipient:        "0x0987654321098765432109876543210987654321",
		Slippage:         "0.5",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		if err.Error() == "socket quote failed: status 401 (unauthorized)" {
			t.Skip("Socket API requires authentication")
		}
		t.Skipf("real API unavailable: %v", err)
	}
	require.NotNil(t, resp)
}

func TestClient_QuoteHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "socket quote failed: status 503")
}

func TestClient_QuoteUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "socket quote failed: status 401")
}

func TestClient_QuoteDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "socket quote decode")
}

func TestClient_QuoteSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"routes": [{"routeId": "socket-123", "toAmount": "990000", "totalGasFeesInUsd": "5", "totalFeeInUsd": "1", "userTxs": []}]}`)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	params := QuoteParams{
		FromChainID:      "1",
		ToChainID:        "8453",
		FromTokenAddress: "0xA",
		ToTokenAddress:   "0xB",
		FromAmount:       "1000000",
		UserAddress:      "0xFrom",
		Recipient:        "0xTo",
		Slippage:         "1",
	}
	resp, err := c.Quote(context.Background(), params)
	require.NoError(t, err)
	require.Len(t, resp.Routes, 1)
	require.Equal(t, "socket-123", resp.Routes[0].RouteID)
}

func TestClient_Status(t *testing.T) {
	apiKey := os.Getenv("SOCKET_API_KEY")
	if apiKey == "" {
		t.Skip("SOCKET_API_KEY not set")
	}
	c := NewClient()
	c.apiKey = apiKey
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	if err != nil {
		if err.Error() == "socket status failed: status 401 (unauthorized)" {
			t.Skip("Socket API requires authentication")
		}
		t.Skipf("real API unavailable: %v", err)
	}
}

func TestClient_StatusHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtxid")
	require.Error(t, err)
	require.Contains(t, err.Error(), "socket status failed: status 503")
}

func TestClient_StatusUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtxid")
	require.Error(t, err)
	require.Contains(t, err.Error(), "socket status failed: status 401")
}

func TestClient_StatusDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtxid")
	require.Error(t, err)
	require.Contains(t, err.Error(), "socket status decode")
}

func TestClient_StatusSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"success": true, "result": {"sourceTxHash": "0xSrc", "destinationTxHash": "0xDst", "status": "completed"}}`)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	resp, err := c.Status(context.Background(), "0xtxid")
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, "completed", resp.Result.Status)
}
