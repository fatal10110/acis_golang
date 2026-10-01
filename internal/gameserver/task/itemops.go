package task

import (
	"slices"
	"sync"
)

// operationGate orders the spans in which a multi-row operation has mutated
// its rows but not yet bound them (Bind) and taken their places against the
// spans in which a write reads rows to land them.
//
// Two kinds of write read rows. The persistence writes (UpdateItems: the
// tick's chunks and a container's last flush) read rows of any owner, so they
// exclude every open operation. A handler's write (its Widen to its last
// place) runs inside its own operation and reads rows of the owners that
// operation holds: an operation names the owners whose containers it mutates
// and the rows it takes from outside them (BeginOperationTaking), holds the
// owners of every bound row its write widens to, and two operations sharing
// an owner never run at once.
//
// Without it, a write can read one of an operation's rows in that span and
// see no group: either it reads the row after the mutation, before the
// binding, and lands that leg alone ahead of the operation's own write; or it
// widens before the binding and reads after the operation took its places,
// so it holds the later place on that leg and the operation's write lands the
// other leg alone. Either way a crash or a database failure between the two
// commits leaves one leg in the database.
//
// A persistence write never waits for another write, and operations on
// different owners do not wait for each other. An operation waits only for a
// write that is already reading, never for one that is still waiting, so an
// operation opened inside another (which names no owners: see BeginOperation)
// never blocks on a write queued behind the outer one. A write's reading span
// is a few instance reads with no I/O, so an operation's wait for it is
// short; a write waits for every open operation, and an operation for those
// holding one of its owners, which is as long as one handler takes to mutate
// its rows and queue their write.
type operationGate struct {
	mu   sync.Mutex
	cond sync.Cond
	// open counts operations between their first mutation and their end.
	open int
	// reading counts writes between their Widen and their last place.
	reading int
	// held names the owners the open operations hold.
	held map[int32]struct{}
	// parked counts operations and writes waiting on the gate, for a test
	// that has to know one is before it lets the other side go on.
	parkedOps, parkedReads int
}

func (g *operationGate) init() {
	g.cond.L = &g.mu
	g.held = make(map[int32]struct{})
}

// beginOperation waits until no write is reading and no open operation holds
// one of the owners claim returns, then opens an operation holding them and
// returns them. claim runs under the gate's lock, again after every wake-up:
// the owners an operation has to hold can grow while it waits.
func (g *operationGate) beginOperation(claim func() []int32) []int32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.parkedOps++
	var owners []int32
	for {
		if g.reading == 0 {
			owners = claim()
			if !g.anyHeld(owners) {
				break
			}
		}
		g.cond.Wait()
	}
	g.parkedOps--
	g.open++
	for _, owner := range owners {
		g.held[owner] = struct{}{}
	}
	return owners
}

func (g *operationGate) anyHeld(owners []int32) bool {
	for _, owner := range owners {
		if _, ok := g.held[owner]; ok {
			return true
		}
	}
	return false
}

func (g *operationGate) endOperation(owners []int32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.open--
	for _, owner := range owners {
		delete(g.held, owner)
	}
	if g.open == 0 || len(owners) > 0 {
		g.cond.Broadcast()
	}
}

// beginRead waits for every open operation to end, then starts a write's
// reading span.
func (g *operationGate) beginRead() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.parkedReads++
	for g.open > 0 {
		g.cond.Wait()
	}
	g.parkedReads--
	g.reading++
}

func (g *operationGate) endRead() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reading--
	if g.reading == 0 {
		g.cond.Broadcast()
	}
}

// isOpen reports whether any operation is open.
func (g *operationGate) isOpen() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.open > 0
}

// holds reports whether an open operation holds owner.
func (g *operationGate) holds(owner int32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.held[owner]
	return ok
}

// parked reports how many operations and writes are waiting on the gate.
func (g *operationGate) parked() (ops, reads int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.parkedOps, g.parkedReads
}

// BeginOperation opens a multi-row operation — one whose rows have to land
// together, which it binds (Bind) once it knows them — and returns the func
// that ends it. Call it before the operation's first mutation, and end it only
// after its rows are bound and have taken their places in the write order;
// until then no persistence write reads a row (UpdateItems), so none can see
// one of the operation's rows changed but not yet bound.
//
// owners names every owner whose container the operation mutates. The
// operation waits until no other open operation holds one of them and holds
// them until it ends, so another actor's operation on one of its rows — the
// partner spending adena it was just traded — runs either before this one
// mutates or after its rows are bound and placed, never in between: that
// operation's write reads the row as it was before this one, or widens to
// every row this one bound. The operation also holds the owners of every row
// still bound to a row of theirs by a write that has not landed (Bind) —
// the owner the row was bound under and the owner its instance has now —
// because its own write widens to those rows and reads them as well.
//
// The owners are taken all at once, so operations never deadlock on each
// other. An operation opened while another is open on the same goroutine
// names no owners and takes no rows (BeginOperationTaking): the outer one
// names every owner the whole operation mutates and already holds them, and
// naming one again would wait for itself. While open, an operation must not
// wait for a persistence write to run — the write waits for it to end — nor
// for anything a goroutine may hold while it waits here, such as a lock its
// caller took before BeginOperation.
//
// Defer the returned func: an operation left open stops every persistence
// write and every operation on its owners. It is safe to call more than once.
func (i *ItemInstances) BeginOperation(owners ...int32) (end func()) {
	return i.BeginOperationTaking(nil, owners...)
}

