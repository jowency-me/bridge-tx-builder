//go:build integration

// Package bridge_tx_builder_test contains end-to-end usage examples for the
// bridge-tx-builder library. Each TestExample_* function is a runnable sample
// that exercises real public mainnet RPC endpoints, so the file is gated by
// the `integration` build tag and is excluded from default `go test` runs.
//
// Run all examples:
//
//	go test -tags=integration -run TestExample_ -v .
//
// The file demonstrates two usage patterns:
//
//  1. Direct builder/simulator usage (TestExample_{EVM,Solana,Tron}BuildAndSimulatePublicMainnet)
//     when the caller has already obtained or hand-built a domain.Quote.
//  2. Router-mediated full flow (TestExample_RouterFullFlowEVMMainnet and
//     TestExample_RouterSelectionStrategies) when the caller wants the
//     library to fan out to multiple providers and pick a winner.
//
// Private keys used here are throwaway test keys; the library never persists
// or transmits them — it only signs in-memory inside the chain builder.
package bridge_tx_builder_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	tronAddress "github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/jowency-me/bridge-tx-builder/builder/evm"
	solanaBuilder "github.com/jowency-me/bridge-tx-builder/builder/solana"
	tronBuilder "github.com/jowency-me/bridge-tx-builder/builder/tron"
	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/jowency-me/bridge-tx-builder/provider/mock"
	"github.com/jowency-me/bridge-tx-builder/router"
	evmSim "github.com/jowency-me/bridge-tx-builder/simulator/evm"
	solanaSim "github.com/jowency-me/bridge-tx-builder/simulator/solana"
	tronSim "github.com/jowency-me/bridge-tx-builder/simulator/tron"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Public mainnet RPC endpoints used by the examples. They are read-only
// (eth_call, simulateTransaction, triggerconstantcontract) — no broadcast.
const (
	exampleEthereumRPC = "https://ethereum-rpc.publicnode.com"
	exampleSolanaRPC   = "https://api.mainnet-beta.solana.com"
	exampleTronRPC     = "https://api.trongrid.io"

	// Throwaway demo private keys. Do NOT use these on mainnet for value transfer.
	// They are only used to derive a deterministic `from` address and to sign
	// the simulated transaction so signature recovery succeeds.
	evmPKHex    = "1d6696230284419a9e566c3eb2796aacf83f822ea828d6aa75327be7ea9ef695"
	solanaPKHex = "358b396ff489e102c035e40a9778f5402528ac0a19e66828f17c1070c9a7901971d4ed402f8ab57da5f364e83035e58c9ffccf437ae6ce81103b957f8ceb6a04"
)

// TestExample_EVMBuildAndSimulatePublicMainnet shows how to build and simulate
// an EIP-1559 ERC-20 transfer on Ethereum mainnet without using the Router or
// any Provider. The caller has already decided what to send and only needs the
// library to assemble + sign the raw transaction and dry-run it.
func TestExample_EVMBuildAndSimulatePublicMainnet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Derive the sender address from the demo private key. In a real app the
	// caller already has both pieces; the library never stores them.
	privateKey := mustHex(t, evmPKHex)
	key, err := crypto.ToECDSA(privateKey)
	require.NoError(t, err)
	from := crypto.PubkeyToAddress(key.PublicKey).Hex()

	// Suggest current EIP-1559 fee caps from the live node. EVM builder
	// requires GasTipCap + GasFeeCap for any EIP-1559-enabled chain.
	gasTipCap, gasFeeCap := evmGasCaps(ctx, t)

	// Assemble the quote manually. Required fields for an EVM EIP-1559 tx:
	//   To / TxData / TxValue:    the call destination, calldata, native value
	//   FromToken.ChainID:        used by the Router (and by the builder for chain dispatch)
	//   EstimateGas:              gas limit; 0 falls back to 300_000 in the builder
	//   GasTipCap / GasFeeCap:    EIP-1559 priority fee and max fee, in wei
	// Nonce is omitted: the builder will use 0 and the caller is expected to
	// patch it via eth_getTransactionCount before broadcast.
	quote := domain.Quote{
		ID:          "example-evm-usdc-transfer",
		Provider:    "example",
		To:          "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC token contract
		TxData:      erc20TransferCalldata("0x1111111111111111111111111111111111111111", big.NewInt(1)),
		TxValue:     decimal.Zero, // ERC-20 transfer carries no native value
		FromToken:   domain.Token{Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:     domain.Token{Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, ChainID: domain.ChainEthereum},
		FromAmount:  decimal.NewFromInt(1),
		ToAmount:    decimal.NewFromInt(1),
		MinAmount:   decimal.NewFromInt(1),
		Slippage:    0.005,
		EstimateGas: 300000,
		GasTipCap:   decimal.NewFromBigInt(gasTipCap, 0),
		GasFeeCap:   decimal.NewFromBigInt(gasFeeCap, 0),
	}

	// Build the signed RLP payload. NewBuilder takes the numeric chain ID
	// (Ethereum mainnet = 1). The returned Transaction.Data is the signed
	// envelope ready for eth_sendRawTransaction.
	tx, err := evm.NewBuilder(1).Build(ctx, quote, from, privateKey)
	require.NoError(t, err)
	require.NotEmpty(t, tx.Data)

	// Dry-run against the live node. Simulator uses eth_call + eth_estimateGas
	// and returns GasUsed plus any revert reason.
	sim, err := evmSim.NewSimulator(exampleEthereumRPC)
	require.NoError(t, err)
	res, err := sim.Simulate(ctx, tx)
	require.NoError(t, err)
	assertMeaningfulSimulation(t, res)
	fmt.Printf("EVM example: success=%v gas=%d revert=%q\n", res.Success, res.GasUsed, res.RevertReason)
}

