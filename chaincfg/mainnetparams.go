// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"math/big"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// MainNetParams returns the network parameters for the main Decred network.
func MainNetParams() *Params {
	// mainPowLimit is the highest proof of work value a Decred block can have
	// for the main network.  It is the value 2^224 - 1.
	mainPowLimit := new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne)

	// mainNetPowLimitBits is the main network proof of work limit in its
	// compact representation.
	//
	// Note that due to the limited precision of the compact representation,
	// this is not exactly equal to the pow limit.  It is the value:
	//
	// 0x00000000ffff0000000000000000000000000000000000000000000000000000
	const mainPowLimitBits = 0x1d00ffff // 486604799

	// genesisBlock defines the genesis block of the block chain which serves as
	// the public transaction ledger for the main network.
	//
	// The genesis block for Decred mainnet, testnet, and simnet are not
	// evaluated for proof of work. The only values that are ever used elsewhere
	// in the blockchain from it are:
	// (1) The genesis block hash is used as the PrevBlock.
	// (2) The difficulty starts off at the value given by Bits.
	// (3) The stake difficulty starts off at the value given by SBits.
	// (4) The timestamp, which guides when blocks can be built on top of it
	//      and what the initial difficulty calculations come out to be.
	//
	// The genesis block is valid by definition and none of the fields within it
	// are validated for correctness.
	genesisBlock := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   1,
			PrevBlock: chainhash.Hash{}, // All zero.
			// MerkleRoot: Calculated below.
			StakeRoot:    chainhash.Hash{},
			Timestamp:    time.Unix(1454954400, 0), // Mon, 08 Feb 2016 18:00:00 GMT
			Bits:         0x1b01ffff,               // Difficulty 32767
			SBits:        2 * 1e8,                  // 2 Coin
			Nonce:        0x00000000,
			StakeVersion: 0,
		},
		Transactions: []*wire.MsgTx{{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: []*wire.TxIn{{
				// Fully null.
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0xffffffff,
					Tree:  0,
				},
				SignatureScript: hexDecode("0000"),
				Sequence:        0xffffffff,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				ValueIn:         wire.NullValueIn,
			}},
			TxOut: []*wire.TxOut{{
				Version: 0x0000,
				Value:   0x00000000,
				PkScript: hexDecode("801679e98561ada96caec2949a5d41c4cab3851e" +
					"b740d951c10ecbcf265c1fd9"),
			}},
			LockTime: 0,
			Expiry:   0,
		}},
	}
	genesisBlock.Header.MerkleRoot = genesisBlock.Transactions[0].TxHashFull()

	return &Params{
		Name:        "mainnet",
		Net:         wire.MainNet,
		DefaultPort: "9108",
		DNSSeeds: []DNSSeed{
			{"mainnet-seed.decred.mindcry.org", true},
			{"mainnet-seed.decred.netpurgatory.com", true},
			{"mainnet-seed.decred.org", true},
		},

		// Chain parameters
		GenesisBlock:         &genesisBlock,
		GenesisHash:          genesisBlock.BlockHash(),
		PowLimit:             mainPowLimit,
		PowLimitBits:         mainPowLimitBits,
		ReduceMinDifficulty:  false,
		MinDiffReductionTime: 0, // Does not apply since ReduceMinDifficulty false
		GenerateSupported:    false,
		MaximumBlockSizes:    []int{393216},
		MaxTxSize:            393216,
		TargetTimePerBlock:   time.Minute * 5,

		// Version 1 difficulty algorithm (EMA + BLAKE256) parameters.
		WorkDiffAlpha:            1,
		WorkDiffWindowSize:       144,
		WorkDiffWindows:          20,
		TargetTimespan:           time.Minute * 5 * 144, // TimePerBlock * WindowSize
		RetargetAdjustmentFactor: 4,

		// Version 2 difficulty algorithm (ASERT + BLAKE3) parameters.
		WorkDiffV2Blake3StartBits: 0x1b00a5a6,
		WorkDiffV2HalfLifeSecs:    43200, // 144 * TimePerBlock (12 hours)

		// Subsidy parameters.
		BaseSubsidy:              3119582664, // 21m
		MulSubsidy:               100,
		DivSubsidy:               101,
		SubsidyReductionInterval: 6144,
		WorkRewardProportion:     6,
		WorkRewardProportionV2:   1,
		StakeRewardProportion:    3,
		StakeRewardProportionV2:  8,
		BlockTaxProportion:       1,

		// AssumeValid is the hash of a block that has been externally verified
		// to be valid.  It allows several validation checks to be skipped for
		// blocks that are both an ancestor of the assumed valid block and an
		// ancestor of the best header.  It is also used to determine the old
		// forks rejection checkpoint.  This is intended to be updated
		// periodically with new releases.
		//
		// Block f04628f2fe7fd0d33055dc326936a6af3772ec5226525bc8fca50631f3081faa
		// Height: 865184
		AssumeValid: *newHashFromStr("f04628f2fe7fd0d33055dc326936a6af3772ec5226525bc8fca50631f3081faa"),

		// MinKnownChainWork is the minimum amount of known total work for the
		// chain at a given point in time.  This is intended to be updated
		// periodically with new releases.
		//
		// Block 0000000000000000149b797ca19e1b4a061ab0e28c73e9c02687c72c8a18bd22
		// Height: 770630
		MinKnownChainWork: hexToBigInt("00000000000000000000000000000000000000000023e312aba3df81d0c21ef0"),

		// The miner confirmation window is defined as:
		//   target proof of work timespan / target proof of work spacing
		RuleChangeActivationQuorum:     4032, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
		RuleChangeActivationMultiplier: 3,    // 75%
		RuleChangeActivationDivisor:    4,
		RuleChangeActivationInterval:   2016 * 4, // 4 weeks
		Deployments: map[uint32][]ConsensusDeployment{
			4: {{
				Vote: Vote{
					Id:          VoteIDSDiffAlgorithm,
					Description: "Change stake difficulty algorithm as defined in DCP0001",
					Mask:        0x0006, // Bits 1 and 2
					Choices: []Choice{{
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
				StartTime:  1493164800, // Apr 26th, 2017
				ExpireTime: 1524700800, // Apr 26th, 2018
			}, {
				Vote: Vote{
					Id:          VoteIDLNSupport,
					Description: "Request developers begin work on Lightning Network (LN) integration",
					Mask:        0x0018, // Bits 3 and 4
					Choices: []Choice{{
						Id:          "abstain",
						Description: "abstain from voting",
						Bits:        0x0000,
						IsAbstain:   true,
						IsNo:        false,
					}, {
						Id:          "no",
						Description: "no, do not work on integrating LN support",
						Bits:        0x0008, // Bit 3
						IsAbstain:   false,
						IsNo:        true,
					}, {
						Id:          "yes",
						Description: "yes, begin work on integrating LN support",
						Bits:        0x0010, // Bit 4
						IsAbstain:   false,
						IsNo:        false,
					}},
				},
				StartTime:  1493164800, // Apr 26th, 2017
				ExpireTime: 1508976000, // Oct 26th, 2017
			}},
			5: {{
				Vote: Vote{
					Id:          VoteIDLNFeatures,
					Description: "Enable features defined in DCP0002 and DCP0003 necessary to support Lightning Network (LN)",
					Mask:        0x0006, // Bits 1 and 2
					Choices: []Choice{{
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
				StartTime:  1505260800, // Sep 13th, 2017
				ExpireTime: 1536796800, // Sep 13th, 2018
			}},
			6: {{
				Vote: Vote{
					Id:          VoteIDFixLNSeqLocks,
					Description: "Modify sequence lock handling as defined in DCP0004",
					Mask:        0x0006, // Bits 1 and 2
					Choices: []Choice{{
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
				StartTime:  1548633600, // Jan 28th, 2019
				ExpireTime: 1580169600, // Jan 28th, 2020
			}},
			7: {{
				Vote: Vote{
					Id:          VoteIDHeaderCommitments,
					Description: "Enable header commitments as defined in DCP0005",
					Mask:        0x0006, // Bits 1 and 2
					Choices: []Choice{{
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
				StartTime:  1567641600, // Sep 5th, 2019
				ExpireTime: 1599264000, // Sep 5th, 2020
			}},
			8: {{
				Vote: Vote{
					Id:          VoteIDTreasury,
					Description: "Enable decentralized Treasury opcodes as defined in DCP0006",
					Mask:        0x0006, // Bits 1 and 2
					Choices: []Choice{{
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
				StartTime:  1596240000, // Aug 1st, 2020
				ExpireTime: 1627776000, // Aug 1st, 2021
			}},
			9: {{
				Vote: Vote{
					Id:          VoteIDRevertTreasuryPolicy,
					Description: "Change maximum treasury expenditure policy as defined in DCP0007",
					Mask:        0x0006, // Bits 1 and 2
					Choices: []Choice{{
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
				StartTime:  1631750400, // Sep 16th, 2021
				ExpireTime: 1694822400, // Sep 16th, 2023
			}, {
				Vote: Vote{
					Id:          VoteIDExplicitVersionUpgrades,
					Description: "Enable explicit version upgrades as defined in DCP0008",
					Mask:        0x0018, // Bits 3 and 4
					Choices: []Choice{{
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
				StartTime:  1631750400, // Sep 16th, 2021
				ExpireTime: 1694822400, // Sep 16th, 2023
			}, {
				Vote: Vote{
					Id:          VoteIDAutoRevocations,
					Description: "Enable automatic ticket revocations as defined in DCP0009",
					Mask:        0x0060, // Bits 5 and 6
					Choices: []Choice{{
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
				StartTime:  1631750400, // Sep 16th, 2021
				ExpireTime: 1694822400, // Sep 16th, 2023
			}, {
				Vote: Vote{
					Id:          VoteIDChangeSubsidySplit,
					Description: "Change block reward subsidy split to 10/80/10 as defined in DCP0010",
					Mask:        0x0180, // Bits 7 and 8
					Choices: []Choice{{
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
				StartTime:  1631750400, // Sep 16th, 2021
				ExpireTime: 1694822400, // Sep 16th, 2023
			}},
			10: {{
				Vote: Vote{
					Id:          VoteIDBlake3Pow,
					Description: "Change proof of work hashing algorithm to BLAKE3 as defined in DCP0011",
					Mask:        0x0006, // Bits 1 and 2
					Choices: []Choice{{
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
				StartTime:  1682294400, // Apr 24th, 2023
				ExpireTime: 1745452800, // Apr 24th, 2025
			}, {
				Vote: Vote{
					Id:          VoteIDChangeSubsidySplitR2,
					Description: "Change block reward subsidy split to 1/89/10 as defined in DCP0012",
					Mask:        0x0060, // Bits 5 and 6
					Choices: []Choice{{
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
				StartTime:  1682294400, // Apr 24th, 2023
				ExpireTime: 1745452800, // Apr 24th, 2025
			}},
		},

		// Enforce current block version once majority of the network has
		// upgraded.
		// 75% (750 / 1000)
		//
		// Reject previous block versions once a majority of the network has
		// upgraded.
		// 95% (950 / 1000)
		BlockEnforceNumRequired: 750,
		BlockRejectNumRequired:  950,
		BlockUpgradeNumToCheck:  1000,

		// AcceptNonStdTxs is a mempool param to either accept and relay non
		// standard txs to the network or reject them
		AcceptNonStdTxs: false,

		// Address encoding magics
		NetworkAddressPrefix: "D",
		PubKeyAddrID:         [2]byte{0x13, 0x86}, // starts with Dk
		PubKeyHashAddrID:     [2]byte{0x07, 0x3f}, // starts with Ds
		PKHEdwardsAddrID:     [2]byte{0x07, 0x1f}, // starts with De
		PKHSchnorrAddrID:     [2]byte{0x07, 0x01}, // starts with DS
		ScriptHashAddrID:     [2]byte{0x07, 0x1a}, // starts with Dc
		PrivateKeyID:         [2]byte{0x22, 0xde}, // starts with Pm

		// BIP32 hierarchical deterministic extended key magics
		HDPrivateKeyID: [4]byte{0x02, 0xfd, 0xa4, 0xe8}, // starts with dprv
		HDPublicKeyID:  [4]byte{0x02, 0xfd, 0xa9, 0x26}, // starts with dpub

		// BIP44 coin type used in the hierarchical deterministic path for
		// address generation.
		SLIP0044CoinType: 42, // SLIP0044, Decred
		LegacyCoinType:   20, // for backwards compatibility

		// Decred PoS parameters
		MinimumStakeDiff:        2 * 1e8, // 2 Coin
		TicketPoolSize:          8192,
		TicketsPerBlock:         5,
		TicketMaturity:          256,
		TicketExpiry:            40960, // 5*TicketPoolSize
		CoinbaseMaturity:        256,
		SStxChangeMaturity:      1,
		TicketPoolSizeWeight:    4,
		StakeDiffAlpha:          1, // Minimal
		StakeDiffWindowSize:     144,
		StakeDiffWindows:        20,
		StakeVersionInterval:    144 * 2 * 7, // ~1 week
		MaxFreshStakePerBlock:   20,          // 4*TicketsPerBlock
		StakeEnabledHeight:      256 + 256,   // CoinbaseMaturity + TicketMaturity
		StakeValidationHeight:   4096,        // ~14 days
		StakeBaseSigScript:      []byte{0x00, 0x00},
		StakeMajorityMultiplier: 3,
		StakeMajorityDivisor:    4,

		// Decred organization related parameters
		// Organization address is Dcur2mcGjmENx4DhNqDctW5wJCVyT3Qeqkx
		OrganizationPkScript:        hexDecode("a914f5916158e3e2c4551c1796708db8367207ed13bb87"),
		OrganizationPkScriptVersion: 0,
		BlockOneLedger:              tokenPayouts_MainNetParams(),

		// Sanctioned Politeia keys.
		PiKeys: [][]byte{
			hexDecode("03f6e7041f1cf51ee10e0a01cd2b0385ce3cd9debaabb2296f7e9dee9329da946c"),
			hexDecode("0319a37405cb4d1691971847d7719cfce70857c0f6e97d7c9174a3998cf0ab86dd"),
		},

		// ~1 day for tspend inclusion
		TreasuryVoteInterval: 288,

		// ~7.2 days for short circuit approval, ~42%
		// target=ticket-pool-equivalent participation
		TreasuryVoteIntervalMultiplier: 12,

		// Sum of tspends within any ~24 day window cannot exceed
		// policy check
		TreasuryExpenditureWindow: 2,

		// policy check is average of prior ~4.8 months + a 50%
		// increase allowance
		TreasuryExpenditurePolicy: 6,

		// 16000 dcr/tew as expense bootstrap
		TreasuryExpenditureBootstrap: 16000 * 1e8,

		TreasuryVoteQuorumMultiplier:   1, // 20% quorum required
		TreasuryVoteQuorumDivisor:      5,
		TreasuryVoteRequiredMultiplier: 3, // 60% yes votes required
		TreasuryVoteRequiredDivisor:    5,

		seeders: []string{
			"mainnet-seed-1.decred.org",
			"mainnet-seed-2.decred.org",
			"mainnet-seed.dcrdata.org",
			"mainnet-seed.jholdstock.uk",
		},
	}
}
