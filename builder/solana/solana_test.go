package solana

import (
	"context"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

type invalidSigner struct{}

func (s *invalidSigner) PublicKey(_ context.Context) ([]byte, error) {
	return nil, errors.New("invalid private key")
}

func (s *invalidSigner) Sign(_ context.Context, _ []byte) ([]byte, error) {
	return nil, errors.New("invalid private key")
}

func TestBuilder_ChainID(t *testing.T) {
	b := NewBuilder()
	assert.Equal(t, domain.ChainSolana, b.ChainID())
}

func TestBuilder_Build(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	signer, err := domain.NewSolanaPrivateKeySigner([]byte(key))
	require.NoError(t, err)
	from := signer.Address()

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "rango",
		FromAmount:  decimal.NewFromInt(1_000_000),
		ToAmount:    decimal.NewFromInt(999_000),
		EstimateGas: 5000,
		BlockHash:   "11111111111111111111111111111111",
		To:          solana.SystemProgramID.String(),
	}

	tx, err := b.Build(ctx, quote, from, signer)
	require.NoError(t, err)
	require.NotNil(t, tx)

	assert.Equal(t, domain.ChainSolana, tx.ChainID)
	assert.Equal(t, from, tx.From)
	assert.NotEmpty(t, tx.Data)
}

func TestBuilder_Build_WithTxData(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	signer, err := domain.NewSolanaPrivateKeySigner([]byte(key))
	require.NoError(t, err)
	from := signer.Address()

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "rango",
		FromAmount:  decimal.NewFromInt(1_000_000),
		ToAmount:    decimal.NewFromInt(999_000),
		EstimateGas: 5000,
		TxData:      []byte{0xde, 0xad, 0xbe, 0xef},
		BlockHash:   "11111111111111111111111111111111",
		To:          solana.SystemProgramID.String(),
	}

	tx, err := b.Build(ctx, quote, from, signer)
	require.NoError(t, err)
	require.NotNil(t, tx)

	assert.Equal(t, domain.ChainSolana, tx.ChainID)
	assert.Equal(t, from, tx.From)
	assert.NotEmpty(t, tx.Data)
}

func TestBuilder_Build_MissingTo(t *testing.T) {
	b := NewBuilder()
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	signer, err := domain.NewSolanaPrivateKeySigner([]byte(key))
	require.NoError(t, err)
	from := signer.Address()

	quote := domain.Quote{
		ID:        "q1",
		BlockHash: "11111111111111111111111111111111",
	}
	_, err = b.Build(context.Background(), quote, from, signer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target program address required")
}

func TestBuilder_Build_InvalidTo(t *testing.T) {
	b := NewBuilder()
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	signer, err := domain.NewSolanaPrivateKeySigner([]byte(key))
	require.NoError(t, err)
	from := signer.Address()

	quote := domain.Quote{
		ID:        "q1",
		To:        "not-a-valid-base58",
		BlockHash: "11111111111111111111111111111111",
	}
	_, err = b.Build(context.Background(), quote, from, signer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid target program address")
}

func TestBuilder_Build_InvalidKey(t *testing.T) {
	b := NewBuilder()
	_, err := b.Build(context.Background(), domain.Quote{ID: "q1"}, "from", &invalidSigner{})
	require.Error(t, err)
}