// TestExample_SolanaBuildAndSimulatePublicMainnet builds a single-instruction
// memo program transaction on Solana mainnet-beta and dry-runs it. Solana
// transactions are replay-protected by a recent blockhash, so BlockHash is a
// required Quote field for this chain.
func TestExample_SolanaBuildAndSimulatePublicMainnet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Solana ed25519 secret keys are 64 bytes (seed || pubkey). PrivateKey()
	// performs no derivation; the bytes are used as-is for signing.
	privateKey := mustHex(t, solanaPKHex)
	solanaKey := solana.PrivateKey(privateKey)

	// Fetch a recent blockhash. Solana rejects transactions whose blockhash
	// is older than ~150 slots (≈ 1 minute), so this must be fetched right
	// before building.
	client := rpc.NewWithHeaders(exampleSolanaRPC, solanaRPCHeaders())
	blockhash, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	require.NoError(t, err)

	// Required Solana Quote fields:
	//   To:        program ID to invoke (here: the Memo program)
	//   TxData:    instruction data passed verbatim to the program
	//   BlockHash: recent blockhash (base58 string)
	memoProgram := solana.MustPublicKeyFromBase58("MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr")
	quote := domain.Quote{
		ID:          "example-solana-memo-instruction",
		Provider:    "example",
		To:          memoProgram.String(),
		TxData:      []byte("bridge-tx-builder example"),
		TxValue:     decimal.Zero,
		FromToken:   domain.Token{Symbol: "SOL", Address: "So11111111111111111111111111111111111111112", Decimals: 9, ChainID: domain.ChainSolana},
		ToToken:     domain.Token{Symbol: "SOL", Address: "So11111111111111111111111111111111111111112", Decimals: 9, ChainID: domain.ChainSolana},
		FromAmount:  decimal.NewFromInt(1),
		ToAmount:    decimal.NewFromInt(1),
		MinAmount:   decimal.NewFromInt(1),
		Slippage:    0.005,
		EstimateGas: 5000,
		BlockHash:   blockhash.Value.Blockhash.String(),
	}

	// Builder serializes the signed Solana transaction (wire format) into tx.Data.
	tx, err := solanaBuilder.NewBuilder().Build(ctx, quote, solanaKey.PublicKey().String(), privateKey)
	require.NoError(t, err)
	require.NotEmpty(t, tx.Data)

	// Simulator calls simulateTransaction; UnitsConsumed is reported as GasUsed.
	res, err := solanaSim.NewSimulator(exampleSolanaRPC).Simulate(ctx, tx)
	require.NoError(t, err)
	assertMeaningfulSimulation(t, res)
	fmt.Printf("Solana example: success=%v gas=%d revert=%q\n", res.Success, res.GasUsed, res.RevertReason)
}

