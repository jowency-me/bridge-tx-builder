# Cross-Chain Transaction Builder

Go library for querying cross-chain quotes, selecting routes, building sign-ready transactions, and simulating transactions against real chain RPC nodes.

## Features

- Query and normalize quotes from implemented bridge/DEX aggregators.
- Build chain-specific transactions for EVM-compatible chains, Solana, and Tron.
- Simulate transactions before broadcast through real RPC/fullnode endpoints.
- Keep core domain types chain-agnostic and provider-independent.

## Implemented Providers

- LI.FI
- deBridge
- Rango
- Squid Router
- Swing.xyz
- OpenOcean
- 1inch
- 0x
- THORChain
- Across
- Hop
- Celer cBridge
- Socket

## Provider Endpoints

The provider clients target production mainnet APIs only:

| Provider | Production endpoint used | Transaction data |
|---|---|---|
| LI.FI | `https://li.quest/v1/quote` | `transactionRequest` |
| deBridge | `https://dln.debridge.finance/v1.0/dln/order/create-tx` | `tx` |
| Squid Router | `https://v2.api.squidrouter.com/v2/route` | `route.transactionRequest` |
| Across | `https://app.across.to/api/swap/approval` | `swapTx` |
| 1inch | `https://api.1inch.dev/swap/v6.1/{chain}/swap` | `tx` |
| 0x | `https://api.0x.org/swap/allowance-holder/quote` with `0x-version: v2` | `transaction` |
| OpenOcean | `https://open-api.openocean.finance/v3/{chain}/swap_quote` | `data` |
| Rango | `https://api.rango.exchange/basic/quote` | quote-only in current adapter |
| Socket | `https://api.socket.tech/v2/quote` | `userTxs` |
| Swing | `https://swap.prod.swing.xyz/v0/transfer/quote` | quote-only in current adapter |
| Hop | `https://api.hop.exchange/v1/quote` | quote-only in current adapter |
| Celer cBridge | `https://cbridge-prod2.celer.app/v2/estimateAmt` | quote-only in current adapter |
| THORChain | `https://thornode.ninerealms.com/thorchain/quote/swap` | memo/inbound quote only |

Single-chain DEX adapters (`1inch`, `0x`, `OpenOcean`) reject cross-chain requests before calling their APIs.

## Chain Support

- **Transaction builders/simulators:** EVM-compatible chains, Solana, Tron.
- **EVM mainnet IDs covered by tests:** Ethereum, Base, BNB Smart Chain, Polygon, Arbitrum One, Optimism, Avalanche C-Chain.
- **Metadata-only constants:** Bitcoin and Cosmos currently exist as domain metadata for provider mappings; there is no transaction builder or simulator for them yet.

## Key Handling

This library does not store or manage private keys. It only constructs transaction payloads. When a builder needs to produce a signed payload, the caller passes the private key bytes in directly; the library uses them only to sign in-memory and never persists, logs, or transmits them. Callers may also build unsigned payloads and sign externally.

## Usage

Register providers, builders, and simulators on a `router.Router`, then select → build → simulate:

```go
r := router.New()
r.RegisterProvider(lifi.NewProvider(os.Getenv("LIFI_API_KEY")))
r.RegisterBuilder(evm.NewBuilder(1))
r.RegisterSimulator(domain.ChainEthereum, evmSim)

quote, err := r.SelectBest(ctx, req, router.StrategyBestAmount)
if err != nil { return err }

tx, err := r.BuildTransaction(ctx, *quote, fromAddr, privateKey)
if err != nil { return err }

res, err := r.Simulate(ctx, tx)
```

`Router` methods: `RegisterProvider` / `RegisterBuilder` / `RegisterSimulator` / `FindProviders` (list providers that quote a request) / `SelectBest` / `BuildTransaction` / `Simulate`. All are safe for concurrent use.

Selection strategies: `StrategyBestAmount` (max `ToAmount`), `StrategyLowestFee` (min `EstimateFee`), `StrategyNamed("lifi")` (force a specific provider).

