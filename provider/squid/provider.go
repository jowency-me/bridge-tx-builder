package squid

import (
	"context"
	"fmt"
	"math"
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
	Status(ctx context.Context, txID string, statusParams ...StatusParams) (*StatusResponse, error)
}

// Provider adapts the squid API to the domain provider interface.
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
	domain.ChainSolana:    "solana",
	domain.ChainTron:      "728126428",
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
const Name domain.ProviderName = "squid"

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
		FromChain:   fromCode,
		ToChain:     toCode,
		FromToken:   req.FromToken.Address,
		ToToken:     req.ToToken.Address,
		FromAmount:  req.Amount.String(),
		FromAddress: req.FromAddr,
		ToAddress:   req.ToAddr,
		Slippage:    int(math.Round(req.Slippage * 100)),
	}

	qr, err := p.client.Quote(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapQuote(qr, req)
}

// Status fetches transaction status from the provider API.
//
// NOTE: Squid's /status endpoint optionally accepts fromChainId, toChainId, and
// quoteId for more accurate status lookups, but the domain.Provider interface
// only exposes txID. Callers that need richer status data should use the squid
// client directly with StatusParams.
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
	fromAmt, err := decimal.NewFromString(qr.Route.Estimate.FromAmount)
	if err != nil {
		fromAmt = decimal.Zero
	}
	toAmt, err := decimal.NewFromString(qr.Route.Estimate.ToAmount)
	if err != nil {
		toAmt = decimal.Zero
	}
	minAmt, err := decimal.NewFromString(qr.Route.Estimate.ToAmountMin)
	if err != nil {
		minAmt = decimal.Zero
	}

	var gas uint64
	if len(qr.Route.Estimate.GasCosts) > 0 && qr.Route.Estimate.GasCosts[0].GasLimit != "" {
		g, err := strconv.ParseUint(qr.Route.Estimate.GasCosts[0].GasLimit, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: parse gas cost: %w", Name, err)
		}
		gas = g
	}
	if gas == 0 && qr.Route.TransactionRequest.GasLimit != "" {
		g, err := strconv.ParseUint(qr.Route.TransactionRequest.GasLimit, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: parse gas limit: %w", Name, err)
		}
		gas = g
	}

	var txData []byte
	if strings.HasPrefix(qr.Route.TransactionRequest.Data, "0x") {
		var err error
		txData, err = hexutil.Decode(qr.Route.TransactionRequest.Data[2:])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
	}

	var txValue decimal.Decimal
	if qr.Route.TransactionRequest.Value != "" && qr.Route.TransactionRequest.Value != "0" {
		v, err := decimal.NewFromString(qr.Route.TransactionRequest.Value)
		if err == nil {
			txValue = v
		}
	}

	var estFee decimal.Decimal
	for _, fc := range qr.Route.Estimate.FeeCosts {
		if fc.Amount != "" {
			f, err := decimal.NewFromString(fc.Amount)
			if err == nil {
				estFee = estFee.Add(f)
			}
		}
	}

	route := []domain.RouteStep{
		{
			ChainID:  domain.NumericToChainID(qr.Route.Estimate.FromToken.ChainID),
			Protocol: "squid",
			Action:   "swap",
		},
		{
			ChainID:  domain.NumericToChainID(qr.Route.Estimate.ToToken.ChainID),
			Protocol: "squid",
			Action:   "bridge",
		},
	}

	quote := &domain.Quote{
		ID:          qr.RequestID,
		FromToken:   mapToken(qr.Route.Estimate.FromToken),
		ToToken:     mapToken(qr.Route.Estimate.ToToken),
		FromAmount:  fromAmt,
		ToAmount:    toAmt,
		MinAmount:   minAmt,
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Route:       route,
		Deadline:    time.Now().Add(time.Duration(qr.Route.Estimate.EstimatedRouteDuration) * time.Second),
		To:          qr.Route.TransactionRequest.Target,
		TxData:      txData,
		TxValue:     txValue,
		EstimateGas: gas,
		EstimateFee: estFee,
	}

	if qr.Route.TransactionRequest.Target != "" &&
		qr.Route.Estimate.FromToken.Address != "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE" {
		quote.ApprovalAddress = qr.Route.TransactionRequest.Target
		allowance := fromAmt
		quote.AllowanceNeeded = &allowance
	}

	return quote, nil
}

func mapStatus(sr *StatusResponse) *domain.Status {
	if sr == nil {
		return &domain.Status{State: "unknown"}
	}
	state := sr.SquidTransactionStatus
	if state == "" {
		state = "unknown"
	}
	var srcTx, dstTx string
	if sr.FromChain != nil {
		srcTx = sr.FromChain.TransactionID
	}
	if sr.ToChain != nil {
		dstTx = sr.ToChain.TransactionID
	}
	return &domain.Status{
		TxID:       sr.ID,
		State:      state,
		SrcChainTx: srcTx,
		DstChainTx: dstTx,
	}
}

func mapToken(t TokenInfo) domain.Token {
	return domain.Token{
		Symbol:   t.Symbol,
		Address:  t.Address,
		Decimals: t.Decimals,
		ChainID:  domain.NumericToChainID(t.ChainID),
	}
}
