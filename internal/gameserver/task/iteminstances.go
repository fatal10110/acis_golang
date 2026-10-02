package task

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

const (
	// ItemInstanceTick is the fixed cadence for lazy item persistence.
	ItemInstanceTick = time.Minute
	// ItemInstanceSaveTimeout bounds one chunk's transaction (Save gives
	// every chunk its own fresh budget of this length, cut short by the
	// caller's ctx) and each step of the shutdown drain
	// (cmd/gameserver/tasks.go), so a hung DB cannot wedge a persistence
	// lane, or the shutdown that waits for those lanes, past this bound.
	//
	// This is a real coupling, not just a shared default: raising it also
	// raises the shutdown drain's three steps and the ticker's stop, which
	// cmd/gameserver's gameServerStopTimeout is summed from, and
	// cmd/gameserver/main_core_test.go pins the sum.
	ItemInstanceSaveTimeout = 10 * time.Second
	// ItemInstanceTickBudget bounds one periodic tick's Save: how much of
	// the tick period a backlog may spend writing chunks, one
	// ItemInstanceSaveTimeout at most per chunk. Half the period leaves the
	// lanes the other half for the writes players queue on them.
	//
	// It is not a shutdown cost. The tick runs under a context the ticker
	// cancels when it stops (scheduler.StartContext), and Save returns as
	// soon as its ctx ends, so the ticker's stop waits only for that
	// cancellation, not for this budget; the chunks it cut short go back to
	// pending for the shutdown drain.
	ItemInstanceTickBudget = ItemInstanceTick / 2
	// ItemInstanceSaveChunkSize bounds how many items one Save transaction
	// covers. Save commits chunks independently, so a batch that grew past
	// what fits in one tick still makes monotonic progress each tick
	// instead of retrying the whole thing and never converging.
	//
	// Measured against MariaDB (BenchmarkItemFlushStore_Flush in
	// data/sql): a chunk of a tick's shape costs about 0.7ms fixed — the
	// transaction and one statement per table — plus about 7.5µs per item,
	// so 100 items take about 1.5ms, some 6,500 times inside
	// ItemInstanceSaveTimeout. The fixed part dominates at this size, which
	// is why a smaller chunk, or one that shrinks when chunks time out,
	// buys a slow database almost nothing: a single item already costs half
	// of what 100 do. A database that cannot write 100 rows in
	// ItemInstanceSaveTimeout cannot write one in much less, and a larger
	// chunk only lengthens how long one transaction holds its rows' write
	// order (persist.Order) and how much one failure sends back to pending.
	ItemInstanceSaveChunkSize = 100
	// ItemInstancePendingCap bounds how many items can wait in the pending
	// set once a write has already failed for them. The set is keyed by
	// object id, so it holds at most one entry per row and the items of
	// live containers are held in memory by those containers anyway; what
	// a long outage keeps adding is rows nothing else holds — destroyed
	// and dropped items. The cap sits an order of magnitude above the
	// distinct rows a full server's players change between ticks, so it is
	// reached only by an outage long enough that the alternative is memory
	// growing until the process dies and every pending row is lost at
	// once.
	//
	// Past the cap a failed write is dropped and logged instead of kept for
	// a retry: failed items are merged back in the order they come back,
	// each owner's longest-waiting first, until pending is full, and the rest
	// are dropped (finishOwner). New changes are always accepted: they carry
	// the freshest state and come from callers that must not block on the
	// database.
	ItemInstancePendingCap = 250_000
)

// errSaveJobPanic is the error a Save owner job reports when it panicked.
// The job recovers its own panic (see Save), so without this the round would
// close with a nil error and Save would report a flush that never happened as
// a success. It is wrapped with the recovered value, but the job logs that
// value itself: round.err keeps only the round's first error, so a caller
// cannot count on seeing this one.
var errSaveJobPanic = errors.New("task: item save job panicked")

// ItemFlusher atomically persists one flush batch: either every change in
// it lands, or, on error, none of it does.
type ItemFlusher interface {
	Flush(ctx context.Context, batch item.FlushBatch) error
}

