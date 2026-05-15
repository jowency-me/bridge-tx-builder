// Package swing provides a quote adapter for the Swing.xyz cross-chain bridge.
//
// API Reference:
//
//	Quote: https://docs.swing.xyz/v3/reference
//	Status: https://docs.swing.xyz/v3/reference
//	API version: v3 (verified 2026-05-15)
package swing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://swap.prod.swing.xyz/v0"

// Client is the raw HTTP client for Swing.xyz API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new Swing.xyz API client.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		apiKey:  apiKey,
	}
}

// QuoteParams contains raw swing quote request parameters.
type QuoteParams struct {
	FromChain       string
	ToChain         string
	FromToken       string
	ToToken         string
	TokenSymbol     string
	ToTokenSymbol   string
	FromAmount      string
	FromUserAddress string
	ToUserAddress   string
	Slippage        string
	ID              float64
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
	ChainID  int    `json:"chainId"`
}

// ChainInfo contains provider chain metadata.
type ChainInfo struct {
	ChainID      int    `json:"chainId"`
	Slug         string `json:"slug"`
	ProtocolType string `json:"protocolType"`
}

// RouteStep contains provider route step metadata.
type RouteStep struct {
	Bridge             string   `json:"bridge"`
	BridgeTokenAddress string   `json:"bridgeTokenAddress"`
	Steps              []string `json:"steps"`
	Name               string   `json:"name"`
	Part               int      `json:"part"`
}

// QuoteDetail contains provider quote details.
type QuoteDetail struct {
	Integration               string `json:"integration"`
	Type                      string `json:"type"`
	Amount                    string `json:"amount"`
	Decimals                  int    `json:"decimals"`
	BridgeFee                 string `json:"bridgeFee"`
	BridgeFeeInNativeToken    string `json:"bridgeFeeInNativeToken"`
	AmountUSD                 string `json:"amountUSD"`
	BridgeFeeUSD              string `json:"bridgeFeeUSD"`
	BridgeFeeInNativeTokenUSD string `json:"bridgeFeeInNativeTokenUSD"`
	Fees                      []Fee  `json:"fees"`
}

// Fee contains provider fee metadata.
type Fee struct {
	Type                    string `json:"type"`
	Amount                  string `json:"amount"`
	AmountUSD               string `json:"amountUSD"`
	TokenSymbol             string `json:"tokenSymbol"`
	TokenAddress            string `json:"tokenAddress"`
	ChainSlug               string `json:"chainSlug"`
	Decimals                int    `json:"decimals"`
	DeductedFromSourceToken bool   `json:"deductedFromSourceToken"`
}

// RouteInfo contains provider route metadata.
type RouteInfo struct {
	Route    []RouteStep `json:"route"`
	Quote    QuoteDetail `json:"quote"`
	Duration int         `json:"duration"`
	Gas      string      `json:"gas"`
	GasUSD   string      `json:"gasUSD"`
}

// QuoteResponse contains raw swing quote response data.
type QuoteResponse struct {
	Routes    []RouteInfo `json:"routes"`
	FromToken TokenInfo   `json:"fromToken"`
	FromChain ChainInfo   `json:"fromChain"`
	ToToken   TokenInfo   `json:"toToken"`
	ToChain   ChainInfo   `json:"toChain"`
}

// StatusResponse contains raw swing status response data.
type StatusResponse struct {
	Status          string `json:"status"`
	FromChainTxHash string `json:"fromChainTxHash"`
	ToChainTxHash   string `json:"toChainTxHash"`
	TxID            string `json:"txId"`
	Reason          string `json:"reason"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/transfer/quote")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("fromChain", params.FromChain)
	q.Set("toChain", params.ToChain)
	q.Set("fromTokenAddress", params.FromToken)
	q.Set("toTokenAddress", params.ToToken)
	if params.TokenSymbol != "" {
		q.Set("tokenSymbol", params.TokenSymbol)
	}
	if params.ToTokenSymbol != "" {
		q.Set("toTokenSymbol", params.ToTokenSymbol)
	}
	q.Set("tokenAmount", params.FromAmount)
	q.Set("fromUserAddress", params.FromUserAddress)
	q.Set("toUserAddress", params.ToUserAddress)
	q.Set("maxSlippage", params.Slippage)
	if params.ID != 0 {
		q.Set("id", strconv.FormatFloat(params.ID, 'f', -1, 64))
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hReq.Header.Set("project-id", c.apiKey)

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("swing quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("swing quote decode: %w", err)
	}
	return &qr, nil
}

// Status fetches transaction status from the provider API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	u, err := url.Parse(c.baseURL + "/transfer/status")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("id", txID)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hReq.Header.Set("project-id", c.apiKey)

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("swing status failed: status %d", resp.StatusCode)
	}

	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("swing status decode: %w", err)
	}
	return &sr, nil
}
