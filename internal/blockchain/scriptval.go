// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"context"
	"fmt"
	"math"
	"runtime"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

// PrevScripter defines an interface that provides access to scripts and their
// associated version keyed by an outpoint.  The boolean return indicates
// whether or not the script and version for the provided outpoint was found.
type PrevScripter interface {
	PrevScript(*wire.OutPoint) (uint16, []byte, bool)
}

// txValidateItem holds a transaction along with which input to validate.
type txValidateItem struct {
	txInIndex int
	txIn      *wire.TxIn
	tx        *dcrutil.Tx
}

// txValidator provides a type which asynchronously validates transaction
// inputs.  It provides several channels for communication and a processing
// function that is intended to be in run multiple goroutines.
type txValidator struct {
	validateChan chan *txValidateItem
	resultChan   chan error
	prevScripts  PrevScripter
	flags        txscript.ScriptFlags
	sigCache     *txscript.SigCache
}

// sendResult sends the result of a script pair validation on the internal
// result channel while respecting the context.  The allows orderly
// shutdown when the validation process is aborted early due to a validation
// error in one of the other goroutines.
func (v *txValidator) sendResult(ctx context.Context, result error) {
	select {
	case v.resultChan <- result:
	case <-ctx.Done():
	}
}

// validateHandler consumes items to validate from the internal validate channel
// and returns the result of the validation on the internal result channel. It
// must be run as a goroutine.
func (v *txValidator) validateHandler(ctx context.Context) {
out:
	for {
		select {
		case <-ctx.Done():
			break out

		case txVI := <-v.validateChan:
			// Ensure the referenced input utxo is available.
			txIn := txVI.txIn
			prevOut := &txIn.PreviousOutPoint
			scriptVersion, pkScript, ok := v.prevScripts.PrevScript(prevOut)
			if !ok {
				str := fmt.Sprintf("unable to find unspent output %v "+
					"referenced from transaction %s:%d", *prevOut,
					txVI.tx.Hash(), txVI.txInIndex)
				err := ruleError(ErrMissingTxOut, str)
				v.sendResult(ctx, err)
				break out
			}
			// Create a new script engine for the script pair.
			sigScript := txIn.SignatureScript
			vm, err := txscript.NewEngine(pkScript, txVI.tx.MsgTx(),
				txVI.txInIndex, v.flags, scriptVersion, v.sigCache)
			if err != nil {
				str := fmt.Sprintf("failed to parse input %s:%d which "+
					"references output %v - %v (input script bytes %x, prev "+
					"output script bytes %x)", txVI.tx.Hash(), txVI.txInIndex,
					*prevOut, err, sigScript, pkScript)
				err := ruleError(ErrScriptMalformed, str)
				v.sendResult(ctx, err)
				break out
			}

			// Execute the script pair.
			if err := vm.Execute(); err != nil {
				str := fmt.Sprintf("failed to validate input %s:%d which "+
					"references output %v - %v (input script bytes %x, prev "+
					"output script bytes %x)", txVI.tx.Hash(), txVI.txInIndex,
					*prevOut, err, sigScript, pkScript)
				err := ruleError(ErrScriptValidation, str)
				v.sendResult(ctx, err)
				break out
			}

			// Validation succeeded.
			v.sendResult(ctx, nil)
		}
	}
}

// Validate validates the scripts for all of the passed transaction inputs using
// multiple goroutines.
func (v *txValidator) Validate(items []*txValidateItem) error {
	if len(items) == 0 {
		return nil
	}

	// Limit the number of goroutines to do script validation based on the
	// number of processor cores.  This help ensure the system stays
	// reasonably responsive under heavy load.
	maxGoRoutines := runtime.NumCPU() * 3
	if maxGoRoutines <= 0 {
		maxGoRoutines = 1
	}
	if maxGoRoutines > len(items) {
		maxGoRoutines = len(items)
	}

	// Start up validation handlers that are used to asynchronously
	// validate each transaction input.
	ctx, cancel := context.WithCancel(context.Background())
	for i := 0; i < maxGoRoutines; i++ {
		go v.validateHandler(ctx)
	}

	// Validate each of the inputs.  The context is canceled when any
	// errors occur so all processing goroutines exit regardless of which
	// input had the validation error.
	numInputs := len(items)
	currentItem := 0
	processedItems := 0
	for processedItems < numInputs {
		// Only send items while there are still items that need to
		// be processed.  The select statement will never select a nil
		// channel.
		var validateChan chan *txValidateItem
		var item *txValidateItem
		if currentItem < numInputs {
			validateChan = v.validateChan
			item = items[currentItem]
		}

		select {
		case validateChan <- item:
			currentItem++

		case err := <-v.resultChan:
			processedItems++
			if err != nil {
				cancel()
				return err
			}
		}
	}

	cancel()
	return nil
}

