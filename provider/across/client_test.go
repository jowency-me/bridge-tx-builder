package across

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
		InputToken:         "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		OutputToken:        "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
		Amount:             "1000000",
		OriginChainID:      "1",
		DestinationChainID: "8453",
		Depositor:          "0x1234567890123456789012345678901234567890",
		Recipient:          "0x1234567890123456789012345678901234567890",
		TradeType:          defaultTradeType,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.QuoteBlock)
}

func TestClient_Status(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	// Across does not support status API.
	require.Error(t, err)
}

func TestClient_Quote_RequestParams(t *testing.T) {
	var path string
	var auth string
	var integrator string
	var depositor string
	var tradeType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		integrator = r.URL.Query().Get("integratorId")
		depositor = r.URL.Query().Get("depositor")
		tradeType = r.URL.Query().Get("tradeType")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			InputAmount:  "1000000",
			OutputAmount: "990000",
			SwapTx:       TxInfo{To: "0xSpoke", Data: "0xdeadbeef", Value: "0", Gas: "210000"},
		})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "api-key"
	c.integratorID = "integrator"
	resp, err := c.Quote(context.Background(), QuoteParams{
		InputToken:         "0xA",
		OutputToken:        "0xB",
		Amount:             "1000000",
		OriginChainID:      "1",
		DestinationChainID: "8453",
		Depositor:          "0xFrom",
		Recipient:          "0xTo",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "/swap/approval", path)
	require.Equal(t, "Bearer api-key", auth)
	require.Equal(t, "integrator", integrator)
	require.Equal(t, "0xFrom", depositor)
	require.Equal(t, defaultTradeType, tradeType)
}

func TestClient_Quote_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "across quote failed: status 502")
}

func TestClient_Quote_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "across quote decode")
}

func TestClient_Quote_DefaultTradeType(t *testing.T) {
	var tradeType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tradeType = r.URL.Query().Get("tradeType")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{OutputAmount: "1"})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.NoError(t, err)
	require.Equal(t, defaultTradeType, tradeType)
}
