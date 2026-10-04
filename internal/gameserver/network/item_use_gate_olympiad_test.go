package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestPlayerOlympiadConditionReadsMatchMode pins the item <player
// olympiad=...> use condition to the player's real Olympiad mode: "true"
// holds only in a match, "false" only outside one, and an unreadable value
// reads as false, as Boolean.parseBoolean does.
func TestPlayerOlympiadConditionReadsMatchMode(t *testing.T) {
	for _, tt := range []struct {
		raw     string
		inMatch bool
		want    bool
	}{
		{"true", true, true},
		{"true", false, false},
		{"false", true, false},
		{"false", false, true},
		{"1", true, false},
		{"1", false, true},
	} {
		live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
		live.SetOlympiadMode(tt.inMatch)
		if got := playerUseConditionHolds(live, map[string]string{"olympiad": tt.raw}); got != tt.want {
			t.Errorf("<player olympiad=%q> in a match %v = %v, want %v", tt.raw, tt.inMatch, got, tt.want)
		}
	}
}

// TestItemRestrictionAnnouncesOlympiadBar pins the worn-item check against
// Item.checkCondition called without messages: the Olympiad bar still
// tells a competitor 1507 for a wearable barred item and 1508 for any other,
// while a failed use condition and an item nothing bars stay silent.
func TestItemRestrictionAnnouncesOlympiadBar(t *testing.T) {
	failing := item.Condition{Kind: "player", Attrs: map[string]string{"level": "99"}}
	for _, tt := range []struct {
		name    string
		tmpl    item.Template
		inMatch bool
		holds   bool
		wantMsg int
	}{
		{"hero weapon in a match", item.Template{ID: 6611, Kind: item.KindWeapon, Slot: item.SlotRHand}, true, false, serverpackets.SystemMessageItemCantBeEquippedForOlympiad},
		{"restricted etc item in a match", item.Template{ID: 728, Kind: item.KindEtcItem, OlyRestricted: true}, true, false, serverpackets.SystemMessageItemUnavailableForOlympiad},
		{"hero weapon outside a match", item.Template{ID: 6611, Kind: item.KindWeapon, Slot: item.SlotRHand}, false, true, 0},
		{"failed use condition in a match", item.Template{ID: 7818, Kind: item.KindWeapon, Slot: item.SlotRHand, UseConditions: []item.UseCondition{{Root: failing, MessageID: 1685}}}, true, false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			capture := &testsupport.FrameCapture{}
			live := newTestLivePlayer(t, 1, capture)
			live.SetOlympiadMode(tt.inMatch)
			if got := itemRestrictionHolds(live, &tt.tmpl); got != tt.holds {
				t.Fatalf("itemRestrictionHolds = %v, want %v", got, tt.holds)
			}
			frames := capture.Frames()
			if tt.wantMsg == 0 {
				if len(frames) != 0 {
					t.Fatalf("frames = %x, want none", frames)
				}
				return
			}
			if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSystemMessage {
				t.Fatalf("frames = %x, want one SystemMessage", frames)
			}
			if id := int32(frames[0][1]) | int32(frames[0][2])<<8 | int32(frames[0][3])<<16 | int32(frames[0][4])<<24; id != int32(tt.wantMsg) {
				t.Fatalf("system message = %d, want %d", id, tt.wantMsg)
			}
		})
	}
}
