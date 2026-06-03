// Package domain defines chain-agnostic types and chain metadata for the cross-chain transaction builder.
package domain

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
	Name    string
	IsEVM   bool
	EIP1559 bool
}

// chainRegistry is the in-memory lookup for well-known chain metadata.
var chainRegistry = map[ChainID]chainMeta{
	ChainEthereum:  {Name: "Ethereum", IsEVM: true, EIP1559: true},
	ChainBase:      {Name: "Base", IsEVM: true, EIP1559: true},
	ChainSolana:    {Name: "Solana", IsEVM: false},
	ChainTron:      {Name: "Tron", IsEVM: false},
	ChainBSC:       {Name: "BNB Smart Chain", IsEVM: true, EIP1559: true},
	ChainPolygon:   {Name: "Polygon", IsEVM: true, EIP1559: true},
	ChainArbitrum:  {Name: "Arbitrum One", IsEVM: true, EIP1559: true},
	ChainOptimism:  {Name: "Optimism", IsEVM: true, EIP1559: true},
	ChainAvalanche: {Name: "Avalanche", IsEVM: true, EIP1559: true},
	ChainBitcoin:   {Name: "Bitcoin", IsEVM: false},
	ChainCosmos:    {Name: "Cosmos", IsEVM: false},
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
