// Package evm builds and signs EVM-compatible transactions for submission to Ethereum and EVM chains.
package evm

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

const maxGasLimit = 50_000_000

// Builder constructs EVM-compatible transactions (Ethereum, Base, etc.).
type Builder struct {
	numericChainID *big.Int
	chainID        domain.ChainID
}

// NewBuilder creates an EVM builder for the given numeric chain ID.
func NewBuilder(numericChainID int64) *Builder {
	cid := domain.NumericToChainID(strconv.FormatInt(numericChainID, 10))
	return &Builder{
		numericChainID: big.NewInt(numericChainID),
		chainID:        cid,
	}
}

func ptr(a common.Address) *common.Address { return &a }

// ChainID returns the chain identifier handled by the builder.
func (b *Builder) ChainID() domain.ChainID { return b.chainID }

// Build constructs a sign-ready transaction from a quote.
func (b *Builder) Build(_ context.Context, quote domain.Quote, from string, privateKey []byte) (*domain.Transaction, error) {
	if quote.TxData == nil && quote.TxValue.IsZero() {
		return nil, errors.New("quote has no transaction data")
	}
	if quote.TxData == nil {
		quote.TxData = []byte{}
	}

	to := quote.To
	if to == "" {
		return nil, errors.New("quote to address required")
	}

	key, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	fromAddr := crypto.PubkeyToAddress(*key.Public().(*ecdsa.PublicKey))
	if common.HexToAddress(from).Hex() != fromAddr.Hex() {
		return nil, errors.New("private key does not match from address")
	}

	// Nonce handling follows the go-ethereum bind.TransactOpts pattern:
	//   - if Quote.Nonce is set, use the caller-provided value (manual nonce management)
	//   - if nil, fall back to 0; the caller is responsible for updating nonce
	//     via eth_getTransactionCount / PendingNonceAt before broadcasting.
	// This library is a pure transaction builder and does not hold an RPC client,
	// so automatic nonce retrieval is intentionally left to the caller.
	nonce := uint64(0)
	if quote.Nonce != nil {
		nonce = *quote.Nonce
	}
	gasLimit := quote.EstimateGas
	if gasLimit == 0 {
		gasLimit = 300_000
	}
	if gasLimit > maxGasLimit {
		return nil, fmt.Errorf("gas limit %d exceeds maximum %d", gasLimit, maxGasLimit)
	}

	var rawTx *types.Transaction

	// Use EIP-1559 for chains that support it.
	if domain.SupportsEIP1559(b.chainID) {
		if quote.GasTipCap.IsZero() || quote.GasFeeCap.IsZero() {
			return nil, errors.New("EIP-1559 gas tip cap and fee cap must be provided")
		}
		ev := &types.DynamicFeeTx{
			ChainID:   b.numericChainID,
			Nonce:     nonce,
			GasTipCap: quote.GasTipCap.BigInt(),
			GasFeeCap: quote.GasFeeCap.BigInt(),
			Gas:       gasLimit,
			To:        ptr(common.HexToAddress(to)),
			Value:     quote.TxValue.BigInt(),
			Data:      quote.TxData,
		}
		rawTx = types.NewTx(ev)
	} else {
		if quote.GasPrice.IsZero() {
			return nil, errors.New("gas price must be provided")
		}
		leg := &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: quote.GasPrice.BigInt(),
			Gas:      gasLimit,
			To:       ptr(common.HexToAddress(to)),
			Value:    quote.TxValue.BigInt(),
			Data:     quote.TxData,
		}
		rawTx = types.NewTx(leg)
	}

	signer := types.LatestSignerForChainID(b.numericChainID)
	signedTx, err := types.SignTx(rawTx, signer, key)
	if err != nil {
		return nil, err
	}

	signedBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &domain.Transaction{
		ChainID: b.chainID,
		From:    from,
		To:      to,
		Data:    signedBytes,
		Value:   quote.TxValue,
		Gas:     gasLimit,
		Nonce:   nonce,
	}, nil
}
