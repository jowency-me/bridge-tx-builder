package celer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Quote(t *testing.T) {
	c := NewClient()
	params := QuoteParams{
		SrcChainID:  "1",
		DstChainID:  "8453",
		TokenSymbol: "USDC",
		Amt:         "1000000",
		UsrAddr:     "0x1234567890123456789012345678901234567890",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	if err != nil {
		t.Skipf("real API unavailable: %v", err)
	}
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.EqValueTokenAmt)
}

func TestClient_Status(t *testing.T) {
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := c.Status(ctx, "0x1234567890123456789012345678901234567890123456789012345678901234")
	// Celer does not support status API.
	require.Error(t, err)
}

func TestClient_Quote_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		q := r.URL.Query()
		require.Equal(t, "1", q.Get("src_chain_id"))
		require.Equal(t, "8453", q.Get("dst_chain_id"))
		require.Equal(t, "USDC", q.Get("token_symbol"))
		require.Equal(t, "1000000", q.Get("amt"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"eq_value_token_amt":"999000","perc_fee":"500","base_fee":"100","slippage_tolerance":50}`))
	}))
	defer ts.Close()

	c := NewClientWithBaseURL(ts.URL)
	params := QuoteParams{
		SrcChainID:  "1",
		DstChainID:  "8453",
		TokenSymbol: "USDC",
		Amt:         "1000000",
		UsrAddr:     "0x1234567890123456789012345678901234567890",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "999000", resp.EqValueTokenAmt)
	require.Equal(t, "500", resp.PercFee)
	require.Equal(t, 50, resp.SlippageTolerance)
}

func TestClient_Quote_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer ts.Close()

	c := NewClientWithBaseURL(ts.URL)
	params := QuoteParams{
		SrcChainID:  "1",
		DstChainID:  "8453",
		TokenSymbol: "USDC",
		Amt:         "1000000",
		UsrAddr:     "0x1234567890123456789012345678901234567890",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Quote(ctx, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 500")
}

func TestClient_Quote_DecodeError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer ts.Close()

	c := NewClientWithBaseURL(ts.URL)
	params := QuoteParams{
		SrcChainID:  "1",
		DstChainID:  "8453",
		TokenSymbol: "USDC",
		Amt:         "1000000",
		UsrAddr:     "0x1234567890123456789012345678901234567890",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Quote(ctx, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode")
}

func TestClient_Quote_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"err":"insufficient liquidity"}`))
	}))
	defer ts.Close()

	c := NewClientWithBaseURL(ts.URL)
	params := QuoteParams{
		SrcChainID:  "1",
		DstChainID:  "8453",
		TokenSymbol: "USDC",
		Amt:         "1000000",
		UsrAddr:     "0x1234567890123456789012345678901234567890",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Quote(ctx, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "celer quote error")
}

func TestClient_NewClient_DefaultBaseURL(t *testing.T) {
	c := NewClient()
	require.Equal(t, defaultBaseURL, c.baseURL)
	require.NotNil(t, c.client)
}

func TestClient_NewClientWithBaseURL(t *testing.T) {
	c := NewClientWithBaseURL("https://custom.example.com")
	require.Equal(t, "https://custom.example.com", c.baseURL)
	require.NotNil(t, c.client)
}

func TestClient_Quote_WithContextCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"value":"999000"}`))
	}))
	defer ts.Close()

	c := NewClientWithBaseURL(ts.URL)
	params := QuoteParams{
		SrcChainID:  "1",
		DstChainID:  "8453",
		TokenSymbol: "USDC",
		Amt:         "1000000",
		UsrAddr:     "0x1234567890123456789012345678901234567890",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := c.Quote(ctx, params)
	require.Error(t, err)
}

func TestClient_Quote_JSONDeserialization(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"err": null,
			"eq_value_token_amt": "995000",
			"bridge_rate": 0.995,
			"perc_fee": "500",
			"base_fee": "0",
			"slippage_tolerance": 5000,
			"max_slippage": 5000,
			"estimated_receive_amt": "990000",
			"drop_gas_amt": "0",
			"op_fee_rebate": 0,
			"op_fee_rebate_portion": 0,
			"op_fee_rebate_end_time": "0"
		}`))
	}))
	defer ts.Close()

	c := NewClientWithBaseURL(ts.URL)
	params := QuoteParams{
		SrcChainID:  "1",
		DstChainID:  "8453",
		TokenSymbol: "USDC",
		Amt:         "1000000",
		UsrAddr:     "0x1234567890123456789012345678901234567890",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Nil(t, resp.Err)
	require.Equal(t, "995000", resp.EqValueTokenAmt)
	require.InDelta(t, 0.995, resp.BridgeRate, 0.0001)
	require.Equal(t, "500", resp.PercFee)
	require.Equal(t, "0", resp.BaseFee)
	require.Equal(t, 5000, resp.SlippageTolerance)
	require.Equal(t, 5000, resp.MaxSlippage)
	require.Equal(t, "990000", resp.EstimatedReceiveAmt)
	require.Equal(t, "0", resp.DropGasAmt)
	require.InDelta(t, 0, resp.OpFeeRebate, 0.0001)
	require.InDelta(t, 0, resp.OpFeeRebatePortion, 0.0001)
	require.Equal(t, "0", resp.OpFeeRebateEndTime)
}

func TestClient_Quote_RequestConstruction(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify all headers and query params
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		q := r.URL.Query()
		require.Equal(t, "42161", q.Get("src_chain_id"))
		require.Equal(t, "10", q.Get("dst_chain_id"))
		require.Equal(t, "USDT", q.Get("token_symbol"))
		require.Equal(t, "5000000", q.Get("amt"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"eq_value_token_amt":"500000","perc_fee":"100","base_fee":"50","slippage_tolerance":30}`))
	}))
	defer ts.Close()

	c := NewClientWithBaseURL(ts.URL)
	params := QuoteParams{
		SrcChainID:  "42161",
		DstChainID:  "10",
		TokenSymbol: "USDT",
		Amt:         "5000000",
		UsrAddr:     "0xABCDEF123456789ABCDEF123456789ABCDEF1234",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Quote(ctx, params)
	require.NoError(t, err)
	require.Equal(t, "500000", resp.EqValueTokenAmt)
	require.Equal(t, "100", resp.PercFee)
	require.Equal(t, 30, resp.SlippageTolerance)
}
