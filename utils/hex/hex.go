// Package hex provides hex-decoding utilities shared across provider adapters.
package hex

import "strconv"

// Decode converts a hex string (without 0x prefix) to a byte slice.
// Odd-length inputs are left-padded with a zero.
func Decode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	res := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		b, err := strconv.ParseUint(s[i:i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		res[i/2] = byte(b)
	}
	return res, nil
}
