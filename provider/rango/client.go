// Package rango provides a quote adapter for the Rango cross-chain routing API.
package rango

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.rango.exchange/basic"

// Client is the raw HTTP client for Rango API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new Rango API client.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		apiKey:  apiKey,
	}
}

// QuoteParams contains raw rango quote request parameters.
type QuoteParams struct {
	From        string
	To          string
	FromToken   string
	ToToken     string
	Amount      string
	FromAddress string
	ToAddress   string
	Slippage    string
}

// QuoteResponse contains raw rango quote response data.
type QuoteResponse struct {
	RequestID    string     `json:"requestId"`
	OutputAmount string     `json:"outputAmount"`
	ResultType   string     `json:"resultType"`
	Error        string     `json:"error"`
	Swaps        []SwapInfo `json:"swaps"`
}

// SwapInfo contains provider swap leg metadata.
type SwapInfo struct {
	From      TokenInfo `json:"from"`
	To        TokenInfo `json:"to"`
	SwapperID string    `json:"swapperId"`
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Symbol     string `json:"symbol"`
	Address    string `json:"address"`
	Blockchain string `json:"blockchain"`
	Decimals   int    `json:"decimals"`
}

// StatusResponse contains raw rango status response data.
type StatusResponse struct {
	Status    string `json:"status"`
	RequestID string `json:"requestId"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/quote")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("from", params.From)
	q.Set("to", params.To)
	q.Set("fromToken", params.FromToken)
	q.Set("toToken", params.ToToken)
	q.Set("amount", params.Amount)
	q.Set("fromAddress", params.FromAddress)
	q.Set("toAddress", params.ToAddress)
	q.Set("slippage", params.Slippage)
	q.Set("apiKey", c.apiKey)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rango quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("rango quote decode: %w", err)
	}
	if qr.ResultType != "OK" {
		return nil, fmt.Errorf("rango quote error: %s", qr.Error)
	}
	return &qr, nil
}

// Status fetches transaction status from the provider API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	u, err := url.Parse(c.baseURL + "/status")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("requestId", txID)
	q.Set("apiKey", c.apiKey)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rango status failed: status %d", resp.StatusCode)
	}

	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("rango status decode: %w", err)
	}
	return &sr, nil
}
