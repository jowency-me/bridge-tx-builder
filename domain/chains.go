// Package domain defines chain-agnostic types and chain metadata for the cross-chain transaction builder.
package domain

import "strconv"

// ChainID identifies a blockchain network.
// The set of supported chains is determined by the registered providers and builders,
// not by this type itself. The constants below are convenience values for well-known chains.
type ChainID string

const (
	// ChainEthereum identifies Ethereum mainnet.
	ChainEthereum ChainID = "ethereum"
	// ChainBase identifies Base mainnet.
	ChainBase ChainID = "base"
	// ChainSolana identifies Solana mainnet.
	ChainSolana ChainID = "solana"
	// ChainTron identifies Tron mainnet.
	ChainTron ChainID = "tron"
	// ChainBSC identifies BNB Smart Chain mainnet.
	ChainBSC ChainID = "bsc"
	// ChainPolygon identifies Polygon mainnet.
	ChainPolygon ChainID = "polygon"
	// ChainArbitrum identifies Arbitrum One mainnet.
	ChainArbitrum ChainID = "arbitrum"
	// ChainOptimism identifies Optimism mainnet.
	ChainOptimism ChainID = "optimism"
	// ChainAvalanche identifies Avalanche C-Chain mainnet.
	ChainAvalanche ChainID = "avalanche"
	// ChainBitcoin identifies Bitcoin mainnet metadata.
	ChainBitcoin ChainID = "bitcoin"
	// ChainCosmos identifies Cosmos ecosystem metadata.
	ChainCosmos ChainID = "cosmos"
)

// Valid returns true if the chain ID is non-empty.
// Full chain support validation is performed by the Router using the registered providers and builders.
func (c ChainID) Valid() bool {
	return c != ""
}

// chainMeta holds canonical chain metadata.
type chainMeta struct {
	NumericID int64
	Name      string
	IsEVM     bool
	EIP1559   bool
}

// chainRegistry is the in-memory lookup for well-known chain metadata.
var chainRegistry = map[ChainID]chainMeta{
	ChainEthereum:  {NumericID: 1, Name: "Ethereum", IsEVM: true, EIP1559: true},
	ChainBase:      {NumericID: 8453, Name: "Base", IsEVM: true, EIP1559: true},
	ChainSolana:    {NumericID: 101, Name: "Solana", IsEVM: false},
	ChainTron:      {NumericID: 728126428, Name: "Tron", IsEVM: false},
	ChainBSC:       {NumericID: 56, Name: "BNB Smart Chain", IsEVM: true, EIP1559: false},
	ChainPolygon:   {NumericID: 137, Name: "Polygon", IsEVM: true, EIP1559: true},
	ChainArbitrum:  {NumericID: 42161, Name: "Arbitrum One", IsEVM: true, EIP1559: true},
	ChainOptimism:  {NumericID: 10, Name: "Optimism", IsEVM: true, EIP1559: true},
	ChainAvalanche: {NumericID: 43114, Name: "Avalanche", IsEVM: true, EIP1559: true},
	ChainBitcoin:   {NumericID: 0, Name: "Bitcoin", IsEVM: false},
	ChainCosmos:    {NumericID: 0, Name: "Cosmos", IsEVM: false},
}

// chainOrder provides deterministic iteration order for NumericToChainID.
var chainOrder = []ChainID{
	ChainEthereum, ChainBase, ChainSolana, ChainTron,
	ChainBSC, ChainPolygon, ChainArbitrum, ChainOptimism,
	ChainAvalanche, ChainBitcoin, ChainCosmos,
}

// NumericID returns the numeric chain ID for a canonical chain ID, or 0 if unknown.
func NumericID(chain ChainID) int64 {
	m, ok := chainRegistry[chain]
	if !ok {
		return 0
	}
	return m.NumericID
}

// IsEVM returns true if the chain is EVM-compatible.
func IsEVM(chain ChainID) bool {
	m, ok := chainRegistry[chain]
	if !ok {
		return false
	}
	return m.IsEVM
}

// SupportsEIP1559 returns true if the chain supports EIP-1559 fee market transactions.
func SupportsEIP1559(chain ChainID) bool {
	m, ok := chainRegistry[chain]
	if !ok {
		return false
	}
	return m.EIP1559
}

// NumericToChainID converts a numeric chain ID string (e.g. "1", "8453") to a canonical ChainID.
func NumericToChainID(s string) ChainID {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return ""
	}
	for _, cid := range chainOrder {
		if chainRegistry[cid].NumericID == n {
			return cid
		}
	}
	return ChainID(s)
}