See `example_test.go` for end-to-end EVM, Solana, and Tron flows (run with `-tags=integration`).

## Provider Constructors

| Provider | Constructor |
|---|---|
| LI.FI | `lifi.NewProvider(apiKey, opts...)` |
| Rango | `rango.NewProvider(apiKey, opts...)` |
| Swing | `swing.NewProvider(projectID, opts...)` |
| 1inch | `inch.NewProvider(apiKey, opts...)` |
| 0x | `zerox.NewProvider(apiKey, opts...)` |
| Squid / deBridge / Across / Hop / Celer / Socket / OpenOcean / THORChain | `NewProvider(opts...)` (no required arg; some honor `WithAPIKey` / `WithIntegratorID` options) |

## Builders & Simulators

| Constructor | Notes |
|---|---|
| `evm.NewBuilder(numericChainID)` | Pass numeric chain ID (Ethereum=1, Base=8453, BSC=56, Polygon=137, Arbitrum=42161, Optimism=10, Avalanche=43114). Use `domain.NumericID(domain.ChainEthereum)` to look up. |
| `solana.NewBuilder()` | No arg. |
| `tron.NewBuilder()` | No arg. |
| `evm.NewSimulator(rpcURL)` | Empty `rpcURL` falls back to `ETH_RPC_URL`. Returns error if unreachable. |
| `solana.NewSimulator(rpcURL)` | Empty falls back to `SOLANA_RPC_URL`. |
| `tron.NewSimulator(rpcURL)` | Empty falls back to `TRON_RPC_URL`. |

## Quote Fields by Chain

Providers normally populate all needed fields; when constructing a `Quote` manually, the following are required in addition to the common fields (`ID`, `Provider`, `From/ToToken`, `From/To/MinAmount`, `Slippage`, `To`, `TxData`, `TxValue`):

| Chain | Required extra fields |
|---|---|
| EVM (EIP-1559: Ethereum, Base, Polygon, Arbitrum, Optimism, Avalanche) | `EstimateGas`, `GasTipCap`, `GasFeeCap`; optional `Nonce` (nil → caller-managed off-chain) |
| EVM legacy (BSC) | `EstimateGas`, `GasPrice`; optional `Nonce` |
| Solana | `BlockHash` (recent blockhash from `GetLatestBlockhash`) |
| Tron | `BlockHash` (block ID hex) and `BlockHeight` |

## Environment Variables

Provider API keys:

| Variable | Provider |
|---|---|
| `LIFI_API_KEY` | LI.FI |
| `RANGO_API_KEY` | Rango |
| `SOCKET_API_KEY` | Socket |
| `SWING_PROJECT_ID` | Swing.xyz |
| `INCH_API_KEY` | 1inch |
| `ZEROX_API_KEY` | 0x |
| `ACROSS_API_KEY` | Across Swap API |
| `ACROSS_INTEGRATOR_ID` | Across Swap API integrator ID |

Runtime simulators can accept RPC URLs directly; when an empty URL is passed they fall back to these optional environment variables:

| Variable | Simulator |
|---|---|
| `ETH_RPC_URL` | EVM simulator fallback |
| `SOLANA_RPC_URL` | Solana simulator fallback |
| `TRON_RPC_URL` | Tron simulator fallback |

Integration tests do not read private deployment files or private RPC variables. They use hardcoded public mainnet RPC nodes for Ethereum, Base, BNB Smart Chain, Polygon, Arbitrum One, Optimism, Avalanche C-Chain, Solana mainnet-beta, and Tron mainnet.

## Testing

Unit tests and coverage:

```bash
go test ./... -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out
```

Quality gates:

```bash
gofmt -w .
go vet ./...
golangci-lint run ./...
go test ./...
```

Mainnet integration tests require real RPC endpoints and the integration build tag:

```bash
go test -tags=integration ./tests/integration/...
```

The integration suite verifies the configured EVM RPC chain IDs, Solana mainnet-beta genesis hash, Tron mainnet contract simulation path, and the router build/simulate flow when a live provider returns transaction data.
