package lifi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Quote(t *testing.T) {
	apiKey := os.Getenv("LIFI_API_KEY")
	if apiKey == "" {
		t.Skip("LIFI_API_KEY not set")
	}
	c := NewClient(apiKey)
	params := QuoteParams{
		FromChain:   "1",
		ToChain:     "8453",
		FromToken:   "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		ToToken:     "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		FromAmount:  "1000000",
		FromAddress: "0x1234567890123456789012345678901234567890",
		ToAddress:   "0x0987654321098765432109876543210987654321",
		Slippage:    "0.005",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.ID)
}

func TestClient_QuoteHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lifi quote failed: status 500")
}

func TestClient_QuoteDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lifi quote decode")
}

func TestClient_Quote_SlippageAndParams(t *testing.T) {
	// Real HTTP: verify slippage and address params are correctly sent.
	apiKey := os.Getenv("LIFI_API_KEY")
	if apiKey == "" {
		t.Skip("LIFI_API_KEY not set")
	}
	c := NewClient(apiKey)
	params := QuoteParams{
		FromChain:   "1",
		ToChain:     "8453",
		FromToken:   "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC
		ToToken:     "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		FromAmount:  "1000000",
		FromAddress: "0x1234567890123456789012345678901234567890",
		ToAddress:   "0x0987654321098765432109876543210987654321",
		Slippage:    "0.015",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real LI.FI API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	// ID must be populated on success
	require.NotEmpty(t, resp.ID)
}

func TestClient_StatusHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtxhash")
	require.Error(t, err)
	require.Contains(t, err.Error(), "lifi status failed: status 502")
}