// ItemInstances lazily persists changed item instances.
//
// mu guards pending. Mutable item fields are guarded by item.Instance.
type ItemInstances struct {
	log       zerolog.Logger
	flusher   ItemFlusher
	templates *item.Table
	worker    *persist.Worker
	// writes orders each row's write against the same row's writes from
	// other lanes — a handler's single-row write, another owner's flush —
	// so the row keeps whichever was produced last (persist.Order).
	writes *persist.Order

	mu      sync.RWMutex
	pending map[int32]pendingItem
	// seq numbers entries as they enter pending, so Save attempts the
	// longest-waiting rows first (see Save).
	seq uint64
	// pendingCap is ItemInstancePendingCap, a field so tests can lower it.
	pendingCap int
	// rounds holds every Save whose owner jobs have not all run yet. Each
	// records the ids RemoveItems dropped while it was outstanding, so its
	// failed items are not merged back over a container that already tore
	// down and wrote its own final state (see RemoveItems and Save).
	rounds map[*saveRound]struct{}

	// groupsMu guards groups. It is taken after mu, never before.
	groupsMu sync.Mutex
	// groups maps each row of a write that has not landed yet to every row
	// that must land with it (Bind).
	groups map[int32]*rowGroup
	// ops keeps UpdateItems, and the writes of operations on the same
	// owners, from reading rows while a multi-row operation has changed them
	// but not yet bound them (BeginOperation). Its lock is taken before
	// groupsMu and before an item instance's lock, never after: an
	// operation's claim reads the owners of bound rows' instances.
	ops operationGate
	// afterWiden, when set, runs inside UpdateItems' reading span, between
	// its Widen and its state reads. Only tests set it.
	afterWiden func()
}

// pendingItem is one changed instance waiting for the next flush, with the
// owner its row belongs to and its place in the wait (seq): when the row
// first went stale, kept across failed writes so a row that keeps failing
// does not lose its turn.
//
// ownerID is remembered rather than read from the instance at flush time,
// because a destroy has already taken the instance through
// item.Instance.DestroyState, which zeroes OwnerID along with the count. A
// flush that keyed the lane off that snapshot would send every destroyed
// item's delete to the lane of owner 0, while the same row's earlier writes
// sit on its real owner's lane — exactly the split laneKey exists to
// prevent. The last owner seen for the object wins, so an item that changes
// hands before the flush is written on the new owner's lane.
type pendingItem struct {
	inst    *item.Instance
	ownerID int32
	seq     uint64
}

// saveRound is one Save's outstanding owner jobs. Its fields are guarded by
// ItemInstances.mu.
type saveRound struct {
	// inflight is the pending map this round took ownership of. It is kept
	// so ContainsID can still answer for an item whose write is queued but
	// has not run: Save empties pending before the first write is enqueued,
	// and without this the item would look settled while its row still
	// holds the stale state.
	inflight  map[int32]pendingItem
	removed   map[int32]struct{}
	remaining int
	err       error
	done      chan struct{}
	// drops totals the failed writes this round's owner jobs dropped for
	// the cap, so the round logs one line rather than one per owner job.
	drops capDrops
}

// capDrops is what a round dropped for ItemInstancePendingCap: the count,
// the first capDropSample ids, and the first dropping job's error.
type capDrops struct {
	count  int
	sample []int32
	err    error
}

// capDropSample bounds the object ids a cap-drop log line names.
const capDropSample = 20

// NewItemInstances returns an empty item persistence task whose writes run
// on worker's lanes. A nil worker writes on the calling goroutine.
//
// log is taken here rather than in Start, the way Effects and PositionUpdates
// take theirs, because those only ever read their logger on the ticker's one
// goroutine. This one is read by whichever goroutine runs an owner job — a
// persistence lane, or Save's own caller on the inline path — and Save runs
// without Start on both the shutdown drain and in tests, so assigning it in
// Start would be a write racing those reads.
func NewItemInstances(flusher ItemFlusher, templates *item.Table, worker *persist.Worker, writes *persist.Order, log zerolog.Logger) *ItemInstances {
	if templates == nil {
		templates = item.NewTable(nil)
	}
	i := &ItemInstances{
		log:        log,
		flusher:    flusher,
		templates:  templates,
		worker:     worker,
		writes:     writes,
		pending:    make(map[int32]pendingItem),
		pendingCap: ItemInstancePendingCap,
		rounds:     make(map[*saveRound]struct{}),
		groups:     make(map[int32]*rowGroup),
	}
	i.ops.init()
	return i
}

// Start launches the fixed item persistence task. Each tick's Save runs
// for at most ItemInstanceTickBudget, under a context the ticker cancels when
// it stops: stopping the ticker cuts a long tick short rather than waiting it
// out, so the tick's budget can be wider than any one shutdown step. The
// chunks a stop cuts short go back to pending, where the shutdown drain
// (cmd/gameserver/tasks.go) writes them.
func (i *ItemInstances) Start(log zerolog.Logger) *scheduler.Ticker {
	return i.start(ItemInstanceTick, log)
}

