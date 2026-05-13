// Package mock provides deterministic test doubles for the domain.Provider interface.
package mock

import (
	"context"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/shopspring/decimal"
)

// Provider is a test double that returns pre-configured quotes.
type Provider struct {
	NameValue   string
	QuoteValue  *domain.Quote
	QuoteError  error
	StatusValue *domain.Status
	StatusError error
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return p.NameValue }

// Quote fetches a quote from the provider API.
func (p *Provider) Quote(_ context.Context, _ domain.QuoteRequest) (*domain.Quote, error) {
	if p.QuoteError != nil {
		return nil, p.QuoteError
	}
	return p.QuoteValue, nil
}

// Status fetches transaction status from the provider API.
func (p *Provider) Status(_ context.Context, _ string) (*domain.Status, error) {
	if p.StatusError != nil {
		return nil, p.StatusError
	}
	return p.StatusValue, nil
}

// WithStatus returns a new Provider with the given status config.
func (p *Provider) WithStatus(s *domain.Status) *Provider {
	p.StatusValue = s
	return p
}

// WithQuote returns a new Provider with the given quote config.
func (p *Provider) WithQuote(q *domain.Quote) *Provider {
	p.QuoteValue = q
	return p
}

// WithQuoteError returns a new Provider with the given quote error.
func (p *Provider) WithQuoteError(err error) *Provider {
	p.QuoteError = err
	return p
}

// NewFixedProvider returns a mock that always returns the given quote.
func NewFixedProvider(name string, quote *domain.Quote) *Provider {
	return &Provider{
		NameValue:  name,
		QuoteValue: quote,
	}
}

// NewErrorProvider returns a mock that always returns an error.
func NewErrorProvider(name string, err error) *Provider {
	return &Provider{
		NameValue:  name,
		QuoteError: err,
	}
}

// StaticQuote builds a predictable quote for tests.
func StaticQuote(id, provider string) *domain.Quote {
	tok := domain.Token{
		Symbol:   "USDC",
		Address:  "0xA0b86a33E6441e0A421e56E4773C3C1C3E1f3e3C",
		Decimals: 6,
		ChainID:  domain.ChainEthereum,
	}
	return &domain.Quote{
		ID:         id,
		FromToken:  tok,
		ToToken:    tok,
		FromAmount: decimal.NewFromInt(1_000_000),
		ToAmount:   decimal.NewFromInt(999_000),
		MinAmount:  decimal.NewFromInt(995_000),
		Slippage:   0.005,
		Provider:   provider,
		Deadline:   time.Now().Add(10 * time.Minute),
	}
}

// StaticStatus builds a predictable status for tests.
func StaticStatus(txID string) *domain.Status {
	return &domain.Status{
		TxID:       txID,
		State:      "completed",
		SrcChainTx: txID + "_src",
		DstChainTx: txID + "_dst",
		UpdatedAt:  time.Now(),
	}
}
