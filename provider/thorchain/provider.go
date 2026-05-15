package thorchain

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/shopspring/decimal"
)

type client interface {
	Quote(ctx context.Context, params QuoteParams) (*QuoteResponse, error)
	Status(ctx context.Context, txID string) (*StatusResponse, error)
}

// Provider adapts the thorchain API to the domain provider interface.
type Provider struct {
	client client
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainAvalanche: "AVAX",
	domain.ChainBSC:       "BSC",
	domain.ChainBase:      "BASE",
	domain.ChainBitcoin:   "BTC",
	domain.ChainEthereum:  "ETH",
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
const Name domain.ProviderName = "thorchain"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

// Quote returns a quote for the given swap request.
func (p *Provider) Quote(ctx context.Context, req domain.QuoteRequest) (*domain.Quote, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	fromChain := chainCodes[req.FromToken.ChainID]
	if fromChain == "" {
		return nil, fmt.Errorf("%s: unsupported from chain %s", Name, req.FromToken.ChainID)
	}
	toChain := chainCodes[req.ToToken.ChainID]
	if toChain == "" {
		return nil, fmt.Errorf("%s: unsupported to chain %s", Name, req.ToToken.ChainID)
	}

	fromAsset := toThorchainAsset(fromChain, req.FromToken)
	toAsset := toThorchainAsset(toChain, req.ToToken)

	// Convert amount to 1e8 units
	amount1e8 := convertTo1e8(req.Amount, req.FromToken.Decimals)

	params := QuoteParams{
		FromAsset:   fromAsset,
		ToAsset:     toAsset,
		Amount:      amount1e8,
		Destination: req.ToAddr,
	}

	qr, err := p.client.Quote(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapQuote(qr, req, fromChain, toChain)
}

// Status returns the status of the transaction.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	sr, err := p.client.Status(ctx, txID)
	if err != nil {
		return nil, err
	}
	return mapStatus(sr, txID), nil
}

func toThorchainAsset(chainCode string, token domain.Token) string {
	addr := strings.ToLower(token.Address)
	// Native token indicators: zero address or 1inch native ETH address
	if addr == "0x0000000000000000000000000000000000000000" ||
		addr == "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
		return chainCode + "." + chainCode
	}
	// For BTC, BSC, etc. native tokens, use chain code as symbol
	if strings.EqualFold(token.Symbol, chainCode) ||
		(strings.EqualFold(chainCode, "BSC") && strings.EqualFold(token.Symbol, "BNB")) {
		return chainCode + "." + chainCode
	}
	return chainCode + "." + token.Symbol + "-" + token.Address
}

func convertTo1e8(amount decimal.Decimal, decimals int) string {
	if decimals < 0 || decimals > 18 {
		decimals = 18
	}
	// Convert from token decimals to 1e8
	factor := decimal.NewFromInt(1).Shift(int32(decimals))
	amountInUnits := amount.Div(factor)
	amount1e8 := amountInUnits.Mul(decimal.NewFromInt(100_000_000))
	return amount1e8.Truncate(0).String()
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest, fromChain, toChain string) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	fromAmt := req.Amount

	toAmt, err := decimal.NewFromString(qr.ExpectedAmountOut)
	if err != nil {
		toAmt = decimal.Zero
	}

	var feeAmt decimal.Decimal
	if qr.Fees.Total != "" {
		f, err := decimal.NewFromString(qr.Fees.Total)
		if err == nil {
			feeAmt = f
		}
	}

	slippage := float64(qr.SlippageBps) / 10000.0
	if slippage == 0 {
		slippage = req.Slippage
	}

	minAmount := toAmt.Mul(decimal.NewFromFloat(1 - slippage))
	if minAmount.IsNegative() || minAmount.IsZero() {
		minAmount = toAmt.Mul(decimal.NewFromFloat(1 - req.Slippage))
	}

	var deadline time.Time
	if qr.Expiry > 0 {
		deadline = time.Unix(qr.Expiry, 0)
	} else {
		deadline = time.Now().Add(10 * time.Minute)
	}

	route := []domain.RouteStep{
		{
			ChainID:  req.FromToken.ChainID,
			Protocol: "thorchain",
			Action:   "swap",
		},
		{
			ChainID:  req.ToToken.ChainID,
			Protocol: "thorchain",
			Action:   "bridge",
		},
	}

	// M-05: THORChain deposits use InboundAddress + Memo.
	// For EVM chains the memo goes as calldata data.
	var txData []byte
	if qr.Memo != "" {
		txData = []byte(qr.Memo)
	}

	quote := &domain.Quote{
		ID:          fmt.Sprintf("thorchain-%s-%s", qr.Memo, qr.ExpectedAmountOut),
		FromToken:   req.FromToken,
		ToToken:     req.ToToken,
		FromAmount:  fromAmt,
		ToAmount:    toAmt,
		MinAmount:   minAmount,
		Slippage:    slippage,
		Provider:    string(Name),
		Route:       route,
		Deadline:    deadline,
		To:          qr.InboundAddress,
		TxData:      txData,
		EstimateFee: feeAmt,
	}

	// ERC-20 deposits require approval to the vault inbound address.
	fromAddr := strings.ToLower(req.FromToken.Address)
	if qr.InboundAddress != "" && fromAddr != "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" &&
		fromAddr != "0x0000000000000000000000000000000000000000" {
		quote.ApprovalAddress = qr.InboundAddress
		allowance := fromAmt
		quote.AllowanceNeeded = &allowance
	}

	return quote, nil
}

func mapStatus(sr *StatusResponse, txID string) *domain.Status {
	if sr == nil {
		return &domain.Status{TxID: txID, State: "unknown"}
	}
	state := "pending"
	if sr.Stages.InboundFinalised.Completed && sr.Stages.SwapFinalised.Completed && !sr.Stages.SwapStatus.Pending {
		state = "completed"
	} else if sr.Stages.InboundFinalised.Completed {
		state = "executing"
	}

	var dstChainTx string
	if len(sr.OutTxs) > 0 && sr.OutTxs[0].ID != "" && sr.OutTxs[0].ID != "0000000000000000000000000000000000000000000000000000000000000000" {
		dstChainTx = sr.OutTxs[0].ID
	}

	return &domain.Status{
		TxID:       txID,
		State:      state,
		SrcChainTx: sr.Tx.ID,
		DstChainTx: dstChainTx,
		UpdatedAt:  time.Now(),
	}
}
