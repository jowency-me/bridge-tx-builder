// Package hop provides a quote adapter for the Hop Protocol cross-chain bridge.
package hop

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/shopspring/decimal"
)

type client interface {
	Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error)
	Status(ctx context.Context, txID string) (*StatusResponse, error)
}

// Provider adapts the hop API to the domain provider interface.
type Provider struct {
	client client
}

// Hop Protocol supports Ethereum L2 rollups only.
// See https://hop.exchange/ for the current list of supported chains.
var chainCodes = map[domain.ChainID]string{
	domain.ChainArbitrum: "arbitrum",
	domain.ChainBase:     "base",
	domain.ChainEthereum: "ethereum",
	domain.ChainOptimism: "optimism",
	domain.ChainPolygon:  "polygon",
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
const Name domain.ProviderName = "hop"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

// Quote returns a cross-chain quote based on the request.
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
		FromChain: fromCode,
		ToChain:   toCode,
		Token:     req.FromToken.Address,
		Amount:    req.Amount.String(),
		Slippage:  req.Slippage,
	}

	qr, err := p.client.Quote(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapQuote(qr, req)
}

// Status returns the status of a transaction.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	return nil, fmt.Errorf("%s: status tracking not supported", Name)
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	toAmt, err := decimal.NewFromString(qr.EstimatedReceived)
	if err != nil {
		toAmt = decimal.Zero
	}

	var estFee decimal.Decimal
	if qr.BonderFee != "" {
		f, err := decimal.NewFromString(qr.BonderFee)
		if err == nil {
			estFee = f
		}
	}

	var slippage float64
	if qr.Slippage > 0 {
		slippage = qr.Slippage / 100.0
	}
	if slippage == 0 {
		slippage = req.Slippage
	}

	var minAmt decimal.Decimal
	if qr.AmountOutMin != "" {
		if m, err := decimal.NewFromString(qr.AmountOutMin); err == nil {
			minAmt = m
		}
	}
	if minAmt.IsZero() {
		minAmt = toAmt.Mul(decimal.NewFromFloat(1 - req.Slippage))
	}

	var deadline time.Time
	if qr.Deadline > 0 {
		deadline = time.Unix(qr.Deadline, 0)
	} else {
		deadline = time.Now().Add(10 * time.Minute)
	}

	return &domain.Quote{
		ID:         qr.Bridge + "-" + req.FromToken.Address,
		FromToken:  req.FromToken,
		ToToken:    req.ToToken,
		FromAmount: req.Amount,
		ToAmount:   toAmt,
		MinAmount:  minAmt,
		Slippage:   slippage,
		Provider:   string(Name),
		Route: []domain.RouteStep{
			{
				ChainID:  req.FromToken.ChainID,
				Protocol: "hop",
				Action:   "bridge",
			},
		},
		Deadline:    deadline,
		EstimateFee: estFee,
	}, nil
}
