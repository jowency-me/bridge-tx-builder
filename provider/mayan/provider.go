package mayan

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	hexutil "github.com/jowency-me/bridge-tx-builder/utils/hex"
	"github.com/shopspring/decimal"
)

type client interface {
	Quote(ctx context.Context, params QuoteParams) ([]QuoteView, error)
	Build(ctx context.Context, quote json.RawMessage, params BuildParams) (*BuildResponse, error)
}

// Provider adapts the Mayan Transaction Builder API to the domain provider interface.
type Provider struct {
	client client
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainEthereum:  "ethereum",
	domain.ChainBSC:       "bsc",
	domain.ChainPolygon:   "polygon",
	domain.ChainArbitrum:  "arbitrum",
	domain.ChainOptimism:  "optimism",
	domain.ChainAvalanche: "avalanche",
	domain.ChainBase:      "base",
	domain.ChainSolana:    "solana",
}

// numericChainID maps EVM chains to the numeric chain id used by /build's signerChainId.
var numericChainID = map[domain.ChainID]int{
	domain.ChainEthereum:  1,
	domain.ChainBSC:       56,
	domain.ChainPolygon:   137,
	domain.ChainArbitrum:  42161,
	domain.ChainOptimism:  10,
	domain.ChainAvalanche: 43114,
	domain.ChainBase:      8453,
}

// Option configures a Provider.
type Option func(*Provider)

// WithBaseURL points the client at a specific tx-builder instance (e.g. a
// self-hosted one). Defaults to the Mayan-hosted https://tx-builder.mayan.finance.
func WithBaseURL(u string) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithHTTPClient sets the underlying HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.client = hc
		}
	}
}

// WithAPIKey sets the X-API-Key header required by the Mayan-hosted /build
// endpoint. Not needed when pointing WithBaseURL at a self-hosted instance.
func WithAPIKey(key string) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.apiKey = key
		}
	}
}

// NewProvider creates a Mayan provider. Pass WithAPIKey to use the Mayan-hosted
// tx-builder, or WithBaseURL to target a self-hosted instance.
func NewProvider(opts ...Option) *Provider {
	p := &Provider{client: NewClient()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name is the provider identifier.
const Name domain.ProviderName = "mayan"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

// Quote fetches the best route from /quote, then builds the unsigned
// transaction via /build, and maps the result to a domain.Quote.
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

	views, err := p.client.Quote(ctx, QuoteParams{
		AmountIn64:  req.Amount.String(),
		FromToken:   req.FromToken.Address,
		ToToken:     req.ToToken.Address,
		FromChain:   fromCode,
		ToChain:     toCode,
		SlippageBps: strconv.Itoa(int(req.Slippage * 10000)),
		Swift:       true,
		MCTP:        true,
		FastMCTP:    true,
		Wormhole:    true,
	})
	if err != nil {
		return nil, err
	}
	best, err := selectBestQuote(views)
	if err != nil {
		return nil, err
	}

	build, err := p.client.Build(ctx, best.Raw, BuildParams{
		SwapperAddress:     req.FromAddr,
		DestinationAddress: req.ToAddr,
		SignerChainID:      numericChainID[req.FromToken.ChainID],
	})
	if err != nil {
		return nil, err
	}
	return mapQuote(best, build, req)
}

// Status is not supported by the Mayan tx-builder API.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	return nil, fmt.Errorf("%s: status tracking not supported", Name)
}

// selectBestQuote picks the option with the highest expected output. The
// tx-builder /build endpoint accepts any route type, so no type is rejected.
func selectBestQuote(views []QuoteView) (QuoteView, error) {
	bestIdx := -1
	var bestOut decimal.Decimal
	for i, v := range views {
		out, err := decimal.NewFromString(v.ExpectedAmountOutBaseUnits)
		if err != nil {
			continue
		}
		if bestIdx < 0 || out.GreaterThan(bestOut) {
			bestIdx = i
			bestOut = out
		}
	}
	if bestIdx < 0 {
		return QuoteView{}, fmt.Errorf("%s: no quote route with a parseable output amount", Name)
	}
	return views[bestIdx], nil
}

