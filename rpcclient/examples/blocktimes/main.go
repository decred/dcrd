package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrutil"
)

func truncateDuration(duration time.Duration) time.Duration {
	return time.Second * (duration / time.Second)
}

func main() {
	numBlocksToCheck := int64(50)
	if len(os.Args) > 1 {
		numToCheck, err := strconv.ParseInt(os.Args[1], 10, 64)
		if err != nil {
			log.Fatal(err)
		}
		numBlocksToCheck = numToCheck
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

	_, blockHeight, err := client.GetBestBlock()
	if err != nil {
		log.Fatal(err)
	}

	// All blocks if user specified 0.
	if numBlocksToCheck == 0 {
		numBlocksToCheck = blockHeight
	}

	headerTimestamps := make([]time.Time, 0, numBlocksToCheck)
	for i := blockHeight - numBlocksToCheck; i < blockHeight; i++ {
		hash, err := client.GetBlockHash(i)
		if err != nil {
			log.Fatal(err)
		}

		hdr, err := client.GetBlockHeader(hash)
		if err != nil {
			log.Fatal(err)
		}

		headerTimestamps = append(headerTimestamps, hdr.Timestamp)
	}

	timeDiffs := make([]time.Duration, 0, numBlocksToCheck-1)
	var totalDuration time.Duration
	for i := 1; i < len(headerTimestamps); i++ {
		prevTime := headerTimestamps[i-1]
		diff := headerTimestamps[i].Sub(prevTime)

		totalDuration += diff
		timeDiffs = append(timeDiffs, diff)
	}

	averageDuration := totalDuration / (time.Duration(numBlocksToCheck) - 1)
	fmt.Printf("Time between last %d blocks: %v\n", numBlocksToCheck, timeDiffs)
	fmt.Printf("Average time between last %d blocks: %v\n", numBlocksToCheck,
		truncateDuration(averageDuration))

	client.Shutdown()
	client.WaitForShutdown()
}
