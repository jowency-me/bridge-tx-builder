package lifi

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

// Provider adapts the lifi API to the domain provider interface.
type Provider struct {
	client client
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainArbitrum:  "ARB",
	domain.ChainAvalanche: "AVA",
	domain.ChainBSC:       "BSC",
	domain.ChainBase:      "BAS",
	domain.ChainBitcoin:   "BTC",
	domain.ChainCosmos:    "OSM",
	domain.ChainEthereum:  "ETH",
	domain.ChainOptimism:  "OPT",
	domain.ChainPolygon:   "POL",
	domain.ChainSolana:    "SOL",
	domain.ChainTron:      "TRX",
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
const Name domain.ProviderName = "lifi"

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
		Slippage:    strconv.FormatFloat(req.Slippage, 'f', -1, 64),
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
	return mapStatus(sr), nil
}

func mapQuote(qr *QuoteResponse, reqs ...domain.QuoteRequest) (*domain.Quote, error) {
	req := domain.QuoteRequest{Slippage: 0.005}
	if len(reqs) > 0 {
		req = reqs[0]
	}
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	fromAmountRaw := qr.FromAmount
	if fromAmountRaw == "" {
		fromAmountRaw = qr.Estimate.FromAmount
	}
	if fromAmountRaw == "" {
		fromAmountRaw = qr.Action.FromAmount
	}
	fromAmt, err := decimal.NewFromString(fromAmountRaw)
	if err != nil {
		fromAmt = decimal.Zero
	}
	toAmountRaw := qr.ToAmount
	if toAmountRaw == "" {
		toAmountRaw = qr.Estimate.ToAmount
	}
	toAmt, err := decimal.NewFromString(toAmountRaw)
	if err != nil {
		toAmt = decimal.Zero
	}
	minAmt, err := decimal.NewFromString(qr.Estimate.ToAmountMin)
	if err != nil {
		minAmt = decimal.Zero
	}

	var gas uint64
	if len(qr.Estimate.GasCosts) > 0 && qr.Estimate.GasCosts[0].Estimate != "" {
		g, err := strconv.ParseUint(qr.Estimate.GasCosts[0].Estimate, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: parse gas cost: %w", Name, err)
		}
		gas = g
	}
	if gas == 0 && qr.TransactionRequest.GasLimit != "" {
		g, err := strconv.ParseUint(qr.TransactionRequest.GasLimit, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: parse gas limit: %w", Name, err)
		}
		gas = g
	}

	var txData []byte
	if strings.HasPrefix(qr.TransactionRequest.Data, "0x") {
		var err error
		txData, err = hexutil.Decode(qr.TransactionRequest.Data[2:])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
	}

	var txValue decimal.Decimal
	if qr.TransactionRequest.Value != "" && qr.TransactionRequest.Value != "0" {
		v, err := decimal.NewFromString(qr.TransactionRequest.Value)
		if err == nil {
			txValue = v
		}
	}

	route := make([]domain.RouteStep, 0, len(qr.IncludedSteps))
	for _, s := range qr.IncludedSteps {
		chainID := domain.NumericToChainID(strconv.Itoa(qr.Action.FromChainID))
		if s.Type == "cross" {
			chainID = domain.NumericToChainID(strconv.Itoa(qr.Action.ToChainID))
		}
		route = append(route, domain.RouteStep{
			ChainID:  chainID,
			Protocol: s.Tool,
			Action:   s.Type,
		})
	}

	return &domain.Quote{
		ID:          qr.ID,
		FromToken:   mapToken(qr.Action.FromToken, qr.Action.FromChainID),
		ToToken:     mapToken(qr.Action.ToToken, qr.Action.ToChainID),
		FromAmount:  fromAmt,
		ToAmount:    toAmt,
		MinAmount:   minAmt,
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Route:       route,
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          qr.TransactionRequest.To,
		TxData:      txData,
		TxValue:     txValue,
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
		TxID:       sr.Sending.TxHash,
		State:      state,
		SrcChainTx: sr.Sending.TxHash,
		DstChainTx: sr.Receiving.TxHash,
	}
}

func mapToken(t TokenInfo, chainID int) domain.Token {
	return domain.Token{
		Symbol:   t.Symbol,
		Address:  t.Address,
		Decimals: t.Decimals,
		ChainID:  domain.NumericToChainID(strconv.Itoa(chainID)),
	}
}
