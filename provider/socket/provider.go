package socket

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	hexutil "github.com/jowency-me/bridge-tx-builder/provider/internal/hex"

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
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	if len(qr.Routes) == 0 {
		return nil, fmt.Errorf("%s: no routes found", Name)
	}

	route := qr.Routes[0]

	toAmt, err := decimal.NewFromString(route.ToAmount)
	if err != nil {
		toAmt = decimal.Zero
	}

	var fee decimal.Decimal
	if route.TotalFee != "" {
		f, err := decimal.NewFromString(route.TotalFee)
		if err == nil {
			fee = f
		}
	}
	if fee.IsZero() && route.TotalGasFees != "" {
		f, err := decimal.NewFromString(route.TotalGasFees)
		if err == nil {
			fee = f
		}
	}

	minAmt := toAmt.Mul(decimal.NewFromInt(995)).Div(decimal.NewFromInt(1000))

	routeSteps := make([]domain.RouteStep, 0, len(route.UserTxs))
	for _, tx := range route.UserTxs {
		action := "swap"
		if tx.TxType == "fund-movr" || tx.TxType == "bridge" {
			action = "bridge"
		}
		chainID := domain.NumericToChainID(tx.ChainID)
		if chainID == "" {
			chainID = req.ToToken.ChainID
		}
		routeSteps = append(routeSteps, domain.RouteStep{
			ChainID:  chainID,
			Protocol: "socket",
			Action:   action,
		})
	}

	var txData []byte
	if len(route.UserTxs) > 0 && strings.HasPrefix(route.UserTxs[0].TxData, "0x") {
		var err error
		txData, err = hexutil.Decode(route.UserTxs[0].TxData[2:])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
	}

	var toAddr string
	if len(route.UserTxs) > 0 {
		toAddr = route.UserTxs[0].TxTarget
	}

	return &domain.Quote{
		ID:          fmt.Sprintf("socket-%s", route.RouteID),
		FromToken:   req.FromToken,
		ToToken:     req.ToToken,
		FromAmount:  req.Amount,
		ToAmount:    toAmt,
		MinAmount:   minAmt,
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Route:       routeSteps,
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          toAddr,
		TxData:      txData,
		EstimateFee: fee,
	}, nil
}

func mapStatus(sr *StatusResponse, txID string) *domain.Status {
	if sr == nil {
		return &domain.Status{TxID: txID, State: "unknown"}
	}
	state := "unknown"
	if sr.Result.Status != "" {
		state = sr.Result.Status
	}
	return &domain.Status{
		TxID:       txID,
		State:      state,
		SrcChainTx: sr.Result.SourceTxHash,
		DstChainTx: sr.Result.DestinationTxHash,
		UpdatedAt:  time.Now(),
	}
}
