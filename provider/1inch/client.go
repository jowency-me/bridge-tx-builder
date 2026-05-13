// Package oneinch provides a quote adapter for the 1inch Aggregation API.
package oneinch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.1inch.dev"

// Client is the raw HTTP client for 1inch Swap API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new 1inch API client.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		apiKey:  apiKey,
	}
}

// QuoteParams contains raw 1inch quote request parameters.
type QuoteParams struct {
	ChainID          string
	Src              string
	Dst              string
	Amount           string
	From             string
	Slippage         string
	DisableEstimate  string
	AllowPartialFill string
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Decimals int    `json:"decimals"`
}

// TxData contains provider transaction payload data.
type TxData struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	Gas      uint64 `json:"gas"`
	GasPrice string `json:"gasPrice"`
}

// QuoteResponse contains raw 1inch quote response data.
type QuoteResponse struct {
	DstAmount string      `json:"dstAmount"`
	SrcAmount string      `json:"srcAmount"`
	FromToken TokenInfo   `json:"fromToken"`
	ToToken   TokenInfo   `json:"toToken"`
	Protocols interface{} `json:"protocols"`
	Tx        TxData      `json:"tx"`
	Gas       uint64      `json:"gas"`
}

// StatusResponse contains raw 1inch status response data.
type StatusResponse struct {
	Status string `json:"status"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s/swap/v6.1/%s/swap", c.baseURL, params.ChainID))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("src", params.Src)
	q.Set("dst", params.Dst)
	q.Set("amount", params.Amount)
	if params.From != "" {
		q.Set("from", params.From)
	}
	if params.Slippage != "" {
		q.Set("slippage", params.Slippage)
	}
	if params.DisableEstimate != "" {
		q.Set("disableEstimate", params.DisableEstimate)
	}
	if params.AllowPartialFill != "" {
		q.Set("allowPartialFill", params.AllowPartialFill)
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("1inch quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("1inch quote decode: %w", err)
	}
	return &qr, nil
}

// Status checks server reachability via a lightweight HEAD request.
// 1inch does not provide a dedicated transaction status API.
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
		return nil, fmt.Errorf("1inch server error: status %d", resp.StatusCode)
	}
	return &StatusResponse{Status: "reachable"}, nil
}
