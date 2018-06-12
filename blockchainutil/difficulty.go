package blockchainutil

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// Difficulty is a struct modeling the concept of how difficult it
//  is to find a hash below a given target (https://en.bitcoin.it/wiki/Difficulty)
//
//  Difficulty has at least three different representations:
//  1) Hex string
//       example: `00000000ffffffffffffffffffffffffffffffffffffffffffffffffffffffff`
//  2) big.Int
//       example: `new(big.Int).Sub(new(big.Int).Lsh(bigOne, 256-32), bigOne)`
//  3) Bits, or in a compact form
//       example: `0x1d00ffff`
//
// All the examples above represent the same entity.
//
//  The purpose of this struct is to properly handle all the representations
//  and to avoid creating multiple occurrences of the same constants in different forms
type Difficulty struct {
	bigInt big.Int
}

//Print outputs Difficulty in all representations for debug purposes
func (dif *Difficulty) Print() *Difficulty {
	fmt.Println("hexstring: ", dif.ToHexString())
	fmt.Println("     Bits: ", dif.ToCompact())
	fmt.Println("  BitsHex: ", fmt.Sprintf("%x", dif.ToCompact()))
	fmt.Println("   bigInt: ", dif.ToBigInt())
	fmt.Println("")
	return dif
}

//ToCompact projects instance into compact representation
func (dif *Difficulty) ToCompact() uint32 {
	return BigToCompact(&dif.bigInt)
}

//ToBigInt projects instance into big integer representation
func (dif *Difficulty) ToBigInt() *big.Int {
	return &dif.bigInt
}

//ToHexString projects instance into hex string representation
func (dif *Difficulty) ToHexString() string {
	return fmt.Sprintf("%064x", &dif.bigInt)
}

//NewDifficultyFromHashString creates a new instance from 64-digits hex
// string. Spaces are allowed for readability. Valid examples inclide:
// 00 00 00 00 ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
// 00 00 00 ff ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
// 7f ff ff ff ffffffffffffffffffffffffffffffffffffffffffffffffffffffff
func NewDifficultyFromHashString(hexstring string) *Difficulty {
	noSpaces := strings.Replace(hexstring, " ", "", -1) //remove spaces if any
	bytes, e := hex.DecodeString(noSpaces)

	if e != nil {
		panic(e) //invalid input is unacceptable
	}

	bigInt := new(big.Int)
	bigInt.SetBytes(bytes)
	return &Difficulty{*bigInt}
}

//NewDifficultyFromCompact creates a new instance from compact form (Bits)
func NewDifficultyFromCompact(compact uint32) *Difficulty {
	bigInt := CompactToBig(compact)
	return &Difficulty{*bigInt}
}
