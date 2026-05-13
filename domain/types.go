package domain

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

// Token represents a fungible token on a specific chain.
type Token struct {
	Symbol   string
	Address  string // contract address or mint address
	Decimals int
	ChainID  ChainID
}

// Validate checks token fields.
func (t Token) Validate() error {
	if t.Symbol == "" {
		return errors.New("token symbol required")
	}
	if t.Address == "" {
		return errors.New("token address required")
	}
	if t.Decimals < 0 || t.Decimals > 18 {
		return errors.New("token decimals must be 0-18")
	}
	if !t.ChainID.Valid() {
		return errors.New("invalid token chain id")
	}
	return nil
}

// RouteStep describes one leg of a cross-chain route.
type RouteStep struct {
	ChainID  ChainID
	Protocol string
	Action   string // "swap", "bridge", "wrap", "unwrap"
}

// Quote represents a cross-chain swap/bridge quote from a provider.
type Quote struct {
	ID          string
	FromToken   Token
	ToToken     Token
	FromAmount  decimal.Decimal // in wei / lamports / sun
	ToAmount    decimal.Decimal // expected output in destination units
	MinAmount   decimal.Decimal // minimum output after slippage
	Slippage    float64         // e.g. 0.005 = 0.5%
	Provider    string
	Route       []RouteStep
	Deadline    time.Time
	To          string          // target contract / router address
	TxData      []byte          // chain-specific call data (EVM calldata, Solana instruction data, Tron contract data)
	TxValue     decimal.Decimal // native value to send
	EstimateGas uint64
	EstimateFee decimal.Decimal
	GasPrice    decimal.Decimal // for legacy EVM transactions
	GasTipCap   decimal.Decimal // for EIP-1559 priority fee
	GasFeeCap   decimal.Decimal // for EIP-1559 max fee
	BlockHash   string          // recent block hash (Solana, Tron, etc.)
	BlockHeight uint64          // block height for ref block derivation (Tron, etc.)
	Nonce       *uint64         // optional nonce for EVM transactions (nil = caller manages)
}

// Validate checks quote fields.
func (q Quote) Validate() error {
	if q.ID == "" {
		return errors.New("quote id required")
	}
	if err := q.FromToken.Validate(); err != nil {
		return errors.New("from token invalid: " + err.Error())
	}
	if err := q.ToToken.Validate(); err != nil {
		return errors.New("to token invalid: " + err.Error())
	}
	if q.FromAmount.IsZero() || q.FromAmount.IsNegative() {
		return errors.New("from amount must be positive")
	}
	if q.ToAmount.IsZero() || q.ToAmount.IsNegative() {
		return errors.New("to amount must be positive")
	}
	if q.MinAmount.IsZero() || q.MinAmount.IsNegative() {
		return errors.New("min amount must be positive")
	}
	if q.Slippage < 0 || q.Slippage > 0.05 {
		return errors.New("slippage must be 0-5%")
	}
	if q.Provider == "" {
		return errors.New("provider required")
	}
	if q.TxValue.IsNegative() {
		return errors.New("tx value must not be negative")
	}
	return nil
}

// QuoteRequest is the input for fetching a quote.
type QuoteRequest struct {
	FromToken Token
	ToToken   Token
	Amount    decimal.Decimal
	Slippage  float64
	FromAddr  string
	ToAddr    string
}

// Validate checks request fields.
func (r QuoteRequest) Validate() error {
	if err := r.FromToken.Validate(); err != nil {
		return err
	}
	if err := r.ToToken.Validate(); err != nil {
		return err
	}
	if r.Amount.IsZero() || r.Amount.IsNegative() {
		return errors.New("amount must be positive")
	}
	if r.Slippage < 0 || r.Slippage > 0.05 {
		return errors.New("slippage must be 0-5%")
	}
	if r.FromAddr == "" {
		return errors.New("from address required")
	}
	if r.ToAddr == "" {
		return errors.New("to address required")
	}
	return nil
}

// Transaction is the chain-agnostic transaction envelope.
// Chain-specific metadata is serialized into Data by the respective ChainBuilder.
// Consumers (simulators, broadcasters) dispatch by ChainID.
type Transaction struct {
	ChainID ChainID
	From    string
	To      string
	Data    []byte // chain-specific serialized transaction (calldata, RLP, protobuf, etc.)
	Value   decimal.Decimal
	Gas     uint64
	Nonce   uint64 // transaction nonce (EVM) or sequence number
}

// Validate checks transaction fields.
func (tx Transaction) Validate() error {
	if !tx.ChainID.Valid() {
		return errors.New("invalid chain id")
	}
	if tx.From == "" {
		return errors.New("from address required")
	}
	if tx.To == "" {
		return errors.New("to address required")
	}
	if tx.Value.IsNegative() {
		return errors.New("tx value must not be negative")
	}
	return nil
}

// Status tracks the state of a cross-chain transfer.
type Status struct {
	TxID       string
	State      string // "pending", "completed", "failed"
	SrcChainTx string
	DstChainTx string
	Error      string
	UpdatedAt  time.Time
}

// SimulationResult holds the outcome of a dry-run.
type SimulationResult struct {
	Success      bool
	GasUsed      uint64
	RevertReason string
	Logs         []string
}

// Provider is the abstraction for cross-chain quote providers.
type Provider interface {
	Name() string
	Quote(ctx context.Context, req QuoteRequest) (*Quote, error)
	Status(ctx context.Context, txID string) (*Status, error)
}

// ChainBuilder builds raw transactions for a specific chain.
type ChainBuilder interface {
	ChainID() ChainID
	Build(ctx context.Context, quote Quote, from string, privateKey []byte) (*Transaction, error)
}

// Simulator dry-runs a transaction without broadcasting.
type Simulator interface {
	Simulate(ctx context.Context, tx *Transaction) (*SimulationResult, error)
}
