package allbridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	hexutil "github.com/jowency-me/bridge-tx-builder/utils/hex"
	"github.com/shopspring/decimal"
)

// defaultMessenger is the Allbridge stablecoin AMM route.
const defaultMessenger = "ALLBRIDGE"

// defaultFeePaymentMethod pays the cross-chain gas fee with the source chain's
// native currency (the most common integration default).
const defaultFeePaymentMethod = "WITH_NATIVE_CURRENCY"

type client interface {
	Calculate(ctx context.Context, params CalcParams) (*BridgeAmounts, error)
	RawBridge(ctx context.Context, params BridgeParams) (json.RawMessage, error)
}

// Provider adapts the Allbridge Core REST API to the domain provider interface.
type Provider struct {
	client    client
	messenger string
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainEthereum:  "ETH",
	domain.ChainBSC:       "BSC",
	domain.ChainPolygon:   "POL",
	domain.ChainArbitrum:  "ARB",
	domain.ChainOptimism:  "OPT",
	domain.ChainAvalanche: "AVA",
	domain.ChainBase:      "BAS",
	domain.ChainSolana:    "SOL",
	domain.ChainTron:      "TRX",
}

// Option configures a Provider.
type Option func(*Provider)

// WithBaseURL points the client at a specific Allbridge Core REST API instance.
// Defaults to the self-hosted http://localhost:3000.
func WithBaseURL(u string) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithHTTPClient configures the provider's underlying HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.client = hc
		}
	}
}

// WithMessenger overrides the Allbridge messenger (route) used. Defaults to
// "ALLBRIDGE" (the stablecoin AMM); other values include "WORMHOLE", "CCTP".
func WithMessenger(m string) Option {
	return func(p *Provider) { p.messenger = m }
}

// NewProvider configures a new Allbridge Core provider. Point WithBaseURL at a
// running Allbridge Core REST API container (self-hosted Docker).
func NewProvider(opts ...Option) *Provider {
	p := &Provider{client: NewClient(), messenger: defaultMessenger}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name is the provider identifier.
const Name domain.ProviderName = "allbridge"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

// Quote calculates the received amount, builds the raw bridge transaction, and
// maps the result to a domain.Quote. Amounts are converted between the domain's
// base units and the Allbridge API's float representation.
func (p *Provider) Quote(ctx context.Context, req domain.QuoteRequest) (*domain.Quote, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if chainCodes[req.FromToken.ChainID] == "" {
		return nil, fmt.Errorf("%s: unsupported from chain %s", Name, req.FromToken.ChainID)
	}
	if chainCodes[req.ToToken.ChainID] == "" {
		return nil, fmt.Errorf("%s: unsupported to chain %s", Name, req.ToToken.ChainID)
	}

	// The Allbridge Core REST API takes integer base units for `amount` and
	// converts to float internally (convertGt0IntAmountToFloat in the controller),
	// so the domain's base-unit amount is passed through verbatim.
	amountBaseUnits := req.Amount.String()

	calc, err := p.client.Calculate(ctx, CalcParams{
		Amount:           amountBaseUnits,
		SourceToken:      req.FromToken.Address,
		DestinationToken: req.ToToken.Address,
		Messenger:        p.messenger,
	})
	if err != nil {
		return nil, err
	}

	raw, err := p.client.RawBridge(ctx, BridgeParams{
		Amount:           amountBaseUnits,
		Sender:           req.FromAddr,
		Recipient:        req.ToAddr,
		SourceToken:      req.FromToken.Address,
		DestinationToken: req.ToToken.Address,
		Messenger:        p.messenger,
		FeePaymentMethod: defaultFeePaymentMethod,
		OutputFormat:     outputFormatFor(req.FromToken.ChainID),
	})
	if err != nil {
		return nil, err
	}

	return p.mapQuote(calc, raw, req)
}

// Status is not supported by the Allbridge Core REST API in this adapter.
func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	return nil, fmt.Errorf("%s: status tracking not supported", Name)
}

// outputFormatFor selects the /raw/bridge outputFormat per source chain: EVM and
// Tron return a JSON transaction object; Solana returns a base64 string.
func outputFormatFor(chain domain.ChainID) string {
	if chain == domain.ChainSolana {
		return "base64"
	}
	return "json"
}

