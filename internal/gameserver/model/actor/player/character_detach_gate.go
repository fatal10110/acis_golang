package player

import "sync"

// detachGate holds a character's script writes against its departure. Each
// write admitted stamps the gate (ScriptRuns.stamp), and the character is
// held while an invocation counted at or below the stamp still runs: the
// invocation that made the write, and any it was raised from, among them.
type detachGate struct {
	mu sync.Mutex
	// runs is the counter the stamp was taken from, nil before any write.
	runs  *ScriptRuns
	stamp uint64
}

// heldLocked reports whether an invocation that may have written the
// character still runs.
func (g *detachGate) heldLocked() bool {
	return g.runs != nil && g.runs.running(g.stamp)
}

// MarkDetaching marks c as leaving the world, waits out every script
// invocation that wrote c, then seals its quest journal. A script invocation
// that wrote c before the mark, from whichever queue, has made all its
// writes to c when MarkDetaching returns, so they are in the final saves of
// c's departure; one that had not is refused every write to c. See
// WhileAttached.
func (c *Character) MarkDetaching() {
	g := &c.detachGate
	g.mu.Lock()
	c.detaching.Store(true)
	for g.heldLocked() {
		runs, stamp := g.runs, g.stamp
		g.mu.Unlock()
		runs.awaitEnd(stamp)
		// A write admitted meanwhile has moved the stamp on: look again.
		g.mu.Lock()
	}
	g.mu.Unlock()
	c.quests.Seal()
}

// WhileAttached runs fn, one script write to c, unless c is leaving the
// world, and reports whether it ran. A write is refused once MarkDetaching
// has begun, unless a script invocation that wrote c before is still
// running: the departure is then waiting for that invocation, and this write
// lands ahead of the final saves too. So a script invocation's writes to c,
// from whichever queue, all land before those saves, or none does. Writes
// may nest.
func (c *Character) WhileAttached(fn func()) bool {
	runs := c.runs()
	// The write counts as an invocation of its own, so the departure waits
	// for fn even when no script invocation raised it.
	epoch := runs.Begin()
	defer runs.End(epoch)
	if !c.admitScriptWrite(runs) {
		return false
	}
	fn()
	return true
}

// whileAttached is WhileAttached for the character's own script helpers.
func (c *Character) whileAttached(fn func()) { c.WhileAttached(fn) }

func (c *Character) admitScriptWrite(runs *ScriptRuns) bool {
	g := &c.detachGate
	g.mu.Lock()
	defer g.mu.Unlock()
	if c.detaching.Load() && !g.heldLocked() {
		return false
	}
	g.runs, g.stamp = runs, runs.stamp()
	return true
}

// runs returns the counter of the script invocations in flight.
func (c *Character) runs() *ScriptRuns {
	c.stateMu.RLock()
	runs := c.scriptRuns
	c.stateMu.RUnlock()
	if runs == nil {
		return &c.ownScriptRuns
	}
	return runs
}
