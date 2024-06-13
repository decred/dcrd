// Copyright (c) 2019-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/container/lru"
	"github.com/decred/dcrd/crypto/rand"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/blockchain"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

const (
	// minVotesTimeoutDuration is the duration that must elapse after a new tip
	// block has been received before other variants that also extend the same
	// parent and received later are considered for the base of new templates.
	minVotesTimeoutDuration = time.Second * 3

	// maxVoteTimeoutDuration is the duration elapsed after the minimum number
	// of votes for a new tip block has been received that a new template with
	// less than the maximum number of votes will be generated.
	maxVoteTimeoutDuration = time.Millisecond * 2500 // 2.5 seconds

	// templateRegenSecs is the required number of seconds elapsed with
	// incoming non vote transactions before template regeneration
	// is required.
	templateRegenSecs = 30
)

// regenEventType represents the type of a template regeneration event message.
type regenEventType int

// Constants for the type of template regeneration event messages.
const (
	// rtReorgStarted indicates a chain reorganization has been started.
	rtReorgStarted regenEventType = iota

	// rtReorgDone indicates a chain reorganization has completed.
	rtReorgDone

	// rtBlockAccepted indicates a new block has been accepted to the block
	// chain which does not necessarily mean it was added to the main chain.
	// That case is rtBlockConnected.
	rtBlockAccepted

	// rtBlockConnected indicates a new block has been connected to the main
	// chain.
	rtBlockConnected

	// rtBlockDisconnected indicates the current tip block of the best chain has
	// been disconnected.
	rtBlockDisconnected

	// rtVote indicates a new vote has been received.  It applies to all votes
	// and therefore may or may not be relevant.
	rtVote

	// rtTemplateUpdated indicates the current template associated with the
	// generator has been updated.
	rtTemplateUpdated

	// rtForceRegen indicates the template should be regenerated even if
	// it's not yet time for it to be regenerated.
	rtForceRegen
)

// TemplateUpdateReason represents the type of a reason why a template is
// being updated.
type TemplateUpdateReason int

// Constants for the type of template update reasons.
const (
	// TURNewParent indicates the associated template has been updated because
	// it builds on a new block as compared to the previous template.
	TURNewParent TemplateUpdateReason = iota

	// TURNewVotes indicates the associated template has been updated because a
	// new vote for the block it builds on has been received.
	TURNewVotes

	// TURNewTxns indicates the associated template has been updated because new
	// non-vote transactions are available and have potentially been included.
	TURNewTxns

	// turUnknown indicates the associated template has either been updated due
	// to an error or cleared for a chain reorg.  It is only used internally to
	// the background template generator.
	turUnknown
)

// TemplateNtfn represents a notification of a new template along with the
// reason it was generated.  It is sent to subscribers on the channel obtained
// from the TemplateSubscription instance returned by Subscribe.
type TemplateNtfn struct {
	Template *BlockTemplate
	Reason   TemplateUpdateReason
}

// templateUpdate defines a type which is used to signal the regen event handler
// that a new template and relevant error have been associated with the
// generator.
type templateUpdate struct {
	template *BlockTemplate
	err      error
}

// regenEvent defines an event which will potentially result in regenerating a
// block template and consists of a regen event type as well as associated data
// that depends on the type as follows:
//   - rtReorgStarted:      nil
//   - rtReorgDone:         nil
//   - rtBlockAccepted:     *dcrutil.Block
//   - rtBlockConnected:    *dcrutil.Block
//   - rtBlockDisconnected: *dcrutil.Block
//   - rtVote:              *dcrutil.Tx
//   - rtTemplateUpdated:   templateUpdate
type regenEvent struct {
	reason regenEventType
	value  interface{}
}

// waitGroup behaves simlarly to a sync.WaitGroup without the restriction that
// Adds() and Waits() must be synchronized if the wait group is empty.
type waitGroup struct {
	mtx sync.Mutex
	c   int64
	dc  chan struct{}
}

func (wg *waitGroup) Add(i int64) {
	wg.mtx.Lock()
	wg.c += i
	switch {
	case wg.c < 0:
		panic("counter cannot be negative")

	case wg.c > 0 && wg.dc == nil:
		// First increase after counter was last zero. Create the
		// doneChan.
		wg.dc = make(chan struct{})

	case wg.c == 0 && wg.dc != nil:
		// Counter decreased to zero. Signal the waitGroup is done.
		close(wg.dc)
		wg.dc = nil
	}
	wg.mtx.Unlock()
}

func (wg *waitGroup) Done() {
	wg.Add(-1)
}

func (wg *waitGroup) Wait() {
	wg.mtx.Lock()
	dc := wg.dc
	wg.mtx.Unlock()
	if dc == nil {
		// No need to wait, given there are no processes running.
		return
	}
	<-dc
}

