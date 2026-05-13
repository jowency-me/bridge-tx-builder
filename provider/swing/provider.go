package swing

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

// Provider adapts the swing API to the domain provider interface.
type Provider struct {
	client client
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainArbitrum:  "arbitrum",
	domain.ChainAvalanche: "avalanche",
	domain.ChainBSC:       "bsc",
	domain.ChainBase:      "base",
	domain.ChainEthereum:  "ethereum",
	domain.ChainOptimism:  "optimism",
	domain.ChainPolygon:   "polygon",
	domain.ChainSolana:    "solana",
	domain.ChainTron:      "tron",
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
const Name domain.ProviderName = "swing"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

// Quote returns a quote for the given swap request.
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
		FromChain:       fromCode,
		ToChain:         toCode,
		FromToken:       req.FromToken.Address,
		ToToken:         req.ToToken.Address,
		FromAmount:      req.Amount.String(),
		FromUserAddress: req.FromAddr,
		ToUserAddress:   req.ToAddr,
		Slippage:        strconv.FormatFloat(req.Slippage*100, 'f', 2, 64),
	}

	qr, err := p.client.Quote(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapQuote(qr, req)
}

// Status returns the status of the transaction.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	sr, err := p.client.Status(ctx, txID)
	if err != nil {
		return nil, err
	}
	return mapStatus(sr), nil
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	if len(qr.Routes) == 0 {
		return nil, fmt.Errorf("%s: no routes found", Name)
	}

	routeInfo := qr.Routes[0]
	toAmt, err := decimal.NewFromString(routeInfo.Quote.Amount)
	if err != nil {
		toAmt = decimal.Zero
	}
	minAmt := toAmt.Mul(decimal.NewFromInt(995)).Div(decimal.NewFromInt(1000))

	gas, _ := strconv.ParseUint(routeInfo.Gas, 10, 64)

	route := make([]domain.RouteStep, 0, len(routeInfo.Route))
	for _, s := range routeInfo.Route {
		action := "swap"
		if len(s.Steps) > 0 {
			action = s.Steps[0]
		}
		route = append(route, domain.RouteStep{
			ChainID:  req.FromToken.ChainID,
			Protocol: s.Bridge,
			Action:   action,
		})
	}

	return &domain.Quote{
		ID:          routeInfo.Quote.Integration + "-" + routeInfo.Quote.Type,
		FromToken:   req.FromToken,
		ToToken:     req.ToToken,
		FromAmount:  req.Amount,
		ToAmount:    toAmt,
		MinAmount:   minAmt,
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Route:       route,
		Deadline:    time.Now().Add(time.Duration(routeInfo.Duration) * time.Second),
		EstimateGas: gas,
	}, nil
}

func mapStatus(sr *StatusResponse) *domain.Status {
	if sr == nil {
		return &domain.Status{State: "unknown"}
	}
	state := sr.Status
	if state == "" {
		state = "unknown"
	}
	return &domain.Status{
		TxID:       sr.TxID,
		State:      state,
		SrcChainTx: sr.FromChainTxHash,
		DstChainTx: sr.ToChainTxHash,
		Error:      sr.Reason,
	}
}
