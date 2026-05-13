// Package lifi provides a quote adapter for the LI.FI cross-chain aggregation API.
package lifi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://li.quest"

// Client is the raw HTTP client for LI.FI API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new LI.FI API client.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		apiKey:  apiKey,
	}
}

// QuoteParams contains raw lifi quote request parameters.
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

// QuoteResponse contains raw lifi quote response data.
type QuoteResponse struct {
	ID                 string    `json:"id"`
	FromAmount         string    `json:"fromAmount"`
	ToAmount           string    `json:"toAmount"`
	Estimate           Estimate  `json:"estimate"`
	Action             Action    `json:"action"`
	IncludedSteps      []Step    `json:"includedSteps"`
	TransactionRequest TxRequest `json:"transactionRequest"`
}

// Estimate contains provider amount and gas estimates.
type Estimate struct {
	ToAmountMin string    `json:"toAmountMin"`
	ToAmount    string    `json:"toAmount"`
	FromAmount  string    `json:"fromAmount"`
	GasCosts    []GasCost `json:"gasCosts"`
}

// GasCost contains a provider gas cost entry.
type GasCost struct {
	Estimate string `json:"estimate"`
}

// Action contains provider route action metadata.
type Action struct {
	FromToken   TokenInfo `json:"fromToken"`
	ToToken     TokenInfo `json:"toToken"`
	FromAmount  string    `json:"fromAmount"`
	FromChainID int       `json:"fromChainId"`
	ToChainID   int       `json:"toChainId"`
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
}

// Step contains a provider route step.
type Step struct {
	Type string `json:"type"`
	Tool string `json:"tool"`
}

// TxRequest contains provider transaction request data.
type TxRequest struct {
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	GasLimit string `json:"gasLimit"`
}

// StatusResponse contains raw lifi status response data.
type StatusResponse struct {
	Sending   TxInfo `json:"sending"`
	Receiving TxInfo `json:"receiving"`
	Status    string `json:"status"`
}

// TxInfo contains provider transaction metadata.
type TxInfo struct {
	TxHash  string `json:"txHash"`
	ChainID int    `json:"chainId"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/v1/quote")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("fromChain", params.FromChain)
	q.Set("toChain", params.ToChain)
	q.Set("fromToken", params.FromToken)
	q.Set("toToken", params.ToToken)
	q.Set("fromAmount", params.FromAmount)
	q.Set("fromAddress", params.FromAddress)
	q.Set("toAddress", params.ToAddress)
	q.Set("slippage", params.Slippage)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	// LI.FI works without an API key; omit the header when no key is configured.
	if c.apiKey != "" {
		hReq.Header.Set("x-lifi-api-key", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lifi quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("lifi quote decode: %w", err)
	}
	return &qr, nil
}

// Status fetches transaction status from the provider API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	u, err := url.Parse(c.baseURL + "/v1/status")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("txHash", txID)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("x-lifi-api-key", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lifi status failed: status %d", resp.StatusCode)
	}

	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("lifi status decode: %w", err)
	}
	return &sr, nil
}
