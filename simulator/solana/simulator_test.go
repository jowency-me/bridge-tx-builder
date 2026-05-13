package solana

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimulator_Success(t *testing.T) {
	sim := NewSimulator("")

	key := solana.NewWallet()

	// Use a deterministic placeholder blockhash; this unit test exercises local
	// deserialization only and must not depend on any RPC endpoint.
	recentBlockhash := solana.MustHashFromBase58("11111111111111111111111111111111")

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			solana.NewInstruction(
				solana.SystemProgramID,
				solana.AccountMetaSlice{
					{PublicKey: key.PublicKey(), IsSigner: true, IsWritable: true},
				},
				[]byte{0x01},
			),
		},
		recentBlockhash,
		solana.TransactionPayer(key.PublicKey()),
	)
	require.NoError(t, err)

	serialized, err := tx.MarshalBinary()
	require.NoError(t, err)

	domainTx := &domain.Transaction{
		ChainID: domain.ChainSolana,
		From:    key.PublicKey().String(),
		To:      solana.SystemProgramID.String(),
		Data:    serialized,
	}

	res, err := sim.Simulate(context.Background(), domainTx)
	require.NoError(t, err)

	assert.True(t, res.Success)
	assert.Empty(t, res.RevertReason)
}

func TestSimulator_InvalidData(t *testing.T) {
	sim := NewSimulator("")

	domainTx := &domain.Transaction{
		ChainID: domain.ChainSolana,
		From:    "someaddr",
		To:      "someaddr",
		Data:    []byte{0xFF, 0xFF}, // invalid
	}

	res, err := sim.Simulate(context.Background(), domainTx)
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.NotEmpty(t, res.RevertReason)
}

func TestSimulator_RPCSuccess(t *testing.T) {
	unitsConsumed := uint64(42_000)
	fake := &fakeRPCClient{
		response: &rpc.SimulateTransactionResponse{
			Value: &rpc.SimulateTransactionResult{
				Logs:          []string{"Program log: ok"},
				UnitsConsumed: &unitsConsumed,
			},
		},
	}
	sim := &Simulator{client: rpc.NewWithCustomRPCClient(fake)}

	res, err := sim.Simulate(context.Background(), validDomainTx(t))
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, uint64(42_000), res.GasUsed)
	assert.Equal(t, []string{"Program log: ok"}, res.Logs)
	assert.Equal(t, "simulateTransaction", fake.method)
}

func TestSimulator_RPCFailure(t *testing.T) {
	sim := &Simulator{client: rpc.NewWithCustomRPCClient(&fakeRPCClient{err: errors.New("node down")})}

	res, err := sim.Simulate(context.Background(), validDomainTx(t))
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.RevertReason, "rpc: node down")
}

func TestSimulator_RPCEmptyResponse(t *testing.T) {
	sim := &Simulator{client: rpc.NewWithCustomRPCClient(&fakeRPCClient{})}

	res, err := sim.Simulate(context.Background(), validDomainTx(t))
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, "empty simulation response", res.RevertReason)
}

func TestSimulator_RPCReturnsExecutionError(t *testing.T) {
	unitsConsumed := uint64(123)
	sim := &Simulator{client: rpc.NewWithCustomRPCClient(&fakeRPCClient{
		response: &rpc.SimulateTransactionResponse{
			Value: &rpc.SimulateTransactionResult{
				Err:           "AccountNotFound",
				Logs:          []string{"Program log: missing account"},
				UnitsConsumed: &unitsConsumed,
			},
		},
	})}

	res, err := sim.Simulate(context.Background(), validDomainTx(t))
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Equal(t, "AccountNotFound", res.RevertReason)
	assert.Equal(t, []string{"Program log: missing account"}, res.Logs)
	assert.Equal(t, uint64(123), res.GasUsed)
}

