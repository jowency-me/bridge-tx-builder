//go:build integration

package integration

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/jowency-me/bridge-tx-builder/provider/1inch"
	"github.com/jowency-me/bridge-tx-builder/provider/across"
	"github.com/jowency-me/bridge-tx-builder/provider/celer"
	"github.com/jowency-me/bridge-tx-builder/provider/debridge"
	"github.com/jowency-me/bridge-tx-builder/provider/hop"
	"github.com/jowency-me/bridge-tx-builder/provider/lifi"
	"github.com/jowency-me/bridge-tx-builder/provider/openocean"
	"github.com/jowency-me/bridge-tx-builder/provider/rango"
	"github.com/jowency-me/bridge-tx-builder/provider/socket"
	"github.com/jowency-me/bridge-tx-builder/provider/squid"
	"github.com/jowency-me/bridge-tx-builder/provider/swing"
	"github.com/jowency-me/bridge-tx-builder/provider/thorchain"
	"github.com/jowency-me/bridge-tx-builder/provider/zerox"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// RPC endpoints for public mainnet nodes.
const (
	publicEthereumRPC  = "https://ethereum-rpc.publicnode.com"
	publicBaseRPC      = "https://base-mainnet.public.blastapi.io"
	publicOptimismRPC  = "https://mainnet.optimism.io"
	publicPolygonRPC   = "https://polygon.drpc.org"
	publicBSCRPC       = "https://bsc-dataseed.binance.org"
	publicArbitrumRPC  = "https://arb1.arbitrum.io/rpc"
	publicAvalancheRPC = "https://api.avax.network/ext/bc/C/rpc"
	publicSolanaRPC    = "https://api.mainnet-beta.solana.com"
	publicTronRPC      = "https://api.trongrid.io"
)

var publicEthereumRPCURLs = []string{
	publicEthereumRPC,
	"https://eth-mainnet.public.blastapi.io",
	"https://eth.drpc.org",
}

var publicTronRPCURLs = []string{
	"https://api.tronstack.io",
	publicTronRPC,
}

// dialEthereumRPC connects to the first reachable Ethereum mainnet RPC.
func dialEthereumRPC(t *testing.T, ctx context.Context) (*ethclient.Client, string) {
	t.Helper()

	var lastErr error
	for _, rpcURL := range publicEthereumRPCURLs {
		client, err := ethclient.DialContext(ctx, rpcURL)
		if err != nil {
			lastErr = err
			continue
		}
		networkID, err := client.NetworkID(ctx)
		if err != nil {
			lastErr = err
			client.Close()
			continue
		}
		if networkID.Int64() != 1 {
			lastErr = fmt.Errorf("network ID mismatch")
			client.Close()
			continue
		}
		return client, rpcURL
	}
	require.NoError(t, lastErr, "no public Ethereum mainnet RPC endpoint reachable")
	return nil, ""
}

// evmTransferCalldata builds an ERC-20 transfer(address,uint256) calldata.
func evmTransferCalldata(to string, amount *big.Int) []byte {
	data := make([]byte, 4+32+32)
	copy(data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb})
	copy(data[16:36], common.HexToAddress(to).Bytes())
	copy(data[68-len(amount.Bytes()):68], amount.Bytes())
	return data
}

// generateKey generates a random ECDSA key and returns an EVMSigner and hex address.
func generateKey(t *testing.T) (*domain.EVMPrivateKeySigner, string) {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	raw := crypto.FromECDSA(key)
	signer, err := domain.NewEVMPrivateKeySigner(raw)
	require.NoError(t, err)
	return signer, signer.Address().Hex()
}

// evmGasParams fetches current EIP-1559 gas parameters from Ethereum mainnet.
func evmGasParams(t *testing.T, ctx context.Context, client *ethclient.Client) (tipCap, feeCap *big.Int) {
	t.Helper()

	tc, err := client.SuggestGasTipCap(ctx)
	require.NoError(t, err)
	if tc.Sign() == 0 {
		tc = big.NewInt(2_000_000_000)
	}

	header, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, header.BaseFee)

	fc := new(big.Int).Mul(header.BaseFee, big.NewInt(2))
	fc.Add(fc, tc)
	return tc, fc
}

// makeProviders returns all available real providers, skipping those requiring
// API keys when the key is absent.
func makeProviders(t *testing.T) []domain.Provider {
	t.Helper()

	providers := []domain.Provider{
		lifi.NewProvider(""),
		debridge.NewProvider(),
		across.NewProvider(),
		hop.NewProvider(),
		celer.NewProvider(),
		thorchain.NewProvider(),
		openocean.NewProvider(),
	}

	if key := os.Getenv("RANGO_API_KEY"); key != "" {
		providers = append(providers, rango.NewProvider(key))
	}
	if key := os.Getenv("SOCKET_API_KEY"); key != "" {
		providers = append(providers, socket.NewProvider(socket.WithAPIKey(key)))
	}
	if key := os.Getenv("SWING_PROJECT_ID"); key != "" {
		providers = append(providers, swing.NewProvider(key))
	}
	if key := os.Getenv("INCH_API_KEY"); key != "" {
		providers = append(providers, oneinch.NewProvider(key))
	}
	if key := os.Getenv("ZEROX_API_KEY"); key != "" {
		providers = append(providers, zerox.NewProvider(key))
	}
	if key := os.Getenv("SQUID_INTEGRATOR_ID"); key != "" {
		providers = append(providers, squid.NewProvider(squid.WithAPIKey(key)))
	}

	return providers
}

// usdcEthereum is the canonical USDC token on Ethereum mainnet.
var usdcEthereum = domain.Token{
	Symbol:   "USDC",
	Address:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
	Decimals: 6,
	ChainID:  domain.ChainEthereum,
}

// usdcBase is the canonical USDC token on Base mainnet.
var usdcBase = domain.Token{
	Symbol:   "USDC",
	Address:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
	Decimals: 6,
	ChainID:  domain.ChainBase,
}

// crossChainQuoteRequest returns a standard USDC ETH→Base quote request.
func crossChainQuoteRequest(fromAddr string) domain.QuoteRequest {
	return domain.QuoteRequest{
		FromToken: usdcEthereum,
		ToToken:   usdcBase,
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  fromAddr,
		ToAddr:    fromAddr,
	}
}

// sameChainQuoteRequest returns a USDC→USDT same-chain swap request on Ethereum.
func sameChainQuoteRequest(fromAddr string) domain.QuoteRequest {
	return domain.QuoteRequest{
		FromToken: usdcEthereum,
		ToToken: domain.Token{
			Symbol:   "USDT",
			Address:  "0xdAC17F958D2ee523a2206206994597C13D831ec7",
			Decimals: 6,
			ChainID:  domain.ChainEthereum,
		},
		Amount:   decimal.NewFromInt(1_000_000),
		Slippage: 0.005,
		FromAddr: fromAddr,
		ToAddr:   fromAddr,
	}
}

// mockProvider returns a fixed quote for tests.
type mockProvider struct {
	name  string
	quote *domain.Quote
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Quote(_ context.Context, _ domain.QuoteRequest) (*domain.Quote, error) {
	return m.quote, nil
}
func (m *mockProvider) Status(_ context.Context, _ string) (*domain.Status, error) {
	return &domain.Status{State: "completed"}, nil
}