func (p *Provider) mapQuote(calc *BridgeAmounts, raw json.RawMessage, req domain.QuoteRequest) (*domain.Quote, error) {
	if calc == nil {
		return nil, fmt.Errorf("%s: empty calculate response", Name)
	}
	// Convert the received float amount back to base units of the destination token.
	toAmt := req.Amount
	if calc.AmountReceivedFloat != "" {
		parsed, perr := decimal.NewFromString(calc.AmountReceivedFloat)
		if perr != nil {
			return nil, fmt.Errorf("%s: invalid amountReceivedInFloat %q: %w", Name, calc.AmountReceivedFloat, perr)
		}
		if parsed.IsPositive() {
			toAmt = parsed.Shift(int32(req.ToToken.Decimals))
		}
	}
	minAmt := toAmt.Mul(decimal.NewFromFloat(1 - req.Slippage)).Floor()
	slippage := req.Slippage
	if slippage == 0 {
		slippage = 0.005
	}

	q := &domain.Quote{
		ID:         fmt.Sprintf("%s-%s-%s-%s", Name, req.FromToken.ChainID, req.FromToken.Address, calc.AmountReceivedFloat),
		FromToken:  req.FromToken,
		ToToken:    req.ToToken,
		FromAmount: req.Amount,
		ToAmount:   toAmt,
		MinAmount:  minAmt,
		Slippage:   slippage,
		Provider:   string(Name),
		Route: []domain.RouteStep{{
			ChainID:  req.FromToken.ChainID,
			Protocol: "allbridge",
			Action:   "bridge",
		}},
		Deadline:    time.Now().Add(10 * time.Minute),
		GasLimit:    decimal.Zero,
		EstimateFee: decimal.Zero,
		EstimateGas: decimal.Zero,
	}

	switch req.FromToken.ChainID {
	case domain.ChainSolana:
		// /raw/bridge with outputFormat=base64 returns a JSON string (base64 tx).
		var b64 string
		if err := json.Unmarshal(raw, &b64); err != nil {
			return nil, fmt.Errorf("%s: decode solana tx envelope: %w", Name, err)
		}
		decoded, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("%s: decode solana tx base64: %w", Name, err)
		}
		if len(decoded) == 0 {
			return nil, fmt.Errorf("%s: empty solana transaction", Name)
		}
		q.TxData = decoded
		q.TxValue = decimal.Zero
		// Solana has no ERC-20 approval.
	case domain.ChainTron:
		// Tron returns a full transaction object; pass it through as JSON for the
		// Tron builder, and surface the contract address as the target.
		var tron map[string]any
		if err := json.Unmarshal(raw, &tron); err != nil {
			return nil, fmt.Errorf("%s: decode tron tx: %w", Name, err)
		}
		contractAddr, _ := tron["contract_address"].(string)
		if contractAddr == "" {
			return nil, fmt.Errorf("%s: tron tx missing contract_address", Name)
		}
		q.To = contractAddr
		q.TxData = raw
		q.ApprovalAddress = contractAddr
		allowance := req.Amount
		q.AllowanceNeeded = &allowance
	default:
		// EVM: {from, to, value, data}.
		var tx EVMRawTransaction
		if err := json.Unmarshal(raw, &tx); err != nil {
			return nil, fmt.Errorf("%s: decode evm tx: %w", Name, err)
		}
		if tx.To == "" {
			return nil, fmt.Errorf("%s: evm tx missing to address", Name)
		}
		data, err := hexutil.Decode(strings.TrimPrefix(tx.Data, "0x"))
		if err != nil {
			return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
		}
		q.To = tx.To
		q.TxData = data
		// A non-zero tx.value here is the cross-chain messenger fee paid in the
		// native gas token (feePaymentMethod=WITH_NATIVE_CURRENCY), NOT the bridged
		// amount. Allbridge Core bridges ERC-20 stablecoins, so the source token
		// always requires approval to the bridge contract (tx.To) regardless of the
		// native fee value attached.
		if tx.Value != "" && tx.Value != "0" {
			if v, verr := decimal.NewFromString(tx.Value); verr == nil {
				q.TxValue = v
			}
		}
		q.ApprovalAddress = tx.To
		allowance := req.Amount
		q.AllowanceNeeded = &allowance
	}
	return q, nil
}
