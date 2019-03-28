// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

var n = flag.Int("n", 1, "prompt n times")

func zero(b []byte) {
	for i := 0; i < len(b); i++ {
		b[i] = 0x00
	}
}

var nl = []byte("\n")

func prompt() {
	fmt.Fprint(os.Stderr, "Secret: ")

	secret, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprint(os.Stderr, "\n")
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to read secret: %v\n", err)
		os.Exit(1)
	}

	_, err = os.Stdout.Write(secret)
	zero(secret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to write to stdout: %v\n", err)
		os.Exit(1)
	}
	os.Stdout.Write(nl)
}

func main() {
	flag.Parse()

	for i := 0; i < *n; i++ {
		prompt()
	}
}
