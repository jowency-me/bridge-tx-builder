// Package solana simulates Solana transactions against a real RPC node using simulateTransaction.
package solana

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

// Simulator dry-runs Solana transactions via the RPC simulateTransaction endpoint.
type Simulator struct {
	client *rpc.Client
}

// NewSimulator creates a Solana simulator. If rpcURL is empty, falls back to
// the SOLANA_RPC_URL environment variable.
func NewSimulator(rpcURL string) *Simulator {
	if rpcURL == "" {
		rpcURL = os.Getenv("SOLANA_RPC_URL")
	}
	var client *rpc.Client
	if rpcURL != "" {
		client = rpc.NewWithHeaders(rpcURL, publicRPCHeaders())
	}
	return &Simulator{client: client}
}

func publicRPCHeaders() map[string]string {
	return map[string]string{"User-Agent": "bridge-tx-builder-tests/1.0"}
}

// Simulate sends the transaction to the Solana RPC simulateTransaction endpoint.
// It returns actual execution results including errors, logs, and compute units consumed.
func (s *Simulator) Simulate(ctx context.Context, tx *domain.Transaction) (*domain.SimulationResult, error) {
	if tx == nil {
		return nil, errors.New("transaction required")
	}
	if tx.ChainID != domain.ChainSolana {
		return nil, errors.New("not a Solana transaction")
	}

	// Deserialize the transaction bytes.
	solTx, err := solana.TransactionFromBytes(tx.Data)
	if err != nil {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: fmt.Sprintf("deserialize: %v", err),
		}, nil
	}

	// If no RPC URL is configured, fall back to local deserialization only.
	if s.client == nil {
		return &domain.SimulationResult{Success: true}, nil
	}

	// Call the RPC simulateTransaction endpoint.
	resp, err := s.client.SimulateTransaction(ctx, solTx)
	if err != nil {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: fmt.Sprintf("rpc: %v", err),
		}, nil
	}

	if resp == nil || resp.Value == nil {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: "empty simulation response",
		}, nil
	}

	result := resp.Value

	var gasUsed uint64
	if result.UnitsConsumed != nil {
		gasUsed = *result.UnitsConsumed
	}

	// Solana returns Err as a generic `any`. It is nil on success.
	if result.Err != nil {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: fmt.Sprintf("%v", result.Err),
			Logs:         result.Logs,
			GasUsed:      gasUsed,
		}, nil
	}

	return &domain.SimulationResult{
		Success: true,
		Logs:    result.Logs,
		GasUsed: gasUsed,
	}, nil
}
