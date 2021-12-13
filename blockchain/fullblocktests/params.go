// Copyright (c) 2016-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fullblocktests

import (
	"encoding/hex"
	"math"
	"math/big"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/wire"
)

// newHashFromStr converts the passed big-endian hex string into a
// wire.Hash.  It only differs from the one available in chainhash in that
// it panics on an error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(hexStr)
	if err != nil {
		panic(err)
	}
	return hash
}

// fromHex converts the passed hex string into a byte slice and will panic if
// there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can be detected. It will only (and must only) be
// called for initialization purposes.
func fromHex(s string) []byte {
	r, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return r
}

var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// regNetPowLimit is the highest proof of work value a Decred block
	// can have for the regression test network.  It is the value 2^255 - 1.
	regNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)

	// regNetGenesisBlock defines the genesis block of the block chain which
	// serves as the public transaction ledger for the regression test network.
	regNetGenesisBlock = wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:     1,
			PrevBlock:   *newHashFromStr("0000000000000000000000000000000000000000000000000000000000000000"),
			MerkleRoot:  *newHashFromStr("66aa7491b9adce110585ccab7e3fb5fe280de174530cca10eba2c6c3df01c10d"),
			StakeRoot:   *newHashFromStr("0000000000000000000000000000000000000000000000000000000000000000"),
			VoteBits:    uint16(0x0000),
			FinalState:  [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			Voters:      uint16(0x0000),
			FreshStake:  uint8(0x00),
			Revocations: uint8(0x00),
			Timestamp:   time.Unix(1538524800, 0), // 2018-10-03 00:00:00 +0000 UTC
			PoolSize:    uint32(0),
			Bits:        0x207fffff, // 545259519
			SBits:       int64(0x0000000000000000),
			Nonce:       0x00000000,
			Height:      uint32(0),
		},
		Transactions: []*wire.MsgTx{{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: []*wire.TxIn{{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0xffffffff,
				},
				SignatureScript: fromHex("0000"),
				Sequence:        0xffffffff,
				BlockIndex:      0xffffffff,
				ValueIn:         -1,
			}},
			TxOut: []*wire.TxOut{{
				Value: 0,
				PkScript: fromHex("801679e98561ada96caec2949a" +
					"5d41c4cab3851eb740d951c10ecbcf265c1fd9"),
			}},
			LockTime: 0,
			Expiry:   0,
		}},
		STransactions: nil,
	}
)

