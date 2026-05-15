// Package thorchain provides a quote adapter for the THORChain cross-chain liquidity protocol.
//
// API Reference:
//
//	Quote: https://docs.thorchain.org
//	Status: https://docs.thorchain.org
//	API version: v1 (verified 2026-05-15)
package thorchain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://thornode.thorchain.network"

// Client is the raw HTTP client for THORChain API.
type Client struct {
	baseURL string
	client  *http.Client
	apiKey  string
}

// NewClient creates a new THORChain API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// QuoteParams contains raw thorchain quote request parameters.
type QuoteParams struct {
	FromAsset   string
	ToAsset     string
	Amount      string
	Destination string
}

// QuoteResponse contains raw thorchain quote response data.
type QuoteResponse struct {
	InboundAddress             string   `json:"inbound_address"`
	InboundConfirmationBlocks  int64    `json:"inbound_confirmation_blocks"`
	InboundConfirmationSeconds int64    `json:"inbound_confirmation_seconds"`
	OutboundDelayBlocks        int64    `json:"outbound_delay_blocks"`
	OutboundDelaySeconds       int64    `json:"outbound_delay_seconds"`
	Fees                       FeesInfo `json:"fees"`
	SlippageBps                int      `json:"slippage_bps"`
	StreamingSlippageBps       int      `json:"streaming_slippage_bps"`
	Expiry                     int64    `json:"expiry"`
	Warning                    string   `json:"warning"`
	Notes                      string   `json:"notes"`
	DustThreshold              string   `json:"dust_threshold"`
	RecommendedMinAmountIn     string   `json:"recommended_min_amount_in"`
	RecommendedGasRate         string   `json:"recommended_gas_rate"`
	GasRateUnits               string   `json:"gas_rate_units"`
	Memo                       string   `json:"memo"`
	ExpectedAmountOut          string   `json:"expected_amount_out"`
	ExpectedAmountOutStreaming string   `json:"expected_amount_out_streaming"`
	MaxStreamingQuantity       int      `json:"max_streaming_quantity"`
	StreamingSwapBlocks        int64    `json:"streaming_swap_blocks"`
	StreamingSwapSeconds       int64    `json:"streaming_swap_seconds"`
	TotalSwapSeconds           int64    `json:"total_swap_seconds"`
}

// FeesInfo contains provider fee breakdown data.
type FeesInfo struct {
	Asset       string `json:"asset"`
	Affiliate   string `json:"affiliate"`
	Outbound    string `json:"outbound"`
	Liquidity   string `json:"liquidity"`
	Total       string `json:"total"`
	SlippageBps int    `json:"slippage_bps"`
	TotalBps    int    `json:"total_bps"`
}

// StatusResponse contains raw thorchain status response data.
type StatusResponse struct {
	Tx            StatusTxDetail `json:"tx"`
	PlannedOutTxs []PlannedOutTx `json:"planned_out_txs"`
	OutTxs        []OutTx        `json:"out_txs"`
	Stages        StatusStages   `json:"stages"`
}

// StatusTxDetail contains inbound transaction details from status response.
type StatusTxDetail struct {
	ID          string       `json:"id"`
	Chain       string       `json:"chain"`
	FromAddress string       `json:"from_address"`
	ToAddress   string       `json:"to_address"`
	Coins       []StatusCoin `json:"coins"`
	Gas         interface{}  `json:"gas"`
	Memo        string       `json:"memo"`
}

// StatusCoin contains coin data in status response.
type StatusCoin struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

// PlannedOutTx contains planned outbound transaction data.
type PlannedOutTx struct {
	Chain     string     `json:"chain"`
	ToAddress string     `json:"to_address"`
	Coin      StatusCoin `json:"coin"`
	Refund    bool       `json:"refund"`
}

// OutTx contains outbound transaction information.
type OutTx struct {
	ID          string       `json:"id"`
	Chain       string       `json:"chain"`
	FromAddress string       `json:"from_address"`
	ToAddress   string       `json:"to_address"`
	Coins       []StatusCoin `json:"coins"`
	Gas         []StatusCoin `json:"gas"`
	Memo        string       `json:"memo"`
}

// StatusStages contains transaction stage statuses (flat structure matching real API).
type StatusStages struct {
	InboundObserved            InboundObservedStage `json:"inbound_observed"`
	InboundConfirmationCounted ConfirmCountedStage  `json:"inbound_confirmation_counted"`
	InboundFinalised           StageCompleted       `json:"inbound_finalised"`
	SwapStatus                 SwapStatusStage      `json:"swap_status"`
	SwapFinalised              StageCompleted       `json:"swap_finalised"`
}

// InboundObservedStage contains inbound observation stage data.
type InboundObservedStage struct {
	FinalCount int  `json:"final_count"`
	Completed  bool `json:"completed"`
}

// ConfirmCountedStage contains confirmation counting stage data.
type ConfirmCountedStage struct {
	RemainingConfirmationSeconds int64 `json:"remaining_confirmation_seconds"`
	Completed                    bool  `json:"completed"`
}

// StageCompleted is a simple completed stage.
type StageCompleted struct {
	Completed bool `json:"completed"`
}

// SwapStatusStage contains swap status data.
type SwapStatusStage struct {
	Pending bool `json:"pending"`
}

// Quote fetches a quote from the provider API.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/thorchain/quote/swap")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("from_asset", params.FromAsset)
	q.Set("to_asset", params.ToAsset)
	q.Set("amount", params.Amount)
	q.Set("destination", params.Destination)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("x-client-id", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thorchain quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("thorchain quote decode: %w", err)
	}
	return &qr, nil
}

// Status fetches transaction status from the provider API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	u, err := url.Parse(c.baseURL + "/thorchain/tx/status/" + txID)
	if err != nil {
		return nil, err
	}

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		hReq.Header.Set("x-client-id", c.apiKey)
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thorchain status failed: status %d", resp.StatusCode)
	}

	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("thorchain status decode: %w", err)
	}
	return &sr, nil
}