func (i *ItemInstances) start(period time.Duration, log zerolog.Logger) *scheduler.Ticker {
	return scheduler.StartContext(period, func(stop context.Context) {
		ctx, cancel := context.WithTimeout(stop, ItemInstanceTickBudget)
		defer cancel()
		err := i.Save(ctx)
		switch {
		case err == nil:
		case stop.Err() != nil:
			log.Info().Err(err).Msg("task: item save cut short by stop; unwritten items stay pending")
		default:
			log.Error().Err(err).Msg("task: save item instances")
		}
	}, log)
}

// Add registers inst for the next persistence tick, remembering the owner its
// row currently belongs to. A destroy reports the instance with its owner
// already zeroed, so a previously recorded owner is kept (see pendingItem);
// AddOwned is the form that does not have to guess.
func (i *ItemInstances) Add(inst *item.Instance) {
	if inst == nil {
		return
	}
	i.AddOwned(inst.Snapshot().OwnerID, inst)
}

// AddOwned registers inst for the next persistence tick as ownerID's row.
// The container holding an item knows that owner even when the item itself no
// longer does, which is exactly what a destroy reports, so this is the form
// the per-container persister hook uses: the recorded owner then survives a
// flush swapping the pending set out, and a restored item whose first
// mutation is its destruction still names an owner.
func (i *ItemInstances) AddOwned(ownerID int32, inst *item.Instance) {
	if inst == nil {
		return
	}
	i.mu.Lock()
	entry, ok := i.pending[inst.ObjectID]
	if !ok {
		entry.seq = i.nextSeq()
	}
	entry.inst = inst
	if ownerID != 0 {
		entry.ownerID = ownerID
	}
	i.pending[inst.ObjectID] = entry
	i.mu.Unlock()
}

// PendingOwner returns the owner recorded for objectID's pending write, which
// names the persistence lane it runs on, and whether that item is pending.
func (i *ItemInstances) PendingOwner(objectID int32) (int32, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	entry, ok := i.pending[objectID]
	return entry.ownerID, ok
}

// Contains reports whether inst's object id is currently pending.
func (i *ItemInstances) Contains(inst *item.Instance) bool {
	if inst == nil {
		return false
	}
	return i.ContainsID(inst.ObjectID)
}

// ContainsID reports whether objectID's row still has a change that has not
// reached the database. A restore path uses it as an overlay on the items
// table: such a row holds state that is known stale, so the item it
// describes must not be rebuilt from it. Taking the id rather than an
// instance lets that check run on a row before it becomes a live instance.
//
// An item counts until its write has actually run, not merely until Save has
// picked it up. Save empties pending before it enqueues anything, and those
// writes are queued per owner lane, so a login that waits on the character's
// lane does not wait for an item routed elsewhere by laneKey — a destroyed
// pet collar runs on the collar's own lane. Answering from pending alone
// would call such a row settled while its delete is still queued behind
// other work, and the row would be restored and then re-inserted by the next
// detach flush. Checking the outstanding rounds as well keeps the answer
// true for the whole write, until every batch has executed.
//
// An id RemoveItems dropped mid-round is excluded, matching the rule
// finishOwner merges by: its container wrote its own final state, so the
// inflight copy is the stale one and the row is safe to restore.
func (i *ItemInstances) ContainsID(objectID int32) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if _, ok := i.pending[objectID]; ok {
		return true
	}
	for round := range i.rounds {
		if _, ok := round.inflight[objectID]; !ok {
			continue
		}
		if _, dropped := round.removed[objectID]; !dropped {
			return true
		}
	}
	return false
}

// RemoveItems removes every provided item from the pending set. If a Save
// flush is currently in progress, the removed ids are also recorded so that
// flush's merge-back does not put them back on error: this container tore
// down and wrote its own final state (network.flushItemPersistence), and
// that write must not be undone by a stale inflight copy.
func (i *ItemInstances) RemoveItems(items []*item.Instance) {
	ids := make([]int32, 0, len(items))
	for _, inst := range items {
		if inst == nil {
			continue
		}
		ids = append(ids, inst.ObjectID)
	}
	i.RemoveIDs(ids...)
}