// BgBlkTmplGenerator provides facilities for asynchronously generating block
// templates in response to various relevant events and allowing clients to
// subscribe for updates when new templates are generated as well as access the
// most recently-generated template in a concurrency-safe manner.
//
// An example of some of the events that trigger a new block template to be
// generated are modifications to the current best chain, receiving relevant
// votes, and periodic timeouts to allow inclusion of new transactions.
//
// The templates are generated based on a given block template generator
// instance which itself is based on a given mining policy and transaction
// source.  See the NewBlockTemplate method for a detailed description of how
// the block template is generated.
//
// The background generation makes use of three main goroutines -- a regen event
// queue to allow asynchronous non-blocking signalling, a regen event handler to
// process the aforementioned queue and react accordingly, and a subscriber
// notification controller.  In addition, the templates themselves are generated
// in their own goroutines with a cancellable context.
//
// A high level overview of the semantics are as follows:
//   - Ignore all vote handling when prior to stake validation height
//   - Generate templates building on the current tip at startup with a fall
//     back to generate a template on its parent if the current tip does not
//     receive enough votes within a timeout
//   - Continue monitoring for votes on any blocks that extend said parent to
//     potentially switch to them and generate a template building on them when
//     possible
//   - Generate new templates building on new best chain tip blocks once they
//     have received the minimum votes after a timeout to provide the additional
//     votes an opportunity to propagate, except when it is an intermediate
//     block in a chain reorganization
//   - In the event the current tip fails to receive the minimum number of
//     required votes, monitor side chain blocks which are siblings of it for
//     votes in order to potentially switch to them and generate a template
//     building on them when possible
//   - Generate new templates on blocks disconnected from the best chain tip,
//     except when it is an intermediate block in a chain reorganization
//   - Generate new templates periodically when there are new regular
//     transactions to include
//   - Bias templates towards building on the first seen block when possible in
//     order to prevent PoW miners from being able to gain an advantage through
//     vote withholding
//   - Schedule retries in the rare event template generation fails
//   - Allow clients to subscribe for updates every time a new template is
//     successfully generated along with a reason why it was generated
//   - Provide direct access to the most-recently generated template
//   - Block direct access while generating new templates that will make the
//     current template stale (e.g. new parent or new votes)
type BgBlkTmplGenerator struct {
	quit chan struct{}

	// These fields are provided by the caller when the generator is created and
	// are either independently safe for concurrent access or do not change after
	// initialization.
	//
	// cfg is the overall configuration options used when building the block
	// templates in the background.
	//
	// tg is a block template generator instance that is used to actually create
	// the block templates the background block template generator stores.
	//
	// maxVotesPerBlock is the maximum number of votes per block and comes from
	// the chain parameters.  It is defined separately for convenience.
	//
	// minVotesRequired is the minimum number of votes required for a block to
	// be built on.  It is derived from the chain parameters and is defined
	// separately for convenience.
	cfg              BgBlkTmplConfig
	tg               *BlkTmplGenerator
	maxVotesPerBlock uint16
	minVotesRequired uint16

	// These fields deal with providing a stream of template updates to
	// subscribers.
	//
	// subscriptions tracks all template update subscriptions.  It is protected
	// for concurrent access by subscriptionMtx.
	//
	// notifySubscribers delivers template updates to the separate subscriber
	// notification goroutine so it can in turn asynchronously deliver
	// notifications to all subscribers.
	subscriptionMtx   sync.Mutex
	subscriptions     map[*TemplateSubscription]struct{}
	notifySubscribers chan *TemplateNtfn
	notifiedParents   *lru.Set[chainhash.Hash]

	// These fields deal with the template regeneration event queue.  This is
	// implemented as a concurrent queue with immediate passthrough when
	// possible to ensure the order of events is maintained and the related
	// callbacks never block.
	//
	// queueRegenEvent either immediately forwards regen events to the
	// regenEventMsgs channel when it would not block or adds the event to a
	// queue that is processed asynchronously as soon as the receiver becomes
	// available.
	//
	// regenEventMsgs delivers relevant regen events to which the generator
	// reacts to the separate regen goroutine so it can in turn asynchronously
	// process the events and regenerate templates as needed.
	queueRegenEvent chan regenEvent
	regenEventMsgs  chan regenEvent

	// staleTemplateWg is used to allow template retrieval to block callers when
	// a new template that will make the current template stale is being
	// generated.  Stale, in this context, means either the parent has changed
	// or there are new votes available.
	//
	// Note that the use of a custom implementation of a wait group is
	// intentional. The stdlib's sync.WaitGroup documentation states in its
	// Add() method:
	//
	// 	[...] calls with a positive delta that occur when the counter
	// 	is zero must happen before a Wait.
	//
	// However, the usage pattern of staleTemplateWg within
	// BgBlkTmplGenerator cannot enforce that guarantee. Specifically,
	// Add(1) and Wait() calls are actually always executed in different
	// goroutines without any synchronization.
	staleTemplateWg waitGroup

	// These fields track the current best template and are protected by the
	// template mutex.  The template will be nil when there is a template error
	// set.
	templateMtx    sync.Mutex
	template       *BlockTemplate
	templateReason TemplateUpdateReason
	templateErr    error

	// These fields are used to provide the ability to cancel a template that
	// is in the process of being asynchronously generated in favor of
	// generating a new one.
	//
	// cancelTemplate is a function which will cancel the current template that
	// is in the process of being asynchronously generated.  It will have no
	// effect if no template generation is in progress.  It is protected for
	// concurrent access by cancelTemplateMtx.
	cancelTemplateMtx sync.Mutex
	cancelTemplate    func()
}

// BgBlkTmplConfig holds the configuration options related to the background
// block template generator.
type BgBlkTmplConfig struct {
	// TemplateGenerator specifies the generator to use when generating the
	// block templates.
	TemplateGenerator *BlkTmplGenerator

	// MiningAddrs specifies the addresses to choose from when paying mining
	// rewards in generated templates.
	MiningAddrs []stdaddr.Address

	// AllowUnsyncedMining indicates block templates should be created even when
	// the chain is not fully synced.
	AllowUnsyncedMining bool

	// IsCurrent defines the function to use to determine whether or not the
	// chain is current (synced).
	IsCurrent func() bool
}

// NewBgBlkTmplGenerator initializes a background block template generator with
// the provided parameters.  The returned instance must be started with the Run
// method to allowing processing.
func NewBgBlkTmplGenerator(cfg *BgBlkTmplConfig) *BgBlkTmplGenerator {
	tg := cfg.TemplateGenerator
	return &BgBlkTmplGenerator{
		quit:              make(chan struct{}),
		cfg:               *cfg,
		tg:                tg,
		maxVotesPerBlock:  tg.cfg.ChainParams.TicketsPerBlock,
		minVotesRequired:  (tg.cfg.ChainParams.TicketsPerBlock / 2) + 1,
		subscriptions:     make(map[*TemplateSubscription]struct{}),
		notifySubscribers: make(chan *TemplateNtfn),
		notifiedParents:   lru.NewSet[chainhash.Hash](3),
		queueRegenEvent:   make(chan regenEvent),
		regenEventMsgs:    make(chan regenEvent),
		cancelTemplate:    func() {},
	}
}

// UpdateBlockTime updates the timestamp in the passed header to the current
// time while taking into account the median time of the last several blocks to
// ensure the new time is after that time per the chain consensus rules.
func (g *BgBlkTmplGenerator) UpdateBlockTime(header *wire.BlockHeader) {
	g.tg.UpdateBlockTime(header)
}

// sendQueueRegenEvent sends the provided regen event on the internal queue
// regen event channel while respecting the quit channel.  The allows orderly
// shutdown when the generator is shutdown.
func (g *BgBlkTmplGenerator) sendQueueRegenEvent(event regenEvent) {
	select {
	case g.queueRegenEvent <- event:
	case <-g.quit:
	}
}

// setCurrentTemplate sets the current template and error associated with the
// background block template generator and notifies the regen event handler
// about the update.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) setCurrentTemplate(template *BlockTemplate, reason TemplateUpdateReason, err error) {
	g.templateMtx.Lock()
	g.template, g.templateReason, g.templateErr = template, reason, err
	g.templateMtx.Unlock()

	tplUpdate := templateUpdate{template: template, err: err}
	g.sendQueueRegenEvent(regenEvent{rtTemplateUpdated, tplUpdate})
}

// currentTemplate returns the current template associated with the background
// template generator along with the associated reason and error.
//
// NOTE: The returned template and block that it contains MUST be treated as
// immutable since they are shared by all callers.
//
// NOTE: The returned template might be nil even if there is no error.  It is
// the responsibility of the caller to properly handle nil templates.
//
// This function differs from the exported version in that it also returns the
// reason associated with the template that is used in notifications.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) currentTemplate() (*BlockTemplate, TemplateUpdateReason, error) {
	g.staleTemplateWg.Wait()
	g.templateMtx.Lock()
	template, reason, err := g.template, g.templateReason, g.templateErr
	g.templateMtx.Unlock()
	return template, reason, err
}

