package player

import "sync"

// ScriptRuns counts the script hook invocations in flight, so a character
// leaving the world can wait out every invocation that may still change it.
// An invocation is counted under the epoch it began in, and the epoch moves
// on each time a script changes a character (stamp): the invocations under
// an epoch at or below a character's last stamp include every one that was
// running at that change, the one making it among them. The zero value is
// ready to use.
type ScriptRuns struct {
	mu sync.Mutex
	// ended is signaled each time the last invocation of an epoch ends.
	ended  sync.Cond
	epoch  uint64
	active map[uint64]int
}

// Begin counts one invocation as running and returns its epoch, for End.
// Invocations nest: one raised from inside another is counted again.
func (r *ScriptRuns) Begin() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil {
		r.active = make(map[uint64]int)
	}
	r.active[r.epoch]++
	return r.epoch
}

// End counts the invocation Begin returned epoch for as over.
func (r *ScriptRuns) End(epoch uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active[epoch]--; r.active[epoch] > 0 {
		return
	}
	delete(r.active, epoch)
	if r.ended.L != nil {
		r.ended.Broadcast()
	}
}

// stamp returns an epoch at or above that of every invocation running now;
// any invocation begun later is counted above it.
func (r *ScriptRuns) stamp() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	stamp := r.epoch
	r.epoch++
	return stamp
}

// running reports whether an invocation counted at or below stamp is still
// running. Once false it stays false: later invocations count above stamp.
func (r *ScriptRuns) running(stamp uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runningLocked(stamp)
}

func (r *ScriptRuns) runningLocked(stamp uint64) bool {
	for epoch := range r.active {
		if epoch <= stamp {
			return true
		}
	}
	return false
}

// awaitEnd blocks until no invocation counted at or below stamp is running.
func (r *ScriptRuns) awaitEnd(stamp uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended.L == nil {
		r.ended.L = &r.mu
	}
	for r.runningLocked(stamp) {
		r.ended.Wait()
	}
}