// RemoveIDs is RemoveItems addressed by object id, for a caller holding the
// ids rather than the instances.
func (i *ItemInstances) RemoveIDs(objectIDs ...int32) {
	if len(objectIDs) == 0 {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, objectID := range objectIDs {
		delete(i.pending, objectID)
		for r := range i.rounds {
			r.removed[objectID] = struct{}{}
		}
	}
}

// ClaimRestoredItems resolves the outstanding writes against the rows a login
// is restoring for ownerID, named by objectIDs, and returns a fresh instance
// carrying each still-owned item's unflushed state, keyed by object id, for
// the caller to restore in place of the row.
//
// An outstanding write means the row is stale; it does not say why, and the
// restore has to tell two cases apart that need opposite answers. Only the
// write's own state can, because that state is what the row will hold once the
// write lands. A write that still places the item in ownerID's hands is one
// that has not landed yet — the container held the item to the end and its
// final state is here rather than in the row — so the item is the player's and
// its freshest description is this state. A write that places it elsewhere, or
// deletes it, is the record of the item leaving the container, and nothing of
// it may come back. The second kind stays pending — its delete, or its new
// owner's row, still has to be written — and ContainsID keeps reporting it,
// which is how the caller knows to skip the row it left behind.
//
// Claiming the first kind hands the row's write over; it does not cancel it.
// The pending entry stays, retargeted at the returned instance, because that
// instance is the one the caller restores and mutates from here on. Both
// halves matter:
//
//   - The entry stays, so the state is still scheduled. Dropping it would
//     leave the unflushed state in memory alone until something else happened
//     to write it, and a second failed logout flush — the same degraded
//     database that caused the first — would then lose it and let the stale
//     row win, which is the destroyed stack coming back. A login that aborts
//     after claiming costs nothing for the same reason.
//   - It is retargeted rather than left pointing at the torn-down container's
//     copy, so the row still has exactly one writer. Two copies of one item in
//     the write path is a rollback waiting for the later write to carry the
//     older state.
//
// An id the caller does not pass is not touched at all, so a row this login
// does not restore keeps its own pending write.
//
// Callers must already have waited for ownerID's persistence lane, so the
// container flush that produced these entries has finished and nothing is
// mutating the instances behind this read.
func (i *ItemInstances) ClaimRestoredItems(ownerID int32, objectIDs []int32) map[int32]*item.Instance {
	if ownerID == 0 || len(objectIDs) == 0 {
		return nil
	}
	// The instances are snapshotted after the lock is dropped: a live mutation
	// takes the instance first and the pending set second (AddOwned), so
	// reading them in the other order here would invert that pair.
	candidates := i.pendingInstances(objectIDs)
	claimed := make(map[int32]*item.Instance, len(candidates))
	for _, inst := range candidates {
		st := inst.Snapshot()
		// The same test addToBatch applies, read forwards: these are exactly
		// the states whose write leaves a row that a restore of ownerID's
		// items would select.
		if st.OwnerID != ownerID || st.Count <= 0 || st.Location == item.LocationVoid {
			continue
		}
		claimed[st.ObjectID] = st.Instance()
	}
	i.retarget(ownerID, claimed)
	return claimed
}

// retarget points ownerID's pending entries at the instances a restore just
// built for them, and records the ids as removed in every outstanding round so
// a failed flush cannot merge the superseded copy back over them.
//
// It overwrites whatever entry the id has, which is safe only because the
// owner is offline and its persistence lane already drained: the sole
// concurrent writer left is a Save, and a Save only ever takes entries out.
func (i *ItemInstances) retarget(ownerID int32, claimed map[int32]*item.Instance) {
	if len(claimed) == 0 {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for objectID, inst := range claimed {
		entry, ok := i.pending[objectID]
		if !ok {
			entry.seq = i.nextSeq()
		}
		i.pending[objectID] = pendingItem{inst: inst, ownerID: ownerID, seq: entry.seq}
		for r := range i.rounds {
			r.removed[objectID] = struct{}{}
		}
	}
	i.retargetGroups(ownerID, claimed)
}

// pendingInstances returns the instance behind each of objectIDs that has an
// outstanding write, looking in the same places ContainsID answers from.
func (i *ItemInstances) pendingInstances(objectIDs []int32) []*item.Instance {
	i.mu.RLock()
	defer i.mu.RUnlock()

	out := make([]*item.Instance, 0, len(objectIDs))
	for _, objectID := range objectIDs {
		if entry, ok := i.pending[objectID]; ok {
			if entry.inst != nil {
				out = append(out, entry.inst)
			}
			continue
		}
		for round := range i.rounds {
			entry, ok := round.inflight[objectID]
			if !ok || entry.inst == nil {
				continue
			}
			if _, dropped := round.removed[objectID]; dropped {
				continue
			}
			out = append(out, entry.inst)
			break
		}
	}
	return out
}

// Save flushes every pending item in chunks of at most
// ItemInstanceSaveChunkSize, each committed by its own UpdateItems call
// under its own fresh ItemInstanceSaveTimeout (bounded by whatever remains
// of ctx). Items are grouped by owner and each owner's chunks run on that
// owner's persistence lane. Save returns once every owner's job has run, or
// when ctx ends: an owner job still queued then skips its writes when it runs
// and puts its items back to pending, so the wait never outlasts ctx. Once
// the worker is closed, Save writes on the calling goroutine. The
// pending map is swapped out before the flush so a concurrent
// Add during I/O lands in the new map and is not dropped when the flush
// succeeds. Chunking makes progress monotonic: a batch that has grown past
// what one ItemInstanceSaveTimeout window can write still gets its earlier
// chunks committed, and only the chunks that failed or were never attempted
// (ctx already expired) go back to pending. A single unbounded flush would
// retry the whole growing batch every tick and never converge. On error,
// ids RemoveItems dropped during the flush are not merged back — those
// already got their own successful write and must not be resurrected.
//
// Save gives no whole-batch atomicity of its own: items that land in
// different owners' jobs, or on either side of a chunk boundary, commit or
// fail independently. Rows one operation bound together (Bind) are the
// exception, since UpdateItems widens every chunk to the rows bound to it:
// both legs of a trade whose own write failed land in one transaction or not
// at all, whichever owner's job reaches them first. Beyond that, no stronger
// guarantee is owed: the item-persistence contract commits its writes as
// five sequential statement batches on an autocommit connection with no
// transaction at all, so per-statement partial visibility on error is
// already part of that contract, at a finer grain than one chunk here.
// flushItemPersistence (network/lifecycle.go) is the caller that still
// needs, and gets, whole-container atomicity: it calls UpdateItems
// directly for one container's items, bypassing Save's chunking entirely.
//
// Rows are attempted longest-waiting first: each owner's items are chunked
// in the order they first went stale (pendingItem.seq, which a failed write
// keeps), and owners are dispatched in the order of their longest-waiting
// item. When a backlog is too large for one ctx, the rows ctx runs out on are
// therefore the newest, and they move up every tick until they are written —
// an order keyed by object id would instead leave the newest items, which get
// the highest ids, last on every tick for as long as the backlog lasts. The
// order is also deterministic, which the tests rely on for chunk boundaries.
//
// A backlog that outgrows ItemInstancePendingCap loses the failed writes past
// the cap, logged, rather than growing memory without bound; see that
// constant and finishOwner.
//
// Concurrent callers of Add and RemoveItems are safe. A Save may overlap
// the owner jobs of an earlier one that returned on its ctx; each keeps its
// own record of removed ids. The shutdown hook is appended before the
// ticker's, so fx's reverse stop order runs the final Save only after the
// ticker has stopped.
func (i *ItemInstances) Save(ctx context.Context) error {
	i.mu.Lock()
	inflight := i.pending
	i.pending = make(map[int32]pendingItem)
	// Each owner's items are written on that owner's persistence lane, so
	// they can never interleave with a container flush for the same owner
	// (network.flushItemPersistence) and land an older snapshot after it.
	byOwner := make(map[int32][]pendingItem)
	for _, entry := range inflight {
		key := i.laneKey(entry.inst.Snapshot(), entry.ownerID)
		byOwner[key] = append(byOwner[key], entry)
	}
	round := &saveRound{inflight: inflight, removed: make(map[int32]struct{}), remaining: len(byOwner), done: make(chan struct{})}
	if round.remaining == 0 {
		i.mu.Unlock()
		return nil
	}
	i.rounds[round] = struct{}{}
	backlog := len(inflight)
	i.mu.Unlock()
	if backlog >= i.pendingCap/2 {
		i.log.Warn().Int("pending", backlog).Int("cap", i.pendingCap).
			Msg("task: item persistence backlog is past half its cap; failed writes are dropped once it is reached")
	}

	for _, owner := range oldestFirst(byOwner) {
		entries := byOwner[owner]
		job := func() {
			// The bookkeeping runs on the panic path too. A panic anywhere
			// inside saveChunks is recovered by the lane (persist.Worker.runJob)
			// so one bad job cannot kill it, which means an unguarded
			// finishOwner call simply never runs: the round never reaches zero
			// remaining, so Save blocks until its own ctx expires and the round
			// stays in i.rounds for the rest of the process, and this owner's
			// items — already swapped out of pending — end up in no map at all,
			// neither written nor retried. The row then keeps pre-write state
			// while memory holds the new one, which a relog turns into a
			// rollback or, for a trade whose other leg saved on another lane, a
			// duplicate.
			//
			// The pre-set values are what a panic reports: every item back to
			// pending (saveChunks has no return value to say which chunks got
			// as far as committing, and a redundant re-save is the safe side of
			// that) and a non-nil error, so a panicked round surfaces to the
			// caller as a failed one rather than as a silent success.
			failed, err := entries, errSaveJobPanic
			defer func() { i.finishOwner(round, failed, err) }()
			// Stopping the panic here rather than letting the lane's recover
			// take it is what makes the guard cover both dispatch paths. The
			// lane recovers a queued job, but the loop below runs the job on
			// this goroutine whenever Enqueue refuses it — a closed worker, or
			// a nil one — and there a panic unwinds out of the dispatch loop
			// and out of Save. Every owner sorted after the panicking one is
			// then never dispatched at all, with its entries already swapped
			// out of pending and nothing left to merge them back, and the round
			// stays in i.rounds forever, where its inflight copy keeps
			// ContainsID answering true for ids no Save will ever write again.
			//
			// That inline path is the shutdown drain's post-Close save
			// (cmd/gameserver/tasks.go, drainItemInstances), the last chance
			// these rows get, and an escaping panic there would leave the fx
			// OnStop hook — no fx.RecoverFromPanics is configured — skipping
			// every stop hook after it. The item-save contract cannot fail
			// that way: a failure is caught around the whole batch and never
			// escapes the shutdown-triggered save.
			//
			// The reason is logged here rather than left to round.err, which
			// is best-effort and cannot be relied on: finishOwner keeps only
			// the first non-nil error of the round, and a round holds one job
			// per owner — a whole tick's worth of players — so any other
			// owner's DB error or DeadlineExceeded takes that slot first.
			// Save can also return on ctx.Done before the round finishes, and
			// that branch returns ctx.Err() without ever reading round.err.
			// Recovering ahead of the lane took away its own unconditional
			// "persist: recovered panic in job" line, so without this a
			// panicking flush could be completely silent. #2403 asks for the
			// panic to be a failed round *and* a log line, not one instead of
			// the other.
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("%w: %v", errSaveJobPanic, r)
					i.log.Error().Interface("panic", r).Int32("owner_id", owner).
						Msg("task: recovered panic in item save job")
				}
			}()
			failed, err = i.saveChunks(ctx, entries)
		}
		// A closed worker has already run every job it accepted, so writing
		// here cannot land ahead of an older queued write.
		if !i.worker.Enqueue(owner, job) {
			job()
		}
	}

	select {
	case <-round.done:
	case <-ctx.Done():
		// The owner jobs still queued fail fast once they run and merge
		// their items back to pending for a later Save.
		select {
		case <-round.done:
		default:
			return ctx.Err()
		}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return round.err
}

// oldestFirst sorts each owner's entries by when they went stale and returns
// the owners in the order of their longest-waiting entry.
func oldestFirst(byOwner map[int32][]pendingItem) []int32 {
	owners := make([]int32, 0, len(byOwner))
	for owner, entries := range byOwner {
		slices.SortFunc(entries, func(a, b pendingItem) int { return cmp.Compare(a.seq, b.seq) })
		owners = append(owners, owner)
	}
	slices.SortFunc(owners, func(a, b int32) int { return cmp.Compare(byOwner[a][0].seq, byOwner[b][0].seq) })
	return owners
}

// nextSeq hands out the next place in the wait. The caller holds mu.
func (i *ItemInstances) nextSeq() uint64 {
	i.seq++
	return i.seq
}

// finishOwner merges one owner job's failed items back to pending, skipping
// any RemoveItems dropped while round was outstanding, and closes round once
// its last owner job has run.
//
// A failed item keeps its place in the wait, and so does a newer change of
// the same row that arrived while the write was out: the row has been stale
// since the earlier one. Once pending holds pendingCap items a failed item is
// dropped instead. failed holds the job's items in the order they were
// attempted, longest-waiting first, so those keep their place and the newest
// are the ones dropped. The round's drops are logged once, by the job that
// closes it: during an outage every owner job of a tick fails, and one line
// per owner would be a line per online player each tick.
func (i *ItemInstances) finishOwner(round *saveRound, failed []pendingItem, err error) {
	drops, pending, closed := i.mergeBack(round, failed, err)
	if closed && drops.count > 0 {
		i.log.Error().Err(drops.err).Int("dropped", drops.count).Ints32("object_ids", drops.sample).
			Int("pending", pending).Int("cap", i.pendingCap).
			Msg("task: item persistence backlog at its cap; dropped failed item writes, their rows keep their last saved state")
	}
}

// mergeBack is finishOwner's bookkeeping under mu. It returns the round's
// cap drops so far, the pending count it left, and whether this job closed
// the round.
func (i *ItemInstances) mergeBack(round *saveRound, failed []pendingItem, err error) (capDrops, int, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, entry := range failed {
		objectID := entry.inst.ObjectID
		if _, wasRemoved := round.removed[objectID]; wasRemoved {
			continue
		}
		if newer, ok := i.pending[objectID]; ok {
			newer.seq = min(newer.seq, entry.seq)
			i.pending[objectID] = newer
			continue
		}
		if len(i.pending) >= i.pendingCap {
			if round.drops.count == 0 {
				round.drops.err = err
			}
			round.drops.count++
			if len(round.drops.sample) < capDropSample {
				round.drops.sample = append(round.drops.sample, objectID)
			}
			continue
		}
		// Keeps the owner this round resolved: a retry of a destroyed item
		// must not fall back to the zeroed owner on its instance.
		i.pending[objectID] = entry
	}
	if round.err == nil {
		round.err = err
	}
	round.remaining--
	closed := round.remaining == 0
	if closed {
		delete(i.rounds, round)
		close(round.done)
	}
	return round.drops, len(i.pending), closed
}

// laneKey picks the persistence lane an item's write runs on: its owner's,
// except a destroyed pet collar, whose write also deletes its pets row. That
// one runs on the collar's own lane, where every pets-row save is queued, so
// a save still waiting there cannot land after the delete and recreate the
// row. The same argument is why every other destroyed item's write stays on
// its owner's lane, which is where that row's earlier writes are: ownerID
// comes from the pending entry, not from the instance, whose owner a destroy
// has already zeroed (see pendingItem).
func (i *ItemInstances) laneKey(st item.InstanceState, ownerID int32) int32 {
	if st.Count <= 0 {
		if tmpl, _ := i.templates.Get(st.TemplateID); isPetCollar(tmpl) {
			return st.ObjectID
		}
	}
	return ownerID
}

// saveChunks writes items in chunks of at most ItemInstanceSaveChunkSize,
// each under its own ItemInstanceSaveTimeout, and returns the items whose
// chunk failed or was never attempted because ctx had already ended.
func (i *ItemInstances) saveChunks(ctx context.Context, entries []pendingItem) ([]pendingItem, error) {
	var firstErr error
	var failed []pendingItem
	for chunk := range slices.Chunk(entries, ItemInstanceSaveChunkSize) {
		if ctx.Err() != nil {
			failed = append(failed, chunk...)
			if firstErr == nil {
				firstErr = ctx.Err()
			}
			continue
		}
		items := make([]*item.Instance, 0, len(chunk))
		for _, entry := range chunk {
			items = append(items, entry.inst)
		}
		chunkCtx, cancel := context.WithTimeout(ctx, ItemInstanceSaveTimeout)
		err := i.UpdateItems(chunkCtx, items)
		cancel()
		if err != nil {
			failed = append(failed, chunk...)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return failed, firstErr
}

// UpdateItems persists the provided item instances immediately, as one
// atomic flush: either every row lands, or, on error, none of them do. A
// non-nil error means nothing was written, so callers must keep their
// items pending for a retry rather than dropping them.
//
// The flush also carries every row bound to one of items (Bind), read
// where it takes its place like the rest, and a flush that lands settles the
// groups it carried whose every row it wrote (Landed). It reads no row while a
// multi-row operation is open (BeginOperation), so it waits for those.
//
// This per-call guarantee is unchanged by Save's chunking (Save simply
// calls UpdateItems once per chunk); network.flushItemPersistence relies on
// it directly, calling UpdateItems for one container's items outside of
// Save, and needs it to stay whole-batch atomic. Do not chunk that call
// site too without checking its callers still get what they need.
func (i *ItemInstances) UpdateItems(ctx context.Context, items []*item.Instance) error {
	if len(items) == 0 {
		return nil
	}
	if i.flusher == nil {
		return errors.New("task: item persistence is nil")
	}

	items = slices.DeleteFunc(items, func(inst *item.Instance) bool { return inst == nil })
	write, states, deletes, carried := i.placeRows(items)
	var err error
	var landed []int32
	write.Run(func(keep []int32) error {
		var batch item.FlushBatch
		for _, st := range states {
			if _, found := slices.BinarySearch(keep, st.ObjectID); !found {
				continue
			}
			i.addToBatch(&batch, st)
		}
		for _, objectID := range deletes {
			if _, found := slices.BinarySearch(keep, objectID); found {
				batch.Deletes = append(batch.Deletes, objectID)
			}
		}
		err = i.flusher.Flush(ctx, batch)
		landed = keep
		return err
	})
	if err == nil {
		i.Landed(carried, landed)
	}
	return err
}

// placeRows reads the state of items and of every row bound to one of them
// (Widen), each with its place in its row's write order, for UpdateItems to
// land in one flush. It returns the write holding the places, the states,
// the bound rows with no instance left, which are deleted, and the groups the
// rows came from.
//
// The whole span runs while no multi-row operation is open (BeginOperation),
// so an operation's rows are read either before it changed any of them or
// after it bound them all: a row is never read changed while the group it
// belongs to is still invisible, and no operation can bind rows and take
// their places between this Widen and these reads.
func (i *ItemInstances) placeRows(items []*item.Instance) (*persist.Write, []item.InstanceState, []int32, BoundGroups) {
	i.ops.beginRead()
	defer i.ops.endRead()

	// A row bound to rows outside this flush takes them along, so the rows
	// one operation changed land in one transaction whichever write lands
	// them (Bind).
	ids := make([]int32, 0, len(items))
	for _, inst := range items {
		ids = append(ids, inst.ObjectID)
	}
	var deletes []int32
	widened, carried := i.Widen(ids)
	if i.afterWiden != nil {
		i.afterWiden()
	}
	for _, row := range widened {
		if row.Inst == nil {
			deletes = append(deletes, row.ObjectID)
			continue
		}
		items = append(items, row.Inst)
	}
	slices.SortFunc(items, func(a, b *item.Instance) int { return cmp.Compare(a.ObjectID, b.ObjectID) })

	// Each row's state and its place in that row's write order are taken
	// together, under the instance: the state this flush will land is fixed
	// here, not when the write finally runs, so a single-row write produced
	// after it always holds the later place — and one produced before it the
	// earlier, however long this flush then waits for the rows. Reading the
	// state later than the place is what let a flush hold an older place
	// while carrying a newer state, and an older write then landed on top of
	// its delete.
	write := i.writes.Begin()
	states := make([]item.InstanceState, 0, len(items))
	for _, inst := range items {
		inst.WithState(func(st item.InstanceState) {
			write.Add(st.ObjectID)
			states = append(states, st)
		})
	}
	for _, objectID := range deletes {
		write.Add(objectID)
	}
	return write, states, deletes, carried
}

// addToBatch resolves st's persistence effect and appends it to batch,
// matching the per-item semantics updateItem used to apply immediately:
// delete when count <= 0 or location == VOID, augmentation delete/save
// only for weapons, pet-row delete only for a pet collar at zero count.
//
// It takes the state rather than the instance because the state is read where
// the write's place in its row's order is taken (UpdateItems), not here.
func (i *ItemInstances) addToBatch(batch *item.FlushBatch, st item.InstanceState) {
	tmpl, _ := i.templates.Get(st.TemplateID)
	isWeapon := tmpl != nil && tmpl.Kind == item.KindWeapon

	if st.Count <= 0 || st.Location == item.LocationVoid {
		batch.Deletes = append(batch.Deletes, st.ObjectID)
		if st.Count <= 0 {
			if isWeapon {
				batch.AugmentationDeletes = append(batch.AugmentationDeletes, st.ObjectID)
			}
			if isPetCollar(tmpl) {
				batch.PetDeletes = append(batch.PetDeletes, st.ObjectID)
			}
		}
		return
	}

	batch.Saves = append(batch.Saves, st)
	if isWeapon {
		if st.Augmentation == nil {
			batch.AugmentationDeletes = append(batch.AugmentationDeletes, st.ObjectID)
		} else {
			batch.AugmentationSaves = append(batch.AugmentationSaves, item.FlushAugmentationSave{
				ObjectID:     st.ObjectID,
				Augmentation: *st.Augmentation,
			})
		}
	}
}

func isPetCollar(tmpl *item.Template) bool {
	return tmpl != nil && tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemPetCollar
}
