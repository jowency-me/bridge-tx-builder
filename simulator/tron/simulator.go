// Package tron simulates Tron transactions against a real fullnode using wallet/triggersmartcontract.
package tron

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/jowency-me/bridge-tx-builder/domain"
)

// Simulator dry-runs Tron transactions via the fullnode HTTP API.
type Simulator struct {
	baseURL string
	client  *http.Client
}

// NewSimulator creates a Tron simulator. If rpcURL is empty, falls back to
// the TRON_RPC_URL environment variable.
func NewSimulator(rpcURL string) *Simulator {
	if rpcURL == "" {
		rpcURL = os.Getenv("TRON_RPC_URL")
	}
	return &Simulator{baseURL: rpcURL, client: &http.Client{}}
}

// triggerConstantContractResponse mirrors the Tron HTTP API response.
type triggerConstantContractResponse struct {
	Result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"result"`
	ConstantResult []string `json:"constant_result"`
	EnergyUsed     uint64   `json:"energy_used"`
	Receipt        struct {
		EnergyUsageTotal uint64 `json:"energy_usage_total"`
	} `json:"receipt"`
}

// Simulate sends the transaction to the Tron fullnode wallet/triggerconstantcontract
// endpoint to validate execution without broadcasting.
func (s *Simulator) Simulate(ctx context.Context, tx *domain.Transaction) (*domain.SimulationResult, error) {
	if tx == nil {
		return nil, errors.New("transaction required")
	}
	if tx.ChainID != domain.ChainTron {
		return nil, errors.New("not a Tron transaction")
	}

	// Validate protobuf structure. Builders return a signed core.Transaction,
	// while older callers may pass core.TransactionRaw directly; support both.
	raw, err := decodeTransactionRaw(tx.Data)
	if err != nil {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: fmt.Sprintf("protobuf: %v", err),
		}, nil
	}

	// Extract the TriggerSmartContract payload.
	var tsc *core.TriggerSmartContract
	for _, c := range raw.Contract {
		if c.Type == core.Transaction_Contract_TriggerSmartContract {
			var sc core.TriggerSmartContract
			if err := anypb.UnmarshalTo(c.Parameter, &sc, proto.UnmarshalOptions{}); err == nil {
				tsc = &sc
				break
			}
		}
	}
	if tsc == nil {
		// No smart-contract call to simulate; protobuf validity is sufficient.
		return &domain.SimulationResult{Success: true}, nil
	}
	if s.baseURL == "" {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: "tron simulator: rpc URL required",
		}, nil
	}

	// Build the request for wallet/triggerconstantcontract.
	ownerAddr := address.Address(tsc.OwnerAddress).String()
	contractAddr := address.Address(tsc.ContractAddress).String()

	reqBody, _ := json.Marshal(map[string]any{
		"owner_address":    ownerAddr,
		"contract_address": contractAddr,
		"data":             fmt.Sprintf("0x%x", tsc.Data),
		"call_value":       tsc.CallValue,
		"visible":          true,
	})

	url := s.baseURL + "/wallet/triggerconstantcontract"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: fmt.Sprintf("request: %v", err),
		}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: fmt.Sprintf("http: %v", err),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	var result triggerConstantContractResponse
	if resp.StatusCode >= http.StatusBadRequest {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: fmt.Sprintf("http status %d", resp.StatusCode),
		}, nil
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: fmt.Sprintf("decode: %v", err),
		}, nil
	}

	// Tron returns result.code == "SUCCESS" on success.
	if result.Result.Code != "SUCCESS" && result.Result.Code != "" {
		reason := result.Result.Message
		if reason == "" {
			reason = result.Result.Code
		}
		return &domain.SimulationResult{
			Success:      false,
			RevertReason: reason,
		}, nil
	}

	gasUsed := result.EnergyUsed
	if gasUsed == 0 {
		gasUsed = result.Receipt.EnergyUsageTotal
	}

	return &domain.SimulationResult{Success: true, GasUsed: gasUsed}, nil
}

func decodeTransactionRaw(data []byte) (*core.TransactionRaw, error) {
	var signed core.Transaction
	if err := proto.Unmarshal(data, &signed); err == nil && signed.RawData != nil {
		return signed.RawData, nil
	}

	var raw core.TransactionRaw
	if err := proto.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &raw, nil
}