// CurrentTemplate returns the current template associated with the background
// template generator along with any associated error.
//
// NOTE: The returned template and block that it contains MUST be treated as
// immutable since they are shared by all callers.
//
// NOTE: The returned template might be nil even if there is no error.  It is
// the responsibility of the caller to properly handle nil templates.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) CurrentTemplate() (*BlockTemplate, error) {
	template, _, err := g.currentTemplate()
	return template, err
}

// TemplateSubscription defines a subscription to receive block template updates
// from the background block template generator.  The caller must call Stop on
// the subscription when it is no longer needed to free resources.
//
// NOTE: Notifications are dropped to make up for slow receivers to ensure
// notifications to other subscribers, as well as senders, are not blocked
// indefinitely.  Since templates are typically only generated infrequently and
// receives must fall several templates behind before new ones are dropped, this
// should not affect callers in practice, however, if a caller wishes to
// guarantee that no templates are being dropped, they will need to ensure the
// channel is always processed quickly.
type TemplateSubscription struct {
	g     *BgBlkTmplGenerator
	privC chan *TemplateNtfn
}

// C returns a channel that produces a stream of block templates as each new
// template is generated.  Successive calls to C return the same channel.
//
// NOTE: Notifications are dropped to make up for slow receivers.  See the
// template subscription type documentation for more details.
func (s *TemplateSubscription) C() <-chan *TemplateNtfn {
	return s.privC
}

// Stop prevents any future template updates from being delivered and
// unsubscribes the associated subscription.
//
// NOTE: The channel is not closed to prevent a read from the channel succeeding
// incorrectly.
func (s *TemplateSubscription) Stop() {
	s.g.subscriptionMtx.Lock()
	delete(s.g.subscriptions, s)
	s.g.subscriptionMtx.Unlock()
}

// publishTemplateNtfn sends the provided template notification on the channel
// associated with the subscription.
func (s *TemplateSubscription) publishTemplateNtfn(templateNtfn *TemplateNtfn) {
	// Make use of a non-blocking send along with the buffered channel to allow
	// notifications to be dropped to make up for slow receivers.
	select {
	case s.privC <- templateNtfn:
	default:
	}
}

// notifySubscribersHandler updates subscribers with newly created block
// templates.
//
// This must be run as a goroutine.
func (g *BgBlkTmplGenerator) notifySubscribersHandler(ctx context.Context) {
	for {
		select {
		case templateNtfn := <-g.notifySubscribers:
			g.subscriptionMtx.Lock()
			for subscription := range g.subscriptions {
				subscription.publishTemplateNtfn(templateNtfn)
			}
			g.subscriptionMtx.Unlock()

		case <-ctx.Done():
			return
		}
	}
}

// Subscribe subscribes a client for block template updates.  The returned
// template subscription contains functions to retrieve a channel that produces
// the stream of block templates and to stop the stream when the caller no
// longer wishes to receive new templates.
//
// The current template associated with the background block template generator,
// if any, is immediately sent to the returned subscription stream.
func (g *BgBlkTmplGenerator) Subscribe() *TemplateSubscription {
	// Create the subscription with a buffered channel that is large enough to
	// handle twice the number of templates that can be induced due votes in
	// order to provide a reasonable amount of buffering before dropping
	// notifications due to a slow receiver.
	maxVoteInducedRegens := g.maxVotesPerBlock - g.minVotesRequired + 1
	c := make(chan *TemplateNtfn, maxVoteInducedRegens*2)
	subscription := &TemplateSubscription{
		g:     g,
		privC: c,
	}
	g.subscriptionMtx.Lock()
	g.subscriptions[subscription] = struct{}{}
	g.subscriptionMtx.Unlock()

	// Send existing valid template immediately.
	template, reason, err := g.currentTemplate()
	if err == nil && template != nil {
		subscription.publishTemplateNtfn(&TemplateNtfn{template, reason})
	}

	return subscription
}

// regenQueueHandler immediately forwards items from the regen event queue
// channel to the regen event messages channel when it would not block or adds
// the event to an internal queue to be processed as soon as the receiver
// becomes available.  This ensures that queueing regen events never blocks
// despite how busy the regen handler might become during a burst of events.
//
// This must be run as a goroutine.
func (g *BgBlkTmplGenerator) regenQueueHandler(ctx context.Context) {
	var q []regenEvent
	var out, dequeue chan<- regenEvent = g.regenEventMsgs, nil
	skipQueue := out
	var next regenEvent
	for {
		select {
		case n := <-g.queueRegenEvent:
			// Either send to destination channel immediately when skipQueue is
			// non-nil (queue is empty) and reader is ready, or append to the
			// queue and send later.
			select {
			case skipQueue <- n:
			default:
				q = append(q, n)
				dequeue = out
				skipQueue = nil
				next = q[0]
			}

		case dequeue <- next:
			copy(q, q[1:])
			q = q[:len(q)-1]
			if len(q) == 0 {
				dequeue = nil
				skipQueue = out
			} else {
				next = q[0]
			}

		case <-ctx.Done():
			return
		}
	}
}

