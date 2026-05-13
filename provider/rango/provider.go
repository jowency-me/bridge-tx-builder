package rango

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

// Provider adapts the rango API to the domain provider interface.
type Provider struct {
	client client
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainArbitrum:  "ARBITRUM",
	domain.ChainAvalanche: "AVAX_CCHAIN",
	domain.ChainBSC:       "BSC",
	domain.ChainBase:      "BASE",
	domain.ChainBitcoin:   "BTC",
	domain.ChainEthereum:  "ETH",
	domain.ChainOptimism:  "OPTIMISM",
	domain.ChainPolygon:   "POLYGON",
	domain.ChainSolana:    "SOLANA",
	domain.ChainTron:      "TRON",
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
func NewProvider(apiKey string, opts ...Option) *Provider {
	p := &Provider{client: NewClient(apiKey)}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name is the provider identifier.
const Name domain.ProviderName = "rango"

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
		From:        fromCode,
		To:          toCode,
		FromToken:   req.FromToken.Address,
		ToToken:     req.ToToken.Address,
		Amount:      req.Amount.String(),
		FromAddress: req.FromAddr,
		ToAddress:   req.ToAddr,
		Slippage:    strconv.FormatFloat(req.Slippage*100, 'f', 4, 64),
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
	return &domain.Status{
		TxID:  sr.RequestID,
		State: sr.Status,
	}, nil
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	toAmt, err := decimal.NewFromString(qr.OutputAmount)
	if err != nil {
		toAmt = decimal.Zero
	}
	route := make([]domain.RouteStep, 0, len(qr.Swaps))
	for _, s := range qr.Swaps {
		route = append(route, domain.RouteStep{
			ChainID:  domain.ChainID(strings.ToLower(s.From.Blockchain)),
			Protocol: s.SwapperID,
			Action:   "swap",
		})
	}
	return &domain.Quote{
		ID:         qr.RequestID,
		FromToken:  req.FromToken,
		ToToken:    req.ToToken,
		FromAmount: req.Amount,
		ToAmount:   toAmt,
		MinAmount:  toAmt.Mul(decimal.NewFromInt(995)).Div(decimal.NewFromInt(1000)),
		Slippage:   req.Slippage,
		Provider:   string(Name),
		Route:      route,
		Deadline:   time.Now().Add(10 * time.Minute),
	}, nil
}