func TestSimulator_NonSolanaTransaction(t *testing.T) {
	sim := NewSimulator("")

	tx := &domain.Transaction{
		ChainID: domain.ChainEthereum,
		From:    "0x1111111111111111111111111111111111111111",
		To:      "0x2222222222222222222222222222222222222222",
	}

	_, err := sim.Simulate(context.Background(), tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a Solana transaction")
}

func TestSimulator_NilTransaction(t *testing.T) {
	sim := NewSimulator("")
	_, err := sim.Simulate(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction required")
}

func TestSimulator_NoClientReturnsEarly(t *testing.T) {
	// Create simulator with nil client directly to hit the early-return branch.
	sim := &Simulator{client: nil}

	// Build a real Solana transaction with 0 signatures — it serialises to valid
	// binary but has no signatures so cannot be executed on-chain. Deserialising
	// it succeeds, then the nil-client branch returns early with Success=true.
	wallet := solana.MustPublicKeyFromBase58("6cyreWJtMYptzFXQcnyP1p5G4C8FNW6ZBHuURztDnx32")
	recentBlockhash := solana.Hash(solana.Hash{}) // zero hash — not executed anyway
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				1,
				wallet,
				wallet,
			).Build(),
		},
		recentBlockhash,
		solana.TransactionPayer(wallet),
	)
	require.NoError(t, err)
	// Override Signatures to be empty so numSignatures=0 in the wire format.
	tx.Signatures = nil

	serialized, err := tx.MarshalBinary()
	require.NoError(t, err)

	tx2 := &domain.Transaction{
		ChainID: domain.ChainSolana,
		From:    wallet.String(),
		To:      wallet.String(),
		Data:    serialized,
	}
	res, err := sim.Simulate(context.Background(), tx2)
	require.NoError(t, err)
	assert.True(t, res.Success)
}

func TestSimulator_EmptyResponse(t *testing.T) {
	// Build a real Solana transaction that deserialises successfully.
	wallet := solana.MustPublicKeyFromBase58("6cyreWJtMYptzFXQcnyP1p5G4C8FNW6ZBHuURztDnx32")
	recentBlockhash := solana.Hash(solana.Hash{})
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(1, wallet, wallet).Build(),
		},
		recentBlockhash,
		solana.TransactionPayer(wallet),
	)
	require.NoError(t, err)
	tx.Signatures = nil
	serialized, err := tx.MarshalBinary()
	require.NoError(t, err)

	// nil-client path: verify early return without RPC call.
	sim := &Simulator{client: nil}
	res, err := sim.Simulate(context.Background(), &domain.Transaction{
		ChainID: domain.ChainSolana,
		From:    wallet.String(),
		To:      wallet.String(),
		Data:    serialized,
	})
	require.NoError(t, err)
	assert.True(t, res.Success)
}

func validDomainTx(t *testing.T) *domain.Transaction {
	t.Helper()

	wallet := solana.MustPublicKeyFromBase58("6cyreWJtMYptzFXQcnyP1p5G4C8FNW6ZBHuURztDnx32")
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(1, wallet, wallet).Build(),
		},
		solana.Hash{},
		solana.TransactionPayer(wallet),
	)
	require.NoError(t, err)
	tx.Signatures = nil

	serialized, err := tx.MarshalBinary()
	require.NoError(t, err)

	return &domain.Transaction{
		ChainID: domain.ChainSolana,
		From:    wallet.String(),
		To:      wallet.String(),
		Data:    serialized,
	}
}

type fakeRPCClient struct {
	response *rpc.SimulateTransactionResponse
	err      error
	method   string
}

func (f *fakeRPCClient) CallForInto(_ context.Context, out any, method string, _ []any) error {
	f.method = method
	if f.err != nil {
		return f.err
	}
	responsePtr, ok := out.(**rpc.SimulateTransactionResponse)
	if !ok {
		return errors.New("unexpected response type")
	}
	*responsePtr = f.response
	return nil
}

func (f *fakeRPCClient) CallWithCallback(_ context.Context, _ string, _ []any, _ func(*http.Request, *http.Response) error) error {
	return nil
}

func (f *fakeRPCClient) CallBatch(_ context.Context, _ jsonrpc.RPCRequests) (jsonrpc.RPCResponses, error) {
	return nil, nil
}