// regenHandlerState houses the state used in the regen event handler goroutine.
// It is separated from the background template generator to ensure it is only
// available within the scope of the goroutine.
type regenHandlerState struct {
	// isReorganizing indicates the chain is currently undergoing a
	// reorganization and therefore the generator should not attempt to create
	// new templates until the reorganization has completed.
	isReorganizing bool

	// These fields are used to implement a periodic regeneration timeout that
	// can be reset at any time without needing to create a new one and the
	// associated extra garbage.
	//
	// regenTimer is an underlying timer that is used to implement the timeout.
	//
	// regenChanDrained indicates whether or not the channel for the regen timer
	// has already been read and is used when resetting the timer to ensure the
	// channel is drained when the timer is stopped as described in the timer
	// documentation.
	//
	// lastGeneratedTime specifies the timestamp the current template was
	// generated.
	regenTimer        *time.Timer
	regenChanDrained  bool
	lastGeneratedTime int64

	// These fields are used to control the various generation states when a new
	// block that requires votes has been received.
	//
	// awaitingMinVotesHash is selectively set when a new tip block has been
	// received that requires votes until the minimum number of required votes
	// has been received.
	//
	// maxVotesTimeout is selectively enabled once the minimum number of
	// required votes for the current tip block has been received and is
	// disabled once the maximum number of votes has been received.  This
	// effectively sets a timeout to give the remaining votes an opportunity to
	// propagate prior to forcing a template with less than the maximum number
	// of votes.
	awaitingMinVotesHash *chainhash.Hash
	maxVotesTimeout      <-chan time.Time

	// These fields are used to handle detection of side chain votes and
	// potentially reorganizing the chain to a variant of the current tip when
	// it is unable to obtain the minimum required votes.
	//
	// awaitingSideChainMinVotes houses the known blocks that build from the
	// same parent as the current tip and will only be selectively populated
	// when none of the current possible tips have the minimum number of
	// required votes.
	//
	// trackSideChainsTimeout is selectively enabled when a new tip block has
	// been received in order to give the minimum number of required votes
	// needed to build a block template on it an opportunity to propagate before
	// attempting to find any other variants that extend the same parent as the
	// current tip with enough votes to force a reorganization.  This ensures the
	// first block that is seen is chosen to build templates on so long as it
	// receives the minimum required votes in order to prevent PoW miners from
	// being able to gain an advantage through vote withholding.  It is disabled
	// if the minimum number of votes is received prior to the timeout.
	awaitingSideChainMinVotes map[chainhash.Hash]struct{}
	trackSideChainsTimeout    <-chan time.Time

	// failedGenRetryTimeout is selectively enabled in the rare case a template
	// fails to generate so it can be regenerated again after a delay.  A
	// template should never fail to generate in practice, however, future code
	// changes might break that assumption and thus it is important to handle
	// the case properly.
	failedGenRetryTimeout <-chan time.Time

	// These fields track the block and height that the next template to be
	// generated will build on.  This may not be the same as the current tip in
	// the case it has not yet received the minimum number of required votes
	// needed to build a template on it.
	//
	// baseBlockHash is the hash of the block the next template to be generated
	// will build on.
	//
	// baseBlockHeight is the height of the block identified by the base block
	// hash.
	baseBlockHash   chainhash.Hash
	baseBlockHeight uint32
}

// makeRegenHandlerState returns a regen handler state that is ready to use.
func makeRegenHandlerState() regenHandlerState {
	regenTimer := time.NewTimer(math.MaxInt64)
	regenTimer.Stop()
	return regenHandlerState{
		regenTimer:                regenTimer,
		regenChanDrained:          true,
		awaitingSideChainMinVotes: make(map[chainhash.Hash]struct{}),
	}
}

// stopRegenTimer stops the regen timer while ensuring to read from the timer's
// channel in the case the timer already expired which can happen due to the
// fact the stop happens in between channel reads.   This behavior is well
// documented in the Timer docs.
//
// NOTE: This function must not be called concurrent with any other receives on
// the timer's channel.
func (state *regenHandlerState) stopRegenTimer() {
	t := state.regenTimer
	if !t.Stop() && !state.regenChanDrained {
		<-t.C
	}
	state.regenChanDrained = true
}

// resetRegenTimer resets the regen timer to the given duration while ensuring
// to read from the timer's channel in the case the timer already expired which
// can happen due to the fact the reset happens in between channel reads.   This
// behavior is well documented in the Timer docs.
//
// NOTE: This function must not be called concurrent with any other receives on
// the timer's channel.
func (state *regenHandlerState) resetRegenTimer(d time.Duration) {
	state.stopRegenTimer()
	state.regenTimer.Reset(d)
	state.regenChanDrained = false
}

// clearSideChainTracking removes all tracking for minimum required votes on
// side chain blocks as well as clears the associated timeout that must
// transpire before said tracking is enabled.
func (state *regenHandlerState) clearSideChainTracking() {
	for hash := range state.awaitingSideChainMinVotes {
		delete(state.awaitingSideChainMinVotes, hash)
	}
	state.trackSideChainsTimeout = nil
}

// genTemplateAsync cancels any asynchronous block template that is already
// currently being generated and launches a new goroutine to asynchronously
// generate a new one with the provided reason.  It also handles updating the
// current template and error associated with the generator with the results in
// a concurrent safe fashion and, in the case a successful template is
// generated, notifies the subscription handler goroutine with the new template.
func (g *BgBlkTmplGenerator) genTemplateAsync(ctx context.Context, reason TemplateUpdateReason) {
	// Cancel any other templates that might currently be in the process of
	// being generated and create a new context that can be cancelled for the
	// new template that is about to be generated.
	g.cancelTemplateMtx.Lock()
	g.cancelTemplate()
	ctx, g.cancelTemplate = context.WithCancel(ctx)
	g.cancelTemplateMtx.Unlock()

	// Ensure that attempts to retrieve the current template block until the
	// new template is generated when it is because the parent has changed or
	// new votes are available in order to avoid handing out a template that
	// is guaranteed to be stale soon after.
	blockRetrieval := reason == TURNewParent || reason == TURNewVotes
	if blockRetrieval {
		g.staleTemplateWg.Add(1)
	}
	go func(ctx context.Context, reason TemplateUpdateReason, blockRetrieval bool) {
		if blockRetrieval {
			defer g.staleTemplateWg.Done()
		}

		// Pick a mining address at random and generate a block template that
		// pays to it.
		payToAddr := g.cfg.MiningAddrs[rand.IntN(len(g.cfg.MiningAddrs))]
		template, err := g.tg.NewBlockTemplate(payToAddr)
		// NOTE: err is handled below.
		if err != nil {
			log.Tracef("NewBlockTemplate: %v", err)
		}

		// Don't update the state or notify subscribers when the template
		// generation was cancelled.
		if ctx.Err() != nil {
			return
		}

		// Update the current template state with the results and notify
		// subscribed clients of the new template so long as it's valid.
		if err != nil {
			reason = turUnknown
		}
		g.setCurrentTemplate(template, reason, err)
		if err == nil && template != nil {
			// It is possible for a new vote to show up while the template for a
			// new parent is still being generated which causes that template to
			// be canceled in favor of the new one with the vote.  So, ensure
			// the first notification sent for a new parent has that reason.
			header := &template.Block.Header
			if reason == TURNewVotes {
				if !g.notifiedParents.Contains(header.PrevBlock) {
					reason = TURNewParent
				}
			}
			if reason == TURNewParent {
				g.notifiedParents.Put(header.PrevBlock)
			}

			// Ensure the goroutine exits cleanly during shutdown.
			select {
			case <-ctx.Done():
				return

			case g.notifySubscribers <- &TemplateNtfn{template, reason}:
			}
		}
	}(ctx, reason, blockRetrieval)
}

// curTplHasNumVotes returns whether or not the current template is valid,
// builds on the provided hash, and contains the specified number of votes.
func (g *BgBlkTmplGenerator) curTplHasNumVotes(votedOnHash *chainhash.Hash, numVotes uint16) bool {
	g.templateMtx.Lock()
	template, err := g.template, g.templateErr
	g.templateMtx.Unlock()
	if template == nil || err != nil {
		return false
	}
	if template.Block.Header.PrevBlock != *votedOnHash {
		return false
	}
	return template.Block.Header.Voters == numVotes
}