func mapQuote(view QuoteView, build *BuildResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if build == nil {
		return nil, fmt.Errorf("%s: nil build response", Name)
	}
	toAmt, err := decimal.NewFromString(view.ExpectedAmountOutBaseUnits)
	if err != nil || toAmt.IsZero() {
		toAmt = req.Amount
	}
	// Only fall back to the slippage-derived minAmount if the API omitted the
	// value; a present-but-unparseable amount is a server bug we surface.
	var minAmt decimal.Decimal
	if view.MinAmountOutBaseUnits != "" {
		parsed, perr := decimal.NewFromString(view.MinAmountOutBaseUnits)
		if perr != nil {
			return nil, fmt.Errorf("%s: invalid minAmountOutBaseUnits %q: %w", Name, view.MinAmountOutBaseUnits, perr)
		}
		minAmt = parsed
	}
	if minAmt.IsZero() {
		minAmt = toAmt.Mul(decimal.NewFromFloat(1 - req.Slippage)).Floor()
	}
	slippage := req.Slippage
	if slippage == 0 {
		slippage = 0.005
	}
	deadline := time.Now().Add(10 * time.Minute)
	if view.Deadline64 != "" {
		if ts, perr := strconv.ParseInt(view.Deadline64, 10, 64); perr == nil && ts > 0 {
			deadline = time.Unix(ts, 0)
		}
	}

	quote := &domain.Quote{
		ID:         fmt.Sprintf("%s-%s-%s-%s", Name, req.FromToken.ChainID, view.Type, view.ExpectedAmountOutBaseUnits),
		FromToken:  req.FromToken,
		ToToken:    req.ToToken,
		FromAmount: req.Amount,
		ToAmount:   toAmt,
		MinAmount:  minAmt,
		Slippage:   slippage,
		Provider:   string(Name),
		Route: []domain.RouteStep{{
			ChainID:  req.FromToken.ChainID,
			Protocol: "mayan",
			Action:   "bridge",
		}},
		Deadline:    deadline,
		EstimateFee: decimal.Zero,
		GasLimit:    decimal.Zero,
		EstimateGas: decimal.Zero,
	}

	switch build.Transaction.ChainCategory {
	case "svm":
		// Solana/Fogo: the inner transaction is a base64-encoded JSON string.
		var b64 string
		if err := json.Unmarshal(build.Transaction.Transaction, &b64); err != nil {
			return nil, fmt.Errorf("%s: decode svm tx envelope: %w", Name, err)
		}
		decoded, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("%s: decode svm tx base64: %w", Name, err)
		}
		if len(decoded) == 0 {
			return nil, fmt.Errorf("%s: empty svm transaction", Name)
		}
		quote.TxData = decoded
		quote.TxValue = decimal.Zero
		// Solana has no ERC-20 approval.
	default:
		// EVM: the inner transaction is an object {to,data,value,chainId}.
		var tx EVMTx
		if err := json.Unmarshal(build.Transaction.Transaction, &tx); err != nil {
			return nil, fmt.Errorf("%s: decode evm tx: %w", Name, err)
		}
		if tx.To == "" {
			return nil, fmt.Errorf("%s: build returned empty tx.to", Name)
		}
		data, err := hexutil.Decode(strings.TrimPrefix(tx.Data, "0x"))
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
		quote.To = tx.To
		quote.TxData = data
		if tx.Value != "" && tx.Value != "0" {
			if v, verr := decimal.NewFromString(tx.Value); verr == nil {
				quote.TxValue = v
			}
		}
		// The Mayan Forwarder (tx.To) is the spender that must be approved for
		// ERC-20 inputs. Native inputs carry a non-zero TxValue and need no approval.
		if quote.TxValue.IsZero() {
			quote.ApprovalAddress = tx.To
			allowance := req.Amount
			quote.AllowanceNeeded = &allowance
		}
	}
	return quote, nil
}
