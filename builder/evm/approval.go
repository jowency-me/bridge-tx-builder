package evm

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// erc20ApproveMethodID is the first 4 bytes of keccak256("approve(address,uint256)").
var erc20ApproveMethodID = [4]byte{0x09, 0x5e, 0xa7, 0xb2}

// BuildApproveCallData encodes an ERC-20 approve(spender, amount) call.
func BuildApproveCallData(spender string, amount *big.Int) ([]byte, error) {
	if spender == "" {
		return nil, errors.New("spender address is required")
	}
	if amount == nil {
		return nil, errors.New("amount is required")
	}

	addressType, _ := abi.NewType("address", "", nil)
	uint256Type, _ := abi.NewType("uint256", "", nil)

	args := abi.Arguments{
		{Type: addressType},
		{Type: uint256Type},
	}

	packed, err := args.Pack(common.HexToAddress(spender), amount)
	if err != nil {
		return nil, err
	}

	return append(erc20ApproveMethodID[:], packed...), nil
}
