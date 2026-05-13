// Package openocean provides a quote adapter for the OpenOcean DEX aggregation API.
package openocean

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://open-api.openocean.finance/v3"

// Client is the raw HTTP client for OpenOcean API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new OpenOcean API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// QuoteParams contains raw openocean quote request parameters.
type QuoteParams struct {
	ChainCode       string
	InTokenAddress  string
	OutTokenAddress string
	Amount          string
	GasPrice        string
	Slippage        string
	Account         string
	Referrer        string
}

// TokenDetail contains provider token metadata.
type TokenDetail struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
}

// QuoteData contains provider-specific API data.
type QuoteData struct {
	To           string      `json:"to"`
	Data         string      `json:"data"`
	Value        string      `json:"value"`
	OutAmount    string      `json:"outAmount"`
	EstimatedGas uint64      `json:"estimatedGas"`
	InToken      TokenDetail `json:"inToken"`
	OutToken     TokenDetail `json:"outToken"`
	InAmount     string      `json:"inAmount"`
}

// QuoteResponse contains raw openocean quote response data.
type QuoteResponse struct {
	Code     int        `json:"code"`
	ErrorMsg string     `json:"errorMsg"`
	Data     *QuoteData `json:"data"`
}

// Quote fetches a quote from the provider /{chain}/swap_quote API endpoint.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s/%s/swap_quote", c.baseURL, params.ChainCode))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("inTokenAddress", params.InTokenAddress)
	q.Set("outTokenAddress", params.OutTokenAddress)
	q.Set("amount", params.Amount)
	q.Set("gasPrice", params.GasPrice)
	q.Set("slippage", params.Slippage)
	q.Set("account", params.Account)
	if params.Referrer != "" {
		q.Set("referrer", params.Referrer)
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("x-api-key", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openocean quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("openocean quote decode: %w", err)
	}
	if qr.Code != 200 {
		return nil, fmt.Errorf("openocean quote error: %s", qr.ErrorMsg)
	}
	return &qr, nil
}

// Status checks server reachability via a lightweight HEAD request.
// OpenOcean exposes /getTransaction but requires a chain code; the domain Status
// interface only carries txID, so fall back to a reachability probe.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.baseURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, fmt.Errorf("openocean server error: status %d", resp.StatusCode)
	}
	return &StatusResponse{}, nil
}

// StatusResponse is the raw JSON response for OpenOcean status.
type StatusResponse struct {
	TxHash string `json:"txHash"`
	Status string `json:"status"`
}
