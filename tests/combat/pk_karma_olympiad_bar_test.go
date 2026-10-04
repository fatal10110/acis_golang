package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPKKillInOlympiadAnnouncesTheBarBeforeTheItemComesOff pins the worn
// item check a PK karma gain runs (Player.checkItemRestriction) for an item
// the Olympiad bars: Item.checkCondition sends the bar's message whatever
// its sendMessage flag, so a competitor whose weapon the bar takes off reads
// 1507 (cannot be equipped for the Olympiad) right after the karma change
// and just before S1_DISARMED, and the weapon comes off.
func TestPKKillInOlympiadAnnouncesTheBarBeforeTheItemComesOff(t *testing.T) {
	t.Parallel()
	knife := shippedItemTemplate(t, apprenticeKnifeID)
	barred := *knife
	barred.Weight = 0
	barred.OlyRestricted = true
	templates := item.NewTable(append(gameservertest.ItemTemplates().All(), &barred))
	s := bootPKKillSceneWith(t, []gameservertest.Option{gameservertest.WithItemTemplates(templates)})
	// Every swing hits, so the scenario never waits on a lucky roll.
	s.setRollSource(func(int) int { return 0 })
	s.srv.SetPlayerOlympiadMode(t, s.objID, true)

	selectPlayerTarget(t, s.c, s.victimID)
	s.c.Send(encodeAttackRequest(s.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames := readQuiet(s.c)
	bar := len(s.karmaTail)
	if len(frames) < bar+2 || string(opcodesOf(frames[:bar])) != string(s.karmaTail) {
		t.Fatalf("frames after the karma change = %x, want %x then the Olympiad bar and S1_DISARMED", opcodesOf(frames), s.karmaTail)
	}
	if f := frames[bar]; f[0] != serverpackets.OpcodeSystemMessage || wireReader(f[1:]).ReadInt32() != serverpackets.SystemMessageItemCantBeEquippedForOlympiad {
		t.Fatalf("frame after the karma change = %x, want SystemMessage %d", f, serverpackets.SystemMessageItemCantBeEquippedForOlympiad)
	}
	if f := frames[bar+1]; f[0] != serverpackets.OpcodeSystemMessage || wireReader(f[1:]).ReadInt32() != serverpackets.SystemMessageS1Disarmed {
		t.Fatalf("frame after the Olympiad bar = %x, want S1_DISARMED", f)
	}
	if s.inv.ItemByObjectID(s.knifeObjID).Equipped() {
		t.Fatal("barred knife still equipped after the PK kill")
	}
}
