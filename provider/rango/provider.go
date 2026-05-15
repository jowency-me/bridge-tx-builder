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
	Status(ctx context.Context, txID string, statusParams ...StatusParams) (*StatusResponse, error)
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
	domain.ChainCosmos:    "COSMOS",
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
	return mapStatus(sr, txID), nil
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	outputAmount := qr.Route.OutputAmount
	toAmt, err := decimal.NewFromString(outputAmount)
	if err != nil {
		toAmt = decimal.Zero
	}

	var minAmt decimal.Decimal
	if qr.Route.OutputAmountMin != "" {
		parsed, pErr := decimal.NewFromString(qr.Route.OutputAmountMin)
		if pErr == nil && parsed.IsPositive() {
			minAmt = parsed
		}
	}
	if minAmt.IsZero() {
		minAmt = toAmt.Mul(decimal.NewFromFloat(1 - req.Slippage))
	}

	var estFee decimal.Decimal
	for _, f := range qr.Route.Fee {
		if f.Amount != "" {
			amt, err := decimal.NewFromString(f.Amount)
			if err == nil {
				estFee = estFee.Add(amt)
			}
		}
	}

	route := make([]domain.RouteStep, 0, len(qr.Route.Path))
	for _, p := range qr.Route.Path {
		chainID := domain.ChainID(strings.ToLower(p.From.Blockchain))
		if chainID == "" {
			chainID = req.FromToken.ChainID
		}
		route = append(route, domain.RouteStep{
			ChainID:  chainID,
			Protocol: p.Swapper.ID,
			Action:   "swap",
		})
	}

	// LIMITATION: Rango quote endpoint does not return transaction execution data.
	// The /basic/swap endpoint is needed to get tx.to, tx.data, tx.value, tx.approveTo.
	// This quote is valid for rate comparison but cannot build transactions directly.
	return &domain.Quote{
		ID:          qr.RequestID,
		FromToken:   mapToken(qr.Route.From, req.FromToken.ChainID),
		ToToken:     mapToken(qr.Route.To, req.ToToken.ChainID),
		FromAmount:  req.Amount,
		ToAmount:    toAmt,
		MinAmount:   minAmt,
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Route:       route,
		Deadline:    time.Now().Add(time.Duration(qr.Route.EstimatedTimeInSeconds) * time.Second),
		EstimateFee: estFee,
	}, nil
}

func mapStatus(sr *StatusResponse, txID string) *domain.Status {
	if sr == nil {
		return &domain.Status{TxID: txID, State: "unknown"}
	}
	state := sr.Status
	if state == "" {
		state = "unknown"
	}
	var srcTx, dstTx string
	if sr.BridgeData != nil {
		srcTx = sr.BridgeData.SrcTxHash
		dstTx = sr.BridgeData.DestTxHash
	}
	return &domain.Status{
		TxID:       txID,
		State:      state,
		SrcChainTx: srcTx,
		DstChainTx: dstTx,
		Error:      sr.Error,
	}
}

func mapToken(t TokenInfo, fallbackChainID domain.ChainID) domain.Token {
	chainID := domain.ChainID(strings.ToLower(t.Blockchain))
	if chainID == "" {
		chainID = fallbackChainID
	}
	return domain.Token{
		Symbol:   t.Symbol,
		Address:  t.Address,
		Decimals: t.Decimals,
		ChainID:  chainID,
	}
}
