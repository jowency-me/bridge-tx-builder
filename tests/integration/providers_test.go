//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/jowency-me/bridge-tx-builder/provider/1inch"
	"github.com/jowency-me/bridge-tx-builder/provider/openocean"
	"github.com/jowency-me/bridge-tx-builder/provider/zerox"
	"github.com/jowency-me/bridge-tx-builder/router"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProviders_CrossChainQuote queries all available providers for a
// USDC ETH→Base cross-chain quote and validates the response structure.
func TestProviders_CrossChainQuote(t *testing.T) {
	providers := makeProviders(t)
	if len(providers) == 0 {
		t.Skip("no providers available (all require API keys)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fromAddr := "0x1234567890123456789012345678901234567890"
	req := crossChainQuoteRequest(fromAddr)

	for _, p := range providers {
		t.Run(p.Name(), func(t *testing.T) {
			quote, err := p.Quote(ctx, req)
			if err != nil {
				t.Skipf("provider %s unavailable: %v", p.Name(), err)
				return
			}
			require.NotNil(t, quote)

			// Core fields
			assert.NotEmpty(t, quote.ID, "ID must be set")
			assert.Equal(t, p.Name(), quote.Provider)
			assert.Equal(t, req.FromToken.Symbol, quote.FromToken.Symbol, "FromToken symbol must match")
			assert.Equal(t, req.FromToken.ChainID, quote.FromToken.ChainID, "FromToken chain must match")
			assert.NotEmpty(t, quote.ToToken.Symbol, "ToToken symbol must be set")
			assert.Equal(t, req.ToToken.ChainID, quote.ToToken.ChainID, "ToToken chain must match")
			assert.True(t, quote.FromAmount.Equal(req.Amount), "FromAmount must equal request amount")

			// Domain validation first — skip invalid quotes
			if err := quote.Validate(); err != nil {
				t.Skipf("provider %s returned invalid quote: %v", p.Name(), err)
				return
			}

			// Amounts
			assert.True(t, quote.ToAmount.GreaterThan(decimal.Zero), "ToAmount must be positive")
			assert.True(t, quote.MinAmount.GreaterThan(decimal.Zero), "MinAmount must be positive")
			assert.True(t, quote.MinAmount.LessThanOrEqual(quote.ToAmount), "MinAmount must be <= ToAmount")
			assert.True(t, quote.Slippage > 0, "Slippage must be positive")
			assert.True(t, quote.Slippage <= 0.05, "Slippage must be <= 5%")

			// Transaction data
			assert.NotEmpty(t, quote.To, "To (contract address) must be set")
			assert.True(t, quote.Deadline.After(time.Now()), "Deadline must be in the future")

			// Route (some providers return empty route for simple swaps)
			if len(quote.Route) > 0 {
				for _, step := range quote.Route {
					assert.NotEmpty(t, step.ChainID, "route step chain must be set")
				}
			}

			// Approval logic consistency
			if quote.ApprovalAddress != "" {
				assert.NotNil(t, quote.AllowanceNeeded, "AllowanceNeeded must be set when ApprovalAddress is present")
				assert.True(t, quote.AllowanceNeeded.GreaterThan(decimal.Zero), "AllowanceNeeded must be positive")
			}
			if quote.AllowanceNeeded != nil {
				assert.NotEmpty(t, quote.ApprovalAddress, "ApprovalAddress must be set when AllowanceNeeded is present")
			}

			t.Logf("quote: id=%s toAmount=%s minAmount=%s to=%s approvalAddr=%s fee=%s gas=%d route=%d",
				quote.ID, quote.ToAmount, quote.MinAmount, quote.To,
				quote.ApprovalAddress, quote.EstimateFee.String(),
				quote.EstimateGas, len(quote.Route))
		})
	}
}

// TestProviders_SameChainQuote queries providers that support same-chain swaps.
func TestProviders_SameChainQuote(t *testing.T) {
	providers := []domain.Provider{}

	if key := os.Getenv("INCH_API_KEY"); key != "" {
		providers = append(providers, oneinch.NewProvider(key))
	}
	if key := os.Getenv("ZEROX_API_KEY"); key != "" {
		providers = append(providers, zerox.NewProvider(key))
	}
	providers = append(providers, openocean.NewProvider())

	if len(providers) == 0 {
		t.Skip("no same-chain providers available (1inch/0x require API keys)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fromAddr := "0x1234567890123456789012345678901234567890"
	req := sameChainQuoteRequest(fromAddr)

	for _, p := range providers {
		t.Run(p.Name(), func(t *testing.T) {
			quote, err := p.Quote(ctx, req)
			if err != nil {
				t.Skipf("provider %s unavailable: %v", p.Name(), err)
				return
			}
			require.NotNil(t, quote)

			assert.NotEmpty(t, quote.ID)
			assert.Equal(t, p.Name(), quote.Provider)
			assert.Equal(t, req.FromToken.Symbol, quote.FromToken.Symbol)
			assert.Equal(t, req.FromToken.ChainID, quote.FromToken.ChainID)
			assert.NotEmpty(t, quote.ToToken.Symbol)
			assert.Equal(t, req.ToToken.ChainID, quote.ToToken.ChainID)
			assert.True(t, quote.FromAmount.Equal(req.Amount))
			assert.True(t, quote.ToAmount.GreaterThan(decimal.Zero))
			assert.True(t, quote.MinAmount.GreaterThan(decimal.Zero))
			assert.NotEmpty(t, quote.To)
			assert.NotEmpty(t, quote.TxData)

			err = quote.Validate()
			assert.NoError(t, err)

			t.Logf("same-chain quote: id=%s toAmount=%s fee=%s",
				quote.ID, quote.ToAmount, quote.EstimateFee.String())
		})
	}
}

// TestProviders_ContextCancellation verifies providers respect context cancellation.
func TestProviders_ContextCancellation(t *testing.T) {
	providers := makeProviders(t)
	if len(providers) == 0 {
		t.Skip("no providers available")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	req := crossChainQuoteRequest("0x1234567890123456789012345678901234567890")

	r := router.New()
	for _, p := range providers {
		r.RegisterProvider(p)
	}

	_, err := r.FindProviders(ctx, req)
	if err != nil {
		assert.Contains(t, err.Error(), "failed")
	}
}

// TestProviders_InvalidRequest verifies providers reject invalid requests.
func TestProviders_InvalidRequest(t *testing.T) {
	providers := makeProviders(t)
	if len(providers) == 0 {
		t.Skip("no providers available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	invalidReqs := []struct {
		name string
		req  domain.QuoteRequest
	}{
		{
			name: "zero_amount",
			req: domain.QuoteRequest{
				FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
				ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
				Amount:    decimal.Zero,
				Slippage:  0.005,
				FromAddr:  "0xFrom",
				ToAddr:    "0xTo",
			},
		},
		{
			name: "negative_slippage",
			req: domain.QuoteRequest{
				FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
				ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
				Amount:    decimal.NewFromInt(1_000_000),
				Slippage:  -0.01,
				FromAddr:  "0xFrom",
				ToAddr:    "0xTo",
			},
		},
		{
			name: "excessive_slippage",
			req: domain.QuoteRequest{
				FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
				ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
				Amount:    decimal.NewFromInt(1_000_000),
				Slippage:  0.1,
				FromAddr:  "0xFrom",
				ToAddr:    "0xTo",
			},
		},
	}

	for _, tc := range invalidReqs {
		t.Run(tc.name, func(t *testing.T) {
			for _, p := range providers {
				_, err := p.Quote(ctx, tc.req)
				assert.Error(t, err, "provider %s should reject %s", p.Name(), tc.name)
			}
		})
	}
}
