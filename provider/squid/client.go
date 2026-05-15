// Package squid provides a quote adapter for the Squid Router cross-chain routing API.
//
// API Reference:
//
//	Quote: https://docs.squidrouter.com
//	Status: https://docs.squidrouter.com
//	API version: v2 (verified 2026-05-15)
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
	Slippage    int
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
	ChainID  string `json:"chainId"`
	Name     string `json:"name"`
}

// TransactionRequest contains provider transaction request data.
type TransactionRequest struct {
	Type     string `json:"type"`
	Target   string `json:"target"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	GasLimit string `json:"gasLimit"`
}

// GasCost contains a provider gas cost entry.
type GasCost struct {
	Type      string    `json:"type"`
	Amount    string    `json:"amount"`
	GasLimit  string    `json:"gasLimit"`
	Token     TokenInfo `json:"token"`
	AmountUSD string    `json:"amountUsd"`
}

// FeeCost contains a provider fee cost entry.
type FeeCost struct {
	Amount    string    `json:"amount"`
	AmountUSD string    `json:"amountUsd"`
	Name      string    `json:"name"`
	Token     TokenInfo `json:"token"`
	LogoURI   string    `json:"logoURI"`
}

// Estimate contains provider amount and gas estimates.
type Estimate struct {
	FromAmount             string    `json:"fromAmount"`
	ToAmount               string    `json:"toAmount"`
	ToAmountMin            string    `json:"toAmountMin"`
	FromToken              TokenInfo `json:"fromToken"`
	ToToken                TokenInfo `json:"toToken"`
	GasCosts               []GasCost `json:"gasCosts"`
	FeeCosts               []FeeCost `json:"feeCosts"`
	EstimatedRouteDuration int       `json:"estimatedRouteDuration"`
}

// RouteParams contains the route parameters.
type RouteParams struct {
	FromChain   string      `json:"fromChain"`
	ToChain     string      `json:"toChain"`
	FromToken   string      `json:"fromToken"`
	ToToken     string      `json:"toToken"`
	FromAmount  string      `json:"fromAmount"`
	FromAddress string      `json:"fromAddress"`
	ToAddress   string      `json:"toAddress"`
	Slippage    json.Number `json:"slippage"`
}

// RouteData contains provider route metadata.
type RouteData struct {
	QuoteID            string             `json:"quoteId"`
	Estimate           Estimate           `json:"estimate"`
	TransactionRequest TransactionRequest `json:"transactionRequest"`
	Params             RouteParams        `json:"params"`
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
	ID                     string       `json:"id"`
	SquidTransactionStatus string       `json:"squidTransactionStatus"`
	FromChain              *ChainTxInfo `json:"fromChain"`
	ToChain                *ChainTxInfo `json:"toChain"`
}

// StatusParams contains raw squid status request parameters.
type StatusParams struct {
	FromChainID string
	ToChainID   string
	QuoteID     string
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/route")
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
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

	// The Squid API returns an x-request-id header in the response which is needed
	// for /status tracking. Prefer the header value over the JSON body requestId.
	if hdr := resp.Header.Get("x-request-id"); hdr != "" {
		qr.RequestID = hdr
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
	q.Set("transactionId", txID)
	if len(statusParams) > 0 {
		sp := statusParams[0]
		if sp.FromChainID != "" {
			q.Set("fromChainId", sp.FromChainID)
		}
		if sp.ToChainID != "" {
			q.Set("toChainId", sp.ToChainID)
		}
		if sp.QuoteID != "" {
			q.Set("quoteId", sp.QuoteID)
		}
	}
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
