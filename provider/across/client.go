// Package across provides a quote adapter for the Across Protocol cross-chain bridge.
//
// API Reference:
//
//	Quote: https://docs.across.to/developers/across-api
//	Status: https://docs.across.to/developers/across-api
//	API version: v2 (verified 2026-05-15)
package across

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultBaseURL   = "https://app.across.to/api"
	defaultTradeType = "exactInput"
)

// Client is the raw HTTP client for Across API.
type Client struct {
	baseURL      string
	client       *http.Client
	apiKey       string
	integratorID string
}

// NewClient creates a new Across API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// QuoteParams holds the parameters for an Across swap approval request.
type QuoteParams struct {
	InputToken         string
	OutputToken        string
	Amount             string
	OriginChainID      string
	DestinationChainID string
	Depositor          string
	Recipient          string
	TradeType          string
	Slippage           float64
}

// TxInfo contains provider transaction metadata.
type TxInfo struct {
	Ecosystem         string `json:"ecosystem"`
	SimulationSuccess bool   `json:"simulationSuccess"`
	ChainID           int    `json:"chainId"`
	To                string `json:"to"`
	Data              string `json:"data"`
	Value             string `json:"value"`
	Gas               string `json:"gas"`
}

// ApprovalTxn contains an ERC-20 approval transaction.
type ApprovalTxn struct {
	ChainID int    `json:"chainId"`
	To      string `json:"to"`
	Data    string `json:"data"`
}

// AllowanceCheck contains token allowance information from the API.
type AllowanceCheck struct {
	Token    string `json:"token"`
	Spender  string `json:"spender"`
	Actual   string `json:"actual"`
	Expected string `json:"expected"`
}

// BalanceCheck contains token balance information from the API.
type BalanceCheck struct {
	Token    string `json:"token"`
	Actual   string `json:"actual"`
	Expected string `json:"expected"`
}

// Checks contains pre-flight validation data for a swap.
type Checks struct {
	Allowance AllowanceCheck `json:"allowance"`
	Balance   BalanceCheck   `json:"balance"`
}

// ApprovalData contains token approval requirements for a swap.
// In the new API this is populated from Checks.Allowance.
type ApprovalData struct {
	Allowance    string `json:"allowance"`
	Spender      string `json:"spender"`
	TokenAddress string `json:"tokenAddress"`
}

// BridgeStep contains the bridge execution step data.
type BridgeStep struct {
	InputAmount  string `json:"inputAmount"`
	OutputAmount string `json:"outputAmount"`
	Provider     string `json:"provider"`
}

// Steps contains the execution steps for a swap.
type Steps struct {
	Bridge BridgeStep `json:"bridge"`
}

// QuoteResponse is the raw JSON response from the Across /api/swap/approval endpoint.
//
// Verified (2026-05-15): The API at app.across.to/api/swap/approval returns a
// redesigned response. The primary fields are: CrossSwapType, AmountType, Checks,
// ApprovalTxns, Steps, InputAmount, ExpectedOutputAmount, MinOutputAmount,
// ExpectedFillTime, SwapTx (with ecosystem/simulationSuccess/chainId/to/data/gas),
// QuoteExpiryTimestamp, and Id.
type QuoteResponse struct {
	CrossSwapType        string        `json:"crossSwapType"`
	AmountType           string        `json:"amountType"`
	Checks               Checks        `json:"checks"`
	ApprovalTxns         []ApprovalTxn `json:"approvalTxns"`
	Steps                Steps         `json:"steps"`
	InputAmount          string        `json:"inputAmount"`
	MaxInputAmount       string        `json:"maxInputAmount"`
	ExpectedOutputAmount string        `json:"expectedOutputAmount"`
	MinOutputAmount      string        `json:"minOutputAmount"`
	ExpectedFillTime     int           `json:"expectedFillTime"`
	SwapTx               TxInfo        `json:"swapTx"`
	QuoteExpiryTimestamp int64         `json:"quoteExpiryTimestamp"`
	ID                   string        `json:"id"`
}

// StatusResponse is not supported by Across.
type StatusResponse struct {
	Status string `json:"status"`
}

// Quote makes a real HTTP request to the Across swap approval endpoint.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/swap/approval")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("inputToken", params.InputToken)
	q.Set("outputToken", params.OutputToken)
	q.Set("amount", params.Amount)
	q.Set("originChainId", params.OriginChainID)
	q.Set("destinationChainId", params.DestinationChainID)
	q.Set("depositor", params.Depositor)
	q.Set("recipient", params.Recipient)
	tradeType := params.TradeType
	if tradeType == "" {
		tradeType = defaultTradeType
	}
	q.Set("tradeType", tradeType)
	if params.Slippage > 0 {
		q.Set("slippage", strconv.FormatFloat(params.Slippage, 'f', 4, 64))
	}
	if c.integratorID != "" {
		q.Set("integratorId", c.integratorID)
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("across quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("across quote decode: %w", err)
	}
	return &qr, nil
}

// Status is not supported by Across API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	return nil, fmt.Errorf("across status not supported")
}
