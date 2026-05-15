package debridge

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
		SrcChainID:                    "1",
		SrcChainTokenIn:               "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		SrcChainTokenInAmount:         "1000000",
		DstChainID:                    "8453",
		DstChainTokenOut:              "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		SrcChainOrderAuthorityAddress: "0x1234567890123456789012345678901234567890",
		DstChainOrderAuthorityAddress: "0x0987654321098765432109876543210987654321",
		DstChainTokenOutRecipient:     "0x0987654321098765432109876543210987654321",
		DstChainTokenOutAmount:        "auto",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.OrderID)
}

func TestClient_Status(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	if err != nil {
		t.Skipf("real API unavailable: %v", err)
	}
}

func TestClient_Quote_RequestParams(t *testing.T) {
	var recipient string
	var dstAuthority string
	var apiKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recipient = r.URL.Query().Get("dstChainTokenOutRecipient")
		dstAuthority = r.URL.Query().Get("dstChainOrderAuthorityAddress")
		apiKey = r.Header.Get("X-DeBridge-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{OrderID: "order-1"})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "debridge-key"
	_, err := c.Quote(context.Background(), QuoteParams{
		SrcChainID:                    "1",
		SrcChainTokenIn:               "0xA",
		SrcChainTokenInAmount:         "1000",
		DstChainID:                    "8453",
		DstChainTokenOut:              "0xB",
		SrcChainOrderAuthorityAddress: "0xFrom",
		DstChainOrderAuthorityAddress: "0xTo",
		DstChainTokenOutRecipient:     "0xTo",
		DstChainTokenOutAmount:        "auto",
	})
	require.NoError(t, err)
	require.Equal(t, "0xTo", recipient)
	require.Equal(t, "0xTo", dstAuthority)
	require.Equal(t, "debridge-key", apiKey)
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
	require.Contains(t, err.Error(), "debridge quote failed: status 502")
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
	require.Contains(t, err.Error(), "debridge quote decode")
}

func TestClient_Status_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtx")
	require.Error(t, err)
	require.Contains(t, err.Error(), "debridge status failed: status 500")
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
	require.Contains(t, err.Error(), "debridge status decode")
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

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	// Verify all QuoteResponse fields are correctly deserialized from real deBridge DLN API response.
	// Captured 2026-05-15.
	raw := []byte(`{
			"orderId": "0xf2a2e4d76821a778e16c49cdd668ea02ed4d6887147acf2c1fe09e646b9d4fbb",
			"estimation": {
				"srcChainTokenIn": {
					"chainId": 1,
					"address": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
					"name": "USD Coin",
					"symbol": "USDC",
					"decimals": 6,
					"amount": "1000000"
				},
				"dstChainTokenOut": {
					"chainId": 8453,
					"address": "0x833589fcd6edb6e08f4c7c32d4f71b54bdA02913",
					"name": "USD Coin",
					"symbol": "USDC",
					"decimals": 6,
					"amount": "685481"
				}
			},
			"tx": {
				"data": "0xb9303701",
				"to": "0xeF4fB24aD0916217251F553c0596F8Edc630EB66",
				"value": "1000000000000000"
			}
		}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{
		SrcChainID:            "1",
		SrcChainTokenIn:       "0xA",
		SrcChainTokenInAmount: "1000000",
		DstChainID:            "8453",
		DstChainTokenOut:      "0xB",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify top-level fields
	require.Equal(t, "0xf2a2e4d76821a778e16c49cdd668ea02ed4d6887147acf2c1fe09e646b9d4fbb", resp.OrderID)

	// Verify estimation - srcChainTokenIn
	require.Equal(t, "1000000", resp.Estimation.SrcChainTokenIn.Amount)
	require.Equal(t, "USDC", resp.Estimation.SrcChainTokenIn.Symbol)
	require.Equal(t, "USD Coin", resp.Estimation.SrcChainTokenIn.Name)
	require.Equal(t, "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", resp.Estimation.SrcChainTokenIn.Address)
	require.Equal(t, 6, resp.Estimation.SrcChainTokenIn.Decimals)
	require.Equal(t, 1, resp.Estimation.SrcChainTokenIn.ChainID)

	// Verify estimation - dstChainTokenOut
	require.Equal(t, "685481", resp.Estimation.DstChainTokenOut.Amount)
	require.Equal(t, "USDC", resp.Estimation.DstChainTokenOut.Symbol)
	require.Equal(t, "USD Coin", resp.Estimation.DstChainTokenOut.Name)
	require.Equal(t, "0x833589fcd6edb6e08f4c7c32d4f71b54bdA02913", resp.Estimation.DstChainTokenOut.Address)
	require.Equal(t, 6, resp.Estimation.DstChainTokenOut.Decimals)
	require.Equal(t, 8453, resp.Estimation.DstChainTokenOut.ChainID)

	// Verify tx
	require.Equal(t, "0xeF4fB24aD0916217251F553c0596F8Edc630EB66", resp.Tx.To)
	require.Equal(t, "0xb9303701", resp.Tx.Data)
	require.Equal(t, "1000000000000000", resp.Tx.Value)
}

func TestClient_Status_JSONDeserialization(t *testing.T) {
	// Verify StatusResponse fields are correctly deserialized from raw JSON.
	raw := []byte(`{
			"orderId": "0xOrderStatus999",
			"status": "COMPLETED"
		}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Status(context.Background(), "0xOrderStatus999")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "0xOrderStatus999", resp.OrderID)
	require.Equal(t, "COMPLETED", resp.Status)
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
