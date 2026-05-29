package hex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecode_OddLength(t *testing.T) {
	got, err := Decode("abc")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0a, 0xbc}, got)
}

func TestDecode_EvenLength(t *testing.T) {
	got, err := Decode("deadbeef")
	require.NoError(t, err)
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, got)
}

func TestDecode_Empty(t *testing.T) {
	got, err := Decode("")
	require.NoError(t, err)
	assert.Equal(t, []byte{}, got)
}

func TestDecode_InvalidChar(t *testing.T) {
	_, err := Decode("zz")
	require.Error(t, err)
}

func TestDecode_SingleDigit(t *testing.T) {
	got, err := Decode("f")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0f}, got)
}
