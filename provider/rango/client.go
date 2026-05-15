// Package rango provides a quote adapter for the Rango cross-chain routing API.
//
// API Reference:
//
//	Quote: https://docs.rango.exchange/api-integration/basic-api-single-step/api-reference/get-quote
//	Status: https://docs.rango.exchange/api-integration/basic-api-single-step/api-reference/check-transaction-status
//	API version: Basic API (verified 2026-05-15)
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
	RequestID  string  `json:"requestId"`
	ResultType string  `json:"resultType"`
	Error      string  `json:"error"`
	ErrorCode  *string `json:"errorCode"`
	TraceID    *string `json:"traceId"`
	Route      Route   `json:"route"`
}

// Route contains the route details for a quote response.
// Maps to QuoteSimulationResult in the Rango API.
type Route struct {
	From                   TokenInfo   `json:"from"`
	To                     TokenInfo   `json:"to"`
	OutputAmount           string      `json:"outputAmount"`
	OutputAmountMin        string      `json:"outputAmountMin"`
	OutputAmountUsd        *float64    `json:"outputAmountUsd"`
	Swapper                SwapperMeta `json:"swapper"`
	Path                   []QuotePath `json:"path"`
	Fee                    []SwapFee   `json:"fee"`
	FeeUsd                 *float64    `json:"feeUsd"`
	EstimatedTimeInSeconds int         `json:"estimatedTimeInSeconds"`
}

// QuotePath represents a single leg in a multi-hop route.
type QuotePath struct {
	From                   TokenInfo   `json:"from"`
	To                     TokenInfo   `json:"to"`
	Swapper                SwapperMeta `json:"swapper"`
	SwapperType            string      `json:"swapperType"`
	InputAmount            string      `json:"inputAmount"`
	ExpectedOutput         string      `json:"expectedOutput"`
	EstimatedTimeInSeconds int         `json:"estimatedTimeInSeconds"`
}

// SwapperMeta contains swapper identification metadata.
type SwapperMeta struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Logo         string   `json:"logo"`
	SwapperGroup string   `json:"swapperGroup"`
	Types        []string `json:"types"`
	Enabled      bool     `json:"enabled"`
}

// SwapFee contains a single fee entry from the route.
type SwapFee struct {
	Token       TokenInfo       `json:"token"`
	ExpenseType string          `json:"expenseType"`
	Amount      string          `json:"amount"`
	Name        string          `json:"name"`
	Meta        json.RawMessage `json:"meta"`
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Symbol     string `json:"symbol"`
	Address    string `json:"address"`
	Blockchain string `json:"blockchain"`
	Decimals   int    `json:"decimals"`
}

// StatusParams contains optional parameters for the status endpoint.
type StatusParams struct {
	TxID string
}

// StatusResponse contains raw rango status response data.
type StatusResponse struct {
	Status       string            `json:"status"`
	Error        string            `json:"error"`
	Output       *StatusOutput     `json:"output"`
	ExplorerURL  []SwapExplorerURL `json:"explorerUrl"`
	DiagnosisURL string            `json:"diagnosisUrl"`
	BridgeData   *BridgeData       `json:"bridgeData"`
}

// StatusOutput contains output amount and type information.
type StatusOutput struct {
	Type          string    `json:"type"`
	Amount        string    `json:"amount"`
	ReceivedToken TokenInfo `json:"receivedToken"`
}

// SwapExplorerURL contains a single explorer URL entry.
type SwapExplorerURL struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// BridgeData contains detailed bridge transaction data.
type BridgeData struct {
	SrcTxHash  string    `json:"srcTxHash"`
	DestTxHash string    `json:"destTxHash"`
	SrcToken   TokenInfo `json:"srcToken"`
	DestToken  TokenInfo `json:"destToken"`
	SrcAmount  string    `json:"srcAmount"`
	DestAmount string    `json:"destAmount"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/quote")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("from", params.From+"."+params.FromToken)
	q.Set("to", params.To+"--"+params.ToToken)
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
func (c *Client) Status(ctx context.Context, txID string, statusParams ...StatusParams) (*StatusResponse, error) {
	u, err := url.Parse(c.baseURL + "/status")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("requestId", txID)
	if len(statusParams) > 0 && statusParams[0].TxID != "" {
		q.Set("txId", statusParams[0].TxID)
	}
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
