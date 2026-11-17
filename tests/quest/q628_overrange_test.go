package quest

import (
	"testing"
)

// TestQ628OverRangeBuffAnswersNothing: at cond 3, a digit-only event past
// int32 names no buff and fails to parse as one; the player sees nothing,
// not its own digits echoed back.
func TestQ628OverRangeBuffAnswersNothing(t *testing.T) {
	t.Parallel()
	w := q628Boot(t, "3", q628Soldier)
	w.interact(t, w.npcs["abercrombie"])
	w.srv.ReadQueued(t, w.srv.Client)
	for _, event := range []string{"99999999999", "99999999999999999999"} {
		w.offerEvent(t, q628Name, event)
		if got := w.send(t, encodeBypass("Quest "+q628Name+" "+event)); len(got) != 0 {
			t.Fatalf("event %s answered %q, want nothing", event, got)
		}
	}
}