// numVotesForBlock returns the number of votes on the provided block hash that
// are known.
func (g *BgBlkTmplGenerator) numVotesForBlock(votedOnBlock *chainhash.Hash) uint16 {
	return uint16(len(g.tg.cfg.TxSource.VoteHashesForBlock(votedOnBlock)))
}

// handleBlockConnected handles the rtBlockConnected event by either immediately
// generating a new template building on the block when it will still be prior
// to stake validation height or selectively setting up timeouts to give the
// votes a chance to propagate once the template will be at or after stake
// validation height.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleBlockConnected(ctx context.Context, state *regenHandlerState, block *dcrutil.Block, chainTip *blockchain.BestState) {
	// Clear all vote tracking when the current chain tip changes.
	state.awaitingMinVotesHash = nil
	state.clearSideChainTracking()

	// Nothing more to do if the connected block is not the current chain tip.
	// This can happen in rare cases such as if more than one new block shows up
	// while generating a template.  Due to the requirement for votes later in
	// the chain, it should almost never happen in practice once the chain has
	// progressed that far, however, it is required for correctness.  It is also
	// worth noting that it happens more frequently earlier in the chain before
	// voting starts, particularly in simulation networks with low difficulty.
	blockHeight := block.MsgBlock().Header.Height
	blockHash := block.Hash()
	if int64(blockHeight) != chainTip.Height || *blockHash != chainTip.Hash {
		return
	}

	// Generate a new template immediately when it will be prior to stake
	// validation height which means no votes are required.
	newTemplateHeight := blockHeight + 1
	if newTemplateHeight < uint32(g.tg.cfg.ChainParams.StakeValidationHeight) {
		state.stopRegenTimer()
		state.failedGenRetryTimeout = nil
		state.baseBlockHash = *blockHash
		state.baseBlockHeight = blockHeight
		g.genTemplateAsync(ctx, TURNewParent)
		return
	}

	// At this point the template will be at or after stake validation height,
	// and therefore requires the inclusion of votes on the previous block to be
	// valid.

	// Generate a new template immediately when the maximum number of votes
	// for the block are already known.
	numVotes := g.numVotesForBlock(blockHash)
	if numVotes >= g.maxVotesPerBlock {
		state.stopRegenTimer()
		state.failedGenRetryTimeout = nil
		state.baseBlockHash = *blockHash
		state.baseBlockHeight = blockHeight
		g.genTemplateAsync(ctx, TURNewParent)
		return
	}

	// Update the state so the next template generated will build on the block
	// and set a timeout to give the remaining votes an opportunity to propagate
	// when the minimum number of required votes for the block are already
	// known.  This provides a balance between preferring to generate block
	// templates with max votes and not waiting too long before starting work on
	// the next block.
	if numVotes >= g.minVotesRequired {
		state.stopRegenTimer()
		state.failedGenRetryTimeout = nil
		state.baseBlockHash = *blockHash
		state.baseBlockHeight = blockHeight
		state.maxVotesTimeout = time.After(maxVoteTimeoutDuration)
		return
	}

	// Mark the state as waiting for the minimum number of required votes needed
	// to build a template on the block to be received and set a timeout to give
	// them an opportunity to propagate before attempting to find any other
	// variants that extend the same parent with enough votes to force a
	// reorganization.  This ensures the first block that is seen is chosen to
	// build templates on so long as it receives the minimum required votes in
	// order to prevent PoW miners from being able to gain an advantage through
	// vote withholding.
	//
	// Also, the regen timer for the current template is stopped since chances
	// are high that the votes will be received and it is ideal to avoid
	// regenerating a template that will likely be stale shortly.  The regen
	// timer is reset after the timeout if needed.
	state.stopRegenTimer()
	state.awaitingMinVotesHash = blockHash
	state.trackSideChainsTimeout = time.After(minVotesTimeoutDuration)
}

// handleBlockDisconnected handles the rtBlockDisconnected event by immediately
// generating a new template based on the new tip since votes for it are
// either necessarily already known due to being included in the block being
// disconnected or not required due to moving before stake validation height.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleBlockDisconnected(ctx context.Context, state *regenHandlerState, block *dcrutil.Block, chainTip *blockchain.BestState) {
	// Clear all vote tracking when the current chain tip changes.
	state.awaitingMinVotesHash = nil
	state.clearSideChainTracking()

	// Nothing more to do if the current chain tip is not the block prior to the
	// block that was disconnected.  This can happen in rare cases such as when
	// forcing disconnects via block invalidation.  In practice, disconnects
	// happen as a result of chain reorganizations and thus this code will not
	// be executed, however, it is required for correctness.
	prevHeight := block.MsgBlock().Header.Height - 1
	prevHash := &block.MsgBlock().Header.PrevBlock
	if int64(prevHeight) != chainTip.Height || *prevHash != chainTip.Hash {
		return
	}

	// NOTE: The block being disconnected necessarily has votes for the block
	// that is becoming the new tip and they should ideally be extracted here to
	// ensure they are available for use when building the template.  However,
	// the underlying template generator currently relies on pulling the votes
	// out of the mempool and performs this task itself.  In the future, the
	// template generator should ideally accept the votes to include directly.

	// Generate a new template building on the new tip.
	state.stopRegenTimer()
	state.failedGenRetryTimeout = nil
	state.baseBlockHash = *prevHash
	state.baseBlockHeight = prevHeight
	g.genTemplateAsync(ctx, TURNewParent)
}

// handleBlockAccepted handles the rtBlockAccepted event by establishing vote
// tracking for the block when it is a variant that extends the same parent as
// the current tip, the current tip does not have the minimum number of required
// votes, and the initial timeout to provide them an opportunity to propagate
// has already expired.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleBlockAccepted(_ context.Context, state *regenHandlerState, block *dcrutil.Block, chainTip *blockchain.BestState) {
	// Ignore side chain blocks while still waiting for the side chain tracking
	// timeout to expire.  This provides a bias towards the first block that is
	// seen in order to prevent PoW miners from being able to gain an advantage
	// through vote withholding.
	if state.trackSideChainsTimeout != nil {
		return
	}

	// Ignore side chain blocks when building on it would produce a block prior
	// to stake validation height which means no votes are required and
	// therefore no additional handling is necessary.
	blockHeight := block.MsgBlock().Header.Height
	newTemplateHeight := blockHeight + 1
	if newTemplateHeight < uint32(g.tg.cfg.ChainParams.StakeValidationHeight) {
		return
	}

	// Ignore side chain blocks when the current tip already has enough votes
	// for a template to be built on it.  This ensures the first block that is
	// seen is chosen to build templates on so long as it receives the minimum
	// required votes in order to prevent PoW miners from being able to gain an
	// advantage through vote withholding.
	if state.awaitingMinVotesHash == nil {
		return
	}

	// Ignore blocks that are prior to the current tip.
	if blockHeight < uint32(chainTip.Height) {
		return
	}

	// Ignore main chain tip block since it is handled by the connect path.
	blockHash := block.Hash()
	if *blockHash == chainTip.Hash {
		return
	}

	// Ignore side chain blocks when the current template is already building on
	// the current tip or the accepted block is not a sibling of the current
	// best chain tip.
	alreadyBuildingOnCurTip := state.baseBlockHash == chainTip.Hash
	acceptedPrevHash := &block.MsgBlock().Header.PrevBlock
	if alreadyBuildingOnCurTip || *acceptedPrevHash != chainTip.PrevHash {
		return
	}

	// Setup tracking for votes on the block.
	state.awaitingSideChainMinVotes[*blockHash] = struct{}{}
}