// TestExample_TronBuildAndSimulatePublicMainnet builds a TRC-20 USDT transfer
// on Tron mainnet and dry-runs it via triggerconstantcontract. Tron requires
// BOTH a recent block hash and the matching block height for ref-block derivation.
func TestExample_TronBuildAndSimulatePublicMainnet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Tron uses the same secp256k1 keys as EVM; the address is derived
	// differently (Base58 with checksum) but the private key bytes are identical.
	privateKey := mustHex(t, evmPKHex)
	key, err := crypto.ToECDSA(privateKey)
	require.NoError(t, err)
	from := tronAddress.PubkeyToAddress(key.PublicKey).String()
	usdtContract := "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"

	// Tron requires the ref-block hash AND height; the builder uses these to
	// fill raw_data.ref_block_hash / ref_block_bytes.
	blockID, blockHeight := latestTronBlock(ctx, t)

	// Required Tron Quote fields:
	//   To:          contract address (Base58)
	//   TxData:      contract calldata (selector + ABI-encoded args)
	//   BlockHash:   hex block ID from getnowblock
	//   BlockHeight: matching block number
	quote := domain.Quote{
		ID:          "example-tron-usdt-transfer",
		Provider:    "example",
		To:          usdtContract,
		TxData:      tronTransferCalldata(t, from, big.NewInt(1)),
		TxValue:     decimal.Zero,
		FromToken:   domain.Token{Symbol: "USDT", Address: usdtContract, Decimals: 6, ChainID: domain.ChainTron},
		ToToken:     domain.Token{Symbol: "USDT", Address: usdtContract, Decimals: 6, ChainID: domain.ChainTron},
		FromAmount:  decimal.NewFromInt(1),
		ToAmount:    decimal.NewFromInt(1),
		MinAmount:   decimal.NewFromInt(1),
		Slippage:    0.005,
		EstimateGas: 200000,
		BlockHash:   blockID,
		BlockHeight: blockHeight,
	}

	// Builder produces the signed core.Transaction protobuf bytes.
	tx, err := tronBuilder.NewBuilder().Build(ctx, quote, from, privateKey)
	require.NoError(t, err)
	require.NotEmpty(t, tx.Data)

	res, err := tronSim.NewSimulator(exampleTronRPC).Simulate(ctx, tx)
	require.NoError(t, err)
	assertMeaningfulSimulation(t, res)
	fmt.Printf("Tron example: success=%v revert=%q\n", res.Success, res.RevertReason)
}

