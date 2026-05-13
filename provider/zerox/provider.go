package zerox

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

// Provider adapts the zerox API to the domain provider interface.
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
func NewProvider(apiKey string, opts ...Option) *Provider {
	p := &Provider{client: NewClient(apiKey)}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name is the provider identifier.
const Name domain.ProviderName = "0x"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

// Quote returns a quote for the given swap request.
func (p *Provider) Quote(ctx context.Context, req domain.QuoteRequest) (*domain.Quote, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	chainID := chainCodes[req.FromToken.ChainID]
	if chainID == "" {
		return nil, fmt.Errorf("%s: unsupported from chain %s", Name, req.FromToken.ChainID)
	}
	toChainID := chainCodes[req.ToToken.ChainID]
	if toChainID == "" {
		return nil, fmt.Errorf("%s: unsupported to chain %s", Name, req.ToToken.ChainID)
	}
	if chainID != toChainID {
		return nil, fmt.Errorf("%s: cross-chain swaps are not supported", Name)
	}

	params := QuoteParams{
		ChainID:      chainID,
		SellToken:    req.FromToken.Address,
		BuyToken:     req.ToToken.Address,
		SellAmount:   req.Amount.String(),
		TakerAddress: req.FromAddr,
		SlippageBps:  strconv.FormatInt(int64(req.Slippage*10000), 10),
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
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	state := "unknown"
	if sr != nil && sr.Status != "" {
		state = sr.Status
	}
	return &domain.Status{
		TxID:      txID,
		State:     state,
		UpdatedAt: time.Now(),
	}, nil
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	fromAmt, err := decimal.NewFromString(qr.SellAmount)
	if err != nil {
		fromAmt = req.Amount
	}
	toAmt, err := decimal.NewFromString(qr.BuyAmount)
	if err != nil {
		toAmt = decimal.Zero
	}

	var minAmount decimal.Decimal
	if qr.GuaranteedPrice != "" && qr.GuaranteedPrice != "0" {
		guaranteedPrice, err := decimal.NewFromString(qr.GuaranteedPrice)
		if err == nil && !guaranteedPrice.IsZero() {
			minAmount = fromAmt.Mul(guaranteedPrice)
		}
	}
	if minAmount.IsZero() {
		minAmount = toAmt.Mul(decimal.NewFromInt(995)).Div(decimal.NewFromInt(1000))
	}

	txTo := qr.To
	txDataHex := qr.Data
	txValueRaw := qr.Value
	txGasRaw := qr.Gas
	if qr.Transaction.To != "" {
		txTo = qr.Transaction.To
	}
	if qr.Transaction.Data != "" {
		txDataHex = qr.Transaction.Data
	}
	if qr.Transaction.Value != "" {
		txValueRaw = qr.Transaction.Value
	}
	if qr.Transaction.Gas != "" {
		txGasRaw = qr.Transaction.Gas
	}

	var txData []byte
	if strings.HasPrefix(txDataHex, "0x") {
		var err error
		txData, err = hexutil.Decode(txDataHex[2:])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
	}

	var txValue decimal.Decimal
	if txValueRaw != "" && txValueRaw != "0" {
		v, err := decimal.NewFromString(txValueRaw)
		if err == nil {
			txValue = v
		}
	}

	var gas uint64
	if txGasRaw != "" {
		g, err := strconv.ParseUint(txGasRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: parse gas: %w", Name, err)
		}
		gas = g
	}

	var estimateFee decimal.Decimal
	if qr.Fee.FeeAmount != "" {
		f, err := decimal.NewFromString(qr.Fee.FeeAmount)
		if err == nil {
			estimateFee = f
		}
	}

	route := make([]domain.RouteStep, 0, len(qr.Sources))
	for _, s := range qr.Sources {
		if s.Proportion != "0" {
			route = append(route, domain.RouteStep{
				ChainID:  req.FromToken.ChainID,
				Protocol: s.Name,
				Action:   "swap",
			})
		}
	}
	if len(route) == 0 {
		route = []domain.RouteStep{
			{
				ChainID:  req.FromToken.ChainID,
				Protocol: "0x",
				Action:   "swap",
			},
		}
	}

	return &domain.Quote{
		ID:          "0x-" + qr.BuyAmount + "-" + qr.SellAmount,
		FromToken:   req.FromToken,
		ToToken:     req.ToToken,
		FromAmount:  fromAmt,
		ToAmount:    toAmt,
		MinAmount:   minAmount,
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Route:       route,
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          txTo,
		TxData:      txData,
		TxValue:     txValue,
		EstimateGas: gas,
		EstimateFee: estimateFee,
	}, nil
}
