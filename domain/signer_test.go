package domain

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEVMPrivateKeySigner(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	privateKeyBytes := crypto.FromECDSA(key)
	signer, err := NewEVMPrivateKeySigner(privateKeyBytes)
	require.NoError(t, err)

	assert.Equal(t, crypto.PubkeyToAddress(key.PublicKey).Hex(), signer.Address().Hex())

	pk, err := signer.PublicKey(context.Background())
	require.NoError(t, err)
	compressedPk := crypto.CompressPubkey(&key.PublicKey)
	assert.Equal(t, compressedPk, pk)

	digest := crypto.Keccak256Hash([]byte("test message"))
	sig, err := signer.Sign(context.Background(), digest.Bytes())
	require.NoError(t, err)
	assert.Len(t, sig, 65)

	pubKey, err := crypto.SigToPub(digest.Bytes(), sig)
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey, *pubKey)
}

func TestEVMPrivateKeySigner_InvalidKey(t *testing.T) {
	_, err := NewEVMPrivateKeySigner([]byte("too short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 32 bytes")

	_, err = NewEVMPrivateKeySigner(make([]byte, 32))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid EVM private key")
}

func TestSolanaPrivateKeySigner(t *testing.T) {
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	signer, err := NewSolanaPrivateKeySigner([]byte(key))
	require.NoError(t, err)

	assert.Equal(t, key.PublicKey().String(), signer.Address())

	pk, err := signer.PublicKey(context.Background())
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey().Bytes(), pk)

	payload := []byte("test message")
	sig, err := signer.Sign(context.Background(), payload)
	require.NoError(t, err)
	assert.NotEmpty(t, sig)
}

func TestSolanaPrivateKeySigner_InvalidKey(t *testing.T) {
	_, err := NewSolanaPrivateKeySigner([]byte("too short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 64 bytes")

	_, err = NewSolanaPrivateKeySigner(make([]byte, 64))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Solana private key")
}

func TestTronPrivateKeySigner(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	privateKeyBytes := crypto.FromECDSA(key)
	signer, err := NewTronPrivateKeySigner(privateKeyBytes)
	require.NoError(t, err)

	tronAddr := signer.Address()
	assert.NotEmpty(t, tronAddr)
	assert.Contains(t, tronAddr, "T")

	pk, err := signer.PublicKey(context.Background())
	require.NoError(t, err)
	compressedPk := crypto.CompressPubkey(&key.PublicKey)
	assert.Equal(t, compressedPk, pk)

	digest := crypto.Keccak256Hash([]byte("test message"))
	sig, err := signer.Sign(context.Background(), digest.Bytes())
	require.NoError(t, err)
	assert.Len(t, sig, 65)

	pubKey, err := crypto.SigToPub(digest.Bytes(), sig)
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey, *pubKey)
}

func TestTronPrivateKeySigner_InvalidKey(t *testing.T) {
	_, err := NewTronPrivateKeySigner([]byte("too short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 32 bytes")

	_, err = NewTronPrivateKeySigner(make([]byte, 32))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Tron private key")
}
