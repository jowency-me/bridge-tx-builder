//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/jowency-me/bridge-tx-builder/domain"
	evmSim "github.com/jowency-me/bridge-tx-builder/simulator/evm"
	solanaSim "github.com/jowency-me/bridge-tx-builder/simulator/solana"
	tronSim "github.com/jowency-me/bridge-tx-builder/simulator/tron"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// rpcNode holds config for a single EVM chain node under test.
type rpcNode struct {
	name     string
	urls     []string
	chainID  int64
	usdcAddr string
}

var evmNodes = []rpcNode{
	{
		name:     "ethereum",
		urls:     publicEthereumRPCURLs,
		chainID:  1,
		usdcAddr: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
	},
	{
		name:     "base",
		urls:     []string{publicBaseRPC, "https://base-rpc.publicnode.com"},
		chainID:  8453,
		usdcAddr: "0xd9aAEc86B65D86f6A7B5B1b0c42FFA531710b6CA",
	},
	{
		name:     "optimism",
		urls:     []string{publicOptimismRPC},
		chainID:  10,
		usdcAddr: "0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85",
	},
	{
		name:     "polygon",
		urls:     []string{publicPolygonRPC},
		chainID:  137,
		usdcAddr: "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174",
	},
	{
		name:     "bsc",
		urls:     []string{publicBSCRPC},
		chainID:  56,
		usdcAddr: "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d",
	},
	{
		name:     "arbitrum",
		urls:     []string{publicArbitrumRPC},
		chainID:  42161,
		usdcAddr: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831",
	},
	{
		name:     "avalanche",
		urls:     []string{publicAvalancheRPC, "https://avalanche-c-chain-rpc.publicnode.com"},
		chainID:  43114,
		usdcAddr: "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E",
	},
}

func solanaRPCHeaders() map[string]string {
	return map[string]string{"User-Agent": "bridge-tx-builder-tests/1.0"}
}

// TestEVMNodes verifies ethclient connectivity, blockNumber, eth_call
// and our Simulator against each provided EVM RPC endpoint.
func TestEVMNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for _, node := range evmNodes {
		urls := node.urls
		if len(urls) == 0 {
			t.Fatalf("no public RPC URLs configured for %s", node.name)
		}

		t.Run(node.name, func(t *testing.T) {
			// 1. Connect with retry — intermittent DNS flakes in CI.
			var client *ethclient.Client
			var selectedURL string
			var latestBlock uint64
			var lastErr error
			for candidateIndex, candidateURL := range urls {
				for i := 0; i < 3; i++ {
					candidateClient, err := ethclient.DialContext(ctx, candidateURL)
					if err != nil {
						lastErr = fmt.Errorf("dial failed")
						t.Logf("rpc candidate %d dial attempt %d failed", candidateIndex+1, i+1)
						time.Sleep(2 * time.Second)
						continue
					}

					blockNumber, err := candidateClient.BlockNumber(ctx)
					if err != nil {
						lastErr = fmt.Errorf("blockNumber failed")
						candidateClient.Close()
						t.Logf("rpc candidate %d blockNumber attempt %d failed", candidateIndex+1, i+1)
						time.Sleep(2 * time.Second)
						continue
					}
					networkID, err := candidateClient.NetworkID(ctx)
					if err != nil {
						lastErr = fmt.Errorf("networkID failed")
						candidateClient.Close()
						t.Logf("rpc candidate %d networkID attempt %d failed", candidateIndex+1, i+1)
						time.Sleep(2 * time.Second)
						continue
					}
					if networkID.Int64() != node.chainID {
						lastErr = fmt.Errorf("networkID got %d, want %d", networkID.Int64(), node.chainID)
						candidateClient.Close()
						break
					}

					client = candidateClient
					selectedURL = candidateURL
					latestBlock = blockNumber
					break
				}
				if selectedURL != "" {
					break
				}
			}
			require.NotEmpty(t, selectedURL, "no reachable mainnet RPC endpoint for %s: %v", node.name, lastErr)
			defer client.Close()

			// 2. Latest block
			require.NotZero(t, latestBlock, "block number must be > 0")
			t.Logf("latest block: %d", latestBlock)

			// 3. eth_call — USDC transfer() against a zero-balance address.
			// Different nodes/clients handle the 'from' field differently, so the
			// call may or may not revert.  We only verify the node processed it.
			data := common.Hex2Bytes("a9059cbb" +
				"0000000000000000000000001111111111111111111111111111111111111111" +
				"00000000000000000000000000000000000000000000000000000000000f4240")
			toAddr := common.HexToAddress(node.usdcAddr)
			_, callErr := client.CallContract(ctx, ethereum.CallMsg{
				From: common.HexToAddress("0x1111111111111111111111111111111111111111"),
				To:   &toAddr,
				Data: data,
			}, nil)
			if callErr != nil {
				t.Logf("eth_call revert: %v", callErr)
			} else {
				t.Logf("eth_call succeeded (node may ignore from address)")
			}

			// 4. Our Simulator
			sim, err := evmSim.NewSimulator(selectedURL)
			require.NoError(t, err, "simulator init failed")

			tx := &domain.Transaction{
				ChainID: domain.ChainID(node.name),
				From:    "0x1111111111111111111111111111111111111111",
				To:      node.usdcAddr,
				Data:    data,
				Value:   decimal.Zero,
				Gas:     300000,
			}
			res, err := sim.Simulate(ctx, tx)
			require.NoError(t, err, "simulate failed")
			require.NotNil(t, res)
			// Node behaviour varies per chain/client; just verify the response is real.
			if res.Success {
				assert.NotZero(t, res.GasUsed, "gasUsed must be reported on success")
			} else {
				assert.NotEmpty(t, res.RevertReason, "revert reason must be provided on failure")
			}
			t.Logf("simulation result: success=%v gasUsed=%d revert=%q",
				res.Success, res.GasUsed, res.RevertReason)
		})
	}
}

