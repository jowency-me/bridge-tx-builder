// Package across provides a quote adapter for the Across Protocol cross-chain bridge.
package across

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
}

// FeeBreakdown holds a fee with percentage and total.
type FeeBreakdown struct {
	Pct   string `json:"pct"`
	Total string `json:"total"`
}

// TxInfo contains provider transaction metadata.
type TxInfo struct {
	To    string `json:"to"`
	Data  string `json:"data"`
	Value string `json:"value"`
	Gas   string `json:"gas"`
}

// QuoteResponse is the raw JSON response from Across swap approval endpoint.
type QuoteResponse struct {
	TotalRelayFee       FeeBreakdown `json:"totalRelayFee"`
	RelayerFee          FeeBreakdown `json:"relayerFee"`
	LpFee               FeeBreakdown `json:"lpFee"`
	Timestamp           string       `json:"timestamp"`
	IsAmountTooLow      bool         `json:"isAmountTooLow"`
	QuoteBlock          string       `json:"quoteBlock"`
	SpokePoolAddress    string       `json:"spokePoolAddress"`
	ExpectedFillTimeSec int          `json:"expectedFillTimeSec"`
	CapitalCostFeePct   string       `json:"capitalCostFeePct"`
	RelayFeeFullPct     string       `json:"relayFeeFullPct"`
	InputAmount         string       `json:"inputAmount"`
	OutputAmount        string       `json:"outputAmount"`
	SwapTx              TxInfo       `json:"swapTx"`
	ApprovalTxns        []TxInfo     `json:"approvalTxns"`
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