// handleVote handles the rtVote event by determining if the vote is for a block
// the current state is monitoring and reacting accordingly.  At a high level,
// this entails either establishing a timeout once the minimum number of
// required votes for the current tip have been received to provide the
// remaining votes an opportunity to propagate, regenerating the current
// template as a result of the vote, or potentially reorganizing the chain to a
// new tip that has enough votes in the case the current tip is unable to obtain
// the required votes.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleVote(ctx context.Context, state *regenHandlerState, voteTx *dcrutil.Tx, chainTip *blockchain.BestState) {
	votedOnHash, _ := stake.SSGenBlockVotedOn(voteTx.MsgTx())

	// The awaiting min votes hash is selectively set once a block is connected
	// such that a new template that builds on it will be at or after stake
	// validation height until the minimum number of votes required to build a
	// template are received.
	//
	// Update the state so the next template generated will build on the current
	// tip once at least the minimum number of required votes for it has been
	// received and either set a timeout to give the remaining votes an
	// opportunity to propagate if the maximum number of votes is not already
	// known or generate a new template immediately when they are.  This
	// provides a balance between preferring to generate block templates with
	// max votes and not waiting too long before starting work on the next
	// block.
	minVotesHash := state.awaitingMinVotesHash
	if minVotesHash != nil && votedOnHash == *minVotesHash {
		numVotes := g.numVotesForBlock(minVotesHash)
		log.Debugf("Received vote %s for tip block %s (%d total)",
			voteTx.Hash(), minVotesHash, numVotes)
		if numVotes >= g.minVotesRequired {
			// Ensure the next template generated builds on the tip and clear
			// all vote tracking to lock the current tip in now that it
			// has the minimum required votes.
			state.stopRegenTimer()
			state.failedGenRetryTimeout = nil
			state.baseBlockHash = *minVotesHash
			state.baseBlockHeight = uint32(chainTip.Height)
			state.awaitingMinVotesHash = nil
			state.clearSideChainTracking()

			// Generate a new template immediately when the maximum number of
			// votes for the block are already known.
			if numVotes >= g.maxVotesPerBlock {
				g.genTemplateAsync(ctx, TURNewParent)
				return
			}

			// Set a timeout to give the remaining votes an opportunity to
			// propagate.
			state.maxVotesTimeout = time.After(maxVoteTimeoutDuration)
		}
		return
	}

	// Generate a template on new votes for the block the current state is
	// configured to build the next block template on when either the maximum
	// number of votes is received for it or once the minimum number of required
	// votes has been received and the propagation delay timeout that is started
	// upon receipt of said minimum votes has expired.
	//
	// Note that the base block hash is only updated to the current tip once it
	// has received the minimum number of required votes, so this will continue
	// to detect votes for the parent of the current tip prior to the point the
	// new tip has received enough votes.
	//
	// This ensures new templates that include the new votes are generated
	// immediately upon receiving the maximum number of votes as well as any
	// additional votes that arrive after the initial timeout.
	if votedOnHash == state.baseBlockHash {
		// Avoid regenerating the current template if it is already building on
		// the expected block and already has the maximum number of votes.
		if g.curTplHasNumVotes(&votedOnHash, g.maxVotesPerBlock) {
			state.maxVotesTimeout = nil
			return
		}

		numVotes := g.numVotesForBlock(&votedOnHash)
		log.Debugf("Received vote %s for current template %s (%d total)",
			voteTx.Hash(), votedOnHash, numVotes)
		if numVotes >= g.maxVotesPerBlock || state.maxVotesTimeout == nil {
			// The template needs to be updated due to a new parent the first
			// time it is generated and due to new votes on subsequent votes.
			// The max votes timeout is only non-nil before the first time it is
			// generated.
			tplUpdateReason := TURNewVotes
			if state.maxVotesTimeout != nil {
				tplUpdateReason = TURNewParent
			}

			// Cancel the max votes timeout (if set).
			state.maxVotesTimeout = nil

			state.stopRegenTimer()
			state.failedGenRetryTimeout = nil
			g.genTemplateAsync(ctx, tplUpdateReason)
			return
		}
	}

	// Reorganize to an alternative chain tip when it receives at least the
	// minimum required number of votes in the case the current chain tip does
	// not receive the minimum number of required votes within an initial
	// timeout period.
	//
	// Note that the potential side chain blocks to consider are only populated
	// in the aforementioned case.
	if _, ok := state.awaitingSideChainMinVotes[votedOnHash]; ok {
		numVotes := g.numVotesForBlock(&votedOnHash)
		log.Debugf("Received vote %s for side chain block %s (%d total)",
			voteTx.Hash(), votedOnHash, numVotes)
		if numVotes >= g.minVotesRequired {
			err := g.tg.cfg.ForceHeadReorganization(chainTip.Hash, votedOnHash)
			if err != nil {
				return
			}

			// Prevent votes on other tip candidates from causing reorg again
			// since the new chain tip has enough votes.
			state.clearSideChainTracking()
			return
		}
	}
}

// handleTemplateUpdate handles the rtTemplateUpdate event by updating the state
// accordingly.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleTemplateUpdate(state *regenHandlerState, tplUpdate templateUpdate) {
	// Schedule a template regen if it failed to generate for some reason.  This
	// should be exceedingly rare in practice.
	if tplUpdate.err != nil && state.failedGenRetryTimeout == nil {
		state.failedGenRetryTimeout = time.After(time.Second)
		return
	}
	if tplUpdate.template == nil {
		return
	}

	// Ensure the base block details match the template.
	state.baseBlockHash = tplUpdate.template.Block.Header.PrevBlock
	state.baseBlockHeight = tplUpdate.template.Block.Header.Height - 1

	// Update the state related to template regeneration due to new regular
	// transactions.
	state.lastGeneratedTime = time.Now().Unix()
	state.resetRegenTimer(templateRegenSecs * time.Second)
}

