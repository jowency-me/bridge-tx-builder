package thorchain

import (
	"context"
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
		FromAsset:   "ETH.ETH",
		ToAsset:     "BTC.BTC",
		Amount:      "100000000",
		Destination: "bc1qyl7wjm2ldfezgnjk2c78adqlk7dvtm8sd7gn0q",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skip("real API unavailable:", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.ExpectedAmountOut)
}

func TestClient_Status(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	if err != nil {
		t.Skip("real API unavailable:", err)
	}
}

func TestClient_Quote_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "thorchain quote failed: status 500")
}

func TestClient_Quote_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "thorchain quote decode")
}

func TestClient_Quote_RequestParams(t *testing.T) {
	var fromAsset, toAsset, amount, destination string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		fromAsset = q.Get("from_asset")
		toAsset = q.Get("to_asset")
		amount = q.Get("amount")
		destination = q.Get("destination")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"expected_amount_out":"1000000","inbound_address":"bc1qtest","memo":"=:ETH.ETH:0xTo","expiry":1234567890,"slippage_bps":50}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	resp, err := c.Quote(context.Background(), QuoteParams{
		FromAsset:   "ETH.ETH",
		ToAsset:     "BTC.BTC",
		Amount:      "100000000",
		Destination: "bc1qdest",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "ETH.ETH", fromAsset)
	require.Equal(t, "BTC.BTC", toAsset)
	require.Equal(t, "100000000", amount)
	require.Equal(t, "bc1qdest", destination)
}

func TestClient_Quote_WithAPIKey(t *testing.T) {
	var apiKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey = r.Header.Get("x-client-id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"expected_amount_out":"1000000","inbound_address":"bc1qtest","memo":"=:ETH","expiry":1234567890}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "thor-key"
	_, err := c.Quote(context.Background(), QuoteParams{})
	require.NoError(t, err)
	require.Equal(t, "thor-key", apiKey)
}

func TestClient_Status_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtx")
	require.Error(t, err)
	require.Contains(t, err.Error(), "thorchain status failed: status 404")
}

func TestClient_Status_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "0xtx")
	require.Error(t, err)
	require.Contains(t, err.Error(), "thorchain status decode")
}

func TestClient_Status_RequestParams(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tx":{"id":"tx-1","chain":"THOR","from_address":"thor1addr","to_address":"thor1addr","coins":[{"asset":"THOR.RUNE","amount":"1000"}]},"stages":{"inbound_observed":{"final_count":0,"completed":true},"inbound_confirmation_counted":{"remaining_confirmation_seconds":0,"completed":true},"inbound_finalised":{"completed":true},"swap_status":{"pending":false},"swap_finalised":{"completed":true}}}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Status(context.Background(), "tx-123")
	require.NoError(t, err)
	require.Equal(t, "/thorchain/tx/status/tx-123", path)
}

