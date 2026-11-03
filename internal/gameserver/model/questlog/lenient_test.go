package questlog

import (
	"reflect"
	"testing"
)

// TestSetCondReadsUnreadableReservedValues pins the condition change on a
// hand-edited state: a <cond> that is not an int reads 0 and a <flags> that
// is not an int reads as no flags, so the change goes ahead, writes only the
// new <cond>, and leaves the unreadable <flags> in place, where the quest
// window gives way to the flags the new condition gives.
func TestSetCondReadsUnreadableReservedValues(t *testing.T) {
	j, st := seeded(quests[0], map[string]string{KeyState: "STARTED", KeyCond: "x", KeyFlags: "y"})
	list, ok := st.SetCond(3)
	if !ok {
		t.Fatal("SetCond(3) refused")
	}
	if want := []Entry{{1, condFlags(3)}}; !reflect.DeepEqual(list, want) {
		t.Fatalf("SetCond(3) list = %v, want %v", list, want)
	}
	if want := map[string]string{KeyState: "STARTED", KeyCond: "3", KeyFlags: "y"}; !reflect.DeepEqual(st.vars, want) {
		t.Fatalf("vars = %v, want %v", st.vars, want)
	}
	if got, want := j.TakePending(), []Write{{Op: OpSet, Quest: quests[0].Name, Var: KeyCond, Value: "3"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pending = %v, want %v", got, want)
	}
}
