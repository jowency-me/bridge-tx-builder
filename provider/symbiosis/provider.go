package symbiosis

import (
	"context"
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
	Quote(ctx context.Context, req QuoteRequest) (*QuoteResponse, error)
}

type Provider struct {
	client client
}

var chainCodes = map[domain.ChainID]string{
	domain.ChainEthereum:  "1",
	domain.ChainBase:      "8453",
	domain.ChainBSC:       "56",
	domain.ChainPolygon:   "137",
	domain.ChainArbitrum:  "42161",
	domain.ChainOptimism:  "10",
	domain.ChainAvalanche: "43114",
	domain.ChainTron:      "728126428",
}

type Option func(*Provider)

func WithBaseURL(u string) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.baseURL = strings.TrimRight(u, "/")
		}
	}
}

func WithHTTPClient(hc *http.Client) Option {
	return func(p *Provider) {
		if c, ok := p.client.(*Client); ok {
			c.client = hc
		}
	}
}

func NewProvider(opts ...Option) *Provider {
	p := &Provider{client: NewClient()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name is the provider identifier.
const Name domain.ProviderName = "symbiosis"

// Name returns the provider name.
func (p *Provider) Name() string { return string(Name) }

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
	apiReq := QuoteRequest{
		TokenAmountIn: TokenAmount{
			Symbol:   req.FromToken.Symbol,
			Address:  req.FromToken.Address,
			ChainID:  parseChainCode(fromCode),
			Decimals: req.FromToken.Decimals,
			Amount:   req.Amount.String(),
		},
		TokenOut: TokenAmount{
			Symbol:   req.ToToken.Symbol,
			Address:  req.ToToken.Address,
			ChainID:  parseChainCode(toCode),
			Decimals: req.ToToken.Decimals,
		},
		From:     req.FromAddr,
		To:       req.ToAddr,
		Slippage: int(req.Slippage * 10000),
	}
	resp, err := p.client.Quote(ctx, apiReq)
	if err != nil {
		return nil, err
	}
	return mapQuote(resp, req)
}

func (p *Provider) Status(ctx context.Context, txID string) (*domain.Status, error) {
	return nil, fmt.Errorf("%s: status tracking not supported", Name)
}

func parseChainCode(code string) int {
	n, _ := strconv.Atoi(code)
	return n
}

func mapQuote(qr *QuoteResponse, req domain.QuoteRequest) (*domain.Quote, error) {
	if qr == nil {
		return nil, fmt.Errorf("%s: empty quote response", Name)
	}
	fromAmt := req.Amount
	toAmtDec, err := decimal.NewFromString(qr.TokenAmountOut.Amount)
	if err != nil || toAmtDec.IsZero() {
		toAmtDec = fromAmt
	}
	// Only fall back to the slippage-derived minAmount if the API explicitly returned
	// zero (or omitted) — not when the value is unparseable. A malformed amount is a
	// server bug; silently widening the user's acceptance window would risk money loss.
	var minAmtDec decimal.Decimal
	if qr.TokenAmountOutMin.Amount != "" {
		if parsed, perr := decimal.NewFromString(qr.TokenAmountOutMin.Amount); perr == nil {
			minAmtDec = parsed
		} else {
			return nil, fmt.Errorf("%s: invalid TokenAmountOutMin.Amount %q: %w", Name, qr.TokenAmountOutMin.Amount, perr)
		}
	}
	if minAmtDec.IsZero() {
		minAmtDec = toAmtDec.Mul(decimal.NewFromFloat(1 - req.Slippage)).Floor()
	}
	slippage := req.Slippage
	if slippage == 0 {
		slippage = 0.005
	}
	// The /v1/swap response carries no quote id or expiry timestamp; derive a
	// stable id from the source token and the bridge target, and default the
	// deadline to 10 minutes out.
	deadline := time.Now().Add(10 * time.Minute)
	quote := &domain.Quote{
		ID:         fmt.Sprintf("%s-%s-%s-%s", Name, req.FromToken.ChainID, req.FromToken.Address, qr.Tx.To),
		FromToken:  req.FromToken,
		ToToken:    req.ToToken,
		FromAmount: fromAmt,
		ToAmount:   toAmtDec,
		MinAmount:  minAmtDec,
		Slippage:   slippage,
		Provider:   string(Name),
		Route: []domain.RouteStep{{
			ChainID:  req.FromToken.ChainID,
			Protocol: "symbiosis",
			Action:   "bridge",
		}},
		Deadline:    deadline,
		EstimateFee: decimal.Zero,
		GasLimit:    decimal.Zero,
		EstimateGas: decimal.Zero,
	}
	switch req.FromToken.ChainID {
	case domain.ChainTron:
		quote.To = qr.Tx.To
		// Symbiosis returns a JSON envelope for Tron: {contract_address, function_selector, parameter, call_value, fee_limit}.
		// The actual calldata is in the `parameter` field (hex). Decode it; if the envelope is malformed (not JSON,
		// missing parameter, or non-hex parameter) treat it as a server bug and surface the error rather than
		// silently storing the raw JSON envelope bytes (which would make the downstream Tron builder sign garbage).
		txData, ok := decodeSymbiosisTronData(qr.Tx.Data)
		if !ok {
			return nil, fmt.Errorf("%s: tron tx data is not a valid JSON envelope with hex parameter field", Name)
		}
		quote.TxData = txData
		if qr.Tx.Value != "" && qr.Tx.Value != "0" {
			if v, err := decimal.NewFromString(qr.Tx.Value); err == nil {
				quote.TxValue = v
			}
		}
		if qr.ApproveTo != "" {
			quote.ApprovalAddress = qr.ApproveTo
			allowance := req.Amount
			quote.AllowanceNeeded = &allowance
		}
	default:
		quote.To = qr.Tx.To
		hexData := strings.TrimPrefix(qr.Tx.Data, "0x")
		if hexData != "" {
			decoded, err := hexutil.Decode(hexData)
			if err != nil {
				return nil, fmt.Errorf("%s: invalid tx data: %w", Name, err)
			}
			quote.TxData = decoded
		}
		if qr.Tx.Value != "" && qr.Tx.Value != "0" {
			if v, err := decimal.NewFromString(qr.Tx.Value); err == nil {
				quote.TxValue = v
			}
		}
		if qr.ApproveTo != "" {
			quote.ApprovalAddress = qr.ApproveTo
			allowance := req.Amount
			quote.AllowanceNeeded = &allowance
		}
	}
	return quote, nil
}

// decodeSymbiosisTronData extracts the hex `parameter` field from a Symbiosis
// Tron JSON envelope. Returns (bytes, true) on success. If the input is not
// JSON, or the parameter field is missing/non-hex, returns (nil, false) so the
// caller can fall back to the raw bytes.
func decodeSymbiosisTronData(raw string) ([]byte, bool) {
	var env struct {
		Parameter string `json:"parameter"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return nil, false
	}
	if env.Parameter == "" {
		return nil, false
	}
	decoded, err := hexutil.Decode(strings.TrimPrefix(env.Parameter, "0x"))
	if err != nil {
		return nil, false
	}
	return decoded, true
}
