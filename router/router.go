// Package router orchestrates quote selection, transaction building, and simulation across registered providers and chain builders.
package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

// SelectionStrategy defines how to choose the best quote among providers.
type SelectionStrategy string

const (
	// StrategyBestAmount selects the provider with the highest expected output amount.
	StrategyBestAmount SelectionStrategy = "best_amount"
	// StrategyLowestFee selects the provider with the lowest estimated fee.
	StrategyLowestFee SelectionStrategy = "lowest_fee"
)

// StrategyNamed creates a strategy that forces a specific provider by name.
func StrategyNamed(name string) SelectionStrategy {
	return SelectionStrategy("named:" + name)
}

// Router is the unified entry point for cross-chain swap operations.
type Router struct {
	mu         sync.RWMutex
	providers  []domain.Provider
	builders   map[domain.ChainID]domain.ChainBuilder
	simulators map[domain.ChainID]domain.Simulator
}

// New creates a new Router.
func New() *Router {
	return &Router{
		builders:   make(map[domain.ChainID]domain.ChainBuilder),
		simulators: make(map[domain.ChainID]domain.Simulator),
	}
}

// RegisterProvider adds a provider.
func (r *Router) RegisterProvider(p domain.Provider) {
	if p == nil {
		return
	}
	r.mu.Lock()
	r.providers = append(r.providers, p)
	r.mu.Unlock()
}

// RegisterBuilder adds a builder for its ChainID().
func (r *Router) RegisterBuilder(b domain.ChainBuilder) {
	if b == nil {
		return
	}
	r.mu.Lock()
	r.builders[b.ChainID()] = b
	r.mu.Unlock()
}

// RegisterSimulator adds a simulator for a chain.
func (r *Router) RegisterSimulator(chain domain.ChainID, s domain.Simulator) {
	if s == nil {
		return
	}
	r.mu.Lock()
	r.simulators[chain] = s
	r.mu.Unlock()
}

// FindProviders queries all registered providers and returns the names of those
// that successfully return a quote for the given request.
func (r *Router) FindProviders(ctx context.Context, req domain.QuoteRequest) ([]string, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	quotes, errs := r.quoteAll(ctx, req)
	var supported []string
	for name := range quotes {
		supported = append(supported, name)
	}
	if len(supported) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all %d providers failed", len(errs))
	}
	return supported, nil
}

// SelectBest fetches quotes from all providers and selects one according to the strategy.
func (r *Router) SelectBest(ctx context.Context, req domain.QuoteRequest, strategy SelectionStrategy) (*domain.Quote, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	quotes, errs := r.quoteAll(ctx, req)
	if len(quotes) == 0 {
		if len(errs) > 0 {
			return nil, fmt.Errorf("no provider returned a valid quote (%d errors)", len(errs))
		}
		return nil, errors.New("no provider returned a valid quote")
	}

	var quoteList []*domain.Quote
	for _, q := range quotes {
		quoteList = append(quoteList, q)
	}

	switch {
	case strategy == StrategyBestAmount:
		return selectByBestAmount(quoteList), nil
	case strategy == StrategyLowestFee:
		return selectByLowestFee(quoteList), nil
	case strings.HasPrefix(string(strategy), "named:"):
		name := strings.TrimPrefix(string(strategy), "named:")
		q, ok := quotes[name]
		if !ok {
			return nil, errors.New("named provider not available: " + name)
		}
		return q, nil
	default:
		return nil, errors.New("unknown selection strategy: " + string(strategy))
	}
}

// BuildTransaction routes the quote to the correct chain builder and returns a signed-ready transaction.
func (r *Router) BuildTransaction(ctx context.Context, quote domain.Quote, from string, signer any) (*domain.Transaction, error) {
	if err := quote.Validate(); err != nil {
		return nil, err
	}
	chainID := quote.FromToken.ChainID
	r.mu.RLock()
	builder, ok := r.builders[chainID]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no builder registered for chain %q", chainID)
	}
	return builder.Build(ctx, quote, from, signer)
}

// Simulate dry-runs a transaction without broadcasting.
func (r *Router) Simulate(ctx context.Context, tx *domain.Transaction) (*domain.SimulationResult, error) {
	if tx == nil {
		return nil, errors.New("transaction required")
	}
	if err := tx.Validate(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	sim, ok := r.simulators[tx.ChainID]
	r.mu.RUnlock()
	if !ok {
		return nil, errors.New("no simulator for chain: " + string(tx.ChainID))
	}
	return sim.Simulate(ctx, tx)
}

// quoteAll queries every registered provider concurrently.
func (r *Router) quoteAll(ctx context.Context, req domain.QuoteRequest) (map[string]*domain.Quote, map[string]error) {
	r.mu.RLock()
	providers := make([]domain.Provider, len(r.providers))
	copy(providers, r.providers)
	r.mu.RUnlock()

	if len(providers) == 0 {
		return nil, nil
	}

	type result struct {
		name  string
		quote *domain.Quote
		err   error
	}

	var wg sync.WaitGroup
	results := make(chan result, len(providers))

	for _, p := range providers {
		wg.Add(1)
		go func(prov domain.Provider) {
			defer wg.Done()
			q, err := prov.Quote(ctx, req)
			results <- result{name: prov.Name(), quote: q, err: err}
		}(p)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	quotes := make(map[string]*domain.Quote, len(providers))
	errs := make(map[string]error, len(providers))
	for res := range results {
		if res.err != nil {
			errs[res.name] = res.err
		} else if res.quote != nil {
			quotes[res.name] = res.quote
		}
	}
	return quotes, errs
}

func selectByBestAmount(quotes []*domain.Quote) *domain.Quote {
	var best *domain.Quote
	for _, q := range quotes {
		if best == nil || q.ToAmount.Cmp(best.ToAmount) > 0 {
			best = q
		}
	}
	return best
}

func selectByLowestFee(quotes []*domain.Quote) *domain.Quote {
	var best *domain.Quote
	for _, q := range quotes {
		if best == nil {
			best = q
			continue
		}
		if q.EstimateFee.Cmp(best.EstimateFee) < 0 {
			best = q
		}
	}
	return best
}
