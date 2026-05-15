package socket

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/shopspring/decimal"
)

type client interface {
	Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error)
	Status(ctx context.Context, txID string) (*StatusResponse, error)
}

// Provider adapts the socket API to the domain provider interface.
type Provider struct {
	client client
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainArbitrum:  "42161",
	domain.ChainAvalanche: "43114",
	domain.ChainBSC:       "56",
	domain.ChainBase:      "8453",
	domain.ChainEthereum:  "1",
	domain.ChainOptimism:  "10",
	domain.ChainPolygon:   "137",
	domain.ChainSolana:    "101",
}

// Option configures a Provider.
type Option func(*Provider)

// WithBaseURL configures the provider.
func WithBaseURL(u string) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithAPIKey configures the provider.
func WithAPIKey(key string) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.apiKey = key
		}
	}
}

// WithHTTPClient configures the provider.
func WithHTTPClient(hc *http.Client) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.client = hc
		}
	}
}

// NewProvider configures the provider.
func NewProvider(opts ...Option) *Provider {
	p := &Provider{client: NewClient()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name is the provider identifier.
const Name domain.ProviderName = "socket"

// Name returns the provider identifier.
func (p *Provider) Name() string { return string(Name) }

// Quote fetches a quote from the provider API.
func (p *Provider) Quote(ctx context.Context, req domain.QuoteRequest) (*domain.Quote, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	fromCode := chainCodes[req.FromToken.ChainID]
	if fromCode == "" {
		return nil, fmt.Errorf("%s: unsupported from chain %s", Name, req.FromToken.ChainID)
	}
	toCode := chainCodes[req.ToToken.ChainID]
	if toCode == "" {
		return nil, fmt.Errorf("%s: unsupported to chain %s", Name, req.ToToken.ChainID)
	}

	params := QuoteParams{
		FromChainID:      fromCode,
		ToChainID:        toCode,
		FromTokenAddress: req.FromToken.Address,
		ToTokenAddress:   req.ToToken.Address,
		FromAmount:       req.Amount.String(),
		UserAddress:      req.FromAddr,
		Recipient:        req.ToAddr,
		Slippage:         strconv.FormatFloat(req.Slippage*100, 'f', 2, 64),
	}

	qr, err := p.client.Quote(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapQuote(qr, req)
}

// Status fetches transaction status from the provider API.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	sr, err := p.client.Status(ctx, txID)
	if err != nil {
		return nil, err
	}
	return mapStatus(sr, txID), nil
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil || qr.Result == nil || qr.Result.AutoRoute == nil {
		return nil, fmt.Errorf("%s: no route found", Name)
	}

	ar := qr.Result.AutoRoute

	toAmt, err := decimal.NewFromString(ar.OutputAmount)
	if err != nil {
		toAmt = decimal.Zero
	}

	var minAmt decimal.Decimal
	if ar.Output.MinAmountOut != "" {
		parsed, pErr := decimal.NewFromString(ar.Output.MinAmountOut)
		if pErr == nil && parsed.IsPositive() {
			minAmt = parsed
		}
	}
	if minAmt.IsZero() {
		minAmt = toAmt.Mul(decimal.NewFromFloat(1 - req.Slippage))
	}

	// Build route steps from the auto-route data
	routeSteps := []domain.RouteStep{
		{
			ChainID:  req.FromToken.ChainID,
			Protocol: "socket",
			Action:   "bridge",
		},
	}

	// LIMITATION: The new Bungee API uses Permit2 (signTypedData) instead of
	// traditional transaction data. TxData, To, and TxValue are NOT populated
	// in the quote response. The user must sign the EIP-712 typed data instead.
	// This quote is valid for rate comparison but cannot be used to build a
	// transaction directly.

	var deadline time.Time
	if ar.QuoteExpiry > 0 {
		deadline = time.Unix(ar.QuoteExpiry, 0)
	} else {
		deadline = time.Now().Add(10 * time.Minute)
	}

	quote := &domain.Quote{
		ID:         fmt.Sprintf("socket-%s", ar.QuoteID),
		FromToken:  req.FromToken,
		ToToken:    req.ToToken,
		FromAmount: req.Amount,
		ToAmount:   toAmt,
		MinAmount:  minAmt,
		Slippage:   ar.Slippage,
		Provider:   string(Name),
		Route:      routeSteps,
		Deadline:   deadline,
	}

	if ar.ApprovalData != nil && ar.ApprovalData.SpenderAddress != "" {
		quote.ApprovalAddress = ar.ApprovalData.SpenderAddress
		if ar.ApprovalData.Amount != "" {
			a, err := decimal.NewFromString(ar.ApprovalData.Amount)
			if err == nil {
				quote.AllowanceNeeded = &a
			}
		}
	}

	return quote, nil
}

func mapStatus(sr *StatusResponse, txID string) *domain.Status {
	if sr == nil {
		return &domain.Status{TxID: txID, State: "unknown"}
	}
	state := "unknown"
	var srcTx, dstTx string
	if len(sr.Result) > 0 {
		r := sr.Result[0]
		// Use destination status if available (more meaningful for bridge completion),
		// otherwise fall back to origin status
		if r.DestinationData.Status != "" {
			state = strings.ToLower(r.DestinationData.Status)
		} else if r.OriginData.Status != "" {
			state = strings.ToLower(r.OriginData.Status)
		}
		srcTx = r.OriginData.TxHash
		dstTx = r.DestinationData.TxHash
	}
	// Normalize Bungee-specific states to domain conventions
	switch state {
	case "pending":
		state = "pending"
	case "completed", "done":
		state = "completed"
	case "failed":
		state = "failed"
	}
	return &domain.Status{
		TxID:       txID,
		State:      state,
		SrcChainTx: srcTx,
		DstChainTx: dstTx,
		UpdatedAt:  time.Now(),
	}
}
