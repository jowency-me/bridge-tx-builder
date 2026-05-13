package oneinch

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

// Provider adapts the 1inch API to the domain provider interface.
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
const Name domain.ProviderName = "1inch"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

// Quote returns a cross-chain quote based on the request.
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
		ChainID:          chainID,
		Src:              req.FromToken.Address,
		Dst:              req.ToToken.Address,
		Amount:           req.Amount.String(),
		From:             req.FromAddr,
		Slippage:         strconv.FormatFloat(req.Slippage*100, 'f', 2, 64),
		DisableEstimate:  "false",
		AllowPartialFill: "false",
	}

	qr, err := p.client.Quote(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapQuote(qr, req)
}

// Status returns the status of a transaction.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	_, err := p.client.Status(ctx, txID)
	if err != nil {
		return nil, err
	}
	// 1inch has no transaction status API; return reachable as a best effort.
	return &domain.Status{
		State:      "completed",
		SrcChainTx: txID,
	}, nil
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	fromAmt, err := decimal.NewFromString(qr.SrcAmount)
	if err != nil {
		fromAmt = req.Amount
	}
	toAmt, err := decimal.NewFromString(qr.DstAmount)
	if err != nil {
		toAmt = decimal.Zero
	}

	var txData []byte
	if strings.HasPrefix(qr.Tx.Data, "0x") {
		var err error
		txData, err = hexutil.Decode(qr.Tx.Data[2:])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
	}

	var txValue decimal.Decimal
	if qr.Tx.Value != "" && qr.Tx.Value != "0" {
		v, err := decimal.NewFromString(qr.Tx.Value)
		if err == nil {
			txValue = v
		}
	}

	gas := qr.Gas
	if gas == 0 {
		gas = qr.Tx.Gas
	}

	minAmount := toAmt.Mul(decimal.NewFromInt(995)).Div(decimal.NewFromInt(1000))

	route := []domain.RouteStep{
		{
			ChainID:  req.FromToken.ChainID,
			Protocol: "1inch",
			Action:   "swap",
		},
	}

	return &domain.Quote{
		ID:          "1inch-" + qr.DstAmount + "-" + qr.SrcAmount,
		FromToken:   mapToken(qr.FromToken, req.FromToken.ChainID),
		ToToken:     mapToken(qr.ToToken, req.ToToken.ChainID),
		FromAmount:  fromAmt,
		ToAmount:    toAmt,
		MinAmount:   minAmount,
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Route:       route,
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          qr.Tx.To,
		TxData:      txData,
		TxValue:     txValue,
		EstimateGas: gas,
	}, nil
}

func mapToken(t TokenInfo, chainID domain.ChainID) domain.Token {
	return domain.Token{
		Symbol:   t.Symbol,
		Address:  t.Address,
		Decimals: t.Decimals,
		ChainID:  chainID,
	}
}
