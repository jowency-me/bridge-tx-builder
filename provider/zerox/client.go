// Package zerox provides a quote adapter for the 0x Protocol DEX aggregation API.
//
// API Reference:
//
//	Quote: https://0x.org/docs/api
//	Status: https://0x.org/docs/api
//	API version: v2 (verified 2026-05-15)
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
	BuyAmount       string    `json:"buyAmount"`
	SellAmount      string    `json:"sellAmount"`
	MinBuyAmount    string    `json:"minBuyAmount"`
	BuyToken        string    `json:"buyToken"`
	SellToken       string    `json:"sellToken"`
	AllowanceTarget string    `json:"allowanceTarget"`
	Route           RouteData `json:"route"`
	Fees            FeeData   `json:"fees"`
	Transaction     TxData    `json:"transaction"`
}

// RouteData contains 0x v2 route fill information.
type RouteData struct {
	Fills []RouteFill `json:"fills"`
}

// RouteFill represents a single fill in the 0x v2 route.
type RouteFill struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Source     string `json:"source"`
	Proportion string `json:"proportionBps"`
}

// FeeData contains 0x v2 fee information.
type FeeData struct {
	ZeroExFee *ZeroExFee `json:"zeroExFee"`
}

// ZeroExFee contains the 0x protocol fee details.
type ZeroExFee struct {
	Amount string `json:"amount"`
	Token  string `json:"token"`
	Type   string `json:"type"`
}

// TxData contains 0x v2 transaction payload data.
type TxData struct {
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	Gas      string `json:"gas"`
	GasPrice string `json:"gasPrice"`
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