// TestTronNode verifies Tron RPC connectivity and our Simulator
// against a real TriggerSmartContract call (USDT transfer) on mainnet.
func TestTronNode(t *testing.T) {
	for _, rpcURL := range publicTronRPCURLs {
		if strings.Contains(strings.ToLower(rpcURL), "nile") || strings.Contains(strings.ToLower(rpcURL), "shasta") {
			t.Fatalf("TRON RPC endpoint must be mainnet, got testnet-looking URL")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Known wallet with TRX for TriggerSmartContract call.
	fromAddr := "TNPdqto8HiuMzoG7Vv9wyyYhWzCojLeHAF"
	// USDT TRC20 contract on Tron.
	usdtContract := "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"

	// Build TriggerSmartContract protobuf for USDT transfer(address, uint256).
	// function selector: a9059cbb (transfer(address,uint256))
	ownerAddr, err := address.Base58ToAddress(fromAddr)
	require.NoError(t, err, "invalid from address")
	contractAddr, err := address.Base58ToAddress(usdtContract)
	require.NoError(t, err, "invalid USDT contract")
	toAddr, err := address.Base58ToAddress("TNPdqto8HiuMzoG7Vv9wyyYhWzCojLeHAF") // self-transfer
	require.NoError(t, err, "invalid to address")

	// a9059cbb + recipient (32 bytes) + amount (32 bytes)
	data := make([]byte, 4+64+64)
	copy(data[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer()
	padr := toAddr.Bytes()
	copy(data[36:68], padr) // recipient (padded left)
	amount := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01} // 1 unit
	copy(data[68:100], amount)

	tsc := &core.TriggerSmartContract{
		OwnerAddress:    ownerAddr.Bytes(),
		ContractAddress: contractAddr.Bytes(),
		Data:            data,
		CallValue:       0,
	}
	param, err := anypb.New(tsc)
	require.NoError(t, err, "pack TriggerSmartContract failed")

	raw := &core.TransactionRaw{
		Contract: []*core.Transaction_Contract{
			{
				Type:      core.Transaction_Contract_TriggerSmartContract,
				Parameter: param,
			},
		},
		RefBlockBytes: []byte{0x00, 0x00},
		RefBlockHash:  []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Timestamp:     0,
		Expiration:    0,
	}

	rawBytes, err := proto.Marshal(raw)
	require.NoError(t, err, "serialize TransactionRaw failed")

	domainTx := &domain.Transaction{
		ChainID: domain.ChainTron,
		From:    fromAddr,
		To:      usdtContract,
		Data:    rawBytes,
	}

	// Simulate via our Simulator.
	var sim *tronSim.Simulator

	var res *domain.SimulationResult
	for _, rpcURL := range publicTronRPCURLs {
		sim = tronSim.NewSimulator(rpcURL)
		for i := 0; i < 2; i++ {
			res, err = sim.Simulate(ctx, domainTx)
			require.NoError(t, err, "Simulate should not return error")
			require.NotNil(t, res, "SimulationResult must not be nil")
			if res.Success || !strings.HasPrefix(strings.ToLower(res.RevertReason), "http:") {
				break
			}
			t.Logf("tron rpc %s simulate transport attempt %d failed: %s", rpcURL, i+1, res.RevertReason)
			time.Sleep(2 * time.Second)
		}
		if res != nil && (res.Success || !strings.HasPrefix(strings.ToLower(res.RevertReason), "http:")) {
			break
		}
	}

	t.Logf("simulation result: success=%v revert=%q", res.Success, res.RevertReason)

	// Either success (balance sufficient) or a benign contract revert (no USDT
	// balance) is acceptable. Transport failures or missing-contract/account errors
	// mean this test did not validate the mainnet USDT TRC20 contract.
	if !res.Success {
		reason := strings.ToLower(res.RevertReason)
		require.NotContains(t, reason, "http:")
		require.NotContains(t, reason, "contract validate error")
		require.NotContains(t, reason, "account does not exist")
	}
}
func TestSolanaNode(t *testing.T) {
	url := publicSolanaRPC

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Verify the RPC node is reachable with retry.
	client := rpc.NewWithHeaders(url, solanaRPCHeaders())
	var health string
	var err error
	for i := 0; i < 3; i++ {
		health, err = client.GetHealth(ctx)
		if err == nil {
			break
		}
		t.Logf("getHealth attempt %d failed: %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	require.NoError(t, err, "getHealth failed after retries")
	t.Logf("Solana health: %s", health)

	genesisHash, err := client.GetGenesisHash(ctx)
	require.NoError(t, err, "getGenesisHash failed")
	require.Equal(t, "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d", genesisHash.String(), "RPC endpoint must be Solana mainnet-beta")

	// 2. Build a real Solana transaction using a known wallet with SOL.
	// This ensures simulation succeeds (not AccountNotFound) and we get real logs.
	wallet := solana.MustPublicKeyFromBase58("6cyreWJtMYptzFXQcnyP1p5G4C8FNW6ZBHuURztDnx32")

	blockhashResp, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	require.NoError(t, err, "getLatestBlockhash failed")
	recentBlockhash := blockhashResp.Value.Blockhash

	// Transfer 1 lamport to self — benign operation that succeeds on-chain.
	dest := solana.MustPublicKeyFromBase58("7F8y61cwBWxgdVHZ17qx4KSfPsdhXLvgMQAfQY6xv9ty")

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				1, // 1 lamport
				wallet,
				dest,
			).Build(),
		},
		recentBlockhash,
		solana.TransactionPayer(wallet),
	)
	require.NoError(t, err, "build transaction failed")

	serialized, err := tx.MarshalBinary()
	require.NoError(t, err, "serialize transaction failed")

	// 3. Simulate via our Simulator — this is the real functional test.
	sim := solanaSim.NewSimulator(url)
	require.NotNil(t, sim, "simulator should be created")

	domainTx := &domain.Transaction{
		ChainID: domain.ChainSolana,
		From:    wallet.String(),
		To:      dest.String(),
		Data:    serialized,
	}

	res, err := sim.Simulate(ctx, domainTx)
	require.NoError(t, err, "Simulate should not return error")
	require.NotNil(t, res, "SimulationResult must not be nil")

	// A random wallet has no lamports, so we expect AccountNotFound.
	// If we get BlockhashNotFound, the node may be stale — either way
	// the RPC processed the transaction and we got a real response.
	t.Logf("simulation result: success=%v revert=%q logs=%v gasUsed=%d",
		res.Success, res.RevertReason, res.Logs, res.GasUsed)

	// The key invariant: we must get a definitive result, not an RPC error.
	isBenignRevert := res.RevertReason == "AccountNotFound" ||
		res.RevertReason == "BlockhashNotFound"
	if !res.Success {
		assert.True(t, isBenignRevert,
			"unexpected revert reason for random-key tx: %s", res.RevertReason)
	}
}
