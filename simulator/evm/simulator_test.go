package evm

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rpcRequest is a minimal JSON-RPC request envelope for parsing in tests.
type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      interface{}   `json:"id"`
}

func TestSimulator_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		var resp map[string]interface{}
		switch req.Method {
		case "eth_call":
			resp = map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  "0x",
			}
		case "eth_estimateGas":
			resp = map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  "0x5208", // 21000
			}
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	sim, err := NewSimulator(srv.URL)
	require.NoError(t, err)

	tx := &domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    "0x1111111111111111111111111111111111111111",
		To:      "0x2222222222222222222222222222222222222222",
		Data:    common.Hex2Bytes("deadbeef"),
		Value:   decimal.Zero,
		Gas:     200000,
	}

	res, err := sim.Simulate(context.Background(), tx)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Empty(t, res.RevertReason)
}

func TestSimulator_Revert(t *testing.T) {
	// Revert data encoding the string "insufficient allowance"
	// Error selector for Error(string): 0x08c379a0
	// offset: 0x0000000000000000000000000000000000000000000000000000000000000020
	// length: 0x0000000000000000000000000000000000000000000000000000000000000016 (22)
	// data:   insufficient allowance (22 bytes)
	revertData := "0x08c379a00000000000000000000000000000000000000000000000000000000000000020" +
		"0000000000000000000000000000000000000000000000000000000000000016" +
		"696e73756666696369656e7420616c6c6f77616e636500000000000000000000"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		require.Equal(t, "eth_call", req.Method)

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]interface{}{
				"code":    3,
				"message": "execution reverted",
				"data":    revertData,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	sim, err := NewSimulator(srv.URL)
	require.NoError(t, err)

	tx := &domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    "0x1111111111111111111111111111111111111111",
		To:      "0x2222222222222222222222222222222222222222",
		Data:    common.Hex2Bytes("deadbeef"),
		Value:   decimal.Zero,
		Gas:     200000,
	}

	res, err := sim.Simulate(context.Background(), tx)
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.RevertReason, "insufficient allowance")
}

func TestSimulator_SignedRawTransaction(t *testing.T) {
	var callData string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "eth_call" {
			callObject, ok := req.Params[0].(map[string]interface{})
			require.True(t, ok)
			callData, _ = callObject["data"].(string)
			if callData == "" {
				callData, _ = callObject["input"].(string)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": "0x"})
			return
		}
		if req.Method == "eth_estimateGas" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": "0x5208"})
			return
		}
		t.Fatalf("unexpected method: %s", req.Method)
	}))
	defer srv.Close()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	from := crypto.PubkeyToAddress(key.PublicKey)
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	raw := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(1),
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Gas:       200000,
		To:        &to,
		Data:      common.Hex2Bytes("deadbeef"),
	})
	signed, err := types.SignTx(raw, types.LatestSignerForChainID(big.NewInt(1)), key)
	require.NoError(t, err)
	signedBytes, err := signed.MarshalBinary()
	require.NoError(t, err)

	sim, err := NewSimulator(srv.URL)
	require.NoError(t, err)
	res, err := sim.Simulate(context.Background(), &domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    from.Hex(),
		To:      to.Hex(),
		Data:    signedBytes,
		Value:   decimal.Zero,
		Gas:     200000,
	})
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "0xdeadbeef", callData)
}