// handleForceRegen handles the rtForceRegen event by initiating the generation
// of a new template.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleForceRegen(ctx context.Context, state *regenHandlerState) {
	// Ignore requests to force regeneration if the minimum amount of votes
	// has been received and it's just waiting for the last ones to arrive.
	// The template will be regenerated shortly in that case.
	if state.maxVotesTimeout != nil {
		return
	}

	state.stopRegenTimer()
	state.failedGenRetryTimeout = nil
	g.genTemplateAsync(ctx, turUnknown)
}

// handleRegenEvent handles all regen events by determining the event reason and
// reacting accordingly.  For example, it calls the appropriate associated event
// handler for the events that have one and prevents templates from being
// generating in the middle of reorgs.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleRegenEvent(ctx context.Context, state *regenHandlerState, event regenEvent) {
	// Handle chain reorg messages up front since all of the following logic
	// only applies when not in the middle of reorganizing.
	switch event.reason {
	case rtReorgStarted:
		// Ensure that attempts to retrieve the current template block until the
		// new template after the reorg is generated.
		g.staleTemplateWg.Add(1)

		// Mark the state as reorganizing.
		state.isReorganizing = true

		// Stop all timeouts and clear all vote tracking.
		state.stopRegenTimer()
		state.failedGenRetryTimeout = nil
		state.awaitingMinVotesHash = nil
		state.maxVotesTimeout = nil
		state.clearSideChainTracking()

		// Clear the current template and associated base block for the next
		// generated template.
		g.setCurrentTemplate(nil, turUnknown, nil)
		state.baseBlockHash = zeroHash
		state.baseBlockHeight = 0
		return

	case rtReorgDone:
		state.isReorganizing = false

		// Treat the tip block as if it was just connected when a reorganize
		// finishes so the existing code paths are run.
		//
		// An error should be impossible here since the request is for the block
		// the chain believes is the current tip which means it must exist.
		chainTip := g.tg.cfg.BestSnapshot()
		tipBlock, err := g.tg.cfg.BlockByHash(&chainTip.Hash)
		if err != nil {
			g.setCurrentTemplate(nil, turUnknown, err)
		} else {
			g.handleBlockConnected(ctx, state, tipBlock, chainTip)
		}

		g.staleTemplateWg.Done()
		return
	}

	// Do not generate block templates when the chain is in the middle of
	// reorganizing.
	if state.isReorganizing {
		return
	}

	// Do not generate block templates when the chain is not synced unless
	// specifically requested to.
	if !g.cfg.AllowUnsyncedMining && !g.cfg.IsCurrent() {
		return
	}

	chainTip := g.tg.cfg.BestSnapshot()
	switch event.reason {
	case rtBlockConnected:
		block := event.value.(*dcrutil.Block)
		g.handleBlockConnected(ctx, state, block, chainTip)

	case rtBlockDisconnected:
		block := event.value.(*dcrutil.Block)
		g.handleBlockDisconnected(ctx, state, block, chainTip)

	case rtBlockAccepted:
		block := event.value.(*dcrutil.Block)
		g.handleBlockAccepted(ctx, state, block, chainTip)

	case rtVote:
		voteTx := event.value.(*dcrutil.Tx)
		g.handleVote(ctx, state, voteTx, chainTip)

	case rtTemplateUpdated:
		tplUpdate := event.value.(templateUpdate)
		g.handleTemplateUpdate(state, tplUpdate)

	case rtForceRegen:
		g.handleForceRegen(ctx, state)
	}
}

// tipSiblingsSortedByVotes returns all blocks other than the current tip block
// that also extend its parent sorted by the number of votes each has in
// descending order.
func (g *BgBlkTmplGenerator) tipSiblingsSortedByVotes(state *regenHandlerState) []*blockWithNumVotes {
	// Obtain all of the current blocks that extend the same parent as the
	// current tip.
	generation := g.tg.cfg.TipGeneration()

	// Nothing else to consider if there is only a single block which will be
	// the current tip itself.
	if len(generation) <= 1 {
		return nil
	}

	siblings := make([]*blockWithNumVotes, 0, len(generation)-1)
	for i := range generation {
		hash := &generation[i]
		if *hash == *state.awaitingMinVotesHash {
			continue
		}

		numVotes := g.numVotesForBlock(hash)
		siblings = append(siblings, &blockWithNumVotes{
			Hash:     *hash,
			NumVotes: numVotes,
		})
	}
	sort.Sort(sort.Reverse(byNumberOfVotes(siblings)))
	return siblings
}

// handleTrackSideChainsTimeout handles potentially reorganizing the chain to a
// side chain block with the most votes in the case the minimum number of
// votes needed to build a block template on the current tip have not been
// received within a certain timeout.
//
// It also doubles to reset the regen timer for the current template in the case
// no validate candidates are found since it is disabled when setting up this
// timeout to prevent creating new templates that would very likely be stale
// soon after.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleTrackSideChainsTimeout(ctx context.Context, state *regenHandlerState) {
	// Don't allow side chain variants to override the current tip when it
	// already has the minimum required votes.
	if state.awaitingMinVotesHash == nil {
		return
	}

	// Reorganize the chain to a valid sibling of the current tip that has at
	// least the minimum number of required votes while preferring the most
	// votes.
	//
	// Also, while looping, add each tip the map of side chain blocks to monitor
	// for votes in the event there are not currently any eligible candidates
	// since they may become eligible as votes arrive.
	sortedSiblings := g.tipSiblingsSortedByVotes(state)
	for _, sibling := range sortedSiblings {
		if sibling.NumVotes >= g.minVotesRequired {
			err := g.tg.cfg.ForceHeadReorganization(*state.awaitingMinVotesHash,
				sibling.Hash)
			if err != nil {
				// Try the next block in the case of failure to reorg.
				continue
			}

			// Prevent votes on other tip candidates from causing reorg again
			// since the new chain tip has enough votes.  The reorg event clears
			// the state, but, since there is a backing queue for the events,
			// and the reorg itself might haven taken a bit of time, it could
			// allow new side chain blocks or votes on existing ones in before
			// the reorg events are processed.  Thus, update the state to
			// indicate the next template is to be built on the new tip to
			// prevent any possible logic races.
			state.awaitingMinVotesHash = nil
			state.clearSideChainTracking()
			state.stopRegenTimer()
			state.failedGenRetryTimeout = nil
			state.baseBlockHash = sibling.Hash
			return
		}

		state.awaitingSideChainMinVotes[sibling.Hash] = struct{}{}
	}

	// Generate a new template building on the parent of the current tip when
	// there is not already an existing template and the initial timeout has
	// elapsed upon receiving the new tip without receiving votes for it.  There
	// will typically only not be an existing template when the generator is
	// first instantiated and after a chain reorganization.
	if state.baseBlockHash == zeroHash {
		chainTip := g.tg.cfg.BestSnapshot()
		state.failedGenRetryTimeout = nil
		state.baseBlockHash = chainTip.PrevHash
		state.baseBlockHeight = uint32(chainTip.Height - 1)
		g.genTemplateAsync(ctx, TURNewParent)
		return
	}

	// At this point, no viable candidates to change the current template were
	// found, so reset the regen timer for the current template.
	state.resetRegenTimer(templateRegenSecs * time.Second)
}