// TestExample_RouterFullFlowEVMMainnet shows the canonical end-to-end Router pipeline:
// RegisterProvider -> RegisterBuilder -> RegisterSimulator -> FindProviders -> SelectBest ->
// BuildTransaction -> Simulate. The quote layer is served by a mock.Provider so the test
// stays deterministic; the builder and simulator hit Ethereum mainnet over public RPC.
func TestExample_RouterFullFlowEVMMainnet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	privateKey := mustHex(t, evmPKHex)
	key, err := crypto.ToECDSA(privateKey)
	require.NoError(t, err)
	from := crypto.PubkeyToAddress(key.PublicKey).Hex()

	gasTipCap, gasFeeCap := evmGasCaps(ctx, t)
	usdc := domain.Token{Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, ChainID: domain.ChainEthereum}
	quote := &domain.Quote{
		ID:          "router-example-evm",
		Provider:    "fixture",
		To:          usdc.Address,
		TxData:      erc20TransferCalldata("0x1111111111111111111111111111111111111111", big.NewInt(1)),
		TxValue:     decimal.Zero,
		FromToken:   usdc,
		ToToken:     usdc,
		FromAmount:  decimal.NewFromInt(1),
		ToAmount:    decimal.NewFromInt(1),
		MinAmount:   decimal.NewFromInt(1),
		Slippage:    0.005,
		EstimateGas: 300000,
		GasTipCap:   decimal.NewFromBigInt(gasTipCap, 0),
		GasFeeCap:   decimal.NewFromBigInt(gasFeeCap, 0),
	}

	sim, err := evmSim.NewSimulator(exampleEthereumRPC)
	require.NoError(t, err)

	// Wire up the Router. A real app would register one or more concrete
	// providers (lifi.NewProvider, debridge.NewProvider, ...) — here we use
	// a mock that always returns the quote built above, so the test is
	// deterministic and offline-friendly for the quote layer.
	r := router.New()
	r.RegisterProvider(mock.NewFixedProvider("fixture", quote))
	r.RegisterBuilder(evm.NewBuilder(1))
	r.RegisterSimulator(domain.ChainEthereum, sim)

	// The Router takes a QuoteRequest (intent) rather than a Quote (offer);
	// it fans out to every registered provider and collects their quotes.
	req := domain.QuoteRequest{
		FromToken: usdc,
		ToToken:   usdc,
		Amount:    decimal.NewFromInt(1),
		Slippage:  0.005,
		FromAddr:  from,
		ToAddr:    from,
	}

	// FindProviders reports which providers can quote this request — useful
	// for UI surfaces that want to show "supported routes" before committing.
	names, err := r.FindProviders(ctx, req)
	require.NoError(t, err)
	require.Contains(t, names, "fixture")

	// SelectBest queries all providers concurrently and picks one according
	// to the strategy. StrategyBestAmount maximizes Quote.ToAmount.
	selected, err := r.SelectBest(ctx, req, router.StrategyBestAmount)
	require.NoError(t, err)
	require.Equal(t, "fixture", selected.Provider)

	// BuildTransaction dispatches by Quote.FromToken.ChainID to the matching
	// registered ChainBuilder. The private key is passed in and used only
	// for in-memory signing; it is not retained by the Router or builder.
	tx, err := r.BuildTransaction(ctx, *selected, from, privateKey)
	require.NoError(t, err)
	require.NotEmpty(t, tx.Data)

	// Simulate dispatches by Transaction.ChainID to the registered Simulator.
	res, err := r.Simulate(ctx, tx)
	require.NoError(t, err)
	assertMeaningfulSimulation(t, res)
	fmt.Printf("Router EVM example: provider=%s success=%v gas=%d\n", selected.Provider, res.Success, res.GasUsed)
}

// TestExample_RouterSelectionStrategies exercises the three SelectionStrategy variants
// (StrategyBestAmount, StrategyLowestFee, StrategyNamed) against two mock providers.
// No network calls are made — the goal is to document how the Router decides.
func TestExample_RouterSelectionStrategies(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Two synthetic quotes for the same request:
	//   alpha: better output (995) but higher fee (5)
	//   beta:  lower output (990)  but lower fee (1)
	// Each strategy should pick a different winner.
	tok := domain.Token{Symbol: "USDC", Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, ChainID: domain.ChainEthereum}
	cheapHighOut := &domain.Quote{
		ID: "a", Provider: "alpha",
		FromToken: tok, ToToken: tok,
		FromAmount: decimal.NewFromInt(1000), ToAmount: decimal.NewFromInt(995), MinAmount: decimal.NewFromInt(990),
		Slippage: 0.005, EstimateFee: decimal.NewFromInt(5),
	}
	expensiveLowOut := &domain.Quote{
		ID: "b", Provider: "beta",
		FromToken: tok, ToToken: tok,
		FromAmount: decimal.NewFromInt(1000), ToAmount: decimal.NewFromInt(990), MinAmount: decimal.NewFromInt(985),
		Slippage: 0.005, EstimateFee: decimal.NewFromInt(1),
	}

	r := router.New()
	r.RegisterProvider(mock.NewFixedProvider("alpha", cheapHighOut))
	r.RegisterProvider(mock.NewFixedProvider("beta", expensiveLowOut))

	req := domain.QuoteRequest{
		FromToken: tok, ToToken: tok,
		Amount: decimal.NewFromInt(1000), Slippage: 0.005,
		FromAddr: "0x0000000000000000000000000000000000000001",
		ToAddr:   "0x0000000000000000000000000000000000000002",
	}

	// StrategyBestAmount: maximize Quote.ToAmount (alpha wins with 995 > 990).
	best, err := r.SelectBest(ctx, req, router.StrategyBestAmount)
	require.NoError(t, err)
	require.Equal(t, "alpha", best.Provider)

	// StrategyLowestFee: minimize Quote.EstimateFee (beta wins with 1 < 5).
	cheap, err := r.SelectBest(ctx, req, router.StrategyLowestFee)
	require.NoError(t, err)
	require.Equal(t, "beta", cheap.Provider)

	// StrategyNamed: force a specific provider by name, ignoring metrics.
	forced, err := r.SelectBest(ctx, req, router.StrategyNamed("beta"))
	require.NoError(t, err)
	require.Equal(t, "beta", forced.Provider)
}

