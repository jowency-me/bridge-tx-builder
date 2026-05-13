// Package solana builds and signs Solana transactions for submission to the Solana runtime.
package solana

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

// Builder constructs Solana transactions.
type Builder struct{}

// NewBuilder creates a Solana transaction builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// ChainID returns the chain identifier handled by the builder.
func (b *Builder) ChainID() domain.ChainID { return domain.ChainSolana }

// Build creates a signed Solana transaction from the given quote.
func (b *Builder) Build(_ context.Context, quote domain.Quote, from string, privateKey []byte) (*domain.Transaction, error) {
	if quote.ID == "" {
		return nil, errors.New("quote id required")
	}
	if quote.BlockHash == "" {
		return nil, errors.New("recent blockhash required")
	}
	if quote.To == "" {
		return nil, errors.New("target program address required")
	}

	programID, err := solana.PublicKeyFromBase58(quote.To)
	if err != nil {
		return nil, fmt.Errorf("invalid target program address: %w", err)
	}

	key := solana.PrivateKey(privateKey)
	if !key.IsValid() {
		return nil, errors.New("invalid solana private key")
	}
	if key.PublicKey().String() != from {
		return nil, errors.New("private key does not match from address")
	}

	recentBlockhash := solana.MustHashFromBase58(quote.BlockHash)

	var instructionData []byte
	if len(quote.TxData) > 0 {
		instructionData = quote.TxData
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			solana.NewInstruction(
				programID,
				solana.AccountMetaSlice{
					{PublicKey: key.PublicKey(), IsSigner: true, IsWritable: true},
				},
				instructionData,
			),
		},
		recentBlockhash,
		solana.TransactionPayer(key.PublicKey()),
	)
	if err != nil {
		return nil, err
	}
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		pk := solana.PrivateKey(privateKey)
		return &pk
	})
	if err != nil {
		return nil, err
	}

	serialized, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &domain.Transaction{
		ChainID: domain.ChainSolana,
		From:    from,
		To:      quote.To,
		Data:    serialized,
		Value:   quote.TxValue,
		Gas:     quote.EstimateGas,
	}, nil
}
