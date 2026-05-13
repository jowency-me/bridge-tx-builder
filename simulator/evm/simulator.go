// Package evm simulates EVM transactions against a real RPC node using eth_call.
package evm

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

// Simulator dry-runs EVM transactions via eth_call.
type Simulator struct {
	client *ethclient.Client
	url    string
}

// NewSimulator creates an EVM simulator. If rpcURL is empty, falls back to
// the ETH_RPC_URL environment variable.
func NewSimulator(rpcURL string) (*Simulator, error) {
	if rpcURL == "" {
		rpcURL = os.Getenv("ETH_RPC_URL")
	}
	if rpcURL == "" {
		return nil, errors.New("evm simulator: rpc URL required (set ETH_RPC_URL or pass explicitly)")
	}
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("evm simulator: dial %q: %w", rpcURL, err)
	}
	return &Simulator{client: client, url: rpcURL}, nil
}

// Close releases the RPC client connection.
func (s *Simulator) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

// Simulate executes an eth_call for the given transaction.
func (s *Simulator) Simulate(ctx context.Context, tx *domain.Transaction) (*domain.SimulationResult, error) {
	if tx == nil {
		return nil, errors.New("transaction required")
	}
	if !domain.IsEVM(tx.ChainID) {
		return nil, errors.New("not an EVM transaction")
	}

	msg, err := callMsgFromTransaction(tx)
	if err != nil {
		return nil, err
	}

	// eth_call returns the result bytes or an error containing the revert reason.
	_, err = s.client.CallContract(ctx, msg, nil)
	if err != nil {
		reason := parseRevertReason(err)
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: reason,
		}, nil
	}

	// Estimate actual gas consumption after eth_call succeeds.
	gasUsed, err := s.client.EstimateGas(ctx, msg)
	if err != nil {
		// Fallback to the provided gas limit if estimation fails.
		gasUsed = tx.Gas
	}

	return &domain.SimulationResult{
		Success: true,
		GasUsed: gasUsed,
	}, nil
}

func callMsgFromTransaction(tx *domain.Transaction) (ethereum.CallMsg, error) {
	msg := ethereum.CallMsg{
		From:  common.HexToAddress(tx.From),
		To:    ptr(common.HexToAddress(tx.To)),
		Data:  tx.Data,
		Value: tx.Value.BigInt(),
		Gas:   tx.Gas,
	}

	var signed types.Transaction
	if err := signed.UnmarshalBinary(tx.Data); err != nil {
		return msg, nil
	}

	msg.To = signed.To()
	msg.Data = signed.Data()
	msg.Value = signed.Value()
	msg.Gas = signed.Gas()

	signer := types.LatestSignerForChainID(signed.ChainId())
	sender, err := types.Sender(signer, &signed)
	if err != nil {
		return msg, nil
	}
	if tx.From != "" && common.HexToAddress(tx.From) != sender {
		return ethereum.CallMsg{}, errors.New("signed transaction sender does not match from address")
	}
	msg.From = sender

	return msg, nil
}

// parseRevertReason extracts a human-readable revert reason from an ethclient
// error. It first tries to read the raw JSON-RPC error data, then falls back to
// parsing the error message string.
func parseRevertReason(err error) string {
	if err == nil {
		return ""
	}

	// Try to get the raw revert data from the RPC error.
	if de, ok := err.(rpc.DataError); ok {
		if dataStr, ok := de.ErrorData().(string); ok && strings.HasPrefix(dataStr, "0x") {
			if reason := decodeRevertData(dataStr); reason != "" {
				return reason
			}
		}
	}

	s := err.Error()
	// Fallback: try to find a 0x-prefixed hex string in the error message.
	idx := strings.LastIndex(s, "0x")
	if idx == -1 {
		return s
	}
	hexData := s[idx:]
	hexData = strings.TrimSpace(hexData)
	hexData = strings.TrimSuffix(hexData, ".")
	hexData = strings.TrimSuffix(hexData, ")")
	hexData = strings.TrimSuffix(hexData, "}")

	if reason := decodeRevertData(hexData); reason != "" {
		return reason
	}
	return s
}

// decodeRevertData decodes abi-encoded Error(string) revert data.
func decodeRevertData(hexData string) string {
	decoded, err := hex.DecodeString(strings.TrimPrefix(hexData, "0x"))
	if err != nil || len(decoded) < 68 {
		return ""
	}
	// abi-encoded string: 4 bytes selector + 32 offset + 32 length + data
	length := new(big.Int).SetBytes(decoded[36:68]).Int64()
	if length < 0 || int(length) > len(decoded)-68 {
		return ""
	}
	reason := string(decoded[68 : 68+length])
	reason = strings.TrimRight(reason, "\x00")
	return reason
}

func ptr(a common.Address) *common.Address { return &a }
