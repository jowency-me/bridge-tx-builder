// Package allbridge provides a quote adapter for the Allbridge Core cross-chain AMM.
//
// API Reference (official Allbridge Core REST API):
//
//	Repo:   https://github.com/allbridge-public/allbridge-core-rest-api
//	Docker: allbridge/io.allbridge.rest-api  (self-hosted; default localhost:3000)
//	Calc:   GET /bridge/receive/calculate  -> {amountInFloat, amountReceivedInFloat}
//	Tx:     GET /raw/bridge                -> RawTransaction (EVM object / Solana base64 / Tron object)
//
// The Allbridge Core REST API is distributed as a self-hosted Docker image (it
// wraps the @allbridge/bridge-core-sdk using your own RPC nodes). There is no
// public hosted instance, so the default baseURL targets localhost:3000; point
// WithBaseURL at wherever you run the container. The public
// core.api.allbridgecoreapi.net host only serves /token-info and is NOT this API.
//
// Request amounts on this API are INTEGER BASE UNITS: the controller converts
// them to float internally via convertGt0IntAmountToFloat(amount, decimals).
// Response amounts (amountInFloat / amountReceivedInFloat) are FLOAT; the adapter
// converts the received float back to base units.
//
// Endpoint params and response shapes are taken verbatim from the REST API
// controller source (transfers.controller.ts) and the SDK RawTransaction model.
package allbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "http://localhost:3000"

// Client is the raw HTTP client for the Allbridge Core REST API.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient creates a new Allbridge Core REST API client targeting the
// self-hosted default (localhost:3000).
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// CalcParams holds the GET /bridge/receive/calculate query parameters.
type CalcParams struct {
	Amount           string // integer base units (e.g. "1000000" for 1 USDC)
	SourceToken      string // source token address
	DestinationToken string // destination token address
	Messenger        string // Messenger enum key (e.g. "ALLBRIDGE")
}

// BridgeAmounts is the GET /bridge/receive/calculate response (both fields float).
type BridgeAmounts struct {
	AmountInFloat       string `json:"amountInFloat"`
	AmountReceivedFloat string `json:"amountReceivedInFloat"`
}

// BridgeParams holds the GET /raw/bridge query parameters.
type BridgeParams struct {
	Amount           string // integer base units
	Sender           string
	Recipient        string
	SourceToken      string
	DestinationToken string
	Messenger        string // required Messenger enum key
	FeePaymentMethod string // required FeePaymentMethod enum key
	OutputFormat     string // "json" | "base64" | "hex"
}

// EVMRawTransaction is the EVM RawTransaction shape (EssentialWeb3Transaction)
// returned by /raw/bridge with outputFormat=json.
type EVMRawTransaction struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `json:"value"`
	Data  string `json:"data"`
}

// Calculate calls GET /bridge/receive/calculate and returns the expected
// received amount (float).
func (c *Client) Calculate(ctx context.Context, params CalcParams) (*BridgeAmounts, error) {
	u, err := url.Parse(c.baseURL + "/bridge/receive/calculate")
	if err != nil {
		return nil, fmt.Errorf("allbridge: parse url: %w", err)
	}
	q := u.Query()
	q.Set("amount", params.Amount)
	q.Set("sourceToken", params.SourceToken)
	q.Set("destinationToken", params.DestinationToken)
	q.Set("messenger", params.Messenger)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("allbridge: build request: %w", err)
	}
	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, fmt.Errorf("allbridge: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("allbridge calculate failed: status %d", resp.StatusCode)
	}
	var ba BridgeAmounts
	if err := json.NewDecoder(resp.Body).Decode(&ba); err != nil {
		return nil, fmt.Errorf("allbridge calculate decode: %w", err)
	}
	return &ba, nil
}

// RawBridge calls GET /raw/bridge and returns the raw transaction as the exact
// JSON the server produced (shape depends on the source chain and outputFormat).
func (c *Client) RawBridge(ctx context.Context, params BridgeParams) (json.RawMessage, error) {
	u, err := url.Parse(c.baseURL + "/raw/bridge")
	if err != nil {
		return nil, fmt.Errorf("allbridge: parse url: %w", err)
	}
	q := u.Query()
	q.Set("amount", params.Amount)
	q.Set("sender", params.Sender)
	q.Set("recipient", params.Recipient)
	q.Set("sourceToken", params.SourceToken)
	q.Set("destinationToken", params.DestinationToken)
	q.Set("messenger", params.Messenger)
	q.Set("feePaymentMethod", params.FeePaymentMethod)
	if params.OutputFormat != "" {
		q.Set("outputFormat", params.OutputFormat)
	}
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("allbridge: build request: %w", err)
	}
	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, fmt.Errorf("allbridge: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("allbridge raw bridge failed: status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("allbridge raw bridge read: %w", err)
	}
	return json.RawMessage(raw), nil
}
