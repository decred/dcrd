package main

import (
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrutil"
)

func main() {
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

	_, blockHeight, err := client.GetBestBlock()
	if err != nil {
		log.Fatal(err)
	}

	var totalMissed int
	for i := int64(4096); i < blockHeight; i++ {
		hash, err := client.GetBlockHash(i)
		if err != nil {
			log.Fatal(err)
		}

		hdr, err := client.GetBlockHeader(hash)
		if err != nil {
			log.Fatal(err)
		}

		totalMissed += (5 - int(hdr.Voters))
	}

	log.Printf("Total votes missed up to block height %d: %d", blockHeight,
		totalMissed)

	client.Shutdown()
	client.WaitForShutdown()
}
