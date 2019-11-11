package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/decred/dcrwallet/walletseed"
	"os"
)

var (
	s = flag.Bool("mne", false, "Optional command line argument to print as seed as a mnemonic phrase rather then its hexadecimal form")
	b = flag.Uint("size", 32, "Optional command line argument to print a seed as a certain size.  The default size is 32 and recommended size is between 16 and 64.  Anything under 16 or 64 will cause the program to crash")
)

func main() {
	flag.Parse()

	sb, err := walletseed.GenerateRandomSeed(*b)
	if err != nil {
		fmt.Print(err)
		os.Exit(1)
	}

	var seedString string
	if *s {
		seedString = walletseed.EncodeMnemonic(sb)
		fmt.Printf("%v\n", seedString)
		os.Exit(1)
	}

	seedString = hex.EncodeToString(sb)
	fmt.Printf("%v\n", seedString)
}
