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
func (b *Builder) Build(ctx context.Context, quote domain.Quote, from string, signer domain.Signer) (*domain.Transaction, error) {
	if quote.ID == "" {
		return nil, errors.New("quote id required")
	}
	if quote.BlockHash == "" {
		return nil, errors.New("recent blockhash required")
	}

	publicKeyBytes, err := signer.PublicKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("get public key error: %w", err)
	}
	publicKey := solana.PublicKeyFromBytes(publicKeyBytes)
	if publicKey.String() != from {
		return nil, errors.New("private key does not match from address")
	}

	tx, err := b.buildFromPrebuiltTx(ctx, quote, publicKey, signer)
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

func (b *Builder) buildFromPrebuiltTx(ctx context.Context, quote domain.Quote, pubKey solana.PublicKey, signer domain.Signer) (*solana.Transaction, error) {
	tx, err := solana.TransactionFromBytes(quote.TxData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize Solana transaction: %w", err)
	}

	tx.Message.RecentBlockhash = solana.MustHashFromBase58(quote.BlockHash)
	tx.Message.AccountKeys = append(tx.Message.AccountKeys, pubKey)

	_, err = b.sign(ctx, tx, signer)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (b *Builder) sign(ctx context.Context, tx *solana.Transaction, signer domain.Signer) (*solana.Transaction, error) {
	messageContent, err := tx.Message.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to encode message for signing: %w", err)
	}

	signature, err := signer.Sign(ctx, messageContent)
	if err != nil {
		return nil, fmt.Errorf("sign message error: %w", err)
	}
	tx.Signatures = append(tx.Signatures, solana.SignatureFromBytes(signature))

	return tx, nil
}
