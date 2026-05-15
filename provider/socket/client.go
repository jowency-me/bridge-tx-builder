// Package socket provides a quote adapter for the Bungee (formerly Socket) cross-chain bridge.
//
// API Reference:
//
//	Quote: https://docs.bungee.exchange
//	Status: https://docs.bungee.exchange
//	API version: v2 (verified 2026-05-15)
package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://public-backend.bungee.exchange"

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
	FromChainID      string // maps to originChainId
	ToChainID        string // maps to destinationChainId
	FromTokenAddress string // maps to inputToken
	ToTokenAddress   string // maps to outputToken
	FromAmount       string // maps to inputAmount
	UserAddress      string
	Recipient        string // maps to receiverAddress
	Slippage         string
}

// QuoteResponse contains raw socket quote response data.
type QuoteResponse struct {
	Success    bool         `json:"success"`
	StatusCode int          `json:"statusCode"`
	Result     *QuoteResult `json:"result"`
	Message    *string      `json:"message"`
}

// QuoteResult contains the quote result data.
type QuoteResult struct {
	OriginChainID      int        `json:"originChainId"`
	DestinationChainID int        `json:"destinationChainId"`
	UserAddress        string     `json:"userAddress"`
	ReceiverAddress    string     `json:"receiverAddress"`
	Input              InputData  `json:"input"`
	AutoRoute          *AutoRoute `json:"autoRoute"`
}

// InputData contains the input token and amount data.
type InputData struct {
	Token      TokenData `json:"token"`
	Amount     string    `json:"amount"`
	PriceInUsd float64   `json:"priceInUsd"`
	ValueInUsd float64   `json:"valueInUsd"`
}

// TokenData contains token metadata from the API.
type TokenData struct {
	ChainID  int    `json:"chainId"`
	Address  string `json:"address"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	LogoURI  string `json:"logoURI"`
	Icon     string `json:"icon"`
}

// AutoRoute contains the auto-routing result data.
type AutoRoute struct {
	UserOp        string        `json:"userOp"`
	RequestHash   string        `json:"requestHash"`
	Output        OutputData    `json:"output"`
	RequestType   string        `json:"requestType"`
	ApprovalData  *ApprovalData `json:"approvalData"`
	GasFee        interface{}   `json:"gasFee"`
	Slippage      float64       `json:"slippage"`
	TxData        interface{}   `json:"txData"`
	EstimatedTime int           `json:"estimatedTime"`
	RouteDetails  RouteDetails  `json:"routeDetails"`
	QuoteID       string        `json:"quoteId"`
	QuoteExpiry   int64         `json:"quoteExpiry"`
	OutputAmount  string        `json:"outputAmount"`
	RouteTags     []string      `json:"routeTags"`
}

// OutputData contains the output token and amount data.
type OutputData struct {
	Token                  TokenData `json:"token"`
	PriceInUsd             float64   `json:"priceInUsd"`
	ValueInUsd             float64   `json:"valueInUsd"`
	MinAmountOut           string    `json:"minAmountOut"`
	Amount                 string    `json:"amount"`
	EffectiveAmount        string    `json:"effectiveAmount"`
	EffectiveValueInUsd    float64   `json:"effectiveValueInUsd"`
	EffectiveReceivedInUsd float64   `json:"effectiveReceivedInUsd"`
}

// ApprovalData contains provider approval transaction data.
type ApprovalData struct {
	SpenderAddress string `json:"spenderAddress"`
	Amount         string `json:"amount"`
	TokenAddress   string `json:"tokenAddress"`
	UserAddress    string `json:"userAddress"`
}

// RouteDetails contains route metadata.
type RouteDetails struct {
	Name       string      `json:"name"`
	LogoURI    string      `json:"logoURI"`
	RouteFee   interface{} `json:"routeFee"`
	DexDetails interface{} `json:"dexDetails"`
}

// StatusResponse contains raw socket status response data.
type StatusResponse struct {
	Success    bool           `json:"success"`
	StatusCode int            `json:"statusCode"`
	Result     []StatusResult `json:"result"`
}

// StatusResult contains provider status result data.
type StatusResult struct {
	Hash             string          `json:"hash"`
	OriginData       OriginData      `json:"originData"`
	DestinationData  DestinationData `json:"destinationData"`
	BungeeStatusCode int             `json:"bungeeStatusCode"`
}

// OriginData contains the source chain status data.
type OriginData struct {
	TxHash        string `json:"txHash"`
	OriginChainID int    `json:"originChainId"`
	Status        string `json:"status"`
	UserAddress   string `json:"userAddress"`
}

// DestinationData contains the destination chain status data.
type DestinationData struct {
	TxHash             string `json:"txHash"`
	DestinationChainID int    `json:"destinationChainId"`
	ReceiverAddress    string `json:"receiverAddress"`
	Status             string `json:"status"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/bungee/quote")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("originChainId", params.FromChainID)
	q.Set("destinationChainId", params.ToChainID)
	q.Set("inputToken", params.FromTokenAddress)
	q.Set("outputToken", params.ToTokenAddress)
	q.Set("inputAmount", params.FromAmount)
	q.Set("userAddress", params.UserAddress)
	q.Set("receiverAddress", params.Recipient)
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
	u, err := url.Parse(c.baseURL + "/api/v1/bungee/status")
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
