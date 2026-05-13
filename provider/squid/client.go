// Package squid provides a quote adapter for the Squid Router cross-chain routing API.
package squid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://v2.api.squidrouter.com/v2"

// Client is the raw HTTP client for Squid Router API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new Squid Router API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// QuoteParams contains raw squid quote request parameters.
type QuoteParams struct {
	FromChain   string
	ToChain     string
	FromToken   string
	ToToken     string
	FromAmount  string
	FromAddress string
	ToAddress   string
	Slippage    string
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
	ChainID  int    `json:"chainId"`
}

// TransactionRequest contains provider transaction request data.
type TransactionRequest struct {
	TargetAddress string `json:"targetAddress"`
	Data          string `json:"data"`
	Value         string `json:"value"`
	GasLimit      string `json:"gasLimit"`
}

// GasCost contains a provider gas cost entry.
type GasCost struct {
	Estimate string `json:"estimate"`
}

// FeeCost contains a provider fee cost entry.
type FeeCost struct {
	Amount string `json:"amount"`
}

// Estimate contains provider amount and gas estimates.
type Estimate struct {
	FromAmount             string    `json:"fromAmount"`
	ToAmount               string    `json:"toAmount"`
	ToAmountMin            string    `json:"toAmountMin"`
	GasCosts               []GasCost `json:"gasCosts"`
	FeeCosts               []FeeCost `json:"feeCosts"`
	EstimatedRouteDuration int       `json:"estimatedRouteDuration"`
}

// RouteData contains provider route metadata.
type RouteData struct {
	Estimate           Estimate           `json:"estimate"`
	TransactionRequest TransactionRequest `json:"transactionRequest"`
	Params             struct {
		FromToken TokenInfo `json:"fromToken"`
		ToToken   TokenInfo `json:"toToken"`
	} `json:"params"`
}

// QuoteResponse contains raw squid quote response data.
type QuoteResponse struct {
	Route     RouteData `json:"route"`
	RequestID string    `json:"requestId"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
}

// ChainTxInfo contains provider-specific API data.
type ChainTxInfo struct {
	TransactionID   string `json:"transactionId"`
	TransactionURL  string `json:"transactionUrl"`
	CallEventStatus string `json:"callEventStatus"`
}

// StatusResponse contains raw squid status response data.
type StatusResponse struct {
	ID        string       `json:"id"`
	Status    string       `json:"status"`
	FromChain *ChainTxInfo `json:"fromChain"`
	ToChain   *ChainTxInfo `json:"toChain"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/route")
	if err != nil {
		return nil, err
	}
	body := map[string]string{
		"fromChain":   params.FromChain,
		"toChain":     params.ToChain,
		"fromToken":   params.FromToken,
		"toToken":     params.ToToken,
		"fromAmount":  params.FromAmount,
		"fromAddress": params.FromAddress,
		"toAddress":   params.ToAddress,
		"slippage":    params.Slippage,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	hReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	hReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		hReq.Header.Set("x-integrator-id", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("squid quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("squid quote decode: %w", err)
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
	q.Set("transactionId", txID)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("x-integrator-id", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("squid status failed: status %d", resp.StatusCode)
	}

	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("squid status decode: %w", err)
	}
	return &sr, nil
}
