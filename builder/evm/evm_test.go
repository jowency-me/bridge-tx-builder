package evm

import (
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

func TestBuilder_ChainID(t *testing.T) {
	b := NewBuilder(1)
	assert.Equal(t, domain.ChainEthereum, b.ChainID())

	b2 := NewBuilder(8453)
	assert.Equal(t, domain.ChainBase, b2.ChainID())

	b3 := NewBuilder(56)
	assert.Equal(t, domain.ChainBSC, b3.ChainID())

	b4 := NewBuilder(137)
	assert.Equal(t, domain.ChainPolygon, b4.ChainID())

	b5 := NewBuilder(42161)
	assert.Equal(t, domain.ChainArbitrum, b5.ChainID())

	b6 := NewBuilder(10)
	assert.Equal(t, domain.ChainOptimism, b6.ChainID())

	b7 := NewBuilder(43114)
	assert.Equal(t, domain.ChainAvalanche, b7.ChainID())
}

func TestBuilder_Build_EIP1559(t *testing.T) {
	b := NewBuilder(1)
	ctx := context.Background()

	// Generate a random key for signing
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	fromAddr := crypto.PubkeyToAddress(key.PublicKey).Hex()

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          "0x1111111111111111111111111111111111111111",
		TxData:      common.Hex2Bytes("deadbeef"),
		TxValue:     decimal.Zero,
		EstimateGas: 200000,
		GasTipCap:   decimal.NewFromInt(1e9),
		GasFeeCap:   decimal.NewFromInt(20e9),
	}

	tx, err := b.Build(ctx, quote, fromAddr, crypto.FromECDSA(key))
	require.NoError(t, err)
	require.NotNil(t, tx)

	assert.Equal(t, domain.ChainEthereum, tx.ChainID)
	assert.Equal(t, fromAddr, tx.From)
	assert.Equal(t, uint64(200_000), tx.Gas)

	assert.NotEmpty(t, tx.Data)
	assert.NotEqual(t, quote.TxData, tx.Data, "Data should contain signed RLP bytes, not raw calldata")
}

func TestBuilder_Build_Legacy(t *testing.T) {
	b := NewBuilder(56) // BSC legacy
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	fromAddr := crypto.PubkeyToAddress(key.PublicKey).Hex()

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          "0x2222222222222222222222222222222222222222",
		TxData:      common.Hex2Bytes("cafebabe"),
		TxValue:     decimal.NewFromInt(1000),
		EstimateGas: 100000,
		GasPrice:    decimal.NewFromInt(5e9),
	}

	tx, err := b.Build(ctx, quote, fromAddr, crypto.FromECDSA(key))
	require.NoError(t, err)
	require.NotNil(t, tx)

	assert.NotEmpty(t, tx.Data)
	assert.NotEqual(t, quote.TxData, tx.Data, "Data should contain signed RLP bytes, not raw calldata")
}

func TestBuilder_Build_MissingTo(t *testing.T) {
	b := NewBuilder(1)
	quote := domain.Quote{TxData: []byte("data")}
	// quote has no To address (no router contract)
	_, err := b.Build(context.Background(), quote, "0xFrom", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quote to address required")
}

func TestBuilder_Build_LowercaseFrom(t *testing.T) {
	b := NewBuilder(1)
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	fromAddr := crypto.PubkeyToAddress(key.PublicKey).Hex()
	lowerFrom := strings.ToLower(fromAddr)

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          "0x1111111111111111111111111111111111111111",
		TxData:      common.Hex2Bytes("deadbeef"),
		TxValue:     decimal.Zero,
		EstimateGas: 200000,
		GasTipCap:   decimal.NewFromInt(1e9),
		GasFeeCap:   decimal.NewFromInt(20e9),
	}

	tx, err := b.Build(ctx, quote, lowerFrom, crypto.FromECDSA(key))
	require.NoError(t, err)
	require.NotNil(t, tx)
	assert.Equal(t, lowerFrom, tx.From)
}

func TestBuilder_Build_GasLimitTooHigh(t *testing.T) {
	b := NewBuilder(1)
	ctx := context.Background()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	fromAddr := crypto.PubkeyToAddress(key.PublicKey).Hex()

	quote := domain.Quote{
		ID:          "q1",
		Provider:    "lifi",
		To:          "0x1111111111111111111111111111111111111111",
		TxData:      common.Hex2Bytes("deadbeef"),
		TxValue:     decimal.Zero,
		EstimateGas: maxGasLimit + 1,
		GasTipCap:   decimal.NewFromInt(1e9),
		GasFeeCap:   decimal.NewFromInt(20e9),
	}

	_, err = b.Build(ctx, quote, fromAddr, crypto.FromECDSA(key))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gas limit")
}

func TestBuilder_Build_InvalidPrivateKey(t *testing.T) {
	b := NewBuilder(1)
	quote := domain.Quote{
		ID:        "q1",
		Provider:  "lifi",
		To:        "0x1111111111111111111111111111111111111111",
		TxData:    common.Hex2Bytes("deadbeef"),
		GasTipCap: decimal.NewFromInt(1e9),
		GasFeeCap: decimal.NewFromInt(20e9),
	}

	_, err := b.Build(context.Background(), quote, "0xFrom", []byte("bad-key"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid private key")
}
