// Package symbiosis provides a quote adapter for the Symbiosis cross-chain bridge.
//
// API Reference:
//
//	Quote+Tx: https://docs.symbiosis.finance/
//	POST /crosschain/v1/swap
//	Status: not supported in v1.
//	API version: v1 (verified against live API 2026-06-11).
package symbiosis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.symbiosis.finance/crosschain"

// Client is the raw HTTP client for Symbiosis.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient creates a new Symbiosis API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// TokenAmount is the Symbiosis in/out token structure.
type TokenAmount struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	ChainID  int    `json:"chainId"`
	Decimals int    `json:"decimals"`
	Amount   string `json:"amount"`
}

// QuoteRequest is the body for POST /v1/swap.
type QuoteRequest struct {
	TokenAmountIn TokenAmount `json:"tokenAmountIn"`
	TokenOut      TokenAmount `json:"tokenOut"`
	From          string      `json:"from"`
	To            string      `json:"to"`
	Slippage      int         `json:"slippage"` // bps; 50 = 0.5%, 500 = 5%
}

// Tx is the Symbiosis on-chain transaction envelope.
type Tx struct {
	ChainID int    `json:"chainId"`
	To      string `json:"to"`
	Data    string `json:"data"`
	Value   string `json:"value"`
}

// Fee is the Symbiosis fee breakdown.
type Fee struct {
	Amount   string `json:"amount"`
	Decimals int    `json:"decimals"`
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	ChainID  int    `json:"chainId"`
}

// RouteStep is one hop in the Symbiosis route.
type RouteStep struct {
	Token SymbolToken `json:"token"`
	From  SymbolToken `json:"from"`
	To    SymbolToken `json:"to"`
}

// SymbolToken pairs a token with its chain.
type SymbolToken struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	ChainID  int    `json:"chainId"`
	Decimals int    `json:"decimals"`
}

// QuoteResponse mirrors the POST /v1/swap response.
//
// Verified against the live API 2026-06-11: the response carries tokenAmountOut,
// tokenAmountOutMin, tx{chainId,to,data,value}, approveTo, fee, priceImpact (string),
// type, estimatedTime, and a routes[] array. There is no top-level id or
// expiryTimestamp field. amountInUsd is an object (not a string), so it is not
// modeled here — adding it as a string would break JSON decoding.
type QuoteResponse struct {
	TokenAmountOut    TokenAmount `json:"tokenAmountOut"`
	TokenAmountOutMin TokenAmount `json:"tokenAmountOutMin"`
	Tx                Tx          `json:"tx"`
	ApproveTo         string      `json:"approveTo"`
	Fee               Fee         `json:"fee"`
	Route             []RouteStep `json:"route"`
	PriceImpact       string      `json:"priceImpact"`
	Type              string      `json:"type"`
	EstimatedTime     int         `json:"estimatedTime"`
}

// Quote calls POST /v1/swap.
func (c *Client) Quote(ctx context.Context, req QuoteRequest) (*QuoteResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("symbiosis: marshal: %w", err)
	}
	u := c.baseURL + "/v1/swap"
	hReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, fmt.Errorf("symbiosis: build request: %w", err)
	}
	hReq.Header.Set("Content-Type", "application/json")
	hReq.Body = httpBytesBody(body)
	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, fmt.Errorf("symbiosis: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("symbiosis quote failed: status %d", resp.StatusCode)
	}
	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("symbiosis quote decode: %w", err)
	}
	return &qr, nil
}

func httpBytesBody(b []byte) io.ReadCloser { return io.NopCloser(bytes.NewReader(b)) }
