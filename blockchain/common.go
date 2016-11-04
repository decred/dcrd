package blockchain

import (
	"bytes"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// testDbRoot is the root directory used to create all test databases.
	testDbRoot = "testdbs_internalapi"

	// blockDataNet is the expected network in the test block data.
	blockDataNet = wire.SimNet
)

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// isSupportedDbType returns whether or not the passed database type is
// currently supported.
func isSupportedDbType(dbType string) bool {
	supportedDrivers := database.SupportedDrivers()
	for _, driver := range supportedDrivers {
		if dbType == driver {
			return true
		}
	}

	return false
}

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.  In addition to the new chain instnce, it returns
// a teardown function the caller should invoke when done testing to clean up.
func chainSetup(dbName string, params *chaincfg.Params) (*BlockChain, func(), error) {
	if !isSupportedDbType(testDbType) {
		return nil, nil, fmt.Errorf("unsupported db type %v", testDbType)
	}

	// Handle memory database specially since it doesn't need the disk
	// specific handling.
	var db database.DB
	var teardown func()
	if testDbType == "memdb" {
		ndb, err := database.Create(testDbType)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			db.Close()
		}
	} else {
		// Create the root directory for test databases.
		if !fileExists(testDbRoot) {
			if err := os.MkdirAll(testDbRoot, 0700); err != nil {
				err := fmt.Errorf("unable to create test db "+
					"root: %v", err)
				return nil, nil, err
			}
		}

		// Create a new database to store the accepted blocks into.
		dbPath := filepath.Join(testDbRoot, dbName)
		_ = os.RemoveAll(dbPath)
		ndb, err := database.Create(testDbType, dbPath, blockDataNet)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			db.Close()
			os.RemoveAll(dbPath)
			os.RemoveAll(testDbRoot)
		}
	}

	// Create the main chain instance.
	chain, err := New(&Config{
		DB:          db,
		ChainParams: params,
	})

	if err != nil {
		teardown()
		err := fmt.Errorf("failed to create chain instance: %v", err)
		return nil, nil, err
	}

	return chain, teardown, nil
}

