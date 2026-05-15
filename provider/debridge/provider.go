package debridge

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

// Provider adapts the debridge API to the domain provider interface.
type Provider struct {
	client client
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
const Name domain.ProviderName = "debridge"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

// Quote returns a cross-chain quote based on the request.
func (p *Provider) Quote(ctx context.Context, req domain.QuoteRequest) (*domain.Quote, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if domain.NumericID(req.FromToken.ChainID) == 0 {
		return nil, fmt.Errorf("%s: unsupported from chain %s", Name, req.FromToken.ChainID)
	}
	if domain.NumericID(req.ToToken.ChainID) == 0 {
		return nil, fmt.Errorf("%s: unsupported to chain %s", Name, req.ToToken.ChainID)
	}

	params := QuoteParams{
		SrcChainID:                    strconv.FormatInt(domain.NumericID(req.FromToken.ChainID), 10),
		SrcChainTokenIn:               req.FromToken.Address,
		SrcChainTokenInAmount:         req.Amount.String(),
		DstChainID:                    strconv.FormatInt(domain.NumericID(req.ToToken.ChainID), 10),
		DstChainTokenOut:              req.ToToken.Address,
		SrcChainOrderAuthorityAddress: req.FromAddr,
		DstChainOrderAuthorityAddress: req.ToAddr,
		DstChainTokenOutRecipient:     req.ToAddr,
		DstChainTokenOutAmount:        "auto",
	}

	qr, err := p.client.Quote(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapQuote(qr, req)
}

// Status returns the status of a transaction.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	sr, err := p.client.Status(ctx, txID)
	if err != nil {
		return nil, err
	}
	return &domain.Status{
		TxID:  sr.OrderID,
		State: sr.Status,
	}, nil
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}

	toAmt, err := decimal.NewFromString(qr.Estimation.DstChainTokenOut.Amount)
	if err != nil {
		toAmt = decimal.Zero
	}
	fromAmt, err := decimal.NewFromString(qr.Estimation.SrcChainTokenIn.Amount)
	if err != nil {
		fromAmt = decimal.Zero
	}

	var estFee decimal.Decimal
	if !fromAmt.IsZero() && !toAmt.IsZero() && fromAmt.GreaterThan(toAmt) {
		estFee = fromAmt.Sub(toAmt)
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

	quote := &domain.Quote{
		ID:          qr.OrderID,
		FromToken:   mapSrcToken(qr.Estimation.SrcChainTokenIn),
		ToToken:     mapDstToken(qr.Estimation.DstChainTokenOut),
		FromAmount:  fromAmt,
		ToAmount:    toAmt,
		MinAmount:   toAmt.Mul(decimal.NewFromFloat(1 - req.Slippage)),
		Slippage:    req.Slippage,
		Provider:    string(Name),
		Deadline:    time.Now().Add(10 * time.Minute),
		To:          qr.Tx.To,
		TxData:      txData,
		TxValue:     txValue,
		EstimateFee: estFee,
	}

	if qr.Tx.To != "" &&
		!strings.EqualFold(qr.Estimation.SrcChainTokenIn.Address, "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE") &&
		!strings.EqualFold(qr.Estimation.SrcChainTokenIn.Address, "0x0000000000000000000000000000000000000000") {
		quote.ApprovalAddress = qr.Tx.To
		allowance := fromAmt
		quote.AllowanceNeeded = &allowance
	}

	return quote, nil
}

func mapSrcToken(t SrcChainTokenInfo) domain.Token {
	return domain.Token{
		Symbol:   t.Symbol,
		Address:  t.Address,
		Decimals: t.Decimals,
		ChainID:  domain.NumericToChainID(strconv.Itoa(t.ChainID)),
	}
}

func mapDstToken(t DstChainTokenInfo) domain.Token {
	return domain.Token{
		Symbol:   t.Symbol,
		Address:  t.Address,
		Decimals: t.Decimals,
		ChainID:  domain.NumericToChainID(strconv.Itoa(t.ChainID)),
	}
}
