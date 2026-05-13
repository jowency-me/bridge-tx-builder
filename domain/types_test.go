package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainID_Valid(t *testing.T) {
	tests := []struct {
		name string
		c    ChainID
		want bool
	}{
		{"ethereum", ChainEthereum, true},
		{"base", ChainBase, true},
		{"solana", ChainSolana, true},
		{"tron", ChainTron, true},
		{"unknown", ChainID("unknown"), true},
		{"empty", ChainID(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.c.Valid())
		})
	}
}

func TestToken_Validate(t *testing.T) {
	tests := []struct {
		name    string
		token   Token
		wantErr string
	}{
		{
			name:  "valid",
			token: Token{Symbol: "USDC", Address: "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C", Decimals: 6, ChainID: ChainEthereum},
		},
		{
			name:    "missing symbol",
			token:   Token{Address: "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C", Decimals: 6, ChainID: ChainEthereum},
			wantErr: "token symbol required",
		},
		{
			name:    "missing address",
			token:   Token{Symbol: "USDC", Decimals: 6, ChainID: ChainEthereum},
			wantErr: "token address required",
		},
		{
			name:    "decimals too high",
			token:   Token{Symbol: "USDC", Address: "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C", Decimals: 19, ChainID: ChainEthereum},
			wantErr: "token decimals must be 0-18",
		},
		{
			name:    "empty chain",
			token:   Token{Symbol: "USDC", Address: "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C", Decimals: 6, ChainID: ChainID("")},
			wantErr: "invalid token chain id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.token.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestQuote_Validate(t *testing.T) {
	validToken := Token{Symbol: "USDC", Address: "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C", Decimals: 6, ChainID: ChainEthereum}
	baseQuote := Quote{
		ID:         "q1",
		FromToken:  validToken,
		ToToken:    validToken,
		FromAmount: decimal.NewFromInt(1_000_000),
		ToAmount:   decimal.NewFromInt(999_000),
		MinAmount:  decimal.NewFromInt(995_000),
		Slippage:   0.005,
		Provider:   "lifi",
		Deadline:   time.Now().Add(time.Minute * 10),
	}

	tests := []struct {
		name    string
		quote   Quote
		wantErr string
	}{
		{"valid", baseQuote, ""},
		{"missing id", func() Quote { q := baseQuote; q.ID = ""; return q }(), "quote id required"},
		{"invalid from token", func() Quote { q := baseQuote; q.FromToken.Symbol = ""; return q }(), "from token invalid"},
		{"invalid to token", func() Quote { q := baseQuote; q.ToToken.Symbol = ""; return q }(), "to token invalid"},
		{"zero from amount", func() Quote { q := baseQuote; q.FromAmount = decimal.Zero; return q }(), "from amount must be positive"},
		{"nil-ish from amount", func() Quote { q := baseQuote; q.FromAmount = decimal.Decimal{}; return q }(), "from amount must be positive"},
		{"zero to amount", func() Quote { q := baseQuote; q.ToAmount = decimal.Zero; return q }(), "to amount must be positive"},
		{"slippage too high", func() Quote { q := baseQuote; q.Slippage = 0.06; return q }(), "slippage must be 0-5%"},
		{"missing provider", func() Quote { q := baseQuote; q.Provider = ""; return q }(), "provider required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.quote.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestQuoteRequest_Validate(t *testing.T) {
	validToken := Token{Symbol: "USDC", Address: "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C", Decimals: 6, ChainID: ChainEthereum}
	base := QuoteRequest{
		FromToken: validToken,
		ToToken:   validToken,
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0x1234567890123456789012345678901234567890",
		ToAddr:    "0x0987654321098765432109876543210987654321",
	}

	tests := []struct {
		name    string
		req     QuoteRequest
		wantErr string
	}{
		{"valid", base, ""},
		{"missing from addr", func() QuoteRequest { r := base; r.FromAddr = ""; return r }(), "from address required"},
		{"missing to addr", func() QuoteRequest { r := base; r.ToAddr = ""; return r }(), "to address required"},
		{"zero amount", func() QuoteRequest { r := base; r.Amount = decimal.Zero; return r }(), "amount must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNumericID(t *testing.T) {
	tests := []struct {
		chain ChainID
		want  int64
	}{
		{ChainEthereum, 1},
		{ChainBase, 8453},
		{ChainSolana, 101},
		{ChainTron, 728126428},
		{ChainID("unknown"), 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.chain), func(t *testing.T) {
			assert.Equal(t, tt.want, NumericID(tt.chain))
		})
	}
}

func TestIsEVM(t *testing.T) {
	tests := []struct {
		chain ChainID
		want  bool
	}{
		{ChainEthereum, true},
		{ChainBase, true},
		{ChainSolana, false},
		{ChainTron, false},
		{ChainID("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.chain), func(t *testing.T) {
			assert.Equal(t, tt.want, IsEVM(tt.chain))
		})
	}
}

func TestSupportsEIP1559(t *testing.T) {
	tests := []struct {
		chain ChainID
		want  bool
	}{
		{ChainEthereum, true},
		{ChainBase, true},
		{ChainPolygon, true},
		{ChainArbitrum, true},
		{ChainOptimism, true},
		{ChainAvalanche, true},
		{ChainBSC, false},
		{ChainSolana, false},
		{ChainTron, false},
		{ChainID("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.chain), func(t *testing.T) {
			assert.Equal(t, tt.want, SupportsEIP1559(tt.chain))
		})
	}
}

func TestNumericToChainID(t *testing.T) {
	tests := []struct {
		input string
		want  ChainID
	}{
		{"1", ChainEthereum},
		{"8453", ChainBase},
		{"101", ChainSolana},
		{"728126428", ChainTron},
		{"56", ChainBSC},
		{"137", ChainPolygon},
		{"42161", ChainArbitrum},
		{"10", ChainOptimism},
		{"43114", ChainAvalanche},
		{"not-a-number", ""},
		{"999999", ChainID("999999")},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, NumericToChainID(tt.input))
		})
	}
}

func TestTransaction_Validate(t *testing.T) {
	tests := []struct {
		name    string
		tx      Transaction
		wantErr string
	}{
		{
			name: "valid evm",
			tx: Transaction{
				ChainID: ChainEthereum,
				From:    "0x1234567890123456789012345678901234567890",
				To:      "0x0987654321098765432109876543210987654321",
				Value:   decimal.Zero,
			},
		},
		{
			name:    "empty chain",
			tx:      Transaction{ChainID: ChainID(""), From: "0x123...", To: "0x098..."},
			wantErr: "invalid chain id",
		},
		{
			name:    "missing from",
			tx:      Transaction{ChainID: ChainEthereum, To: "0x098..."},
			wantErr: "from address required",
		},
		{
			name:    "missing to",
			tx:      Transaction{ChainID: ChainEthereum, From: "0x123..."},
			wantErr: "to address required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tx.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}
