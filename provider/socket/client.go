// Package socket provides a quote adapter for the Socket cross-chain bridge.
package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.socket.tech"

// Client is the raw HTTP client for Socket Protocol API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new Socket API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// QuoteParams contains raw socket quote request parameters.
type QuoteParams struct {
	FromChainID      string
	ToChainID        string
	FromTokenAddress string
	ToTokenAddress   string
	FromAmount       string
	UserAddress      string
	Recipient        string
	Slippage         string
}

// QuoteResponse contains raw socket quote response data.
type QuoteResponse struct {
	Routes []Route `json:"routes"`
}

// Route contains a provider route candidate.
type Route struct {
	RouteID          string            `json:"routeId"`
	ToAmount         string            `json:"toAmount"`
	TotalGasFees     string            `json:"totalGasFeesInUsd"`
	TotalFee         string            `json:"totalFeeInUsd"`
	UserTxs          []UserTx          `json:"userTxs"`
	Sender           string            `json:"sender"`
	ApprovalData     ApprovalData      `json:"approvalData"`
	ChainGasBalances []ChainGasBalance `json:"chainGasBalances"`
}

// UserTx contains a provider user transaction.
type UserTx struct {
	TxType    string `json:"txType"`
	TxData    string `json:"txData"`
	TxTarget  string `json:"txTarget"`
	ChainID   string `json:"chainId"`
	ToAmount  string `json:"toAmount"`
	StepCount int    `json:"stepCount"`
	RoutePath string `json:"routePath"`
}

// ApprovalData contains provider approval transaction data.
type ApprovalData struct {
	ApprovalTokenAddress  string `json:"approvalTokenAddress"`
	AllowanceTarget       string `json:"allowanceTarget"`
	MinimumApprovalAmount string `json:"minimumApprovalAmount"`
}

// ChainGasBalance contains provider gas balance metadata.
type ChainGasBalance struct {
	ChainID string `json:"chainId"`
	Balance string `json:"balance"`
}

// StatusResponse contains raw socket status response data.
type StatusResponse struct {
	Success bool   `json:"success"`
	Result  Result `json:"result"`
}

// Result contains provider status result data.
type Result struct {
	SourceTxHash      string `json:"sourceTxHash"`
	DestinationTxHash string `json:"destinationTxHash"`
	FromChainID       string `json:"fromChainId"`
	ToChainID         string `json:"toChainId"`
	Status            string `json:"status"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/v2/quote")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("fromChainId", params.FromChainID)
	q.Set("toChainId", params.ToChainID)
	q.Set("fromTokenAddress", params.FromTokenAddress)
	q.Set("toTokenAddress", params.ToTokenAddress)
	q.Set("fromAmount", params.FromAmount)
	q.Set("userAddress", params.UserAddress)
	q.Set("recipient", params.Recipient)
	q.Set("uniqueRoutesPerBridge", "true")
	q.Set("sort", "output")
	if params.Slippage != "" {
		q.Set("slippage", params.Slippage)
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("API-KEY", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("socket quote failed: status 401 (unauthorized)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("socket quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("socket quote decode: %w", err)
	}
	return &qr, nil
}

// Status fetches transaction status from the provider API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	u, err := url.Parse(c.baseURL + "/v2/bridge-status")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("transactionHash", txID)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("API-KEY", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("socket status failed: status 401 (unauthorized)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("socket status failed: status %d", resp.StatusCode)
	}

	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("socket status decode: %w", err)
	}
	return &sr, nil
}
