package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestClassSwitchRecalculatesPartyLevel switches a party member between
// its level 75 base class and a level 40 subclass: the party's level
// follows the highest member level each time, as a class change's
// recalculation sets it.
func TestClassSwitchRecalculatesPartyLevel(t *testing.T) {
	link := &GameClientLink{log: zerolog.Nop(), templates: testTemplates(t), parties: party.NewRegistry[*livePlayer](nil)}
	switcher := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	other := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})
	switcher.shortcuts = shortcut.NewList(nil)
	switcher.CharLevel, other.CharLevel = 75, 52
	if !switcher.AddSubclass(player.SubClass{ClassID: switcher.BaseClassID, Index: 1, Level: 40}) {
		t.Fatal("add subclass refused")
	}
	if status, _ := link.parties.BeginInvite(switcher.ObjectID(), 0); status != party.InviteReady {
		t.Fatalf("BeginInvite = %v", status)
	}
	link.parties.Answer(switcher, other, true)

	partyLevel := func() int {
		t.Helper()
		v, ok := link.parties.View(other.ObjectID())
		if !ok {
			t.Fatal("party gone")
		}
		return v.Level
	}
	if got := partyLevel(); got != 75 {
		t.Fatalf("formed party level = %d, want 75", got)
	}
	link.switchClass(switcher, 1, classChangeRows{})
	if got := partyLevel(); got != 52 {
		t.Fatalf("party level after the switch to the level 40 subclass = %d, want 52", got)
	}
	link.switchClass(switcher, 0, classChangeRows{})
	if got := partyLevel(); got != 75 {
		t.Fatalf("party level after the switch back to the base class = %d, want 75", got)
	}
}
