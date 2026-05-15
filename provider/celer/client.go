// Package celer provides a quote adapter for the Celer cBridge cross-chain bridge.
//
// API Reference:
//
//	Quote: https://cbridge-docs.celer.network/developer/api-and-sdk/api-docs#estimategasfee
//	API version: v2 (verified 2026-05-15)
package celer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://cbridge-prod2.celer.app/v2"

// Client is the raw HTTP client for Celer cBridge API.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient creates a new Celer API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientWithBaseURL creates a new Celer API client with a custom base URL.
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// QuoteParams contains raw celer quote request parameters.
type QuoteParams struct {
	SrcChainID        string
	DstChainID        string
	TokenSymbol       string
	Amt               string
	UsrAddr           string
	SlippageTolerance int
}

// QuoteResponse contains raw celer quote response data.
type QuoteResponse struct {
	Err                 interface{} `json:"err"`
	EqValueTokenAmt     string      `json:"eq_value_token_amt"`
	BridgeRate          float64     `json:"bridge_rate"`
	PercFee             string      `json:"perc_fee"`
	BaseFee             string      `json:"base_fee"`
	SlippageTolerance   int         `json:"slippage_tolerance"`
	MaxSlippage         int         `json:"max_slippage"`
	EstimatedReceiveAmt string      `json:"estimated_receive_amt"`
	DropGasAmt          string      `json:"drop_gas_amt"`
	OpFeeRebate         float64     `json:"op_fee_rebate"`
	OpFeeRebatePortion  float64     `json:"op_fee_rebate_portion"`
	OpFeeRebateEndTime  string      `json:"op_fee_rebate_end_time"`
}

// StatusResponse contains raw celer status response data.
type StatusResponse struct {
	Status string `json:"status"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/estimateAmt")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("src_chain_id", params.SrcChainID)
	q.Set("dst_chain_id", params.DstChainID)
	q.Set("token_symbol", params.TokenSymbol)
	q.Set("amt", params.Amt)
	q.Set("usr_addr", params.UsrAddr)
	if params.SlippageTolerance > 0 {
		q.Set("slippage_tolerance", strconv.Itoa(params.SlippageTolerance))
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("celer quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("celer quote decode: %w", err)
	}
	if qr.Err != nil {
		return nil, fmt.Errorf("celer quote error: %v", qr.Err)
	}
	return &qr, nil
}

// Status fetches transaction status from the provider API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	return nil, fmt.Errorf("celer status not supported")
}