// DoStxoTest does a test on a simulated blockchain to ensure that the data
// stored in the STXO buckets is not corrupt.
func (b *BlockChain) DoStxoTest() error {
	err := b.db.View(func(dbTx database.Tx) error {
		for i := int64(2); i <= b.bestNode.height; i++ {
			block, err := dbFetchBlockByHeight(dbTx, i)
			if err != nil {
				return err
			}

			parent, err := dbFetchBlockByHeight(dbTx, i-1)
			if err != nil {
				return err
			}

			ntx := countSpentOutputs(block, parent)
			stxos, err := dbFetchSpendJournalEntry(dbTx, block, parent)
			if err != nil {
				return err
			}

			if int(ntx) != len(stxos) {
				return fmt.Errorf("bad number of stxos calculated at "+
					"height %v, got %v expected %v",
					i, len(stxos), int(ntx))
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// DebugBlockHeaderString dumps a verbose message containing information about
// the block header of a block.
func DebugBlockHeaderString(chainParams *chaincfg.Params,
	block *dcrutil.Block) string {
	bh := block.MsgBlock().Header

	var buffer bytes.Buffer

	str := fmt.Sprintf("Version: %v\n", bh.Version)
	buffer.WriteString(str)

	str = fmt.Sprintf("Previous block: %v\n", bh.PrevBlock)
	buffer.WriteString(str)

	str = fmt.Sprintf("Merkle root (reg): %v\n", bh.MerkleRoot)
	buffer.WriteString(str)

	str = fmt.Sprintf("Merkle root (stk): %v\n", bh.StakeRoot)
	buffer.WriteString(str)

	str = fmt.Sprintf("VoteBits: %v\n", bh.VoteBits)
	buffer.WriteString(str)

	str = fmt.Sprintf("FinalState: %v\n", bh.FinalState)
	buffer.WriteString(str)

	str = fmt.Sprintf("Voters: %v\n", bh.Voters)
	buffer.WriteString(str)

	str = fmt.Sprintf("FreshStake: %v\n", bh.FreshStake)
	buffer.WriteString(str)

	str = fmt.Sprintf("Revocations: %v\n", bh.Revocations)
	buffer.WriteString(str)

	str = fmt.Sprintf("PoolSize: %v\n", bh.PoolSize)
	buffer.WriteString(str)

	str = fmt.Sprintf("Timestamp: %v\n", bh.Timestamp)
	buffer.WriteString(str)

	bitsBig := CompactToBig(bh.Bits)
	if bitsBig.Cmp(bigZero) != 0 {
		bitsBig.Div(chainParams.PowLimit, bitsBig)
	}
	diff := bitsBig.Int64()
	str = fmt.Sprintf("Bits: %v (Difficulty: %v)\n", bh.Bits, diff)
	buffer.WriteString(str)

	str = fmt.Sprintf("SBits: %v (In coins: %v)\n", bh.SBits,
		float64(bh.SBits)/dcrutil.AtomsPerCoin)
	buffer.WriteString(str)

	str = fmt.Sprintf("Nonce: %v \n", bh.Nonce)
	buffer.WriteString(str)

	str = fmt.Sprintf("Height: %v \n", bh.Height)
	buffer.WriteString(str)

	str = fmt.Sprintf("Size: %v \n", bh.Size)
	buffer.WriteString(str)

	return buffer.String()
}

// DebugBlockString dumps a verbose message containing information about
// the transactions of a block.
func DebugBlockString(block *dcrutil.Block) string {
	if block == nil {
		return "block pointer nil"
	}

	var buffer bytes.Buffer

	hash := block.Sha()

	str := fmt.Sprintf("Block Header: %v Height: %v \n",
		hash, block.Height())
	buffer.WriteString(str)

	str = fmt.Sprintf("Block contains %v regular transactions "+
		"and %v stake transactions \n",
		len(block.Transactions()),
		len(block.STransactions()))
	buffer.WriteString(str)

	str = fmt.Sprintf("List of regular transactions \n")
	buffer.WriteString(str)

	for i, tx := range block.Transactions() {
		str = fmt.Sprintf("Index: %v, Hash: %v \n", i, tx.Sha())
		buffer.WriteString(str)
	}

	if len(block.STransactions()) == 0 {
		return buffer.String()
	}

	str = fmt.Sprintf("List of stake transactions \n")
	buffer.WriteString(str)

	for i, stx := range block.STransactions() {
		txTypeStr := ""
		txType := stake.DetermineTxType(stx.MsgTx())
		switch txType {
		case stake.TxTypeSStx:
			txTypeStr = "SStx"
		case stake.TxTypeSSGen:
			txTypeStr = "SSGen"
		case stake.TxTypeSSRtx:
			txTypeStr = "SSRtx"
		default:
			txTypeStr = "Error"
		}

		str = fmt.Sprintf("Index: %v, Type: %v, Hash: %v \n",
			i, txTypeStr, stx.Sha())
		buffer.WriteString(str)
	}

	return buffer.String()
}

// DebugMsgTxString dumps a verbose message containing information about the
// contents of a transaction.
func DebugMsgTxString(msgTx *wire.MsgTx) string {
	isSStx, _ := stake.IsSStx(msgTx)
	isSSGen, _ := stake.IsSSGen(msgTx)
	var sstxType []bool
	var sstxPkhs [][]byte
	var sstxAmts []int64
	var sstxRules [][]bool
	var sstxLimits [][]uint16

	if isSStx {
		sstxType, sstxPkhs, sstxAmts, _, sstxRules, sstxLimits =
			stake.TxSStxStakeOutputInfo(msgTx)
	}

	var buffer bytes.Buffer

	hash := msgTx.TxSha()
	str := fmt.Sprintf("Transaction hash: %v, Version %v, Locktime: %v, "+
		"Expiry %v\n\n", hash, msgTx.Version, msgTx.LockTime, msgTx.Expiry)
	buffer.WriteString(str)

	str = fmt.Sprintf("==INPUTS==\nNumber of inputs: %v\n\n",
		len(msgTx.TxIn))
	buffer.WriteString(str)

	for i, input := range msgTx.TxIn {
		str = fmt.Sprintf("Input number: %v\n", i)
		buffer.WriteString(str)

		str = fmt.Sprintf("Previous outpoint hash: %v, ",
			input.PreviousOutPoint.Hash)
		buffer.WriteString(str)

		str = fmt.Sprintf("Previous outpoint index: %v, ",
			input.PreviousOutPoint.Index)
		buffer.WriteString(str)

		str = fmt.Sprintf("Previous outpoint tree: %v \n",
			input.PreviousOutPoint.Tree)
		buffer.WriteString(str)

		str = fmt.Sprintf("Sequence: %v \n",
			input.Sequence)
		buffer.WriteString(str)

		str = fmt.Sprintf("ValueIn: %v \n",
			input.ValueIn)
		buffer.WriteString(str)

		str = fmt.Sprintf("BlockHeight: %v \n",
			input.BlockHeight)
		buffer.WriteString(str)

		str = fmt.Sprintf("BlockIndex: %v \n",
			input.BlockIndex)
		buffer.WriteString(str)

		str = fmt.Sprintf("Raw signature script: %x \n", input.SignatureScript)
		buffer.WriteString(str)

		sigScr, _ := txscript.DisasmString(input.SignatureScript)
		str = fmt.Sprintf("Disasmed signature script: %v \n\n",
			sigScr)
		buffer.WriteString(str)
	}

	str = fmt.Sprintf("==OUTPUTS==\nNumber of outputs: %v\n\n",
		len(msgTx.TxOut))
	buffer.WriteString(str)

	for i, output := range msgTx.TxOut {
		str = fmt.Sprintf("Output number: %v\n", i)
		buffer.WriteString(str)

		coins := float64(output.Value) / 1e8
		str = fmt.Sprintf("Output amount: %v atoms or %v coins\n", output.Value,
			coins)
		buffer.WriteString(str)

		// SStx OP_RETURNs, dump pkhs and amts committed
		if isSStx && i != 0 && i%2 == 1 {
			coins := float64(sstxAmts[i/2]) / 1e8
			str = fmt.Sprintf("SStx commit amount: %v atoms or %v coins\n",
				sstxAmts[i/2], coins)
			buffer.WriteString(str)
			str = fmt.Sprintf("SStx commit address: %x\n",
				sstxPkhs[i/2])
			buffer.WriteString(str)
			str = fmt.Sprintf("SStx address type is P2SH: %v\n",
				sstxType[i/2])
			buffer.WriteString(str)

			str = fmt.Sprintf("SStx all address types is P2SH: %v\n",
				sstxType)
			buffer.WriteString(str)

			str = fmt.Sprintf("Voting is fee limited: %v\n",
				sstxLimits[i/2][0])
			buffer.WriteString(str)
			if sstxRules[i/2][0] {
				str = fmt.Sprintf("Voting limit imposed: %v\n",
					sstxLimits[i/2][0])
				buffer.WriteString(str)
			}

			str = fmt.Sprintf("Revoking is fee limited: %v\n",
				sstxRules[i/2][1])
			buffer.WriteString(str)

			if sstxRules[i/2][1] {
				str = fmt.Sprintf("Voting limit imposed: %v\n",
					sstxLimits[i/2][1])
				buffer.WriteString(str)
			}
		}

		// SSGen block/block height OP_RETURN.
		if isSSGen && i == 0 {
			blkHash, blkHeight, _ := stake.SSGenBlockVotedOn(msgTx)
			str = fmt.Sprintf("SSGen block hash voted on: %v, height: %v\n",
				blkHash, blkHeight)
			buffer.WriteString(str)
		}

		if isSSGen && i == 1 {
			vb := stake.SSGenVoteBits(msgTx)
			str = fmt.Sprintf("SSGen vote bits: %v\n", vb)
			buffer.WriteString(str)
		}

		str = fmt.Sprintf("Raw script: %x \n", output.PkScript)
		buffer.WriteString(str)

		scr, _ := txscript.DisasmString(output.PkScript)
		str = fmt.Sprintf("Disasmed script: %v \n\n", scr)
		buffer.WriteString(str)
	}

	return buffer.String()
}

// DebugUtxoEntryData returns a string containing information about the data
// stored in the given UtxoEntry.
func DebugUtxoEntryData(hash chainhash.Hash, utx *UtxoEntry) string {
	var buffer bytes.Buffer
	str := fmt.Sprintf("Hash: %v\n", hash)
	buffer.WriteString(str)
	if utx == nil {
		str := fmt.Sprintf("MISSING\n\n")
		buffer.WriteString(str)
		return buffer.String()
	}

	str = fmt.Sprintf("Height: %v\n", utx.height)
	buffer.WriteString(str)
	str = fmt.Sprintf("Index: %v\n", utx.index)
	buffer.WriteString(str)
	str = fmt.Sprintf("TxVersion: %v\n", utx.txVersion)
	buffer.WriteString(str)
	str = fmt.Sprintf("TxType: %v\n", utx.txType)
	buffer.WriteString(str)
	str = fmt.Sprintf("IsCoinbase: %v\n", utx.isCoinBase)
	buffer.WriteString(str)
	str = fmt.Sprintf("HasExpiry: %v\n", utx.hasExpiry)
	buffer.WriteString(str)
	str = fmt.Sprintf("FullySpent: %v\n", utx.IsFullySpent())
	buffer.WriteString(str)
	str = fmt.Sprintf("StakeExtra: %x\n\n", utx.stakeExtra)
	buffer.WriteString(str)

	outputOrdered := make([]int, 0, len(utx.sparseOutputs))
	for outputIndex := range utx.sparseOutputs {
		outputOrdered = append(outputOrdered, int(outputIndex))
	}
	sort.Ints(outputOrdered)
	for _, idx := range outputOrdered {
		utxo := utx.sparseOutputs[uint32(idx)]
		str = fmt.Sprintf("Output index: %v\n", idx)
		buffer.WriteString(str)
		str = fmt.Sprintf("Amount: %v\n", utxo.amount)
		buffer.WriteString(str)
		str = fmt.Sprintf("ScriptVersion: %v\n", utxo.scriptVersion)
		buffer.WriteString(str)
		str = fmt.Sprintf("Script: %x\n", utxo.pkScript)
		buffer.WriteString(str)
		str = fmt.Sprintf("Spent: %v\n", utxo.spent)
		buffer.WriteString(str)
	}
	str = fmt.Sprintf("\n")
	buffer.WriteString(str)

	return buffer.String()
}

// DebugUtxoViewpointData returns a string containing information about the data
// stored in the given UtxoView.
func DebugUtxoViewpointData(uv *UtxoViewpoint) string {
	if uv == nil {
		return ""
	}

	var buffer bytes.Buffer

	for hash, utx := range uv.entries {
		buffer.WriteString(DebugUtxoEntryData(hash, utx))
	}

	return buffer.String()
}

// DebugStxoData returns a string containing information about the data
// stored in the given STXO.
func DebugStxoData(stx *spentTxOut) string {
	if stx == nil {
		return ""
	}

	var buffer bytes.Buffer

	str := fmt.Sprintf("amount: %v\n", stx.amount)
	buffer.WriteString(str)
	str = fmt.Sprintf("scriptVersion: %v\n", stx.scriptVersion)
	buffer.WriteString(str)
	str = fmt.Sprintf("pkScript: %x\n", stx.pkScript)
	buffer.WriteString(str)
	str = fmt.Sprintf("compressed: %v\n", stx.compressed)
	buffer.WriteString(str)
	str = fmt.Sprintf("stakeExtra: %x\n", stx.stakeExtra)
	buffer.WriteString(str)
	str = fmt.Sprintf("txVersion: %v\n", stx.txVersion)
	buffer.WriteString(str)
	str = fmt.Sprintf("height: %v\n", stx.height)
	buffer.WriteString(str)
	str = fmt.Sprintf("index: %v\n", stx.index)
	buffer.WriteString(str)
	str = fmt.Sprintf("isCoinbase: %v\n", stx.isCoinBase)
	buffer.WriteString(str)
	str = fmt.Sprintf("hasExpiry: %v\n", stx.hasExpiry)
	buffer.WriteString(str)
	str = fmt.Sprintf("txType: %v\n", stx.txType)
	buffer.WriteString(str)
	str = fmt.Sprintf("fullySpent: %v\n", stx.txFullySpent)
	buffer.WriteString(str)

	str = fmt.Sprintf("\n")
	buffer.WriteString(str)

	return buffer.String()
}

// DebugStxosData returns a string containing information about the data
// stored in the given slice of STXOs.
func DebugStxosData(stxs []spentTxOut) string {
	if stxs == nil {
		return ""
	}
	var buffer bytes.Buffer

	// Iterate backwards.
	var str string
	for i := len(stxs) - 1; i >= 0; i-- {
		str = fmt.Sprintf("STX index %v\n", i)
		buffer.WriteString(str)
		str = fmt.Sprintf("amount: %v\n", stxs[i].amount)
		buffer.WriteString(str)
		str = fmt.Sprintf("scriptVersion: %v\n", stxs[i].scriptVersion)
		buffer.WriteString(str)
		str = fmt.Sprintf("pkScript: %x\n", stxs[i].pkScript)
		buffer.WriteString(str)
		str = fmt.Sprintf("compressed: %v\n", stxs[i].compressed)
		buffer.WriteString(str)
		str = fmt.Sprintf("stakeExtra: %x\n", stxs[i].stakeExtra)
		buffer.WriteString(str)
		str = fmt.Sprintf("txVersion: %v\n", stxs[i].txVersion)
		buffer.WriteString(str)
		str = fmt.Sprintf("height: %v\n", stxs[i].height)
		buffer.WriteString(str)
		str = fmt.Sprintf("index: %v\n", stxs[i].index)
		buffer.WriteString(str)
		str = fmt.Sprintf("isCoinbase: %v\n", stxs[i].isCoinBase)
		buffer.WriteString(str)
		str = fmt.Sprintf("hasExpiry: %v\n", stxs[i].hasExpiry)
		buffer.WriteString(str)
		str = fmt.Sprintf("txType: %v\n", stxs[i].txType)
		buffer.WriteString(str)
		str = fmt.Sprintf("fullySpent: %v\n\n", stxs[i].txFullySpent)
		buffer.WriteString(str)
	}
	str = fmt.Sprintf("\n")
	buffer.WriteString(str)

	return buffer.String()
}

// testSimNetPowLimit is the highest proof of work value a Decred block
// can have for the simulation test network.  It is the value 2^255 - 1.
var testSimNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)

// TestSimNetParams defines the network parameters for the simulation test Decred
// network.  This network is similar to the normal test network except it is
// intended for private use within a group of individuals doing simulation
// testing.  The functionality is intended to differ in that the only nodes
// which are specifically specified are used to create the network rather than
// following normal discovery rules.  This is important as otherwise it would
// just turn into another public testnet.
var TestSimNetParams = &chaincfg.Params{
	Name:        "simnet",
	Net:         wire.SimNet,
	DefaultPort: "18555",

	// Chain parameters
	GenesisBlock:             &testSimNetGenesisBlock,
	GenesisHash:              &testSimNetGenesisHash,
	CurrentBlockVersion:      0,
	PowLimit:                 testSimNetPowLimit,
	PowLimitBits:             0x207fffff,
	ResetMinDifficulty:       false,
	GenerateSupported:        true,
	MaximumBlockSize:         1000000,
	TimePerBlock:             time.Second * 1,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       8,
	WorkDiffWindows:          4,
	TargetTimespan:           time.Second * 1 * 8, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:           50000000000,
	MulSubsidy:            100,
	DivSubsidy:            101,
	ReductionInterval:     128,
	WorkRewardProportion:  6,
	StakeRewardProportion: 3,
	BlockTaxProportion:    1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// Mempool parameters
	RelayNonStdTxs: true,

	// Address encoding magics
	PubKeyAddrID:     [2]byte{0x27, 0x6f}, // starts with Sk
	PubKeyHashAddrID: [2]byte{0x0e, 0x91}, // starts with Ss
	PKHEdwardsAddrID: [2]byte{0x0e, 0x71}, // starts with Se
	PKHSchnorrAddrID: [2]byte{0x0e, 0x53}, // starts with SS
	ScriptHashAddrID: [2]byte{0x0e, 0x6c}, // starts with Sc
	PrivateKeyID:     [2]byte{0x23, 0x07}, // starts with Ps

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x20, 0xb9, 0x03}, // starts with sprv
	HDPublicKeyID:  [4]byte{0x04, 0x20, 0xbd, 0x3d}, // starts with spub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 115, // ASCII for s

	// Decred PoS parameters
	MinimumStakeDiff:      20000,
	TicketPoolSize:        64,
	TicketsPerBlock:       5,
	TicketMaturity:        16,
	TicketExpiry:          256, // 4*TicketPoolSize
	CoinbaseMaturity:      16,
	SStxChangeMaturity:    1,
	TicketPoolSizeWeight:  4,
	StakeDiffAlpha:        1,
	StakeDiffWindowSize:   8,
	StakeDiffWindows:      8,
	MaxFreshStakePerBlock: 40,            // 8*TicketsPerBlock
	StakeEnabledHeight:    16 + 16,       // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight: 16 + (64 * 2), // CoinbaseMaturity + TicketPoolSize*2
	StakeBaseSigScript:    []byte{0xDE, 0xAD, 0xBE, 0xEF},

	// Decred organization related parameters
	//
	// "Dev org" address is a 3-of-3 P2SH going to wallet:
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// briefcase
	// (seed 0x00000000000000000000000000000000000000000000000000000000000000)
	//
	// This same wallet owns the three ledger outputs for simnet.
	//
	// P2SH details for simnet dev org is below.
	//
	// address: Scc4ZC844nzuZCXsCFXUBXTLks2mD6psWom
	// redeemScript: 532103e8c60c7336744c8dcc7b85c27789950fc52aa4e48f895ebbfb
	// ac383ab893fc4c2103ff9afc246e0921e37d12e17d8296ca06a8f92a07fbe7857ed1d4
	// f0f5d94e988f21033ed09c7fa8b83ed53e6f2c57c5fa99ed2230c0d38edf53c0340d0f
	// c2e79c725a53ae
	//   (3-of-3 multisig)
	// Pubkeys used:
	//   SkQmxbeuEFDByPoTj41TtXat8tWySVuYUQpd4fuNNyUx51tF1csSs
	//   SkQn8ervNvAUEX5Ua3Lwjc6BAuTXRznDoDzsyxgjYqX58znY7w9e4
	//   SkQkfkHZeBbMW8129tZ3KspEh1XBFC1btbkgzs6cjSyPbrgxzsKqk
	//
	OrganizationPkScript:        chaincfg.SimNetParams.OrganizationPkScript,
	OrganizationPkScriptVersion: chaincfg.SimNetParams.OrganizationPkScriptVersion,
	BlockOneLedger:              testBlockOneLedgerSimNet,
}

// testBlockOneLedgerSimNet is the block one output ledger for the simulation
// network. See below under "Decred organization related parameters" for
// information on how to spend these outputs.
var testBlockOneLedgerSimNet = []*chaincfg.TokenPayout{
	{Address: "Sshw6S86G2bV6W32cbc7EhtFy8f93rU6pae", Amount: 100000 * 1e8},
	{Address: "SsjXRK6Xz6CFuBt6PugBvrkdAa4xGbcZ18w", Amount: 100000 * 1e8},
	{Address: "SsfXiYkYkCoo31CuVQw428N6wWKus2ZEw5X", Amount: 100000 * 1e8},
}

// testSimNetGenesisHash is the hash of the first block in the block chain for the
// simulation test network.
var testSimNetGenesisHash = testSimNetGenesisBlock.BlockSha()

// testSimNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the simulation test network.  It is the same as the merkle root for
// the main network.
var testSimNetGenesisMerkleRoot = testGenesisMerkleRoot

// testGenesisCoinbaseTxLegacy legacy is the coinbase transaction for the genesis
// blocks for the regression test network and test network.
var testGenesisCoinbaseTxLegacy = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec|*/
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for |*/
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x00000000,
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// testGenesisMerkleRoot is the hash of the first transaction in the genesis block
// for the main network.
var testGenesisMerkleRoot = testGenesisCoinbaseTxLegacy.TxSha()

var regTestGenesisCoinbaseTx = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec|*/
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for |*/
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value:   0x00000000,
			Version: 0x0000,
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// testSimNetGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the simulation test network.
var testSimNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version: 1,
		PrevBlock: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		MerkleRoot: testSimNetGenesisMerkleRoot,
		StakeRoot: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		VoteBits:    uint16(0x0000),
		FinalState:  [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:      uint16(0x0000),
		FreshStake:  uint8(0x00),
		Revocations: uint8(0x00),
		Timestamp:   time.Unix(1401292357, 0), // 2009-01-08 20:54:25 -0600 CST
		PoolSize:    uint32(0),
		Bits:        0x207fffff, // 545259519
		SBits:       int64(0x0000000000000000),
		Nonce:       0x00000000,
		Height:      uint32(0),
	},
	Transactions:  []*wire.MsgTx{&regTestGenesisCoinbaseTx},
	STransactions: []*wire.MsgTx{},
}
