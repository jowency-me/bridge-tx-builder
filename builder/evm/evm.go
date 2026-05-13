// Package evm builds and signs EVM-compatible transactions for submission to Ethereum and EVM chains.
package evm

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

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
func (b *Builder) Build(_ context.Context, quote domain.Quote, from string, signer any) (*domain.Transaction, error) {
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

	s, ok := signer.(domain.EVMSigner)
	if !ok {
		return nil, fmt.Errorf("expected EVMSigner for chain %s, got %T", b.chainID, signer)
	}

	if common.HexToAddress(from).Hex() != s.Address().Hex() {
		return nil, errors.New("signer address does not match from address")
	}

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

	signedTx, err := s.SignTx(rawTx, b.numericChainID)
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