func TestCallMsgFromTransaction_SignedSenderMismatch(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	raw := types.NewTx(&types.LegacyTx{
		GasPrice: big.NewInt(1),
		Gas:      21000,
		To:       &to,
	})
	signed, err := types.SignTx(raw, types.LatestSignerForChainID(big.NewInt(1)), key)
	require.NoError(t, err)
	signedBytes, err := signed.MarshalBinary()
	require.NoError(t, err)

	_, err = callMsgFromTransaction(&domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    "0x1111111111111111111111111111111111111111",
		To:      to.Hex(),
		Data:    signedBytes,
		Value:   decimal.Zero,
		Gas:     21000,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sender does not match")
}

func TestSimulator_NonEVMTransaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sim, err := NewSimulator(srv.URL)
	require.NoError(t, err)

	tx := &domain.Transaction{
		ChainID: domain.ChainSolana,
		From:    "someaddr",
		To:      "someaddr",
	}

	_, err = sim.Simulate(context.Background(), tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an EVM transaction")
}

func TestSimulator_NilTransaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sim, err := NewSimulator(srv.URL)
	require.NoError(t, err)

	_, err = sim.Simulate(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction required")
}

func TestSimulator_MissingRPC(t *testing.T) {
	_ = os.Unsetenv("ETH_RPC_URL")
	_, err := NewSimulator("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rpc URL required")
}

func TestNewSimulator_EnvFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("ETH_RPC_URL", srv.URL)
	sim, err := NewSimulator("")
	require.NoError(t, err)
	assert.Equal(t, srv.URL, sim.url)
}

func TestParseRevertReason_Nil(t *testing.T) {
	assert.Empty(t, parseRevertReason(nil))
}

func TestParseRevertReason_NoHex(t *testing.T) {
	reason := parseRevertReason(fmt.Errorf("some random error"))
	assert.Equal(t, "some random error", reason)
}

func TestParseRevertReason_InvalidHexInMessage(t *testing.T) {
	reason := parseRevertReason(fmt.Errorf("execution reverted: 0xzzzz}"))
	assert.Equal(t, "execution reverted: 0xzzzz}", reason)
}

func TestParseRevertReason_DataError(t *testing.T) {
	// Create a mock rpc.DataError where ErrorData() returns a hex string
	// whose decodeRevertData successfully extracts a reason.
	// "hello world!" = 12 bytes, padded to 32 bytes in ABI encoding.
	// Error(string) selector + offset 32 + length 12 + "hello world!" + 20 zero bytes
	mock := &mockDataError{data: "0x08c379a0" +
		"0000000000000000000000000000000000000000000000000000000000000020" +
		"000000000000000000000000000000000000000000000000000000000000000c" +
		"68656c6c6f20776f726c6421000000000000000000000000000000000000000000"}
	reason := parseRevertReason(mock)
	assert.Equal(t, "hello world!", reason)
}

func TestParseRevertReason_DataErrorFallbackToMessage(t *testing.T) {
	mock := &mockDataError{data: "0xdeadbeef"}
	reason := parseRevertReason(mock)
	assert.Equal(t, "execution reverted", reason)
}

func TestDecodeRevertData_Invalid(t *testing.T) {
	// Too short
	assert.Equal(t, "", decodeRevertData("0x08c379a0"))
	// Invalid hex
	assert.Equal(t, "", decodeRevertData("0xzzzz"))
	// Valid selector but bad length
	assert.Equal(t, "", decodeRevertData("0x08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000ffff"))
}

func TestSimulator_EstimateGasError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch req.Method {
		case "eth_call":
			// eth_call succeeds but EstimateGas fails.
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  "0x",
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "eth_estimateGas":
			// Return a JSON-RPC error for EstimateGas.
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error": map[string]interface{}{
					"code":    3,
					"message": "gas required exceeds allowance",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
	}))
	defer srv.Close()

	sim, err := NewSimulator(srv.URL)
	require.NoError(t, err)

	tx := &domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    "0x1111111111111111111111111111111111111111",
		To:      "0x2222222222222222222222222222222222222222",
		Data:    common.Hex2Bytes("deadbeef"),
		Value:   decimal.Zero,
		Gas:     200000,
	}

	res, err := sim.Simulate(context.Background(), tx)
	require.NoError(t, err)
	// eth_call succeeds, EstimateGas falls back to tx.Gas.
	assert.True(t, res.Success)
	assert.Equal(t, uint64(200000), res.GasUsed)
}

type mockDataError struct {
	data string
}

func (m *mockDataError) Error() string  { return "execution reverted" }
func (m *mockDataError) ErrorData() any { return m.data }
