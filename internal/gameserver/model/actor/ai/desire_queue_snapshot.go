package ai

// Snapshot returns a copy of every queued Desire, in the order they were
// queued.
func (q *DesireQueue) Snapshot() []Desire {
	q.mu.RLock()
	defer q.mu.RUnlock()

	out := make([]Desire, len(q.desires))
	for i, d := range q.desires {
		out[i] = *d
	}
	return out
}
