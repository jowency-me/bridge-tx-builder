// Package mayan provides a quote adapter for the Mayan Finance cross-chain swap protocol.
//
// API Reference (official Mayan Transaction Builder service):
//
//	Repo:    https://github.com/mayan-finance/tx-builder
//	Quote:   GET  /quote   (public; returns {success, quotes:[...]})
//	Build:   POST /build   (builds an unsigned tx from a chosen quote)
//	Forwarder: GET /forwarder-address
//
// API key is OPTIONAL. The /quote endpoint is public and works with no key (per
// https://docs.mayan.finance/integration/quote-api#api-key — start without a key
// and only request one if you hit rate limits). On the Mayan-hosted instance
// (https://tx-builder.mayan.finance) the /build endpoint additionally requires an
// X-API-Key (set via WithAPIKey); self-hosting the tx-builder avoids any key.
//
// Verified against the live Mayan-hosted instance 2026-06-11: /quote returns
// HTTP 200 with no key ({success:true, quotes:[{type, expectedAmountOutBaseUnits,
// minAmountOutBaseUnits, deadline64, fromToken, toToken, ...}]}); the hosted
// /build returned 401 without an X-API-Key.
package mayan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://tx-builder.mayan.finance"

// Client is the raw HTTP client for the Mayan Transaction Builder API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new Mayan tx-builder API client targeting the
// Mayan-hosted instance by default.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// QuoteParams holds the GET /quote query parameters.
type QuoteParams struct {
	AmountIn64  string // amount in smallest unit (base units)
	FromToken   string
	ToToken     string
	FromChain   string
	ToChain     string
	SlippageBps string // "auto" or basis points as a string (e.g. "50")
	GasDrop     string
	Referrer    string
	ReferrerBps int
	Swift       bool
	MCTP        bool
	FastMCTP    bool
	Wormhole    bool
}

// QuoteView is the subset of a quote option the adapter reads. The full quote
// object is preserved verbatim in Raw so it can be echoed back to POST /build.
type QuoteView struct {
	Type                       string          `json:"type"`
	ExpectedAmountOut          float64         `json:"expectedAmountOut"`
	ExpectedAmountOutBaseUnits string          `json:"expectedAmountOutBaseUnits"`
	MinAmountOutBaseUnits      string          `json:"minAmountOutBaseUnits"`
	Deadline64                 string          `json:"deadline64"`
	Gasless                    bool            `json:"gasless"`
	Raw                        json.RawMessage `json:"-"`
}

// quoteResponse is the GET /quote envelope.
type quoteResponse struct {
	Success bool              `json:"success"`
	Quotes  []json.RawMessage `json:"quotes"`
	Error   string            `json:"error"`
}

// BuildParams holds the per-chain parameters for POST /build.
type BuildParams struct {
	SwapperAddress     string `json:"swapperAddress"`
	DestinationAddress string `json:"destinationAddress"`
	SignerChainID      int    `json:"signerChainId,omitempty"`
}

// buildRequest is the POST /build body: the chosen quote echoed back plus params.
type buildRequest struct {
	Quote  json.RawMessage `json:"quote"`
	Params BuildParams     `json:"params"`
}

// EVMTx is the inner EVM transaction object returned by POST /build.
type EVMTx struct {
	To      string `json:"to"`
	Data    string `json:"data"`
	Value   string `json:"value"`
	ChainID int    `json:"chainId"`
}

// BuildResponse is the POST /build response. For EVM the inner transaction is an
// object ({to,data,value,chainId}); for SVM/Sui it is a base64 string.
type BuildResponse struct {
	Success     bool   `json:"success"`
	Error       string `json:"error"`
	Transaction struct {
		ChainCategory string          `json:"chainCategory"`
		QuoteType     string          `json:"quoteType"`
		Gasless       bool            `json:"gasless"`
		Transaction   json.RawMessage `json:"transaction"`
		Signers       []string        `json:"signers"`
	} `json:"transaction"`
}

// Quote calls GET /quote and returns the available quote options. Each option's
// raw JSON is preserved so it can be passed unchanged to Build.
func (c *Client) Quote(ctx context.Context, params QuoteParams) ([]QuoteView, error) {
	u, err := url.Parse(c.baseURL + "/quote")
	if err != nil {
		return nil, fmt.Errorf("mayan: parse url: %w", err)
	}
	q := u.Query()
	q.Set("amountIn64", params.AmountIn64)
	q.Set("fromToken", params.FromToken)
	q.Set("toToken", params.ToToken)
	q.Set("fromChain", params.FromChain)
	q.Set("toChain", params.ToChain)
	slippage := params.SlippageBps
	if slippage == "" {
		slippage = "auto"
	}
	q.Set("slippageBps", slippage)
	if params.Swift {
		q.Set("swift", "true")
	}
	if params.MCTP {
		q.Set("mctp", "true")
	}
	if params.FastMCTP {
		q.Set("fastMctp", "true")
	}
	if params.Wormhole {
		q.Set("wormhole", "true")
	}
	if params.GasDrop != "" {
		q.Set("gasDrop", params.GasDrop)
	}
	if params.Referrer != "" {
		q.Set("referrer", params.Referrer)
		q.Set("referrerBps", strconv.Itoa(params.ReferrerBps))
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("mayan: build request: %w", err)
	}
	c.setAuth(hReq)
	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, fmt.Errorf("mayan: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mayan quote failed: status %d", resp.StatusCode)
	}
	var qr quoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("mayan quote decode: %w", err)
	}
	if !qr.Success {
		return nil, fmt.Errorf("mayan quote unsuccessful: %s", qr.Error)
	}
	views := make([]QuoteView, 0, len(qr.Quotes))
	for _, raw := range qr.Quotes {
		var v QuoteView
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("mayan quote option decode: %w", err)
		}
		v.Raw = raw
		views = append(views, v)
	}
	return views, nil
}

// Build calls POST /build with the chosen quote echoed back and returns the
// unsigned transaction. Requires an API key when targeting the Mayan-hosted
// instance (set via WithAPIKey); a self-hosted instance needs none.
func (c *Client) Build(ctx context.Context, quote json.RawMessage, params BuildParams) (*BuildResponse, error) {
	payload, err := json.Marshal(buildRequest{Quote: quote, Params: params})
	if err != nil {
		return nil, fmt.Errorf("mayan: marshal build: %w", err)
	}
	hReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/build", nil)
	if err != nil {
		return nil, fmt.Errorf("mayan: build request: %w", err)
	}
	hReq.Header.Set("Content-Type", "application/json")
	c.setAuth(hReq)
	hReq.Body = httpBytesBody(payload)
	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, fmt.Errorf("mayan: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mayan build failed: status %d", resp.StatusCode)
	}
	var br BuildResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return nil, fmt.Errorf("mayan build decode: %w", err)
	}
	if !br.Success {
		return nil, fmt.Errorf("mayan build unsuccessful: %s", br.Error)
	}
	return &br, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
}

func httpBytesBody(b []byte) io.ReadCloser { return io.NopCloser(bytes.NewReader(b)) }