func TestClient_Status_WithAPIKey(t *testing.T) {
	var apiKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey = r.Header.Get("x-client-id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tx":{"id":"tx-1","chain":"THOR","from_address":"thor1addr","to_address":"thor1addr","coins":[]},"stages":{"inbound_observed":{"completed":true},"inbound_finalised":{"completed":true},"swap_status":{"pending":false},"swap_finalised":{"completed":true}}}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	c.apiKey = "thor-status-key"
	_, err := c.Status(context.Background(), "tx-123")
	require.NoError(t, err)
	require.Equal(t, "thor-status-key", apiKey)
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
	// Doc-verified quote response (THORChain trading halted during verification,
	// struct verified against official docs at docs.thorchain.org).
	rawJSON := `{
		"inbound_address": "bc1qt9723ak9t7lu7a97lt9kelq4gnrlmyvk4yhzwr",
		"inbound_confirmation_blocks": 1,
		"inbound_confirmation_seconds": 600,
		"outbound_delay_blocks": 576,
		"outbound_delay_seconds": 7200,
		"fees": {
			"asset": "BTC.BTC",
			"affiliate": "0",
			"outbound": "54840",
			"liquidity": "2037232",
			"total": "2092072",
			"slippage_bps": 9,
			"total_bps": 11
		},
		"slippage_bps": 50,
		"streaming_slippage_bps": 0,
		"expiry": 1715800000,
		"warning": "Do not send more than the recommended amount",
		"notes": "ETH.ETH output amount may vary due to gas deductions",
		"dust_threshold": "0.0001",
		"recommended_min_amount_in": "1000000",
		"recommended_gas_rate": "7",
		"gas_rate_units": "satsperbyte",
		"memo": "=:ETH.ETH:0x86d526d6624AbC0178cF7296cD538Ecc080A95F1:0/1/0",
		"expected_amount_out": "2035299208",
		"expected_amount_out_streaming": "0",
		"max_streaming_quantity": 0,
		"streaming_swap_blocks": 0,
		"streaming_swap_seconds": 0,
		"total_swap_seconds": 7800
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Quote(context.Background(), QuoteParams{
		FromAsset:   "BTC.BTC",
		ToAsset:     "ETH.ETH",
		Amount:      "100000000",
		Destination: "0x86d526d6624AbC0178cF7296cD538Ecc080A95F1",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "bc1qt9723ak9t7lu7a97lt9kelq4gnrlmyvk4yhzwr", resp.InboundAddress)
	assert.Equal(t, int64(1), resp.InboundConfirmationBlocks)
	assert.Equal(t, int64(600), resp.InboundConfirmationSeconds)
	assert.Equal(t, int64(576), resp.OutboundDelayBlocks)
	assert.Equal(t, int64(7200), resp.OutboundDelaySeconds)
	assert.Equal(t, "BTC.BTC", resp.Fees.Asset)
	assert.Equal(t, "0", resp.Fees.Affiliate)
	assert.Equal(t, "54840", resp.Fees.Outbound)
	assert.Equal(t, "2037232", resp.Fees.Liquidity)
	assert.Equal(t, "2092072", resp.Fees.Total)
	assert.Equal(t, 9, resp.Fees.SlippageBps)
	assert.Equal(t, 11, resp.Fees.TotalBps)
	assert.Equal(t, 50, resp.SlippageBps)
	assert.Equal(t, 0, resp.StreamingSlippageBps)
	assert.Equal(t, int64(1715800000), resp.Expiry)
	assert.Equal(t, "Do not send more than the recommended amount", resp.Warning)
	assert.Equal(t, "ETH.ETH output amount may vary due to gas deductions", resp.Notes)
	assert.Equal(t, "0.0001", resp.DustThreshold)
	assert.Equal(t, "1000000", resp.RecommendedMinAmountIn)
	assert.Equal(t, "7", resp.RecommendedGasRate)
	assert.Equal(t, "satsperbyte", resp.GasRateUnits)
	assert.Equal(t, "=:ETH.ETH:0x86d526d6624AbC0178cF7296cD538Ecc080A95F1:0/1/0", resp.Memo)
	assert.Equal(t, "2035299208", resp.ExpectedAmountOut)
	assert.Equal(t, "0", resp.ExpectedAmountOutStreaming)
	assert.Equal(t, 0, resp.MaxStreamingQuantity)
	assert.Equal(t, int64(0), resp.StreamingSwapBlocks)
	assert.Equal(t, int64(0), resp.StreamingSwapSeconds)
	assert.Equal(t, int64(7800), resp.TotalSwapSeconds)
}

func TestClient_Status_JSONDeserialization(t *testing.T) {
	// Captured 2026-05-15 from real THORChain API (completed SOL swap via new thornode.thorchain.network endpoint).
	rawJSON := `{
		"tx": {
			"id": "27208951434B8D7ACB416C6B0B5DAE6449A3A3D2C4484F0B36803A180F24B1E3",
			"chain": "THOR",
			"from_address": "thor17hwqt302e5f2xm4h95ma8wuggqkvfzgvsnh5z9",
			"to_address": "thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0",
			"coins": [
				{"asset": "THOR.RUNE", "amount": "9671547414"}
			],
			"gas": null,
			"memo": "=:SOL~SOL:thor17hwqt302e5f2xm4h95ma8wuggqkvfzgvsnh5z9:61140079/1/1"
		},
		"planned_out_txs": [
			{
				"chain": "THOR",
				"to_address": "thor17hwqt302e5f2xm4h95ma8wuggqkvfzgvsnh5z9",
				"coin": {"asset": "SOL~SOL", "amount": "61164077"},
				"refund": false
			}
		],
		"out_txs": [
			{
				"id": "0000000000000000000000000000000000000000000000000000000000000000",
				"chain": "THOR",
				"from_address": "thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0",
				"to_address": "thor17hwqt302e5f2xm4h95ma8wuggqkvfzgvsnh5z9",
				"coins": [{"asset": "SOL~SOL", "amount": "61164077"}],
				"gas": [{"asset": "THOR.RUNE", "amount": "2000000"}],
				"memo": "OUT:27208951434B8D7ACB416C6B0B5DAE6449A3A3D2C4484F0B36803A180F24B1E3"
			}
		],
		"stages": {
			"inbound_observed": {"final_count": 0, "completed": true},
			"inbound_confirmation_counted": {"remaining_confirmation_seconds": 0, "completed": true},
			"inbound_finalised": {"completed": true},
			"swap_status": {"pending": false},
			"swap_finalised": {"completed": true}
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL

	resp, err := c.Status(context.Background(), "27208951434B8D7ACB416C6B0B5DAE6449A3A3D2C4484F0B36803A180F24B1E3")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Tx details
	assert.Equal(t, "27208951434B8D7ACB416C6B0B5DAE6449A3A3D2C4484F0B36803A180F24B1E3", resp.Tx.ID)
	assert.Equal(t, "THOR", resp.Tx.Chain)
	assert.Equal(t, "thor17hwqt302e5f2xm4h95ma8wuggqkvfzgvsnh5z9", resp.Tx.FromAddress)
	assert.Equal(t, "thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0", resp.Tx.ToAddress)
	require.Len(t, resp.Tx.Coins, 1)
	assert.Equal(t, "THOR.RUNE", resp.Tx.Coins[0].Asset)
	assert.Equal(t, "9671547414", resp.Tx.Coins[0].Amount)
	assert.Equal(t, "=:SOL~SOL:thor17hwqt302e5f2xm4h95ma8wuggqkvfzgvsnh5z9:61140079/1/1", resp.Tx.Memo)

	// Planned out txs
	require.Len(t, resp.PlannedOutTxs, 1)
	assert.Equal(t, "THOR", resp.PlannedOutTxs[0].Chain)
	assert.Equal(t, "SOL~SOL", resp.PlannedOutTxs[0].Coin.Asset)
	assert.Equal(t, "61164077", resp.PlannedOutTxs[0].Coin.Amount)
	assert.False(t, resp.PlannedOutTxs[0].Refund)

	// Out txs
	require.Len(t, resp.OutTxs, 1)
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", resp.OutTxs[0].ID)
	require.Len(t, resp.OutTxs[0].Gas, 1)
	assert.Equal(t, "THOR.RUNE", resp.OutTxs[0].Gas[0].Asset)

	// Stages
	assert.True(t, resp.Stages.InboundObserved.Completed)
	assert.Equal(t, 0, resp.Stages.InboundObserved.FinalCount)
	assert.True(t, resp.Stages.InboundConfirmationCounted.Completed)
	assert.Equal(t, int64(0), resp.Stages.InboundConfirmationCounted.RemainingConfirmationSeconds)
	assert.True(t, resp.Stages.InboundFinalised.Completed)
	assert.False(t, resp.Stages.SwapStatus.Pending)
	assert.True(t, resp.Stages.SwapFinalised.Completed)
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
