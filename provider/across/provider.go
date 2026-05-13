package across

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	hexutil "github.com/jowency-me/bridge-tx-builder/provider/internal/hex"
	"github.com/shopspring/decimal"
)

type client interface {
	Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error)
	Status(ctx context.Context, txID string) (*StatusResponse, error)
}

// Provider adapts the across API to the domain provider interface.
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

// WithAPIKey configures the Across API key.
func WithAPIKey(key string) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.apiKey = key
		}
	}
}

// WithIntegratorID configures the Across integrator identifier.
func WithIntegratorID(id string) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.integratorID = id
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
const Name domain.ProviderName = "across"

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
		InputToken:         req.FromToken.Address,
		OutputToken:        req.ToToken.Address,
		Amount:             req.Amount.String(),
		OriginChainID:      fromCode,
		DestinationChainID: toCode,
		Depositor:          req.FromAddr,
		Recipient:          req.ToAddr,
		TradeType:          defaultTradeType,
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
	fromAmt := req.Amount
	if qr.InputAmount != "" {
		parsed, err := decimal.NewFromString(qr.InputAmount)
		if err == nil {
			fromAmt = parsed
		}
	}

	var toAmt decimal.Decimal
	if qr.OutputAmount != "" {
		parsed, err := decimal.NewFromString(qr.OutputAmount)
		if err == nil {
			toAmt = parsed
		}
	}
	if toAmt.IsZero() && qr.TotalRelayFee.Total != "" {
		fee, err := decimal.NewFromString(qr.TotalRelayFee.Total)
		if err == nil && !fee.IsNegative() {
			toAmt = fromAmt.Sub(fee)
		}
	}
	if toAmt.IsZero() || toAmt.IsNegative() {
		toAmt = fromAmt
	}

	var minAmt decimal.Decimal
	if qr.RelayFeeFullPct != "" {
		pct, err := strconv.ParseFloat(qr.RelayFeeFullPct, 64)
		if err == nil && pct > 0 {
			minAmt = toAmt.Mul(decimal.NewFromFloat(1 - pct))
		}
	}
	if minAmt.IsZero() || minAmt.IsNegative() {
		minAmt = toAmt.Mul(decimal.NewFromInt(995)).Div(decimal.NewFromInt(1000))
	}

	var slippage float64
	if qr.RelayFeeFullPct != "" {
		pct, err := strconv.ParseFloat(qr.RelayFeeFullPct, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: parse relay fee pct: %w", Name, err)
		}
		slippage = pct
	}
	if slippage == 0 {
		slippage = req.Slippage
	}
	if slippage == 0 {
		slippage = 0.005
	}

	var txData []byte
	if strings.HasPrefix(qr.SwapTx.Data, "0x") {
		decoded, err := hexutil.Decode(qr.SwapTx.Data[2:])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
		txData = decoded
	}

	var txValue decimal.Decimal
	if qr.SwapTx.Value != "" && qr.SwapTx.Value != "0" {
		v, err := decimal.NewFromString(qr.SwapTx.Value)
		if err == nil {
			txValue = v
		}
	}

	gas := uint64(200000)
	if qr.SwapTx.Gas != "" {
		parsed, err := strconv.ParseUint(qr.SwapTx.Gas, 10, 64)
		if err == nil && parsed > 0 {
			gas = parsed
		}
	}

	target := qr.SwapTx.To
	if target == "" {
		target = qr.SpokePoolAddress
	}

	idPart := qr.QuoteBlock
	if idPart == "" {
		idPart = qr.OutputAmount
	}

	return &domain.Quote{
		ID:         idPart + "-" + req.FromToken.Address,
		FromToken:  req.FromToken,
		ToToken:    req.ToToken,
		FromAmount: fromAmt,
		ToAmount:   toAmt,
		MinAmount:  minAmt,
		Slippage:   slippage,
		Provider:   string(Name),
		Route: []domain.RouteStep{
			{
				ChainID:  req.FromToken.ChainID,
				Protocol: "across",
				Action:   "bridge",
			},
		},
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          target,
		TxData:      txData,
		TxValue:     txValue,
		EstimateGas: gas,
	}, nil
}
