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
	if quote.To == "" {
		return nil, errors.New("target program address required")
	}

	programID, err := solana.PublicKeyFromBase58(quote.To)
	if err != nil {
		return nil, fmt.Errorf("invalid target program address: %w", err)
	}

	publicKeyBytes, err := signer.PublicKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("get public key error: %w", err)
	}
	publicKey := solana.PublicKeyFromBytes(publicKeyBytes)
	if publicKey.String() != from {
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
					{PublicKey: publicKey, IsSigner: true, IsWritable: true},
				},
				instructionData,
			),
		},
		recentBlockhash,
		solana.TransactionPayer(publicKey),
	)
	if err != nil {
		return nil, err
	}

	_, err = b.sign(ctx, tx, signer)
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
