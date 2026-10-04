package ai

import "testing"

// TestDesireQueueSnapshotCopiesInQueueOrder pins Snapshot: every desire in
// queue order, by value, so a later weight change leaves the copy alone.
func TestDesireQueueSnapshotCopiesInQueueOrder(t *testing.T) {
	q := NewDesireQueue()
	if got := q.Snapshot(); len(got) != 0 {
		t.Fatalf("empty queue Snapshot = %v, want none", got)
	}
	q.AddOrUpdate(&Desire{Kind: IntentionWander, Weight: 5})
	q.AddOrUpdate(&Desire{Kind: IntentionNothing, Weight: 2})
	got := q.Snapshot()
	if len(got) != 2 || got[0].Kind != IntentionWander || got[0].Weight != 5 || got[1].Kind != IntentionNothing || got[1].Weight != 2 {
		t.Fatalf("Snapshot = %+v, want WANDER 5 then NOTHING 2", got)
	}
	q.AddOrUpdate(&Desire{Kind: IntentionWander, Weight: 1})
	if got[0].Weight != 5 {
		t.Fatalf("snapshot weight moved to %v after a merge, want 5", got[0].Weight)
	}
}