// newTxValidator returns a new instance of txValidator to be used for
// validating transaction scripts asynchronously.
func newTxValidator(prevScripts PrevScripter, flags txscript.ScriptFlags, sigCache *txscript.SigCache) *txValidator {
	return &txValidator{
		validateChan: make(chan *txValidateItem),
		resultChan:   make(chan error),
		prevScripts:  prevScripts,
		sigCache:     sigCache,
		flags:        flags,
	}
}

// ValidateTransactionScripts validates the scripts for the passed transaction
// using multiple goroutines.
func ValidateTransactionScripts(tx *dcrutil.Tx, prevScripts PrevScripter,
	flags txscript.ScriptFlags, sigCache *txscript.SigCache,
	isAutoRevocationsEnabled bool) error {

	// Skip revocations if the automatic ticket revocations agenda is active and
	// the transaction version is greater than or equal to 2.  This is allowed
	// since consensus rules enforce that revocations MUST pay to the addresses
	// specified by the original commitments in the ticket.
	msgTx := tx.MsgTx()
	if isAutoRevocationsEnabled &&
		msgTx.Version >= stake.TxVersionAutoRevocations &&
		stake.IsSSRtx(msgTx) {

		return nil
	}

	// Collect all of the transaction inputs and required information for
	// validation.
	txIns := msgTx.TxIn
	txValItems := make([]*txValidateItem, 0, len(txIns))
	for txInIdx, txIn := range txIns {
		// Skip coinbases.
		if txIn.PreviousOutPoint.Index == math.MaxUint32 {
			continue
		}

		txVI := &txValidateItem{
			txInIndex: txInIdx,
			txIn:      txIn,
			tx:        tx,
		}
		txValItems = append(txValItems, txVI)
	}

	// Validate all of the inputs.
	return newTxValidator(prevScripts, flags, sigCache).Validate(txValItems)
}

// checkBlockScripts executes and validates the scripts for all transactions in
// the passed block using multiple goroutines.
// txTree = true is TxTreeRegular, txTree = false is TxTreeStake.
func checkBlockScripts(block *dcrutil.Block, utxoView *UtxoViewpoint, txTree bool,
	scriptFlags txscript.ScriptFlags, sigCache *txscript.SigCache,
	isAutoRevocationsEnabled bool) error {

	// Collect all of the transaction inputs and required information for
	// validation for all transactions in the block into a single slice.
	numInputs := 0
	var txs []*dcrutil.Tx

	// TxTreeRegular handling.
	if txTree {
		txs = block.Transactions()
	} else { // TxTreeStake
		txs = block.STransactions()
	}

	for _, tx := range txs {
		numInputs += len(tx.MsgTx().TxIn)
	}
	txValItems := make([]*txValidateItem, 0, numInputs)
	for _, tx := range txs {
		// Skip revocations if the automatic ticket revocations agenda is active and
		// the transaction version is greater than or equal to 2.  This is allowed
		// since consensus rules enforce that revocations MUST pay to the addresses
		// specified by the original commitments in the ticket.
		msgTx := tx.MsgTx()
		if isAutoRevocationsEnabled && !txTree &&
			msgTx.Version >= stake.TxVersionAutoRevocations &&
			stake.IsSSRtx(msgTx) {

			continue
		}

		for txInIdx, txIn := range msgTx.TxIn {
			// Skip coinbases.
			if txIn.PreviousOutPoint.Index == math.MaxUint32 {
				continue
			}

			txVI := &txValidateItem{
				txInIndex: txInIdx,
				txIn:      txIn,
				tx:        tx,
			}
			txValItems = append(txValItems, txVI)
		}
	}

	// Validate all of the inputs.
	return newTxValidator(utxoView, scriptFlags, sigCache).Validate(txValItems)
}