// regNetParams defines the network parameters for the regression test network.
//
// NOTE: The test generator intentionally does not use the existing definitions
// in the chaincfg package since the intent is to be able to generate known
// good tests which exercise that code.  Using the chaincfg parameters would
// allow them to change without the tests failing as desired.
var regNetParams = &chaincfg.Params{
	Name:        "regnet",
	Net:         wire.RegNet,
	DefaultPort: "18655",
	DNSSeeds:    nil, // NOTE: There must NOT be any seeds.

	// Chain parameters
	GenesisBlock:             &regNetGenesisBlock,
	GenesisHash:              *newHashFromStr("2ced94b4ae95bba344cfa043268732d230649c640f92dce2d9518823d3057cb0"),
	PowLimit:                 regNetPowLimit,
	PowLimitBits:             0x207fffff,
	ReduceMinDifficulty:      false,
	MinDiffReductionTime:     0, // Does not apply since ReduceMinDifficulty false
	GenerateSupported:        true,
	MaximumBlockSizes:        []int{1000000, 1310720},
	MaxTxSize:                1000000,
	TargetTimePerBlock:       time.Second,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       8,
	WorkDiffWindows:          4,
	TargetTimespan:           time.Second * 8, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:              50000000000,
	MulSubsidy:               100,
	DivSubsidy:               101,
	SubsidyReductionInterval: 128,
	WorkRewardProportion:     6,
	WorkRewardProportionV2:   1,
	StakeRewardProportion:    3,
	StakeRewardProportionV2:  8,
	BlockTaxProportion:       1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// Consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationQuorum:     160, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
	RuleChangeActivationMultiplier: 3,   // 75%
	RuleChangeActivationDivisor:    4,
	RuleChangeActivationInterval:   320, // 320 seconds
	Deployments: map[uint32][]chaincfg.ConsensusDeployment{
		4: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDMaxBlockSize,
				Description: "Change maximum allowed block size from 1MiB to 1.25MB",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "reject changing max allowed block size",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "accept changing max allowed block size",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
		5: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDSDiffAlgorithm,
				Description: "Change stake difficulty algorithm as defined in DCP0001",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing algorithm",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new algorithm",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
		6: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDLNFeatures,
				Description: "Enable features defined in DCP0002 and DCP0003 necessary to support Lightning Network (LN)",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
		7: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDFixLNSeqLocks,
				Description: "Modify sequence lock handling as defined in DCP0004",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
		8: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDHeaderCommitments,
				Description: "Enable header commitments as defined in DCP0005",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
		9: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDTreasury,
				Description: "Enable decentralized Treasury opcodes as defined in DCP0006",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
		10: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDRevertTreasuryPolicy,
				Description: "Change maximum treasury expenditure policy as defined in DCP0007",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}, {
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDExplicitVersionUpgrades,
				Description: "Enable explicit version upgrades as defined in DCP0008",
				Mask:        0x0018, // Bits 3 and 4
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain from voting",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0008, // Bit 3
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0010, // Bit 4
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}, {
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDAutoRevocations,
				Description: "Enable automatic ticket revocations as defined in DCP0009",
				Mask:        0x0060, // Bits 5 and 6
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0020, // Bit 5
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0040, // Bit 6
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}, {
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDChangeSubsidySplit,
				Description: "Change block reward subsidy split to 10/80/10 as defined in DCP0010",
				Mask:        0x0180, // Bits 7 and 8
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain from voting",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0080, // Bit 7
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0100, // Bit 8
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
	},

	// Enforce current block version once majority of the network has
	// upgraded.
	// 51% (51 / 100)
	// Reject previous block versions once a majority of the network has
	// upgraded.
	// 75% (75 / 100)
	BlockEnforceNumRequired: 51,
	BlockRejectNumRequired:  75,
	BlockUpgradeNumToCheck:  100,

	// AcceptNonStdTxs is a Mempool param to accept and relay non standard
	// txs to the network or reject them
	AcceptNonStdTxs: true,

	// Address encoding magics
	NetworkAddressPrefix: "R",
	PubKeyAddrID:         [2]byte{0x25, 0xe5}, // starts with Rk
	PubKeyHashAddrID:     [2]byte{0x0e, 0x00}, // starts with Rs
	PKHEdwardsAddrID:     [2]byte{0x0d, 0xe0}, // starts with Re
	PKHSchnorrAddrID:     [2]byte{0x0d, 0xc2}, // starts with RS
	ScriptHashAddrID:     [2]byte{0x0d, 0xdb}, // starts with Rc
	PrivateKeyID:         [2]byte{0x22, 0xfe}, // starts with Pr

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0xea, 0xb4, 0x04, 0x48}, // starts with rprv
	HDPublicKeyID:  [4]byte{0xea, 0xb4, 0xf9, 0x87}, // starts with rpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	SLIP0044CoinType: 1, // SLIP0044, Testnet (all coins)
	LegacyCoinType:   1,

	// Decred PoS parameters
	MinimumStakeDiff:        20000,
	TicketPoolSize:          64,
	TicketsPerBlock:         5,
	TicketMaturity:          16,
	TicketExpiry:            384, // 6*TicketPoolSize
	CoinbaseMaturity:        16,
	SStxChangeMaturity:      1,
	TicketPoolSizeWeight:    4,
	StakeDiffAlpha:          1,
	StakeDiffWindowSize:     8,
	StakeDiffWindows:        8,
	StakeVersionInterval:    8 * 2 * 7,
	MaxFreshStakePerBlock:   20,            // 4*TicketsPerBlock
	StakeEnabledHeight:      16 + 16,       // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight:   16 + (64 * 2), // CoinbaseMaturity + TicketPoolSize*2
	StakeBaseSigScript:      []byte{0x73, 0x57},
	StakeMajorityMultiplier: 3,
	StakeMajorityDivisor:    4,

	// Decred organization related parameters
	OrganizationPkScript:        fromHex("a9146913bcc838bd0087fb3f6b3c868423d5e300078d87"),
	OrganizationPkScriptVersion: 0,
	BlockOneLedger: []chaincfg.TokenPayout{{
		// RsKrWb7Vny1jnzL1sDLgKTAteh9RZcRr5g6
		ScriptVersion: 0,
		Script:        fromHex("76a9147e4765ae88ba9ad5c9e4715c484e90b34d358d5188ac"),
		Amount:        100000 * 1e8,
	}, {
		// Rs8ca5cDALtsMVD4PV3xvFTC7dmuU1juvLv
		ScriptVersion: 0,
		Script:        fromHex("76a91402fb1ac0137666d79165e13cecd403883615270788ac"),
		Amount:        100000 * 1e8,
	}, {
		// RsHzbGt6YajuHpurtpqXXHz57LmYZK8w9tX
		ScriptVersion: 0,
		Script:        fromHex("76a91469de627d3231b14228653dd09cba75eeb872754288ac"),
		Amount:        100000 * 1e8,
	}},
}
