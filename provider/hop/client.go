package hop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.hop.exchange/v1"

// Client is the raw HTTP client for Hop API.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient creates a new Hop API client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// QuoteParams holds the parameters for a Hop quote request.
type QuoteParams struct {
	FromChain string
	ToChain   string
	Token     string
	Amount    string
}

// QuoteResponse is the raw JSON response from Hop quote endpoint.
type QuoteResponse struct {
	AmountOut              string `json:"amountOut"`
	Bridge                 string `json:"bridge"`
	EstimatedRecipientTime int    `json:"estimatedRecipientTime"`
	Fee                    string `json:"fee"`
	Slippage               string `json:"slippage"`
}

// StatusResponse is not supported by Hop.
type StatusResponse struct {
	Status string `json:"status"`
}

// Quote makes a real HTTP request to the Hop quote endpoint.
func (c *Client) Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error) {
	u, err := url.Parse(c.baseURL + "/quote")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("fromChain", params.FromChain)
	q.Set("toChain", params.ToChain)
	q.Set("token", params.Token)
	q.Set("amount", params.Amount)
	u.RawQuery = q.Encode()

	hReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(hReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hop quote failed: status %d", resp.StatusCode)
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("hop quote decode: %w", err)
	}
	return &qr, nil
}

// Status is not supported by Hop API.
func (c *Client) Status(ctx context.Context, txID string) (*StatusResponse, error) {
	return nil, fmt.Errorf("hop status not supported")
}
