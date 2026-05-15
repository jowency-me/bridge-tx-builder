// Package oneinch provides a quote adapter for the 1inch Aggregation API.
//
// API Reference:
//
//	Quote: https://business.1inch.com/portal/documentation/apis/swap/classic-swap/methods/v6.1/1/quote/method/get
//	Swap:  https://business.1inch.com/portal/documentation/apis/swap/classic-swap/methods/v6.1/1/swap/method/get
//	API version: v6.1 (verified 2026-05-15)
package oneinch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.1inch.dev"

// Client is the raw HTTP client for 1inch Swap API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new 1inch API client.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		apiKey:  apiKey,
	}
}

// QuoteParams contains raw 1inch quote request parameters.
type QuoteParams struct {
	ChainID          string
	Src              string
	Dst              string
	Amount           string
	From             string
	Slippage         float64
	DisableEstimate  bool
	AllowPartialFill bool
}

// TokenInfo contains provider token metadata.
type TokenInfo struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Decimals int    `json:"decimals"`
	LogoURI  string `json:"logoURI"`
}

// TxData contains provider transaction payload data.
type TxData struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Data  string `json:"data"`
	Value string `json:"value"`
	Gas   uint64 `json:"gas"`
}

// QuoteResponse contains raw 1inch quote response data.
type QuoteResponse struct {
	DstAmount string       `json:"dstAmount"`
	SrcToken  TokenInfo    `json:"srcToken"`
	DstToken  TokenInfo    `json:"dstToken"`
	Protocols []TokenSwaps `json:"protocols"`
	Tx        TxData       `json:"tx"`
	Gas       uint64       `json:"gas"`
}

// TokenSwaps represents a token entry in the protocols response.
type TokenSwaps struct {
	Token string     `json:"token"`
	Hops  []TokenHop `json:"hops"`
}

// TokenHop represents a hop within a token swap route.
type TokenHop struct {
	Part        int                       `json:"part"`
	Dst         string                    `json:"dst"`
	FromTokenID int                       `json:"fromTokenId"`
	ToTokenID   int                       `json:"toTokenId"`
	Protocols   []SelectedLiquiditySource `json:"protocols"`
}

// SelectedLiquiditySource represents a protocol used in a hop.
type SelectedLiquiditySource struct {
	Name string `json:"name"`
	Part int    `json:"part"`
}

// StatusResponse contains raw 1inch status response data.
type StatusResponse struct {
	Status string `json:"status"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s/swap/v6.1/%s/quote", c.baseURL, params.ChainID))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("src", params.Src)
	q.Set("dst", params.Dst)
	q.Set("amount", params.Amount)
	if params.From != "" {
		q.Set("from", params.From)
	}
	if params.Slippage > 0 {
		q.Set("slippage", strconv.FormatFloat(params.Slippage, 'f', -1, 64))
	}
	if params.DisableEstimate {
		q.Set("disableEstimate", "true")
	}
	if params.AllowPartialFill {
		q.Set("allowPartialFill", "true")
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("1inch quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("1inch quote decode: %w", err)
	}
	return &qr, nil
}

// Status checks server reachability via a lightweight HEAD request.
// 1inch does not provide a dedicated transaction status API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.baseURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, fmt.Errorf("1inch server error: status %d", resp.StatusCode)
	}
	return &StatusResponse{Status: "reachable"}, nil
}
