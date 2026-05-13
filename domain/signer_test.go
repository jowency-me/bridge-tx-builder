package domain

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// --- EVMPrivateKeySigner ---

func TestNewEVMPrivateKeySigner(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	privBytes := crypto.FromECDSA(key)
	expectedAddr := crypto.PubkeyToAddress(key.PublicKey)

	signer, err := NewEVMPrivateKeySigner(privBytes)
	require.NoError(t, err)
	assert.Equal(t, expectedAddr, signer.Address())
}

func TestNewEVMPrivateKeySigner_InvalidKey(t *testing.T) {
	_, err := NewEVMPrivateKeySigner([]byte("not-a-key"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid EVM private key")
}

func TestEVMPrivateKeySigner_SignTx(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer, err := NewEVMPrivateKeySigner(crypto.FromECDSA(key))
	require.NoError(t, err)

	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	rawTx := types.NewTransaction(0, to, big.NewInt(0), 21000, big.NewInt(1e9), nil)

	signedTx, err := signer.SignTx(rawTx, big.NewInt(1))
	require.NoError(t, err)
	require.NotNil(t, signedTx)

	sender, err := types.LatestSignerForChainID(big.NewInt(1)).Sender(signedTx)
	require.NoError(t, err)
	assert.Equal(t, signer.Address(), sender)
}

func TestNewEVMPrivateKeySigner_AddressFromRawBytes(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	signer, err := NewEVMPrivateKeySigner(crypto.FromECDSA(key))
	require.NoError(t, err)

	expectedAddr := crypto.PubkeyToAddress(*key.Public().(*ecdsa.PublicKey))
	assert.Equal(t, expectedAddr, signer.Address())
}

// --- SolanaPrivateKeySigner ---

func TestNewSolanaPrivateKeySigner(t *testing.T) {
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	signer, err := NewSolanaPrivateKeySigner([]byte(key))
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey(), signer.PublicKey())
}

func TestNewSolanaPrivateKeySigner_InvalidKey(t *testing.T) {
	_, err := NewSolanaPrivateKeySigner([]byte("short"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Solana private key")
}

func TestSolanaPrivateKeySigner_Sign(t *testing.T) {
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	signer, err := NewSolanaPrivateKeySigner([]byte(key))
	require.NoError(t, err)

	recentBlockhash := solana.MustHashFromBase58("11111111111111111111111111111111")
	programID := solana.SystemProgramID

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			solana.NewInstruction(
				programID,
				solana.AccountMetaSlice{
					{PublicKey: signer.PublicKey(), IsSigner: true, IsWritable: true},
				},
				[]byte("test"),
			),
		},
		recentBlockhash,
		solana.TransactionPayer(signer.PublicKey()),
	)
	require.NoError(t, err)

	err = signer.Sign(tx)
	require.NoError(t, err)
	assert.Len(t, tx.Signatures, 1)
}

// --- TronPrivateKeySigner ---

func TestNewTronPrivateKeySigner(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	signer, err := NewTronPrivateKeySigner(crypto.FromECDSA(key))
	require.NoError(t, err)
	assert.NotEmpty(t, signer.Address())
}

func TestNewTronPrivateKeySigner_InvalidKey(t *testing.T) {
	_, err := NewTronPrivateKeySigner([]byte("not-a-key"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Tron private key")
}

func TestTronPrivateKeySigner_Sign(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	signer, err := NewTronPrivateKeySigner(crypto.FromECDSA(key))
	require.NoError(t, err)

	tx := &core.Transaction{
		RawData: &core.TransactionRaw{
			Contract: []*core.Transaction_Contract{
				{
					Type: core.Transaction_Contract_TriggerSmartContract,
				},
			},
			RefBlockBytes: []byte{0x00, 0x01},
			RefBlockHash:  make([]byte, 8),
			Timestamp:     0,
		},
	}

	err = signer.Sign(tx)
	require.NoError(t, err)
	assert.NotEmpty(t, tx.Signature)

	rawBytes, err := proto.Marshal(tx)
	require.NoError(t, err)
	assert.NotEmpty(t, rawBytes)
}
