package evm

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildApproveCallData_Success(t *testing.T) {
	spender := "0x1234567890123456789012345678901234567890"
	amount := big.NewInt(1000)

	calldata, err := BuildApproveCallData(spender, amount)

	require.NoError(t, err)
	require.NotNil(t, calldata)

	// ERC-20 approve calldata is 4 (method ID) + 32 (address) + 32 (amount) = 68 bytes
	assert.Len(t, calldata, 68, "calldata must be 68 bytes")

	// Verify method ID is 0x095ea7b2
	assert.Equal(t, []byte{0x09, 0x5e, 0xa7, 0xb2}, calldata[:4], "method ID must be 0x095ea7b2")
}

func TestBuildApproveCallData_KnownValue(t *testing.T) {
	// Use a known spender address and amount to verify exact calldata encoding.
	spender := "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"        // USDC on Ethereum
	amount, _ := new(big.Int).SetString("1000000000000000000", 10) // 1e18 in wei

	calldata, err := BuildApproveCallData(spender, amount)

	require.NoError(t, err)
	require.Len(t, calldata, 68)

	// Verify method ID
	assert.Equal(t, []byte{0x09, 0x5e, 0xa7, 0xb2}, calldata[:4])

	// Verify the spender address is encoded correctly (left-padded to 32 bytes).
	// The address portion starts at byte 4 and is 32 bytes long.
	addrBytes := common.HexToAddress(spender)
	expectedAddrPadded := common.LeftPadBytes(addrBytes.Bytes(), 32)
	assert.Equal(t, expectedAddrPadded, calldata[4:36], "spender address encoding mismatch")

	// Verify the amount is encoded correctly (left-padded to 32 bytes).
	expectedAmountPadded := common.LeftPadBytes(amount.Bytes(), 32)
	assert.Equal(t, expectedAmountPadded, calldata[36:68], "amount encoding mismatch")
}

func TestBuildApproveCallData_MaxApproval(t *testing.T) {
	// Max uint256: 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	spender := "0x0000000000000000000000000000000000000001"

	calldata, err := BuildApproveCallData(spender, maxUint256)

	require.NoError(t, err)
	require.Len(t, calldata, 68)

	// Verify method ID
	assert.Equal(t, []byte{0x09, 0x5e, 0xa7, 0xb2}, calldata[:4])

	// For max uint256, all 32 amount bytes should be 0xff
	amountSlice := calldata[36:68]
	for i, b := range amountSlice {
		assert.Equal(t, byte(0xff), b, "byte %d of max uint256 amount should be 0xff", i)
	}

	// Sanity check: the full hex should be the standard approve(max) pattern.
	// Method ID + zero-padded address + all-ff amount.
	expectedHex := "095ea7b2" +
		"0000000000000000000000000000000000000000000000000000000000000001" +
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	assert.Equal(t, expectedHex, hex.EncodeToString(calldata))
}

func TestBuildApproveCallData_ZeroAmount(t *testing.T) {
	spender := "0x1234567890123456789012345678901234567890"
	amount := big.NewInt(0)

	calldata, err := BuildApproveCallData(spender, amount)

	require.NoError(t, err)
	require.Len(t, calldata, 68)

	// Amount portion (bytes 36-68) should be all zeros for zero amount
	for _, b := range calldata[36:68] {
		assert.Equal(t, byte(0), b)
	}
}

func TestBuildApproveCallData_EmptySpender(t *testing.T) {
	_, err := BuildApproveCallData("", big.NewInt(100))

	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "spender")
}

func TestBuildApproveCallData_NilAmount(t *testing.T) {
	_, err := BuildApproveCallData("0x1234567890123456789012345678901234567890", nil)

	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "amount")
}
