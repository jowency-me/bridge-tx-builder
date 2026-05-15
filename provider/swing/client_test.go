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

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	rawJSON := []byte(`{
		"routes": [
			{
				"route": [
					{
						"bridge": "stargate",
						"bridgeTokenAddress": "0x1234",
						"steps": ["approve", "send"],
						"name": "USDC",
						"part": 100
					}
				],
				"quote": {
					"integration": "stargate",
					"type": "bridge",
					"amount": "995000",
					"decimals": 6,
					"bridgeFee": "20867118",
					"bridgeFeeInNativeToken": "0.005",
					"amountUSD": "995.00",
					"bridgeFeeUSD": "3.50",
					"bridgeFeeInNativeTokenUSD": "2.10",
					"fees": [
						{
							"type": "bridge",
							"amount": "20867118",
							"amountUSD": "3.50",
							"tokenSymbol": "USDC",
							"tokenAddress": "0xA",
							"chainSlug": "ethereum",
							"decimals": 6,
							"deductedFromSourceToken": true
						}
					]
				},
				"duration": 180,
				"gas": "3500000000000000",
				"gasUSD": "12.50"
			}
		],
		"fromToken": {"symbol": "USDC", "address": "0xA", "decimals": 6, "chainId": 1},
		"fromChain": {"chainId": 1, "slug": "ethereum", "protocolType": "evm"},
		"toToken": {"symbol": "USDT", "address": "0xB", "decimals": 6, "chainId": 8453},
		"toChain": {"chainId": 8453, "slug": "base", "protocolType": "evm"}
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawJSON)
	}))
	defer server.Close()

	c := NewClient("test-project")
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{
		FromChain:  "ethereum",
		ToChain:    "base",
		FromToken:  "0xA",
		ToToken:    "0xB",
		FromAmount: "1000000",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify routes
	require.Len(t, resp.Routes, 1)
	ri := resp.Routes[0]

	// Verify route steps
	require.Len(t, ri.Route, 1)
	require.Equal(t, "stargate", ri.Route[0].Bridge)
	require.Equal(t, "0x1234", ri.Route[0].BridgeTokenAddress)
	require.Equal(t, []string{"approve", "send"}, ri.Route[0].Steps)
	require.Equal(t, "USDC", ri.Route[0].Name)
	require.Equal(t, 100, ri.Route[0].Part)

	// Verify quote details
	require.Equal(t, "stargate", ri.Quote.Integration)
	require.Equal(t, "bridge", ri.Quote.Type)
	require.Equal(t, "995000", ri.Quote.Amount)
	require.Equal(t, 6, ri.Quote.Decimals)
	require.Equal(t, "20867118", ri.Quote.BridgeFee)
	require.Equal(t, "995.00", ri.Quote.AmountUSD)

	// Verify fees
	require.Len(t, ri.Quote.Fees, 1)
	require.Equal(t, "bridge", ri.Quote.Fees[0].Type)
	require.Equal(t, "20867118", ri.Quote.Fees[0].Amount)
	require.Equal(t, "3.50", ri.Quote.Fees[0].AmountUSD)
	require.Equal(t, "USDC", ri.Quote.Fees[0].TokenSymbol)
	require.Equal(t, true, ri.Quote.Fees[0].DeductedFromSourceToken)

	// Verify duration/gas
	require.Equal(t, 180, ri.Duration)
	require.Equal(t, "3500000000000000", ri.Gas)
	require.Equal(t, "12.50", ri.GasUSD)

	// Verify top-level token/chain metadata
	require.Equal(t, "USDC", resp.FromToken.Symbol)
	require.Equal(t, "0xA", resp.FromToken.Address)
	require.Equal(t, 1, resp.FromToken.ChainID)
	require.Equal(t, "ethereum", resp.FromChain.Slug)
	require.Equal(t, "USDT", resp.ToToken.Symbol)
	require.Equal(t, 8453, resp.ToToken.ChainID)
	require.Equal(t, "base", resp.ToChain.Slug)
}

func TestClient_Status_JSONDeserialization(t *testing.T) {
	rawJSON := []byte(`{
		"status": "completed",
		"fromChainTxHash": "0xSrcHash",
		"toChainTxHash": "0xDstHash",
		"txId": "swing-tx-789",
		"reason": ""
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawJSON)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	resp, err := c.Status(context.Background(), "swing-tx-789")
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Equal(t, "completed", resp.Status)
	require.Equal(t, "0xSrcHash", resp.FromChainTxHash)
	require.Equal(t, "0xDstHash", resp.ToChainTxHash)
	require.Equal(t, "swing-tx-789", resp.TxID)
	require.Equal(t, "", resp.Reason)
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
