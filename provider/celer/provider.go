package celer

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

// Provider adapts the celer API to the domain provider interface.
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
const Name domain.ProviderName = "celer"

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
		SrcChainID:  fromCode,
		DstChainID:  toCode,
		TokenSymbol: req.FromToken.Symbol,
		Amt:         req.Amount.String(),
		UsrAddr:     req.FromAddr,
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
	toAmt, err := decimal.NewFromString(qr.Value)
	if err != nil {
		toAmt = decimal.Zero
	}

	var fee decimal.Decimal
	if qr.PercFee != "" {
		f, err := decimal.NewFromString(qr.PercFee)
		if err == nil {
			fee = f
		}
	}

	var slippage float64
	if qr.SlippageTolerance > 0 {
		slippage = float64(qr.SlippageTolerance) / 10000.0
	}
	if slippage == 0 {
		slippage = req.Slippage
	}

	minAmt := toAmt.Mul(decimal.NewFromInt(995)).Div(decimal.NewFromInt(1000))

	return &domain.Quote{
		ID:         qr.Value + "-" + req.FromToken.Symbol,
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
				Protocol: "celer",
				Action:   "bridge",
			},
		},
		Deadline:    time.Now().Add(10 * time.Minute),
		EstimateFee: fee,
	}, nil
}
