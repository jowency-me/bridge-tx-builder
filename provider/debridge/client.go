// Package debridge provides a quote adapter for the deBridge DLN cross-chain protocol.
package debridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://dln.debridge.finance"

// Client is the raw HTTP client for deBridge DLN API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new deBridge API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// QuoteParams contains raw debridge quote request parameters.
type QuoteParams struct {
	SrcChainID                    string
	SrcChainTokenIn               string
	SrcChainTokenInAmount         string
	DstChainID                    string
	DstChainTokenOut              string
	SrcChainOrderAuthorityAddress string
	DstChainOrderAuthorityAddress string
	DstChainTokenOutRecipient     string
	DstChainTokenOutAmount        string
	Slippage                      string
}

// QuoteResponse contains raw debridge quote response data.
type QuoteResponse struct {
	OrderID            string    `json:"orderId"`
	EstimateToAmount   string    `json:"estimateToAmount"`
	EstimateFromAmount string    `json:"estimateFromAmount"`
	Tx                 TxInfo    `json:"tx"`
	TokenIn            TokenInfo `json:"tokenIn"`
	TokenOut           TokenInfo `json:"tokenOut"`
}

// TxInfo contains provider transaction metadata.
type TxInfo struct {
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	GasLimit string `json:"gasLimit"`
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
	ChainID  int    `json:"chainId"`
}

// StatusResponse contains raw debridge status response data.
type StatusResponse struct {
	Status  string `json:"status"`
	OrderID string `json:"orderId"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/v1.0/dln/order/create-tx")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("srcChainId", params.SrcChainID)
	q.Set("srcChainTokenIn", params.SrcChainTokenIn)
	q.Set("srcChainTokenInAmount", params.SrcChainTokenInAmount)
	q.Set("dstChainId", params.DstChainID)
	q.Set("dstChainTokenOut", params.DstChainTokenOut)
	q.Set("srcChainOrderAuthorityAddress", params.SrcChainOrderAuthorityAddress)
	q.Set("dstChainOrderAuthorityAddress", params.DstChainOrderAuthorityAddress)
	q.Set("dstChainTokenOutRecipient", params.DstChainTokenOutRecipient)
	q.Set("dstChainTokenOutAmount", params.DstChainTokenOutAmount)
	q.Set("slippage", params.Slippage)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("X-DeBridge-API-Key", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("debridge quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("debridge quote decode: %w", err)
	}
	return &qr, nil
}

// Status fetches transaction status from the provider API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	u, err := url.Parse(c.baseURL + "/v1.0/dln/order/status")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("id", txID)
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
		return nil, fmt.Errorf("debridge status failed: status %d", resp.StatusCode)
	}

	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("debridge status decode: %w", err)
	}
	return &sr, nil
}
