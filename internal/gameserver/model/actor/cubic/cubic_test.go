package cubic

import (
	"reflect"
	"testing"
)

func TestSkillIDs(t *testing.T) {
	tests := []struct {
		id   ID
		want []int
	}{
		{Storm, []int{4049}},
		{Poltergeist, []int{4053, 4054, 4055}},
		{Attract, []int{5115, 5116}},
		{ID(99), nil},
	}
	for _, tt := range tests {
		if got := SkillIDs(tt.id); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SkillIDs(%v) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestList_AddOrRefresh(t *testing.T) {
	var l List

	refreshed, _, evicted := l.AddOrRefresh(Storm, false, 2)
	if refreshed || evicted {
		t.Fatalf("first add: refreshed=%v evicted=%v, want false,false", refreshed, evicted)
	}
	if !l.Has(Storm) || l.Len() != 1 {
		t.Fatalf("after first add: Has(Storm)=%v Len=%d, want true,1", l.Has(Storm), l.Len())
	}

	// Re-adding the same id reports a refresh and changes nothing.
	refreshed, _, evicted = l.AddOrRefresh(Storm, false, 2)
	if !refreshed || evicted {
		t.Fatalf("re-add same id: refreshed=%v evicted=%v, want true,false", refreshed, evicted)
	}
	if l.Len() != 1 {
		t.Fatalf("Len after refresh = %d, want 1", l.Len())
	}
}

func TestList_AddOrRefresh_EvictsOldestPastCap(t *testing.T) {
	var l List
	maxSlots := 1 // isFull is size > maxSlots, so a 2nd add before this cap doesn't evict

	l.AddOrRefresh(Storm, false, maxSlots)
	refreshed, evicted, didEvict := l.AddOrRefresh(Vampiric, false, maxSlots)
	if refreshed || didEvict {
		t.Fatalf("2nd add at size 1 > maxSlots 1 is false: refreshed=%v didEvict=%v, want false,false", refreshed, didEvict)
	}
	if l.Len() != 2 {
		t.Fatalf("Len after 2nd add = %d, want 2", l.Len())
	}

	// Now size (2) > maxSlots (1): the next add evicts the oldest entry
	// (Storm) before admitting the new one.
	refreshed, evicted, didEvict = l.AddOrRefresh(Life, false, maxSlots)
	if refreshed || !didEvict || evicted != Storm {
		t.Fatalf("3rd add: refreshed=%v didEvict=%v evicted=%v, want false,true,Storm", refreshed, didEvict, evicted)
	}
	if l.Has(Storm) {
		t.Errorf("Storm should have been evicted")
	}
	if !l.Has(Vampiric) || !l.Has(Life) {
		t.Errorf("Vampiric and Life should both remain active")
	}
	if l.Len() != 2 {
		t.Fatalf("Len after eviction = %d, want 2", l.Len())
	}
}

func TestList_Remove(t *testing.T) {
	var l List
	l.AddOrRefresh(Storm, false, 5)
	l.AddOrRefresh(Vampiric, false, 5)

	l.Remove(Storm)
	if l.Has(Storm) {
		t.Errorf("Storm should have been removed")
	}
	if !l.Has(Vampiric) {
		t.Errorf("Vampiric should remain")
	}

	// Removing an id that isn't active is a no-op.
	l.Remove(Storm)
	if l.Len() != 1 {
		t.Errorf("Len after removing an absent id = %d, want 1", l.Len())
	}
}

func TestList_StopAll(t *testing.T) {
	var l List
	l.AddOrRefresh(Storm, false, 5)
	l.AddOrRefresh(Vampiric, true, 5)

	stopped := l.StopAll()
	if len(stopped) != 2 {
		t.Fatalf("StopAll() returned %d ids, want 2", len(stopped))
	}
	if l.Len() != 0 {
		t.Errorf("Len after StopAll = %d, want 0", l.Len())
	}
}

func TestList_StopGivenByOthers(t *testing.T) {
	var l List
	l.AddOrRefresh(Storm, false, 5)   // own cubic
	l.AddOrRefresh(Vampiric, true, 5) // granted by a party member
	l.AddOrRefresh(Life, true, 5)     // also granted

	stopped := l.StopGivenByOthers()
	if len(stopped) != 2 {
		t.Fatalf("StopGivenByOthers() returned %d ids, want 2", len(stopped))
	}
	if !l.Has(Storm) {
		t.Errorf("owner's own cubic should remain active")
	}
	if l.Has(Vampiric) || l.Has(Life) {
		t.Errorf("cubics granted by others should have been stopped")
	}
	if l.Len() != 1 {
		t.Errorf("Len after StopGivenByOthers = %d, want 1", l.Len())
	}
}