// regenHandler is the main workhorse for generating new templates in response
// to regen events and also handles generating a new template during initial
// startup.
//
// This must be run as a goroutine.
func (g *BgBlkTmplGenerator) regenHandler(ctx context.Context) {
	state := makeRegenHandlerState()
	for {
		select {
		case event := <-g.regenEventMsgs:
			g.handleRegenEvent(ctx, &state, event)

		// This timeout is selectively enabled once the minimum number of
		// required votes has been received in order to give the remaining votes
		// an opportunity to propagate.  It is disabled if the remaining votes
		// are received prior to the timeout.
		case <-state.maxVotesTimeout:
			state.maxVotesTimeout = nil
			g.genTemplateAsync(ctx, TURNewParent)

		// This timeout is selectively enabled when a new block is connected in
		// order to give the minimum number of required votes needed to build a
		// block template on it an opportunity to propagate before attempting to
		// find any other variants that extend the same parent as the current
		// tip with enough votes to force a reorganization.  This ensures the
		// first block that is seen is chosen to build templates on so long as
		// it receives the minimum required votes in order to prevent PoW miners
		// from being able to gain an advantage through vote withholding.  It is
		// disabled if the minimum number of votes is received prior to the
		// timeout.
		case <-state.trackSideChainsTimeout:
			state.trackSideChainsTimeout = nil
			g.handleTrackSideChainsTimeout(ctx, &state)

		// This timeout is selectively enabled once a template has been
		// generated in order to allow the template to be periodically
		// regenerated with new transactions.  Note that votes have special
		// handling as described above.
		case <-state.regenTimer.C:
			// Mark the timer's channel as having been drained so the timer can
			// safely be reset.
			state.regenChanDrained = true

			// Generate a new template when there are new transactions
			// available.
			if g.tg.cfg.TxSource.LastUpdated().Unix() > state.lastGeneratedTime {
				state.failedGenRetryTimeout = nil
				g.genTemplateAsync(ctx, TURNewTxns)
				continue
			}

			// There are no new transactions to include and the initial timeout
			// has been triggered, so reset the timer to check again in one
			// second.
			state.resetRegenTimer(time.Second)

		// This timeout is selectively enabled in the rare case a template fails
		// to generate and disabled prior to attempts at generating a new one.
		case <-state.failedGenRetryTimeout:
			state.failedGenRetryTimeout = nil
			g.genTemplateAsync(ctx, TURNewParent)

		case <-ctx.Done():
			return
		}
	}
}

// ChainReorgStarted informs the background block template generator that a
// chain reorganization has started.  It is caller's responsibility to ensure
// this is only invoked as described.
func (g *BgBlkTmplGenerator) ChainReorgStarted() {
	g.sendQueueRegenEvent(regenEvent{rtReorgStarted, nil})
}

// ChainReorgDone informs the background block template generator that a chain
// reorganization has completed.  It is caller's responsibility to ensure this
// is only invoked as described.
func (g *BgBlkTmplGenerator) ChainReorgDone() {
	g.sendQueueRegenEvent(regenEvent{rtReorgDone, nil})
}

// BlockAccepted informs the background block template generator that a block
// has been accepted to the block chain.  It is caller's responsibility to
// ensure this is only invoked as described.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) BlockAccepted(block *dcrutil.Block) {
	g.sendQueueRegenEvent(regenEvent{rtBlockAccepted, block})
}

// BlockConnected informs the background block template generator that a block
// has been connected to the main chain.  It is caller's responsibility to
// ensure this is only invoked as described.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) BlockConnected(block *dcrutil.Block) {
	g.sendQueueRegenEvent(regenEvent{rtBlockConnected, block})
}

// BlockDisconnected informs the background block template generator that a
// block has been disconnected from the main chain.  It is caller's
// responsibility to ensure this is only invoked as described.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) BlockDisconnected(block *dcrutil.Block) {
	g.sendQueueRegenEvent(regenEvent{rtBlockDisconnected, block})
}

// VoteReceived informs the background block template generator that a new vote
// has been received.  It is the caller's responsibility to ensure this is only
// invoked with valid votes.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) VoteReceived(tx *dcrutil.Tx) {
	g.sendQueueRegenEvent(regenEvent{rtVote, tx})
}

// ForceRegen asks the background block template generator to generate a new
// template, independently of most of its internal timers.
//
// Note that there is no guarantee on whether a new template will actually be
// generated or when. This function does _not_ block until a new template is
// generated.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) ForceRegen() {
	g.sendQueueRegenEvent(regenEvent{rtForceRegen, nil})
}

// initialStartupHandler handles the initial startup of the background template
// generation process.  This entails treating the tip block as if it was just
// connected after potentially waiting for the initial chain sync to complete
// depending on whether or not unsynced mining is allowed.
//
// This must be run as a goroutine.
func (g *BgBlkTmplGenerator) initialStartupHandler(ctx context.Context) {
	// Wait until the chain is synced when unsynced mining is not allowed.
	if !g.cfg.AllowUnsyncedMining && !g.cfg.IsCurrent() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

	synced:
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if g.cfg.IsCurrent() {
					break synced
				}
			}
		}
	}

	// Treat the tip block as if it was just connected when starting up so the
	// existing code paths are run.
	tipBlock, err := g.tg.cfg.BlockByHash(&g.tg.cfg.BestSnapshot().Hash)
	if err != nil {
		g.setCurrentTemplate(nil, turUnknown, err)
	} else {
		select {
		case <-ctx.Done():
			return
		case g.queueRegenEvent <- regenEvent{rtBlockConnected, tipBlock}:
		}
	}
}

// Run starts the background block template generator and all other goroutines
// necessary for it to function properly and blocks until the provided context
// is cancelled.
func (g *BgBlkTmplGenerator) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		g.regenQueueHandler(ctx)
		wg.Done()
	}()
	go func() {
		g.regenHandler(ctx)
		wg.Done()
	}()
	go func() {
		g.notifySubscribersHandler(ctx)
		wg.Done()
	}()
	go func() {
		g.initialStartupHandler(ctx)
		wg.Done()
	}()

	// Shutdown the generator when the context is cancelled.
	<-ctx.Done()
	close(g.quit)
	wg.Wait()
}
