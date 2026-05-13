package domain

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	gotronSigner "github.com/fbsobreira/gotron-sdk/pkg/signer"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/gagliardetto/solana-go"
)

// EVMSigner signs EVM transactions. The chainID is passed per-call so a single
// signer instance can be reused across different EVM chains.
type EVMSigner interface {
	Address() common.Address
	SignTx(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
}

// EVMPrivateKeySigner implements EVMSigner using a raw secp256k1 private key.
type EVMPrivateKeySigner struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
}

// NewEVMPrivateKeySigner creates a signer from raw private key bytes.
func NewEVMPrivateKeySigner(privateKey []byte) (*EVMPrivateKeySigner, error) {
	key, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid EVM private key: %w", err)
	}
	pub, ok := key.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("invalid EVM public key type")
	}
	addr := crypto.PubkeyToAddress(*pub)
	return &EVMPrivateKeySigner{privateKey: key, address: addr}, nil
}

func (s *EVMPrivateKeySigner) Address() common.Address { return s.address }

func (s *EVMPrivateKeySigner) SignTx(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	signer := types.LatestSignerForChainID(chainID)
	return types.SignTx(tx, signer, s.privateKey)
}

// SolanaSigner signs Solana transactions (Ed25519).
type SolanaSigner interface {
	PublicKey() solana.PublicKey
	Sign(tx *solana.Transaction) error
}

// SolanaPrivateKeySigner implements SolanaSigner using a raw Ed25519 private key.
type SolanaPrivateKeySigner struct {
	privateKey solana.PrivateKey
	publicKey  solana.PublicKey
}

// NewSolanaPrivateKeySigner creates a signer from raw private key bytes (64 bytes: seed || pubkey).
func NewSolanaPrivateKeySigner(privateKey []byte) (*SolanaPrivateKeySigner, error) {
	key := solana.PrivateKey(privateKey)
	if !key.IsValid() {
		return nil, fmt.Errorf("invalid Solana private key")
	}
	return &SolanaPrivateKeySigner{privateKey: key, publicKey: key.PublicKey()}, nil
}

func (s *SolanaPrivateKeySigner) PublicKey() solana.PublicKey { return s.publicKey }

func (s *SolanaPrivateKeySigner) Sign(tx *solana.Transaction) error {
	_, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(s.publicKey) {
			pk := s.privateKey
			return &pk
		}
		return nil
	})
	return err
}

// TronSigner signs Tron transactions (secp256k1, protobuf).
type TronSigner interface {
	Address() string
	Sign(tx *core.Transaction) error
}

// TronPrivateKeySigner implements TronSigner using a raw secp256k1 private key.
type TronPrivateKeySigner struct {
	address string
	signer  gotronSigner.Signer
}

// NewTronPrivateKeySigner creates a signer from raw private key bytes.
func NewTronPrivateKeySigner(privateKey []byte) (*TronPrivateKeySigner, error) {
	key, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid Tron private key: %w", err)
	}
	addr := address.PubkeyToAddress(key.PublicKey).String()
	s, err := gotronSigner.NewPrivateKeySigner(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create Tron signer: %w", err)
	}
	return &TronPrivateKeySigner{address: addr, signer: s}, nil
}

func (s *TronPrivateKeySigner) Address() string { return s.address }

func (s *TronPrivateKeySigner) Sign(tx *core.Transaction) error {
	_, err := s.signer.Sign(tx)
	return err
}
