package domain

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/gagliardetto/solana-go"
)

// Signer is the interface for signing transactions.
// Implement this interface to use custom key management solutions
// (MPC wallets, TEE enclaves, HSMs, etc.).
type Signer interface {
	PublicKey(ctx context.Context) ([]byte, error)
	Sign(ctx context.Context, payload []byte) ([]byte, error)
}

// EVMPrivateKeySigner implements Signer for EVM-compatible chains using secp256k1.
type EVMPrivateKeySigner struct {
	key *ecdsa.PrivateKey
}

// NewEVMPrivateKeySigner creates an EVM signer from a raw private key bytes.
func NewEVMPrivateKeySigner(privateKey []byte) (*EVMPrivateKeySigner, error) {
	if len(privateKey) != 32 {
		return nil, errors.New("EVM private key must be 32 bytes")
	}
	key, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid EVM private key: %w", err)
	}
	return &EVMPrivateKeySigner{key: key}, nil
}

// PublicKey returns the compressed public key bytes.
func (s *EVMPrivateKeySigner) PublicKey(_ context.Context) ([]byte, error) {
	return crypto.CompressPubkey(&s.key.PublicKey), nil
}

// Sign signs the payload using secp256k1.
func (s *EVMPrivateKeySigner) Sign(_ context.Context, payload []byte) ([]byte, error) {
	return crypto.Sign(payload, s.key)
}

// Address returns the Ethereum address derived from the public key.
func (s *EVMPrivateKeySigner) Address() common.Address {
	return crypto.PubkeyToAddress(s.key.PublicKey)
}

// SolanaPrivateKeySigner implements Signer for Solana using ed25519.
type SolanaPrivateKeySigner struct {
	key solana.PrivateKey
}

// NewSolanaPrivateKeySigner creates a Solana signer from raw private key bytes (64 bytes).
func NewSolanaPrivateKeySigner(privateKey []byte) (*SolanaPrivateKeySigner, error) {
	if len(privateKey) != 64 {
		return nil, errors.New("Solana private key must be 64 bytes")
	}
	key := solana.PrivateKey(privateKey)
	if !key.IsValid() {
		return nil, errors.New("invalid Solana private key")
	}
	return &SolanaPrivateKeySigner{key: key}, nil
}

// PublicKey returns the public key bytes.
func (s *SolanaPrivateKeySigner) PublicKey(_ context.Context) ([]byte, error) {
	return s.key.PublicKey().Bytes(), nil
}

// Sign signs the payload using ed25519.
func (s *SolanaPrivateKeySigner) Sign(_ context.Context, payload []byte) ([]byte, error) {
	sig, err := s.key.Sign(payload)
	if err != nil {
		return nil, err
	}
	return sig[:], nil
}

// Address returns the Solana public key as a string.
func (s *SolanaPrivateKeySigner) Address() string {
	return s.key.PublicKey().String()
}

// TronPrivateKeySigner implements Signer for Tron using secp256k1 (same curve as EVM).
type TronPrivateKeySigner struct {
	key *ecdsa.PrivateKey
}

// NewTronPrivateKeySigner creates a Tron signer from raw private key bytes.
func NewTronPrivateKeySigner(privateKey []byte) (*TronPrivateKeySigner, error) {
	if len(privateKey) != 32 {
		return nil, errors.New("Tron private key must be 32 bytes")
	}
	key, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid Tron private key: %w", err)
	}
	return &TronPrivateKeySigner{key: key}, nil
}

// PublicKey returns the compressed public key bytes.
func (s *TronPrivateKeySigner) PublicKey(_ context.Context) ([]byte, error) {
	return crypto.CompressPubkey(&s.key.PublicKey), nil
}

// Sign signs the payload using secp256k1.
func (s *TronPrivateKeySigner) Sign(_ context.Context, payload []byte) ([]byte, error) {
	return crypto.Sign(payload, s.key)
}

// Address returns the Tron Base58 address derived from the public key.
func (s *TronPrivateKeySigner) Address() string {
	return address.PubkeyToAddress(s.key.PublicKey).String()
}