// solanaRPCHeaders sets a friendly User-Agent for the public Solana RPC node,
// which throttles unidentified clients more aggressively.
func solanaRPCHeaders() map[string]string {
	return map[string]string{"User-Agent": "bridge-tx-builder-tests/1.0"}
}

// evmGasCaps queries the live node for current EIP-1559 fee parameters and
// returns (tipCap, feeCap = 2*baseFee + tipCap), the standard recommendation
// from go-ethereum's bind.TransactOpts.
func evmGasCaps(ctx context.Context, t *testing.T) (*big.Int, *big.Int) {
	t.Helper()

	client, err := ethclient.DialContext(ctx, exampleEthereumRPC)
	require.NoError(t, err)
	defer client.Close()

	tipCap, err := client.SuggestGasTipCap(ctx)
	require.NoError(t, err)
	header, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, header.BaseFee)

	feeCap := new(big.Int).Mul(header.BaseFee, big.NewInt(2))
	feeCap.Add(feeCap, tipCap)
	return tipCap, feeCap
}

// erc20TransferCalldata encodes a minimal ERC-20 `transfer(address,uint256)`
// call. Selector 0xa9059cbb || left-padded address (32B) || left-padded amount (32B).
func erc20TransferCalldata(to string, amount *big.Int) []byte {
	data := make([]byte, 4+32+32)
	copy(data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb})
	copy(data[16:36], common.HexToAddress(to).Bytes())
	copy(data[68-len(amount.Bytes()):68], amount.Bytes())
	return data
}

// tronTransferCalldata encodes the same ERC-20 transfer selector for a TRC-20
// contract; the recipient is a Tron Base58 address whose underlying 21-byte
// form is right-aligned into a 32-byte word.
func tronTransferCalldata(t *testing.T, to string, amount *big.Int) []byte {
	t.Helper()

	toAddr, err := tronAddress.Base58ToAddress(to)
	require.NoError(t, err)

	data := make([]byte, 4+32+32)
	copy(data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb})
	recipient := toAddr.Bytes()
	copy(data[36-len(recipient):36], recipient)
	copy(data[68-len(amount.Bytes()):68], amount.Bytes())
	return data
}

type tronLatestBlockResponse struct {
	BlockID     string `json:"blockID"`
	BlockHeader struct {
		RawData struct {
			Number uint64 `json:"number"`
		} `json:"raw_data"`
	} `json:"block_header"`
}

// latestTronBlock fetches the most recent Tron block via /wallet/getnowblock
// and returns (blockID hex, block number). Both are required Quote fields
// for the Tron builder's ref-block derivation.
func latestTronBlock(ctx context.Context, t *testing.T) (string, uint64) {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exampleTronRPC+"/wallet/getnowblock", bytes.NewReader([]byte(`{"visible":true}`)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "bridge-tx-builder-tests/1.0")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var block tronLatestBlockResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&block))
	require.NotEmpty(t, block.BlockID)
	require.NotZero(t, block.BlockHeader.RawData.Number)
	return block.BlockID, block.BlockHeader.RawData.Number
}

// assertMeaningfulSimulation guards against degenerate simulation responses:
// a successful simulation must report non-zero gas consumed, and a failed
// simulation must surface a revert reason. Either outcome is acceptable —
// what matters is that the simulator round-trip produced real RPC data.
func assertMeaningfulSimulation(t *testing.T, res *domain.SimulationResult) {
	t.Helper()
	require.NotNil(t, res)
	if res.Success {
		assert.NotZero(t, res.GasUsed)
		return
	}
	assert.NotEmpty(t, res.RevertReason)
}

// mustHex decodes a hex string or fails the test. Used for embedded demo keys.
func mustHex(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	return decoded
}