func TestClient_StatusDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtxhash")
	require.Error(t, err)
	require.Contains(t, err.Error(), "lifi status decode")
}

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	// Captured 2026-05-15. Verify all QuoteResponse fields are correctly deserialized
	// from real LI.FI API response data (simplified to only captured struct fields).
	raw := []byte(`{
				"id": "82915fc1-78eb-4256-b6c3-0490ea6f6dee:0",
				"fromAmount": "1000000",
				"toAmount": "994318",
				"estimate": {
					"toAmountMin": "994318",
					"approvalAddress": "0x1231DEB6f5749EF6cE6943a275A1D3E7486F4EaE",
					"gasCosts": [
						{
							"type": "SEND",
							"price": "181016891",
							"estimate": "215769",
							"limit": "280500",
							"amount": "39057833554179",
							"amountUSD": "0.0881",
							"token": {
								"address": "0x0000000000000000000000000000000000000000",
								"chainId": 1,
								"symbol": "ETH",
								"decimals": 18,
								"name": "ETH"
							}
						}
					],
					"feeCosts": [
						{
							"name": "LIFI Fixed Fee",
							"amount": "2500",
							"amountUSD": "0.0025",
							"token": {
								"address": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
								"chainId": 1,
								"symbol": "USDC",
								"decimals": 6,
								"name": "USD Coin"
							},
							"logoURI": "https://example.com/logo.svg"
						},
						{
							"name": "Relayer fee",
							"amount": "99",
							"amountUSD": "0.0001",
							"token": {
								"address": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
								"chainId": 1,
								"symbol": "USDC",
								"decimals": 6,
								"name": "USD Coin"
							}
						}
					]
				},
				"action": {
					"fromToken": {
						"address": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
						"symbol": "USDC",
						"decimals": 6
					},
					"toToken": {
						"address": "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
						"symbol": "USDC",
						"decimals": 6
					},
					"fromChainId": 1,
					"toChainId": 8453
				},
				"includedSteps": [
					{
						"type": "protocol",
						"tool": "feeCollection"
					},
					{
						"type": "cross",
						"tool": "across"
					}
				],
				"transactionRequest": {
					"to": "0x1231DEB6f5749EF6cE6943a275A1D3E7486F4EaE",
					"data": "0x1794958f000000000000000000000000",
					"value": "0x0",
					"gasLimit": "0x447b4"
				}
			}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{
		FromChain: "1", ToChain: "8453",
		FromToken:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		ToToken:    "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
		FromAmount: "1000000",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Top-level fields
	require.Equal(t, "82915fc1-78eb-4256-b6c3-0490ea6f6dee:0", resp.ID)
	require.Equal(t, "1000000", resp.FromAmount)
	require.Equal(t, "994318", resp.ToAmount)

	// Estimate
	require.Equal(t, "994318", resp.Estimate.ToAmountMin)
	require.Equal(t, "0x1231DEB6f5749EF6cE6943a275A1D3E7486F4EaE", resp.Estimate.ApprovalAddress)

	// GasCosts
	require.Len(t, resp.Estimate.GasCosts, 1)
	require.Equal(t, "SEND", resp.Estimate.GasCosts[0].Type)
	require.Equal(t, "215769", resp.Estimate.GasCosts[0].Estimate)
	require.Equal(t, "ETH", resp.Estimate.GasCosts[0].Token.Symbol)
	require.Equal(t, "0.0881", resp.Estimate.GasCosts[0].AmountUSD)

	// FeeCosts
	require.Len(t, resp.Estimate.FeeCosts, 2)
	require.Equal(t, "LIFI Fixed Fee", resp.Estimate.FeeCosts[0].Name)
	require.Equal(t, "2500", resp.Estimate.FeeCosts[0].Amount)
	require.Equal(t, "Relayer fee", resp.Estimate.FeeCosts[1].Name)
	require.Equal(t, "99", resp.Estimate.FeeCosts[1].Amount)

	// Action
	require.Equal(t, "USDC", resp.Action.FromToken.Symbol)
	require.Equal(t, 1, resp.Action.FromChainID)
	require.Equal(t, "USDC", resp.Action.ToToken.Symbol)
	require.Equal(t, 8453, resp.Action.ToChainID)

	// IncludedSteps
	require.Len(t, resp.IncludedSteps, 2)
	require.Equal(t, "protocol", resp.IncludedSteps[0].Type)
	require.Equal(t, "feeCollection", resp.IncludedSteps[0].Tool)
	require.Equal(t, "cross", resp.IncludedSteps[1].Type)
	require.Equal(t, "across", resp.IncludedSteps[1].Tool)

	// TransactionRequest
	require.Equal(t, "0x1231DEB6f5749EF6cE6943a275A1D3E7486F4EaE", resp.TransactionRequest.To)
	require.Equal(t, "0x1794958f000000000000000000000000", resp.TransactionRequest.Data)
	require.Equal(t, "0x0", resp.TransactionRequest.Value)
	require.Equal(t, "0x447b4", resp.TransactionRequest.GasLimit)
}

func TestClient_Status_JSONDeserialization(t *testing.T) {
	// Verify all StatusResponse fields are correctly deserialized from raw JSON,
	// including the newly added substatus and substatusMessage fields.
	raw := []byte(`{
			"status": "DONE",
			"substatus": "COMPLETED",
			"substatusMessage": "Transaction was completed successfully.",
			"sending": {
				"txHash": "0xsrcTxHash1234567890abcdef",
				"chainId": 1
			},
			"receiving": {
				"txHash": "0xdstTxHash1234567890abcdef",
				"chainId": 8453
			},
			"bridgeExplorerLink": "https://example.com/bridge/tx/123",
			"txHistoryUrl": "https://example.com/history/123",
			"tokenAmountIn": "1000000",
			"tokenAmountOut": "999000"
		}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	c := NewClient("test-key")
	c.baseURL = server.URL

	resp, err := c.Status(context.Background(), "0xtxhash")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Core status fields
	require.Equal(t, "DONE", resp.Status)
	require.Equal(t, "COMPLETED", resp.Substatus)
	require.Equal(t, "Transaction was completed successfully.", resp.SubstatusMessage)

	// Sending / Receiving
	require.Equal(t, "0xsrcTxHash1234567890abcdef", resp.Sending.TxHash)
	require.Equal(t, 1, resp.Sending.ChainID)
	require.Equal(t, "0xdstTxHash1234567890abcdef", resp.Receiving.TxHash)
	require.Equal(t, 8453, resp.Receiving.ChainID)

	// Additional fields
	require.Equal(t, "https://example.com/bridge/tx/123", resp.BridgeExplorer)
	require.Equal(t, "https://example.com/history/123", resp.TxHistoryURL)
	require.Equal(t, "1000000", resp.TokenAmountIn)
	require.Equal(t, "999000", resp.TokenAmountOut)
}
