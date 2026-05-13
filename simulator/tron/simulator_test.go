package tron

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestSimulator_Success(t *testing.T) {
	sim := NewSimulator("")

	raw := &core.TransactionRaw{
		Contract: []*core.Transaction_Contract{
			{
				Type: core.Transaction_Contract_TransferContract,
			},
		},
		RefBlockBytes: []byte{0x00, 0x01},
		RefBlockHash:  []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
		Expiration:    0,
		Timestamp:     0,
	}
	rawBytes, err := proto.Marshal(raw)
	require.NoError(t, err)

	domainTx := &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    "TFrom",
		To:      "TTo",
		Data:    rawBytes,
	}

	res, err := sim.Simulate(context.Background(), domainTx)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Zero(t, res.GasUsed)
	assert.Empty(t, res.RevertReason)
}

func TestSimulator_InvalidProtobuf(t *testing.T) {
	sim := NewSimulator("")

	domainTx := &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    "TFrom",
		To:      "TTo",
		Data:    []byte{0xFF, 0xFF, 0xFF},
	}

	res, err := sim.Simulate(context.Background(), domainTx)
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.NotEmpty(t, res.RevertReason)
}

func TestSimulator_NonTronTransaction(t *testing.T) {
	sim := NewSimulator("")

	tx := &domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    "0x1111111111111111111111111111111111111111",
		To:      "0x2222222222222222222222222222222222222222",
	}

	_, err := sim.Simulate(context.Background(), tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a Tron transaction")
}

func TestSimulator_TriggerSmartContract_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/wallet/triggerconstantcontract", r.URL.Path)
		resp := map[string]any{
			"result": map[string]any{
				"code":    "SUCCESS",
				"message": "done",
			},
			"constant_result": []string{"0x"},
			"energy_used":     12345,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	sim := NewSimulator(srv.URL)

	raw := &core.TransactionRaw{
		Contract: []*core.Transaction_Contract{
			{
				Type:      core.Transaction_Contract_TriggerSmartContract,
				Parameter: mustPackTriggerSmartContract("TFrom", "TContract", []byte{0x01}),
			},
		},
		RefBlockBytes: []byte{0x00, 0x01},
		RefBlockHash:  []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
	}
	rawBytes, err := proto.Marshal(raw)
	require.NoError(t, err)

	domainTx := &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    "TFrom",
		To:      "TContract",
		Data:    rawBytes,
	}

	res, err := sim.Simulate(context.Background(), domainTx)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, uint64(12345), res.GasUsed)
	assert.Empty(t, res.RevertReason)
}

func TestSimulator_TriggerSmartContract_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"result": map[string]any{
				"code":    "CONTRACT_VALIDATE_ERROR",
				"message": "validation failed",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	sim := NewSimulator(srv.URL)

	raw := &core.TransactionRaw{
		Contract: []*core.Transaction_Contract{
			{
				Type:      core.Transaction_Contract_TriggerSmartContract,
				Parameter: mustPackTriggerSmartContract("TFrom", "TContract", []byte{0x01}),
			},
		},
		RefBlockBytes: []byte{0x00, 0x01},
		RefBlockHash:  []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
	}
	rawBytes, err := proto.Marshal(raw)
	require.NoError(t, err)

	domainTx := &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    "TFrom",
		To:      "TContract",
		Data:    rawBytes,
	}

	res, err := sim.Simulate(context.Background(), domainTx)
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.RevertReason, "validation failed")
}

func TestSimulator_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sim := NewSimulator(srv.URL)

	raw := &core.TransactionRaw{
		Contract: []*core.Transaction_Contract{
			{
				Type:      core.Transaction_Contract_TriggerSmartContract,
				Parameter: mustPackTriggerSmartContract("TFrom", "TContract", []byte{0x01}),
			},
		},
		RefBlockBytes: []byte{0x00, 0x01},
		RefBlockHash:  []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
	}
	rawBytes, err := proto.Marshal(raw)
	require.NoError(t, err)

	domainTx := &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    "TFrom",
		To:      "TContract",
		Data:    rawBytes,
	}

	res, err := sim.Simulate(context.Background(), domainTx)
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.RevertReason, "http status 500")
}

func TestSimulator_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	sim := NewSimulator(srv.URL)

	raw := &core.TransactionRaw{
		Contract: []*core.Transaction_Contract{
			{
				Type:      core.Transaction_Contract_TriggerSmartContract,
				Parameter: mustPackTriggerSmartContract("TFrom", "TContract", []byte{0x01}),
			},
		},
		RefBlockBytes: []byte{0x00, 0x01},
		RefBlockHash:  []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
	}
	rawBytes, err := proto.Marshal(raw)
	require.NoError(t, err)

	domainTx := &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    "TFrom",
		To:      "TContract",
		Data:    rawBytes,
	}

	res, err := sim.Simulate(context.Background(), domainTx)
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.RevertReason, "decode")
}

func TestSimulator_SignedTransactionData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/wallet/triggerconstantcontract", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result":  map[string]any{"code": "SUCCESS"},
			"receipt": map[string]any{"energy_usage_total": 77},
		})
	}))
	defer srv.Close()

	raw := &core.TransactionRaw{
		Contract: []*core.Transaction_Contract{
			{
				Type:      core.Transaction_Contract_TriggerSmartContract,
				Parameter: mustPackTriggerSmartContract("TFrom", "TContract", []byte{0x01}),
			},
		},
	}
	signed := &core.Transaction{RawData: raw, Signature: [][]byte{{0x01, 0x02}}}
	signedBytes, err := proto.Marshal(signed)
	require.NoError(t, err)

	res, err := NewSimulator(srv.URL).Simulate(context.Background(), &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    "TFrom",
		To:      "TContract",
		Data:    signedBytes,
	})
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, uint64(77), res.GasUsed)
}

func mustPackTriggerSmartContract(owner, contract string, data []byte) *anypb.Any {
	ownerAddr, _ := address.Base58ToAddress(owner)
	contractAddr, _ := address.Base58ToAddress(contract)
	sc := &core.TriggerSmartContract{
		OwnerAddress:    ownerAddr.Bytes(),
		ContractAddress: contractAddr.Bytes(),
		Data:            data,
	}
	b, _ := proto.Marshal(sc)
	return &anypb.Any{TypeUrl: "type.googleapis.com/protocol.TriggerSmartContract", Value: b}
}
