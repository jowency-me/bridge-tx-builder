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
		Slippage:                      "0.5",
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recipient = r.URL.Query().Get("dstChainTokenOutRecipient")
		dstAuthority = r.URL.Query().Get("dstChainOrderAuthorityAddress")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{OrderID: "order-1"})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
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
		Slippage:                      "0.5",
	})
	require.NoError(t, err)
	require.Equal(t, "0xTo", recipient)
	require.Equal(t, "0xTo", dstAuthority)
}
