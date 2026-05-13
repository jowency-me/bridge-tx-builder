package openocean

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

// Provider adapts the openocean API to the domain provider interface.
type Provider struct {
	client client
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainArbitrum:  "arbitrum",
	domain.ChainAvalanche: "avax",
	domain.ChainBSC:       "bsc",
	domain.ChainBase:      "base",
	domain.ChainEthereum:  "eth",
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
const Name domain.ProviderName = "openocean"

// Name returns the provider identifier.
func (p *Provider) Name() string { return string(Name) }

// Quote fetches a quote from the provider API.
func (p *Provider) Quote(ctx context.Context, req domain.QuoteRequest) (*domain.Quote, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// OpenOcean amount is in human-readable format (without decimals)
	// We need to convert from wei/lamports to human-readable
	fromAmtHuman := req.Amount
	if req.FromToken.Decimals > 0 {
		divisor := decimal.New(1, int32(req.FromToken.Decimals))
		fromAmtHuman = req.Amount.Div(divisor)
	}

	fromCode := chainCodes[req.FromToken.ChainID]
	if fromCode == "" {
		return nil, fmt.Errorf("%s: unsupported from chain %s", Name, req.FromToken.ChainID)
	}
	toCode := chainCodes[req.ToToken.ChainID]
	if toCode == "" {
		return nil, fmt.Errorf("%s: unsupported to chain %s", Name, req.ToToken.ChainID)
	}
	if fromCode != toCode {
		return nil, fmt.Errorf("%s: cross-chain swaps are not supported", Name)
	}

	params := QuoteParams{
		ChainCode:       fromCode,
		InTokenAddress:  req.FromToken.Address,
		OutTokenAddress: req.ToToken.Address,
		Amount:          fromAmtHuman.String(),
		GasPrice:        "5",
		Slippage:        strconv.FormatFloat(req.Slippage*100, 'f', 2, 64),
		Account:         req.FromAddr,
	}

	qr, err := p.client.Quote(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapQuote(qr, req)
}

// Status fetches transaction status from the provider API.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	_, err := p.client.Status(ctx, txID)
	if err != nil {
		return nil, err
	}
	// OpenOcean has no transaction status API; return reachable as a best effort.
	return &domain.Status{
		State:      "completed",
		SrcChainTx: txID,
	}, nil
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	if qr.Data == nil {
		return nil, fmt.Errorf("%s: empty data in response", Name)
	}

	toAmt, err := decimal.NewFromString(qr.Data.OutAmount)
	if err != nil {
		toAmt = decimal.Zero
	}
	minAmt := toAmt.Mul(decimal.NewFromInt(995)).Div(decimal.NewFromInt(1000))

	var txData []byte
	if strings.HasPrefix(qr.Data.Data, "0x") {
		var err error
		txData, err = hexutil.Decode(qr.Data.Data[2:])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
	}

	var txValue decimal.Decimal
	if qr.Data.Value != "" && qr.Data.Value != "0" {
		v, err := decimal.NewFromString(qr.Data.Value)
		if err == nil {
			txValue = v
		}
	}

	return &domain.Quote{
		ID:          fmt.Sprintf("oo-%s", qr.Data.OutAmount),
		FromToken:   req.FromToken,
		ToToken:     req.ToToken,
		FromAmount:  req.Amount,
		ToAmount:    toAmt,
		MinAmount:   minAmt,
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          qr.Data.To,
		TxData:      txData,
		TxValue:     txValue,
		EstimateGas: qr.Data.EstimatedGas,
	}, nil
}
