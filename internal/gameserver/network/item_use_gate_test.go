package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestUnreadableBoolConditionReadsFalse: a value that is not "true" is a
// false requirement, never a failed condition. moving/riding pass while the
// player is not in that state, so "1" and "yes" let the item be used, and
// only a case-insensitive "true" blocks it.
func TestUnreadableBoolConditionReadsFalse(t *testing.T) {
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	holds := func(attr, raw string) bool {
		tmpl := item.Template{ID: 7818, UseConditions: []item.UseCondition{{Root: item.Condition{Kind: "player", Attrs: map[string]string{attr: raw}}}}}
		got, _ := itemRestrictionHolds(live, &tmpl)
		return got
	}
	for _, attr := range []string{"moving", "riding"} {
		for _, raw := range []string{"1", "t", "yes", "", "false"} {
			if !holds(attr, raw) {
				t.Errorf("<player %s=%q> denied use, want it read as false", attr, raw)
			}
		}
		if holds(attr, "TrUe") {
			t.Errorf("<player %s=\"TrUe\"> allowed use, want it read as true", attr)
		}
	}
}
