// Package tron builds and signs Tron transactions as protobuf-encoded payloads.
package tron

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
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
func (b *Builder) Build(ctx context.Context, quote domain.Quote, from string, signer domain.Signer) (*domain.Transaction, error) {
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

	publicKeyBytes, err := signer.PublicKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("get public key error: %w", err)
	}
	publicKey, err := crypto.DecompressPubkey(publicKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("decompress public key: %w", err)
	}

	addr := address.PubkeyToAddress(*publicKey)
	if addr.String() != from {
		return nil, errors.New("private key does not match from address")
	}

	// Build a TriggerSmartContract transaction for token swaps/bridges.
	// For native TRX transfers, a TransferContract would be used instead.
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

	// Tron ref_block_bytes are the low 2 bytes of the block height, and
	// ref_block_hash is bytes 8..16 of the block ID/hash.
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
	_, err = b.sign(ctx, tx, signer)
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

func (b *Builder) sign(ctx context.Context, tx *core.Transaction, signer domain.Signer) (*core.Transaction, error) {
	rawData, err := proto.Marshal(tx.GetRawData())
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(rawData)
	signature, err := signer.Sign(ctx, h[:])
	if err != nil {
		return nil, err
	}

	tx.Signature = append(tx.Signature, signature)
	return tx, nil
}
