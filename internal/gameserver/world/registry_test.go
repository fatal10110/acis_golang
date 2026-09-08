package world

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
)

type registryTestObject struct{ id int32 }

func (o *registryTestObject) ObjectID() int32 { return o.id }

// TestRegistryAppendAllAppendsRatherThanReplaces is the regression case for
// appendAll shadowing Region.AppendObjects with the opposite contract:
// Region.AppendObjects appends to its out parameter, so appendAll must too
// — a caller reusing a buffer across registries by the same append+return
// convention would otherwise silently lose whatever it already put there.
func TestRegistryAppendAllAppendsRatherThanReplaces(t *testing.T) {
	r := newRegistry()
	r.add(1, &registryTestObject{id: 1})
	r.add(2, &registryTestObject{id: 2})

	sentinel := &registryTestObject{id: 99}
	dst := []worldobject.Object{sentinel}

	got := r.appendAll(dst)

	if len(got) != 3 {
		t.Fatalf("appendAll len = %d, want 3 (1 pre-existing + 2 registered)", len(got))
	}
	if got[0] != worldobject.Object(sentinel) {
		t.Fatalf("appendAll dropped dst's pre-existing entry: got[0] = %v, want sentinel", got[0])
	}
}

// TestRegistryAppendAllFreshScanNeedsExplicitTruncation documents the other
// half of the same contract: a caller that wants a snapshot rather than an
// accumulation must truncate dst itself, exactly like every
// Region.AppendObjects caller in this package does (r.AppendObjects(buf[:0])).
func TestRegistryAppendAllFreshScanNeedsExplicitTruncation(t *testing.T) {
	r := newRegistry()
	r.add(1, &registryTestObject{id: 1})

	buf := r.appendAll(nil)
	buf = r.appendAll(buf[:0])

	if len(buf) != 1 {
		t.Fatalf("appendAll(buf[:0]) len = %d, want 1 (truncated before the second scan)", len(buf))
	}
}
