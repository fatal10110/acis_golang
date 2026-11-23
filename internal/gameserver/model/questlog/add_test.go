package questlog

import (
	"math"
	"strconv"
	"sync"
	"testing"
)

// Concurrent adds to one variable are all counted, and every add is
// written (#5171).
func TestStateAddCountsConcurrentAdds(t *testing.T) {
	j, st := seeded(quests[0], map[string]string{KeyState: "STARTED"})
	const adders, each = 8, 50
	var wg sync.WaitGroup
	for range adders {
		wg.Go(func() {
			for range each {
				if _, ok := st.Add("sacrificed", 1); !ok {
					t.Error("an add to an open journal reported false")
				}
			}
		})
	}
	wg.Wait()
	if v, _ := st.Get("sacrificed"); v != strconv.Itoa(adders*each) {
		t.Fatalf("sacrificed = %q, want %d", v, adders*each)
	}
	if n := len(j.TakePending()); n != adders*each {
		t.Fatalf("%d writes owed, want %d", n, adders*each)
	}
}

// An unset or non-integer variable counts as 0, the sum wraps as a 32-bit
// integer, and a sealed journal takes no add.
func TestStateAddReadsAndWraps(t *testing.T) {
	j, st := seeded(quests[0], map[string]string{KeyState: "STARTED", "bad": "x", "top": strconv.Itoa(math.MaxInt32)})
	for _, c := range []struct {
		key   string
		delta int32
		want  int32
	}{
		{"unset", 2, 2},
		{"unset", -3, -1},
		{"bad", 1, 1},
		{"top", 1, math.MinInt32},
	} {
		if sum, ok := st.Add(c.key, c.delta); !ok || sum != c.want {
			t.Errorf("Add(%s, %d) = %d, %v; want %d, true", c.key, c.delta, sum, ok, c.want)
		}
		if v, _ := st.Get(c.key); v != strconv.Itoa(int(c.want)) {
			t.Errorf("%s = %q after the add, want %d", c.key, v, c.want)
		}
	}

	j.Seal()
	if _, ok := st.Add("unset", 1); ok {
		t.Error("an add to a sealed journal reported true")
	}
	if v, _ := st.Get("unset"); v != "-1" {
		t.Errorf("unset = %q after an add to the sealed journal, want -1", v)
	}
}
