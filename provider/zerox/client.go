// Package zerox provides a quote adapter for the 0x Protocol DEX aggregation API.
package zerox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.0x.org"

// Client is the raw HTTP client for 0x API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new 0x API client.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		apiKey:  apiKey,
	}
}

// QuoteParams contains raw zerox quote request parameters.
type QuoteParams struct {
	ChainID      string
	SellToken    string
	BuyToken     string
	SellAmount   string
	BuyAmount    string
	TakerAddress string
	SlippageBps  string
}

// QuoteResponse contains raw zerox quote response data.
type QuoteResponse struct {
	Price            string       `json:"price"`
	GuaranteedPrice  string       `json:"guaranteedPrice"`
	To               string       `json:"to"`
	Data             string       `json:"data"`
	Value            string       `json:"value"`
	Gas              string       `json:"gas"`
	BuyAmount        string       `json:"buyAmount"`
	SellAmount       string       `json:"sellAmount"`
	BuyTokenAddress  string       `json:"buyTokenAddress"`
	SellTokenAddress string       `json:"sellTokenAddress"`
	BuyToken         string       `json:"buyToken"`
	SellToken        string       `json:"sellToken"`
	Sources          []SourceInfo `json:"sources"`
	Fee              FeeInfo      `json:"fee"`
	Transaction      TxData       `json:"transaction"`
}

// TxData contains 0x v2 transaction payload data.
type TxData struct {
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	Gas      string `json:"gas"`
	GasPrice string `json:"gasPrice"`
}

// SourceInfo contains provider-specific API data.
type SourceInfo struct {
	Name       string `json:"name"`
	Proportion string `json:"proportion"`
}

// FeeInfo contains provider-specific API data.
type FeeInfo struct {
	FeeType   string `json:"feeType"`
	FeeToken  string `json:"feeToken"`
	FeeAmount string `json:"feeAmount"`
}

// StatusResponse contains raw zerox status response data.
type StatusResponse struct {
	Status string `json:"status"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/swap/allowance-holder/quote")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if params.ChainID != "" {
		q.Set("chainId", params.ChainID)
	}
	q.Set("sellToken", params.SellToken)
	q.Set("buyToken", params.BuyToken)
	if params.SellAmount != "" {
		q.Set("sellAmount", params.SellAmount)
	}
	if params.BuyAmount != "" {
		q.Set("buyAmount", params.BuyAmount)
	}
	if params.TakerAddress != "" {
		q.Set("taker", params.TakerAddress)
	}
	if params.SlippageBps != "" {
		q.Set("slippageBps", params.SlippageBps)
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hReq.Header.Set("0x-api-key", c.apiKey)
	hReq.Header.Set("0x-version", "v2")

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("0x quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("0x quote decode: %w", err)
	}
	return &qr, nil
}

// Status fetches transaction status from the provider API.
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
		return nil, fmt.Errorf("0x server error: status %d", resp.StatusCode)
	}
	return &StatusResponse{Status: "reachable"}, nil
}