// BeginOperationTaking is BeginOperation for an operation that also writes
// rows from outside its owners' containers: taken names them, as a pickup
// names the ground item it takes into the inventory. Such a row can still be
// bound (Bind) to rows of other owners — a stack its dropper had just
// received in a trade whose write has not landed — and the operation's write
// widens to them, so the operation holds the owners of every row bound to a
// taken one as well.
func (i *ItemInstances) BeginOperationTaking(taken []int32, owners ...int32) (end func()) {
	if i == nil {
		return func() {}
	}
	held := i.ops.beginOperation(func() []int32 { return i.boundOwners(owners, taken) })
	var once sync.Once
	return func() { once.Do(func() { i.ops.endOperation(held) }) }
}

// boundOwners returns owners, without repeats or the ownerless 0, together
// with the owners of every group (Bind) that holds one of taken or a row of
// one of those owners, and so on until no group adds an owner. A row counts
// for both the owner it was bound under and the owner its instance has now:
// a row that changed hands outside an operation after its binding is written
// by its new owner's operations, which widen to its group.
//
// It runs under the gate's lock, which is taken before groupsMu, never after.
// The instances' owners are read once groupsMu is released, so groupsMu is
// never held while waiting on an instance's lock.
func (i *ItemInstances) boundOwners(owners, taken []int32) []int32 {
	out := make([]int32, 0, len(owners))
	for _, owner := range owners {
		out = appendOwner(out, owner)
	}
	if len(out) == 0 && len(taken) == 0 {
		return nil
	}
	groups := i.boundGroupOwners()
	for grew := true; grew; {
		grew = false
		for n, g := range groups {
			if g == nil || !g.touches(out, taken) {
				continue
			}
			groups[n] = nil
			for _, owner := range g.owners {
				if !slices.Contains(out, owner) {
					out = append(out, owner)
					grew = true
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// groupOwners is one bound group as boundOwners sees it: its rows' ids and
// every owner one of its rows belongs to, at its binding or now.
type groupOwners struct {
	ids    []int32
	owners []int32
}

// touches reports whether g holds one of taken or a row of one of owners.
func (g *groupOwners) touches(owners, taken []int32) bool {
	for _, owner := range g.owners {
		if slices.Contains(owners, owner) {
			return true
		}
	}
	for _, id := range g.ids {
		if slices.Contains(taken, id) {
			return true
		}
	}
	return false
}

// boundGroupOwners lists every group bound (Bind) to a write that has not
// landed, with its rows' ids and owners.
func (i *ItemInstances) boundGroupOwners() []*groupOwners {
	i.groupsMu.Lock()
	seen := make(map[*rowGroup]struct{}, len(i.groups))
	copied := make([][]BoundRow, 0, len(i.groups))
	for _, g := range i.groups {
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		rows := make([]BoundRow, 0, len(g.rows))
		for _, row := range g.rows {
			rows = append(rows, row)
		}
		copied = append(copied, rows)
	}
	i.groupsMu.Unlock()

	out := make([]*groupOwners, 0, len(copied))
	for _, rows := range copied {
		group := &groupOwners{ids: make([]int32, 0, len(rows))}
		for _, row := range rows {
			group.ids = append(group.ids, row.ObjectID)
			group.owners = appendOwner(group.owners, row.OwnerID)
			if row.Inst != nil {
				group.owners = appendOwner(group.owners, row.Inst.Snapshot().OwnerID)
			}
		}
		out = append(out, group)
	}
	return out
}

// appendOwner appends owner to owners unless it is the ownerless 0 or
// already there.
func appendOwner(owners []int32, owner int32) []int32 {
	if owner == 0 || slices.Contains(owners, owner) {
		return owners
	}
	return append(owners, owner)
}

// OperationOpen reports whether any multi-row operation is open
// (BeginOperation), which is when no persistence write reads a row. It is a
// probe for a handler's tests: they assert from inside its mutation, and from
// its write's reservation, that the handler holds an operation across both.
func (i *ItemInstances) OperationOpen() bool {
	if i == nil {
		return false
	}
	return i.ops.isOpen()
}

// OperationHolds reports whether an open operation holds owner
// (BeginOperation). Like OperationOpen it is a probe for a handler's tests,
// which assert that the handler names every owner whose rows it mutates.
func (i *ItemInstances) OperationHolds(owner int32) bool {
	if i == nil {
		return false
	}
	return i.ops.holds(owner)
}
