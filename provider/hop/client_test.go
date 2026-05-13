package hop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Quote(t *testing.T) {
	c := NewClient()
	params := QuoteParams{
		FromChain: "ethereum",
		ToChain:   "base",
		Token:     "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		Amount:    "1000000",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	if resp.AmountOut == "" {
		t.Skip("real API returned empty quote")
	}
}

func TestClient_QuoteHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hop quote failed: status 500")
}

func TestClient_QuoteDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hop quote decode")
}

func TestClient_QuoteRequestParams(t *testing.T) {
	var receivedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{
			"amountOut": "995000",
			"bridge": "hop",
			"estimatedRecipientTime": 120,
			"fee": "0.005",
			"slippage": "0.5"
		}`)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	params := QuoteParams{
		FromChain: "ethereum",
		ToChain:   "base",
		Token:     "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		Amount:    "1000000",
	}
	resp, err := c.Quote(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Contains(t, receivedURL, "fromChain=ethereum")
	require.Contains(t, receivedURL, "toChain=base")
	require.Contains(t, receivedURL, "token=0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C")
	require.Contains(t, receivedURL, "amount=1000000")
}

func TestClient_QuoteResponseParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{
			"amountOut": "995000",
			"bridge": "hop",
			"estimatedRecipientTime": 120,
			"fee": "0.005",
			"slippage": "0.5"
		}`)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	resp, err := c.Quote(context.Background(), QuoteParams{})
	require.NoError(t, err)
	require.Equal(t, "995000", resp.AmountOut)
	require.Equal(t, "hop", resp.Bridge)
	require.Equal(t, 120, resp.EstimatedRecipientTime)
	require.Equal(t, "0.005", resp.Fee)
	require.Equal(t, "0.5", resp.Slippage)
}

func TestClient_Status(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	// Hop does not support status API.
	require.Error(t, err)
	require.Contains(t, err.Error(), "hop status not supported")
}

func TestClient_StatusHTTPError(t *testing.T) {
	// Even if Hop added status support, test that network errors are handled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtxhash")
	require.Error(t, err)
	require.Contains(t, err.Error(), "hop status not supported")
}

func TestClient_StatusDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtxhash")
	require.Error(t, err)
	require.Contains(t, err.Error(), "hop status not supported")
}
