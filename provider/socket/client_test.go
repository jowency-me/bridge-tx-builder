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
		_, _ = fmt.Fprint(w, `{
			"success": true,
			"statusCode": 200,
			"result": {
				"originChainId": 1,
				"destinationChainId": 8453,
				"userAddress": "0x1234567890123456789012345678901234567890",
				"receiverAddress": "0x1234567890123456789012345678901234567890",
				"input": {
					"token": {"chainId": 1, "address": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", "symbol": "USDC", "decimals": 6},
					"amount": "1000000000"
				},
				"autoRoute": {
					"userOp": "sign",
					"output": {
						"token": {"chainId": 8453, "address": "0xd9aaec86b65d86f6a7b5b1b0c42ffa531710b6ca", "symbol": "USDbC", "decimals": 6},
						"amount": "999000000",
						"minAmountOut": "998317548"
					},
					"approvalData": {
						"spenderAddress": "0xSpender",
						"amount": "1000000000",
						"tokenAddress": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
					},
					"quoteId": "ff5adfd18b85cc86",
					"outputAmount": "999000000",
					"slippage": 0.01
				}
			}
		}`)
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
	require.NotNil(t, resp.Result)
	require.NotNil(t, resp.Result.AutoRoute)
	require.Equal(t, "999000000", resp.Result.AutoRoute.OutputAmount)
	require.Equal(t, "ff5adfd18b85cc86", resp.Result.AutoRoute.QuoteID)
}

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	rawJSON := []byte(`{
		"success": true,
		"statusCode": 200,
		"result": {
			"originChainId": 1,
			"destinationChainId": 8453,
			"userAddress": "0x1234567890123456789012345678901234567890",
			"receiverAddress": "0x1234567890123456789012345678901234567890",
			"input": {
				"token": {
					"chainId": 1,
					"address": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
					"name": "USDC",
					"symbol": "USDC",
					"decimals": 6,
					"logoURI": "https://example.com/usdc.png",
					"icon": "https://example.com/usdc.png"
				},
				"amount": "1000000000",
				"priceInUsd": 1,
				"valueInUsd": 1000
			},
			"autoRoute": {
				"userOp": "sign",
				"requestHash": "0xb8a568f141bc49611842c1451981d51679f1d6a6146a5143af5f4fb703845d08",
				"output": {
					"token": {
						"chainId": 8453,
						"address": "0xd9aaec86b65d86f6a7b5b1b0c42ffa531710b6ca",
						"name": "USDbC",
						"symbol": "USDbC",
						"decimals": 6
					},
					"priceInUsd": 1,
					"valueInUsd": 999,
					"minAmountOut": "998317548",
					"amount": "999000000",
					"effectiveAmount": "998417390",
					"effectiveValueInUsd": 998.41739,
					"effectiveReceivedInUsd": 999
				},
				"requestType": "SINGLE_OUTPUT_REQUEST",
				"approvalData": {
					"spenderAddress": "0x000000000022D473030F116dDEE9F6B43aC78BA3",
					"amount": "1000000000",
					"tokenAddress": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
					"userAddress": "0x1234567890123456789012345678901234567890"
				},
				"gasFee": null,
				"slippage": 0.01,
				"suggestedClientSlippage": 0.01,
				"txData": null,
				"estimatedTime": 120,
				"routeDetails": {
					"name": "Bungee Protocol",
					"logoURI": "",
					"routeFee": null,
					"dexDetails": null
				},
				"quoteId": "ff5adfd18b85cc86",
				"quoteExpiry": 1778784271,
				"outputAmount": "999000000",
				"routeTags": ["MAX_OUTPUT", "SUGGESTED"]
			}
		},
		"message": null
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawJSON)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{
		FromChainID:      "1",
		ToChainID:        "8453",
		FromTokenAddress: "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
		ToTokenAddress:   "0xd9aaec86b65d86f6a7b5b1b0c42ffa531710b6ca",
		FromAmount:       "1000000000",
		UserAddress:      "0xFrom",
		Recipient:        "0xTo",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify top-level fields
	require.True(t, resp.Success)
	require.Equal(t, 200, resp.StatusCode)
	require.NotNil(t, resp.Result)

	// Verify result fields
	require.Equal(t, 1, resp.Result.OriginChainID)
	require.Equal(t, 8453, resp.Result.DestinationChainID)
	require.Equal(t, "0x1234567890123456789012345678901234567890", resp.Result.UserAddress)
	require.Equal(t, "0x1234567890123456789012345678901234567890", resp.Result.ReceiverAddress)

	// Verify input data
	require.Equal(t, "1000000000", resp.Result.Input.Amount)
	require.Equal(t, "USDC", resp.Result.Input.Token.Symbol)
	require.Equal(t, 6, resp.Result.Input.Token.Decimals)

	// Verify autoRoute
	require.NotNil(t, resp.Result.AutoRoute)
	ar := resp.Result.AutoRoute
	require.Equal(t, "sign", ar.UserOp)
	require.Equal(t, "999000000", ar.OutputAmount)
	require.Equal(t, 0.01, ar.Slippage)
	require.Equal(t, 120, ar.EstimatedTime)
	require.Equal(t, "ff5adfd18b85cc86", ar.QuoteID)
	require.Equal(t, int64(1778784271), ar.QuoteExpiry)
	require.Contains(t, ar.RouteTags, "MAX_OUTPUT")

	// Verify output data
	require.Equal(t, "999000000", ar.Output.Amount)
	require.Equal(t, "998317548", ar.Output.MinAmountOut)
	require.Equal(t, "USDbC", ar.Output.Token.Symbol)

	// Verify approval data
	require.NotNil(t, ar.ApprovalData)
	require.Equal(t, "0x000000000022D473030F116dDEE9F6B43aC78BA3", ar.ApprovalData.SpenderAddress)
	require.Equal(t, "1000000000", ar.ApprovalData.Amount)
	require.Equal(t, "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", ar.ApprovalData.TokenAddress)

	// Verify route details
	require.Equal(t, "Bungee Protocol", ar.RouteDetails.Name)
}

func TestClient_Status_JSONDeserialization(t *testing.T) {
	rawJSON := []byte(`{
		"success": true,
		"statusCode": 200,
		"result": [
			{
				"hash": "0xReqHash123",
				"originData": {
					"txHash": "0xSourceTx123",
					"originChainId": 1,
					"status": "COMPLETED",
					"userAddress": "0xFrom"
				},
				"destinationData": {
					"txHash": "0xDestTx456",
					"destinationChainId": 8453,
					"receiverAddress": "0xTo",
					"status": "COMPLETED"
				},
				"bungeeStatusCode": 10
			}
		]
	}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rawJSON)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Status(context.Background(), "0xSourceTx123")
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.True(t, resp.Success)
	require.Len(t, resp.Result, 1)
	require.Equal(t, "0xReqHash123", resp.Result[0].Hash)
	require.Equal(t, "0xSourceTx123", resp.Result[0].OriginData.TxHash)
	require.Equal(t, "0xDestTx456", resp.Result[0].DestinationData.TxHash)
	require.Equal(t, 1, resp.Result[0].OriginData.OriginChainID)
	require.Equal(t, 8453, resp.Result[0].DestinationData.DestinationChainID)
	require.Equal(t, "COMPLETED", resp.Result[0].OriginData.Status)
	require.Equal(t, 10, resp.Result[0].BungeeStatusCode)
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
		_, _ = fmt.Fprint(w, `{"success": true, "result": [{"hash": "0xH", "originData": {"txHash": "0xSrc", "status": "COMPLETED"}, "destinationData": {"txHash": "0xDst", "status": "COMPLETED"}, "bungeeStatusCode": 10}]}`)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	resp, err := c.Status(context.Background(), "0xtxid")
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, "COMPLETED", resp.Result[0].OriginData.Status)
}
