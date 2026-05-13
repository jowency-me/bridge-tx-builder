// Package celer provides a quote adapter for the Celer cBridge cross-chain bridge.
package celer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	SrcChainID  string
	DstChainID  string
	TokenSymbol string
	Amt         string
	UsrAddr     string
}

// QuoteResponse contains raw celer quote response data.
type QuoteResponse struct {
	Err               interface{} `json:"err"`
	Value             string      `json:"value"`
	PercFee           string      `json:"percFee"`
	BaseFee           string      `json:"baseFee"`
	SlippageTolerance int         `json:"slippageTolerance"`
}

// StatusResponse contains raw celer status response data.
type StatusResponse struct {
	Status string `json:"status"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	body := map[string]interface{}{
		"src_chain_id": params.SrcChainID,
		"dst_chain_id": params.DstChainID,
		"token_symbol": params.TokenSymbol,
		"amt":          params.Amt,
		"usr_addr":     params.UsrAddr,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	hReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/estimateAmt", bytes.NewReader(bodyBytes))
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
