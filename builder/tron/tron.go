// Package tron builds and signs Tron transactions as protobuf-encoded payloads.
package tron

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

// Builder constructs Tron transactions.
type Builder struct{}

// NewBuilder creates a Tron transaction builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// ChainID returns the chain identifier handled by the builder.
func (b *Builder) ChainID() domain.ChainID { return domain.ChainTron }

// Build constructs a sign-ready transaction from a quote.
func (b *Builder) Build(_ context.Context, quote domain.Quote, from string, signer any) (*domain.Transaction, error) {
	if quote.ID == "" {
		return nil, errors.New("quote id required")
	}
	if quote.To == "" {
		return nil, errors.New("quote to address required")
	}
	if len(quote.TxData) == 0 {
		return nil, errors.New("quote tx data required for TriggerSmartContract")
	}
	if quote.BlockHash == "" {
		return nil, errors.New("block hash required")
	}
	if quote.BlockHeight == 0 {
		return nil, errors.New("block height required")
	}

	ownerAddr, err := address.Base58ToAddress(from)
	if err != nil {
		return nil, errors.New("invalid from address")
	}

	s, ok := signer.(domain.TronSigner)
	if !ok {
		return nil, fmt.Errorf("expected TronSigner for chain %s, got %T", domain.ChainTron, signer)
	}

	if s.Address() != from {
		return nil, errors.New("signer address does not match from address")
	}

	contractAddr, err := address.Base58ToAddress(quote.To)
	if err != nil {
		return nil, errors.New("invalid to address")
	}

	val := quote.TxValue.BigInt()
	if val.Cmp(big.NewInt(math.MaxInt64)) > 0 || val.Cmp(big.NewInt(math.MinInt64)) < 0 {
		return nil, errors.New("tx value overflows int64")
	}

	param, err := packAnyPB(&core.TriggerSmartContract{
		OwnerAddress:    ownerAddr.Bytes(),
		ContractAddress: contractAddr.Bytes(),
		Data:            quote.TxData,
		CallValue:       val.Int64(),
	})
	if err != nil {
		return nil, err
	}

	blockHeightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(blockHeightBytes, quote.BlockHeight)
	refBlockBytes := blockHeightBytes[6:8]

	hashHex := strings.TrimPrefix(quote.BlockHash, "0x")
	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, fmt.Errorf("invalid block hash hex: %w", err)
	}
	if len(hashBytes) < 16 {
		return nil, errors.New("block hash too short")
	}
	refBlockHash := hashBytes[8:16]

	rawData := &core.TransactionRaw{
		Contract: []*core.Transaction_Contract{
			{
				Type:      core.Transaction_Contract_TriggerSmartContract,
				Parameter: param,
			},
		},
		RefBlockBytes: refBlockBytes,
		RefBlockHash:  refBlockHash,
		Expiration:    0,
		Timestamp:     0,
		FeeLimit:      int64(quote.EstimateGas),
	}

	tx := &core.Transaction{RawData: rawData}
	err = s.Sign(tx)
	if err != nil {
		return nil, err
	}

	rawBytes, err := proto.Marshal(tx)
	if err != nil {
		return nil, err
	}

	return &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    from,
		To:      quote.To,
		Data:    rawBytes,
		Value:   quote.TxValue,
		Gas:     quote.EstimateGas,
	}, nil
}

func packAnyPB(msg proto.Message) (*anypb.Any, error) {
	return anypb.New(msg)
}
