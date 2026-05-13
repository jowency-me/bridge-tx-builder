package oneinch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Quote(t *testing.T) {
	apiKey := os.Getenv("INCH_API_KEY")
	if apiKey == "" {
		t.Skip("INCH_API_KEY not set")
	}
	c := NewClient(apiKey)
	params := QuoteParams{
		ChainID: "1",
		Src:     "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
		Dst:     "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		Amount:  "1000000000000000000",
		From:    "0x1234567890123456789012345678901234567890",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skip("real API unavailable:", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.DstAmount)
}

func TestClient_Status(t *testing.T) {
	apiKey := os.Getenv("INCH_API_KEY")
	if apiKey == "" {
		t.Skip("INCH_API_KEY not set")
	}
	c := NewClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1inch has no transaction status API; Status returns reachable via HEAD.
	resp, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	if err != nil {
		t.Skipf("1inch server unreachable: %v", err)
	}
	require.NotNil(t, resp)
}

func TestClient_Quote_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/swap/v6.1/1/swap")
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			DstAmount: "999000",
			SrcAmount: "1000000",
			FromToken: TokenInfo{Symbol: "ETH", Address: "0xEeee", Decimals: 18},
			ToToken:   TokenInfo{Symbol: "USDC", Address: "0xA0b8", Decimals: 6},
			Tx: TxData{
				To:    "0xRouter",
				Data:  "0xdeadbeef",
				Value: "0",
				Gas:   200000,
			},
			Gas: 150000,
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	params := QuoteParams{
		ChainID:  "1",
		Src:      "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",
		Dst:      "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		Amount:   "1000000000000000000",
		From:     "0x1234567890123456789012345678901234567890",
		Slippage: "0.5",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "999000", resp.DstAmount)
	assert.Equal(t, "1000000", resp.SrcAmount)
	assert.Equal(t, "0xRouter", resp.Tx.To)
}

func TestClient_Quote_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	params := QuoteParams{ChainID: "1", Src: "0xA", Dst: "0xB", Amount: "1000"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Quote(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestClient_Quote_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	params := QuoteParams{ChainID: "1", Src: "0xA", Dst: "0xB", Amount: "1000"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Quote(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestClient_Status_ServerReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Status(ctx, "0xTx")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "reachable", resp.Status)
}

func TestClient_Status_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0xTx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server error")
}

func newTestClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, client: &http.Client{Timeout: 5 * time.Second}, apiKey: "test-key"}
}
