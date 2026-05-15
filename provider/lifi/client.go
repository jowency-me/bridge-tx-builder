// Package lifi provides a quote adapter for the LI.FI cross-chain aggregation API.
//
// API Reference:
//
//	Quote: https://docs.li.fi/api-reference/get-a-quote-for-a-token-transfer
//	Status: https://docs.li.fi/api-reference/transaction-status
//	API version: v1 (verified 2026-05-15)
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
	ID                 string     `json:"id"`
	Type               string     `json:"type"`
	Tool               string     `json:"tool"`
	ToolDetails        ToolDetail `json:"toolDetails"`
	Integrator         string     `json:"integrator"`
	FromAmount         string     `json:"fromAmount"`
	ToAmount           string     `json:"toAmount"`
	Estimate           Estimate   `json:"estimate"`
	Action             Action     `json:"action"`
	IncludedSteps      []Step     `json:"includedSteps"`
	TransactionRequest TxRequest  `json:"transactionRequest"`
	TransactionID      string     `json:"transactionId"`
}

// ToolDetail contains provider tool metadata.
type ToolDetail struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	LogoURI string `json:"logoURI"`
}

// Estimate contains provider amount and gas estimates.
type Estimate struct {
	Tool              string    `json:"tool"`
	ToAmountMin       string    `json:"toAmountMin"`
	ToAmount          string    `json:"toAmount"`
	FromAmount        string    `json:"fromAmount"`
	FromAmountUSD     string    `json:"fromAmountUSD"`
	ToAmountUSD       string    `json:"toAmountUSD"`
	ApprovalAddress   string    `json:"approvalAddress"`
	GasCosts          []GasCost `json:"gasCosts"`
	FeeCosts          []FeeCost `json:"feeCosts"`
	ExecutionDuration int       `json:"executionDuration"`
}

// GasCost contains a provider gas cost entry.
type GasCost struct {
	Type      string    `json:"type"`
	Price     string    `json:"price"`
	Estimate  string    `json:"estimate"`
	Limit     string    `json:"limit"`
	Amount    string    `json:"amount"`
	AmountUSD string    `json:"amountUSD"`
	Token     TokenInfo `json:"token"`
}

// FeeCost contains a provider fee cost entry.
type FeeCost struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Amount      string    `json:"amount"`
	AmountUSD   string    `json:"amountUSD"`
	Percentage  string    `json:"percentage"`
	Included    bool      `json:"included"`
	Token       TokenInfo `json:"token"`
	FeeSplit    *FeeSplit `json:"feeSplit"`
}

// FeeSplit contains the fee distribution breakdown.
type FeeSplit struct {
	IntegratorFee string `json:"integratorFee"`
	LifiFee       string `json:"lifiFee"`
}

// Action contains provider route action metadata.
type Action struct {
	FromToken   TokenInfo `json:"fromToken"`
	ToToken     TokenInfo `json:"toToken"`
	FromAmount  string    `json:"fromAmount"`
	FromChainID int       `json:"fromChainId"`
	ToChainID   int       `json:"toChainId"`
	Slippage    float64   `json:"slippage"`
	FromAddress string    `json:"fromAddress"`
	ToAddress   string    `json:"toAddress"`
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Symbol                      string   `json:"symbol"`
	Address                     string   `json:"address"`
	Decimals                    int      `json:"decimals"`
	ChainID                     int      `json:"chainId"`
	Name                        string   `json:"name"`
	CoinKey                     string   `json:"coinKey"`
	PriceUSD                    string   `json:"priceUSD"`
	LogoURI                     string   `json:"logoURI"`
	Tags                        []string `json:"tags"`
	VerificationStatus          string   `json:"verificationStatus"`
	VerificationStatusBreakdown []any    `json:"verificationStatusBreakdown"`
}

// Step contains a provider route step.
type Step struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Tool        string     `json:"tool"`
	ToolDetails ToolDetail `json:"toolDetails"`
	Action      StepAction `json:"action"`
	Estimate    Estimate   `json:"estimate"`
}

// StepAction contains action data within an included step.
type StepAction struct {
	FromToken   TokenInfo `json:"fromToken"`
	ToToken     TokenInfo `json:"toToken"`
	FromAmount  string    `json:"fromAmount"`
	FromChainID int       `json:"fromChainId"`
	ToChainID   int       `json:"toChainId"`
	FromAddress string    `json:"fromAddress"`
	ToAddress   string    `json:"toAddress"`
	Slippage    float64   `json:"slippage"`
}

// TxRequest contains provider transaction request data.
type TxRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	ChainID  int    `json:"chainId"`
	GasPrice string `json:"gasPrice"`
	GasLimit string `json:"gasLimit"`
}

// StatusResponse contains raw lifi status response data.
type StatusResponse struct {
	Sending          TxInfo `json:"sending"`
	Receiving        TxInfo `json:"receiving"`
	Status           string `json:"status"`
	Substatus        string `json:"substatus"`
	SubstatusMessage string `json:"substatusMessage"`
	BridgeExplorer   string `json:"bridgeExplorerLink"`
	TxHistoryURL     string `json:"txHistoryUrl"`
	TokenAmountIn    string `json:"tokenAmountIn"`
	TokenAmountOut   string `json:"tokenAmountOut"`
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
