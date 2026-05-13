package mock

import (
	"context"
	"errors"
	"testing"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestProvider_Name(t *testing.T) {
	p := &Provider{NameValue: "mock"}
	assert.Equal(t, "mock", p.Name())
}

func TestProvider_Quote(t *testing.T) {
	q := StaticQuote("q1", "mock")
	p := &Provider{QuoteValue: q}
	res, err := p.Quote(context.Background(), domain.QuoteRequest{})
	assert.NoError(t, err)
	assert.Equal(t, q, res)
}

func TestProvider_QuoteError(t *testing.T) {
	p := &Provider{QuoteError: errors.New("fail")}
	_, err := p.Quote(context.Background(), domain.QuoteRequest{})
	assert.Error(t, err)
}

func TestProvider_Status(t *testing.T) {
	s := StaticStatus("tx1")
	p := &Provider{StatusValue: s}
	res, err := p.Status(context.Background(), "tx1")
	assert.NoError(t, err)
	assert.Equal(t, s, res)
}

func TestProvider_StatusError(t *testing.T) {
	p := &Provider{StatusError: errors.New("fail")}
	_, err := p.Status(context.Background(), "tx1")
	assert.Error(t, err)
}

func TestProvider_WithStatus(t *testing.T) {
	p := &Provider{}
	s := StaticStatus("tx1")
	p.WithStatus(s)
	assert.Equal(t, s, p.StatusValue)
}

func TestProvider_WithQuote(t *testing.T) {
	p := &Provider{}
	q := StaticQuote("q1", "mock")
	p.WithQuote(q)
	assert.Equal(t, q, p.QuoteValue)
}

func TestProvider_WithQuoteError(t *testing.T) {
	p := &Provider{}
	err := errors.New("fail")
	p.WithQuoteError(err)
	assert.Equal(t, err, p.QuoteError)
}

func TestNewFixedProvider(t *testing.T) {
	q := StaticQuote("q1", "mock")
	p := NewFixedProvider("mock", q)
	assert.Equal(t, "mock", p.NameValue)
	assert.Equal(t, q, p.QuoteValue)
}

func TestNewErrorProvider(t *testing.T) {
	p := NewErrorProvider("mock", errors.New("fail"))
	assert.Equal(t, "mock", p.NameValue)
	assert.NotNil(t, p.QuoteError)
}

func TestStaticQuote(t *testing.T) {
	q := StaticQuote("q1", "mock")
	assert.Equal(t, "q1", q.ID)
	assert.Equal(t, "mock", q.Provider)
	assert.Equal(t, decimal.NewFromInt(1_000_000), q.FromAmount)
}

func TestStaticStatus(t *testing.T) {
	s := StaticStatus("tx1")
	assert.Equal(t, "tx1", s.TxID)
	assert.Equal(t, "completed", s.State)
}
