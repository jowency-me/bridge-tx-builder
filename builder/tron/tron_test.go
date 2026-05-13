package tron

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

type invalidSigner struct{}

func (s *invalidSigner) PublicKey(_ context.Context) ([]byte, error) {
	return nil, errors.New("invalid private key")
}

func (s *invalidSigner) Sign(_ context.Context, _ []byte) ([]byte, error) {
	return nil, errors.New("invalid private key")
}

func addressFromKey(key *ecdsa.PrivateKey) string {
	return address.PubkeyToAddress(key.PublicKey).String()
}

func newTestSigner(t *testing.T, key *ecdsa.PrivateKey) *domain.TronPrivateKeySigner {
	signer, err := domain.NewTronPrivateKeySigner(crypto.FromECDSA(key))
	require.NoError(t, err)
	return signer
}

func TestBuilder_ChainID(t *testing.T) {
	b := NewBuilder()
	assert.Equal(t, domain.ChainTron, b.ChainID())
}

func TestBuilder_Build(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := newTestSigner(t, key)
	from := signer.Address()

	toKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := addressFromKey(toKey)

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          to,
		TxData:      []byte{0xde, 0xad, 0xbe, 0xef},
		TxValue:     decimal.Zero,
		EstimateGas: 200000,
		BlockHash:   "0000000000000000000000000000000000000000000000000000000000000001",
		BlockHeight: 1000000,
	}

	tx, err := b.Build(ctx, quote, from, signer)
	require.NoError(t, err)
	require.NotNil(t, tx)

	assert.Equal(t, domain.ChainTron, tx.ChainID)
	assert.Equal(t, from, tx.From)
	assert.NotEmpty(t, tx.Data)

	var signedTx core.Transaction
	err = proto.Unmarshal(tx.Data, &signedTx)
	require.NoError(t, err)
	assert.NotEmpty(t, signedTx.Signature, "Data should contain signed transaction with signature")
	require.NotNil(t, signedTx.RawData)
	assert.Equal(t, []byte{0x42, 0x40}, signedTx.RawData.RefBlockBytes)
	assert.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, signedTx.RawData.RefBlockHash)
}

func TestBuilder_Build_InvalidKey(t *testing.T) {
	b := NewBuilder()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	from := addressFromKey(key)

	toKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := addressFromKey(toKey)

	_, err = b.Build(context.Background(), domain.Quote{ID: "q1", To: to, TxData: []byte{0xde, 0xad, 0xbe, 0xef}, BlockHash: "0000000000000000000000000000000000000000000000000000000000000001", BlockHeight: 1}, from, &invalidSigner{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid private key")
}

func TestBuilder_Build_MissingTxData(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := newTestSigner(t, key)
	from := signer.Address()

	toKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := addressFromKey(toKey)

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          to,
		TxData:      nil,
		TxValue:     decimal.Zero,
		EstimateGas: 200000,
		BlockHash:   "0000000000000000000000000000000000000000000000000000000000000001",
		BlockHeight: 1,
	}

	_, err = b.Build(ctx, quote, from, signer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tx data required")
}

func TestBuilder_Build_InvalidFromAddress(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := newTestSigner(t, key)

	toKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := addressFromKey(toKey)

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          to,
		TxData:      []byte{0xde, 0xad, 0xbe, 0xef},
		EstimateGas: 200000,
		BlockHash:   "0000000000000000000000000000000000000000000000000000000000000001",
		BlockHeight: 1,
	}

	_, err = b.Build(ctx, quote, "invalid-address", signer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid from address")
}

func TestBuilder_Build_InvalidToAddress(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := newTestSigner(t, key)
	from := signer.Address()

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          "not-a-valid-base58",
		TxData:      []byte{0xde, 0xad, 0xbe, 0xef},
		EstimateGas: 200000,
		BlockHash:   "0000000000000000000000000000000000000000000000000000000000000001",
		BlockHeight: 1,
	}

	_, err = b.Build(ctx, quote, from, signer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid to address")
}

func TestBuilder_Build_ValueOverflow(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := newTestSigner(t, key)
	from := signer.Address()

	toKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := addressFromKey(toKey)

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          to,
		TxData:      []byte{0xde, 0xad, 0xbe, 0xef},
		TxValue:     decimal.New(1, 30),
		EstimateGas: 200000,
		BlockHash:   "0000000000000000000000000000000000000000000000000000000000000001",
		BlockHeight: 1,
	}

	_, err = b.Build(ctx, quote, from, signer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows int64")
}

func TestBuilder_Build_InvalidBlockHashHex(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := newTestSigner(t, key)
	from := signer.Address()

	toKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := addressFromKey(toKey)

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          to,
		TxData:      []byte{0xde, 0xad, 0xbe, 0xef},
		TxValue:     decimal.Zero,
		EstimateGas: 200000,
		BlockHash:   "not-hex!",
		BlockHeight: 1,
	}

	_, err = b.Build(ctx, quote, from, signer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid block hash hex")
}

func TestBuilder_Build_BlockHashTooShort(t *testing.T) {
	b := NewBuilder()
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := newTestSigner(t, key)
	from := signer.Address()

	toKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := addressFromKey(toKey)

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          to,
		TxData:      []byte{0xde, 0xad, 0xbe, 0xef},
		TxValue:     decimal.Zero,
		EstimateGas: 200000,
		BlockHash:   "0x1234",
		BlockHeight: 1,
	}

	_, err = b.Build(ctx, quote, from, signer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "block hash too short")
}
