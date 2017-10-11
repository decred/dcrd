package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// verifyMessage returns whether or not the given signature was produced by
// signing the provided message using the private key associated with the
// provided address .
func verifyMessage(addr dcrutil.Address, signature string, message string) (bool, error) {
	// Only P2PKH addresses are valid for signing.
	if _, ok := addr.(*dcrutil.AddressPubKeyHash); !ok {
		return false, fmt.Errorf("address %q is not a "+
			"pay-to-pubkey-hash address", addr)
	}

	// Decode base64 signature.
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false, fmt.Errorf("failed to decode base64 signature: %v", err)
	}

	// Validate the signature - this just shows that it was valid at all.
	// The key will be computed next.
	var buf bytes.Buffer
	wire.WriteVarString(&buf, 0, "Decred Signed Message:\n")
	wire.WriteVarString(&buf, 0, message)
	expectedMessageHash := chainhash.HashB(buf.Bytes())
	pk, wasCompressed, err := chainec.Secp256k1.RecoverCompact(sig,
		expectedMessageHash)
	if err != nil {
		return false, nil
	}

	// Reconstruct the pubkey hash.
	var serializedPK []byte
	if wasCompressed {
		serializedPK = pk.SerializeCompressed()
	} else {
		serializedPK = pk.SerializeUncompressed()
	}
	expectedAddr, err := dcrutil.NewAddressSecpPubKey(serializedPK,
		&chaincfg.MainNetParams)
	if err != nil {
		return false, nil
	}

	// Return boolean if addresses match.
	return expectedAddr.EncodeAddress() == addr.EncodeAddress(), nil
}

func main() {
	sig := os.Args[1]
	commitmentAddress := os.Args[2]
	message := os.Args[3]

	// TODO: Remove this!
	commitmentAddr, err := dcrutil.DecodeAddress(commitmentAddress)
	if err != nil {
		log.Fatal(err)
	}

	// Connect to local dcrd RPC server using websockets.
	dcrdHomeDir := dcrutil.AppDataDir("dcrd", false)
	certs, err := ioutil.ReadFile(filepath.Join(dcrdHomeDir, "rpc.cert"))
	if err != nil {
		log.Fatal(err)
	}
	connCfg := &rpcclient.ConnConfig{
		Host:         "localhost:9109",
		Endpoint:     "ws",
		User:         "test",
		Pass:         "tabc",
		Certificates: certs,
	}
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}

	blockHash, blockHeight, err := client.GetBestBlock()
	if err != nil {
		log.Fatal(err)
	}

	ticketHashes, err := client.LiveTickets()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Block: %v (height: %d), Num tickets %d", blockHash, blockHeight,
		len(ticketHashes))
	if len(ticketHashes) > 0 {
		tx, err := client.GetRawTransaction(ticketHashes[0])
		if err != nil {
			log.Fatal(err)
		}

		// Ensure the transaction is a ticket.
		isTicket, _ := stake.IsSStx(tx.MsgTx())
		if !isTicket {
			log.Fatalf("tx %s is not a ticket", ticketHashes[0])
		}

		// Extract the commitment address from the ticket.
		commitmentScript := tx.MsgTx().TxOut[1].PkScript
		addr, err := stake.AddrFromSStxPkScrCommitment(commitmentScript,
			&chaincfg.MainNetParams)
		if err != nil {
			log.Fatal(err)
		}

		// TODO: Remove this override!
		addr = commitmentAddr

		// Verify the signature.
		log.Printf("Verifying signature %q of message %q for address %q",
			sig, message, addr)
		verified, err := verifyMessage(addr, sig, message)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("Verified:", verified)
	}

	client.Shutdown()
	client.WaitForShutdown()
}
