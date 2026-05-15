package across

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	require.NotEmpty(t, resp.ID)
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
			InputAmount:          "1000000",
			ExpectedOutputAmount: "990000",
			SwapTx:               TxInfo{To: "0xSpoke", Data: "0xdeadbeef", Value: "0", Gas: "210000"},
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

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	// Tests full JSON deserialization using a response shaped from a real Across API call. Captured 2026-05-15.
	rawJSON := `{
		"crossSwapType": "bridgeableToBridgeable",
		"amountType": "exactInput",
		"checks": {
			"allowance": {"token": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", "spender": "0x5c7BCd6E7De5423a257D81B442095A1a6ced35C5", "actual": "0", "expected": "1000000"},
			"balance": {"token": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", "actual": "614170", "expected": "1000000"}
		},
		"approvalTxns": [{"chainId": 1, "to": "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", "data": "0x095ea7b3"}],
		"steps": {
			"bridge": {"inputAmount": "1000000", "outputAmount": "996821", "provider": "across"}
		},
		"inputAmount": "1000000",
		"maxInputAmount": "1000000",
		"expectedOutputAmount": "996821",
		"minOutputAmount": "996821",
		"expectedFillTime": 2,
		"swapTx": {
			"ecosystem": "evm",
			"simulationSuccess": false,
			"chainId": 1,
			"to": "0x5c7BCd6E7De5423a257D81B442095A1a6ced35C5",
			"data": "0xad5425c6",
			"value": "0",
			"gas": "0"
		},
		"quoteExpiryTimestamp": 1778836703,
		"id": "chqpg-1778833239607-ffaac4e43d73"
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	}))
	defer srv.Close()

	c := NewClient()
	c.baseURL = srv.URL
	resp, err := c.Quote(context.Background(), QuoteParams{
		InputToken: "0xA", OutputToken: "0xB", Amount: "1000000",
		OriginChainID: "1", DestinationChainID: "8453",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify all fields deserialized correctly from raw JSON
	assert.Equal(t, "bridgeableToBridgeable", resp.CrossSwapType)
	assert.Equal(t, "exactInput", resp.AmountType)
	assert.Equal(t, "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", resp.Checks.Allowance.Token)
	assert.Equal(t, "0x5c7BCd6E7De5423a257D81B442095A1a6ced35C5", resp.Checks.Allowance.Spender)
	assert.Equal(t, "0", resp.Checks.Allowance.Actual)
	assert.Equal(t, "1000000", resp.Checks.Allowance.Expected)
	assert.Equal(t, "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", resp.Checks.Balance.Token)
	assert.Equal(t, "614170", resp.Checks.Balance.Actual)
	assert.Equal(t, "1000000", resp.Checks.Balance.Expected)
	require.Len(t, resp.ApprovalTxns, 1)
	assert.Equal(t, 1, resp.ApprovalTxns[0].ChainID)
	assert.Equal(t, "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", resp.ApprovalTxns[0].To)
	assert.Equal(t, "1000000", resp.Steps.Bridge.InputAmount)
	assert.Equal(t, "996821", resp.Steps.Bridge.OutputAmount)
	assert.Equal(t, "across", resp.Steps.Bridge.Provider)
	assert.Equal(t, "1000000", resp.InputAmount)
	assert.Equal(t, "1000000", resp.MaxInputAmount)
	assert.Equal(t, "996821", resp.ExpectedOutputAmount)
	assert.Equal(t, "996821", resp.MinOutputAmount)
	assert.Equal(t, 2, resp.ExpectedFillTime)
	assert.Equal(t, "0x5c7BCd6E7De5423a257D81B442095A1a6ced35C5", resp.SwapTx.To)
	assert.Equal(t, "0xad5425c6", resp.SwapTx.Data)
	assert.Equal(t, "0", resp.SwapTx.Value)
	assert.Equal(t, "0", resp.SwapTx.Gas)
	assert.Equal(t, "evm", resp.SwapTx.Ecosystem)
	assert.False(t, resp.SwapTx.SimulationSuccess)
	assert.Equal(t, 1, resp.SwapTx.ChainID)
	assert.Equal(t, int64(1778836703), resp.QuoteExpiryTimestamp)
	assert.Equal(t, "chqpg-1778833239607-ffaac4e43d73", resp.ID)
}

func TestClient_Quote_DefaultTradeType(t *testing.T) {
	var tradeType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tradeType = r.URL.Query().Get("tradeType")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{ExpectedOutputAmount: "1"})
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.NoError(t, err)
	require.Equal(t, defaultTradeType, tradeType)
}
