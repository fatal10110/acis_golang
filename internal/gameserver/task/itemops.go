package task

import "sync"

// operationGate keeps the persistence writes that read rows to land them
// (UpdateItems: the tick's chunks and a container's last flush) out of the
// span in which a multi-row operation has mutated its rows but not yet bound
// them (Bind) and taken their places.
//
// Without it, such a write can read one of the rows in that span and see no
// group: either it reads the row after the mutation, before the binding, and
// lands that leg alone ahead of the operation's own write; or it widens
// before the binding and reads after the operation took its places, so it
// holds the later place on that leg and the operation's write lands the other
// leg alone. Either way a crash or a database failure between the two
// commits leaves one leg in the database.
//
// Operations do not exclude each other, and a write never waits for another
// write: only the two kinds exclude each other. An operation waits only for
// a write that is already reading, never for one that is still waiting, so an
// operation opened inside another (a herb a pet loots and uses at once) never
// blocks on a write queued behind the outer one. A write's reading span is a
// few instance reads with no I/O, so an operation's wait is short; a write
// waits for every open operation, which is as long as one handler takes to
// mutate its rows and queue their write.
type operationGate struct {
	mu   sync.Mutex
	cond sync.Cond
	// open counts operations between their first mutation and their end.
	open int
	// reading counts writes between their Widen and their last place.
	reading int
	// parked counts operations and writes waiting on the gate, for a test
	// that has to know one is before it lets the other side go on.
	parkedOps, parkedReads int
}

func (g *operationGate) init() {
	g.cond.L = &g.mu
}

// beginOperation waits for every write that is reading, then opens an
// operation.
func (g *operationGate) beginOperation() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.parkedOps++
	for g.reading > 0 {
		g.cond.Wait()
	}
	g.parkedOps--
	g.open++
}

func (g *operationGate) endOperation() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.open--
	if g.open == 0 {
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
// Defer the returned func: an operation left open stops every persistence
// write. It is safe to call more than once. Operations may nest and run
// concurrently. The operation must not wait, while open, for a persistence
// write to run — a write is waiting for it to end.
func (i *ItemInstances) BeginOperation() (end func()) {
	if i == nil {
		return func() {}
	}
	i.ops.beginOperation()
	var once sync.Once
	return func() { once.Do(i.ops.endOperation) }
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
