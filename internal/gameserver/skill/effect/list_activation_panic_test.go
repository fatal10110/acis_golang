package effect

import "testing"

// TestPanickingStartLeavesListActivating panics in a start hook that queued
// another activation on the same list first. The panic ends that run, but
// the queued activation and the ones added later still start.
func TestPanickingStartLeavesListActivating(t *testing.T) {
	list := newTestList(newModLedger())
	queued := newEffect("queued", 2, "queued", 1, false)
	bad := newEffect("bad", 1, "bad", 1, false)
	bad.OnStart = func(*Effect) bool {
		list.Add(queued)
		panic("start hook failed")
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("start hook panic did not reach the caller")
			}
		}()
		list.Add(bad)
	}()
	if queued.InUse() {
		t.Fatal("queued activation ran inside the panicking run")
	}

	later := newEffect("later", 3, "later", 1, false)
	list.Add(later)

	if !queued.InUse() || !later.InUse() {
		t.Fatalf("queued/later in use = %v/%v, want both active after the panic", queued.InUse(), later.InUse())
	}
}
