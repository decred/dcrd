// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// genesisBlockNode creates a fake chain of blockNodes.  It is used for testing
// the mechanical properties of the version code.
func genesisBlockNode(params *chaincfg.Params) *blockNode {
	// Create a new node from the genesis block.
	genesisBlock := dcrutil.NewBlock(params.GenesisBlock)
	header := &genesisBlock.MsgBlock().Header
	node := newBlockNode(header, genesisBlock.Sha(), 0, []chainhash.Hash{},
		[]chainhash.Hash{}, []uint32{})
	node.inMainChain = true

	return node
}

func TestNilNode(t *testing.T) {
	bc := &BlockChain{
		chainParams: &chaincfg.MainNetParams,
	}
	_, err := bc.calcNextStakeVersion(nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCalcNextStakeVersion(t *testing.T) {

	params := &chaincfg.MainNetParams

	// Calculate super majority for 5 and 3 ticket maxes.
	maxTickets5 := params.StakeDiffWindowSize * int64(params.TicketsPerBlock)
	sm5 := maxTickets5 * params.SuperMajorityMultiplier / params.SuperMajorityDivisor
	maxTickets3 := params.StakeDiffWindowSize * int64(params.TicketsPerBlock-2)
	sm3 := maxTickets3 * params.SuperMajorityMultiplier / params.SuperMajorityDivisor

	// Keep track of ticketcount in set.  Must be reset every test.
	ticketCount := int64(0)

	tests := []struct {
		name                 string
		numNodes             int64
		set                  func(*blockNode)
		headerStakeVersion   uint32
		expectedStakeVersion uint32
		expectedError        error
	}{
		{
			name:                 "too shallow",
			numNodes:             params.StakeValidationHeight + params.StakeDiffWindowSize - 1,
			headerStakeVersion:   1,
			expectedStakeVersion: 1,
			expectedError:        nil,
		},
		{
			name:                 "just enough",
			numNodes:             params.StakeValidationHeight + params.StakeDiffWindowSize,
			headerStakeVersion:   1,
			expectedStakeVersion: 1,
			expectedError:        nil,
		},
		{
			name:                 "odd",
			numNodes:             params.StakeValidationHeight + params.StakeDiffWindowSize + 1,
			headerStakeVersion:   1,
			expectedStakeVersion: 1,
			expectedError:        nil,
		},
		{
			name:     "100%",
			numNodes: params.StakeValidationHeight + (params.StakeDiffWindowSize * 2),
			set: func(b *blockNode) {
				if int64(b.header.Height) > params.StakeValidationHeight {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, 2)
					}
				}
			},
			headerStakeVersion:   1,
			expectedStakeVersion: 2,
			expectedError:        nil,
		},
		{
			name:     "50%",
			numNodes: params.StakeValidationHeight + params.StakeDiffWindowSize,
			set: func(b *blockNode) {
				if int64(b.header.Height) > (params.StakeValidationHeight + (params.StakeDiffWindowSize / 2)) {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, 2)
					}
				} else {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, 1)
					}
				}
			},
			headerStakeVersion:   1,
			expectedStakeVersion: 1,
			expectedError:        nil,
		},
		{
			name:     "95%-1%",
			numNodes: params.StakeValidationHeight + params.StakeDiffWindowSize,
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					for x := 0; x < int(params.TicketsPerBlock); x++ {
						b.voterVersions = append(b.voterVersions, 1)
					}
					return
				}

				threshold := maxTickets5 - sm5 + 1

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
					ticketCount++
				}
			},
			headerStakeVersion:   1,
			expectedStakeVersion: 1,
			expectedError:        nil,
		},
		{
			name:     "95%",
			numNodes: params.StakeValidationHeight + params.StakeDiffWindowSize,
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				threshold := maxTickets5 - sm5

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
					ticketCount++
				}
			},
			headerStakeVersion:   1,
			expectedStakeVersion: 2,
			expectedError:        nil,
		},
		{
			name:     "95%-1 with 3 votes",
			numNodes: params.StakeValidationHeight + params.StakeDiffWindowSize,
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				threshold := maxTickets3 - sm3 + 1

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock-2); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
					ticketCount++
				}
			},
			headerStakeVersion:   1,
			expectedStakeVersion: 1,
			expectedError:        nil,
		},
		{
			name:     "95% with 3 votes",
			numNodes: params.StakeValidationHeight + params.StakeDiffWindowSize,
			set: func(b *blockNode) {
				if int64(b.header.Height) <= params.StakeValidationHeight {
					return
				}

				threshold := maxTickets3 - sm3

				v := uint32(1)
				for x := 0; x < int(params.TicketsPerBlock-2); x++ {
					if ticketCount >= threshold {
						v = 2
					}
					b.voterVersions = append(b.voterVersions, v)
					ticketCount++
				}
			},
			headerStakeVersion:   1,
			expectedStakeVersion: 2,
			expectedError:        nil,
		},
	}

	bc := &BlockChain{
		chainParams: params,
	}
	for _, test := range tests {
		ticketCount = 0

		genesisNode := genesisBlockNode(params)
		genesisNode.header.StakeVersion = test.headerStakeVersion

		var currentNode *blockNode
		currentNode = genesisNode
		for i := int64(1); i <= test.numNodes; i++ {
			// Make up a header.
			header := &wire.BlockHeader{
				Version:      1,
				Height:       uint32(i),
				Nonce:        uint32(0),
				StakeVersion: test.headerStakeVersion,
			}
			node := newBlockNode(header, &chainhash.Hash{}, 0,
				[]chainhash.Hash{}, []chainhash.Hash{},
				[]uint32{})
			node.height = i
			node.parent = currentNode

			// Override version.
			if test.set != nil {
				test.set(node)
			}
			currentNode = node
		}

		version, err := bc.calcNextStakeVersion(currentNode)
		if err != test.expectedError {
			t.Fatalf("failed test %v\n", test.name)
		}
		if version != test.expectedStakeVersion {
			t.Fatalf("%v: expected %v got %v\n", test.name,
				test.expectedStakeVersion, version)
		}
	}
}
