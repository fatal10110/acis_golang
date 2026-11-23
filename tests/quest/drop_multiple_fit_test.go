package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q163"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	q163Name    = "Q163_LegacyOfThePoet"
	q163Path    = "quest." + q163Name
	q163OlMahum = 20372
	q163Filler  = 1060
)

// TestQ163PoemThatDoesNotFitLeavesCondOne: a character holding three of
// Rumiel's four poems with no free inventory slot rolls the fourth on an Ol
// Mahum's death, but it does not fit: he gets nothing and the quest stays
// at cond 1, as the multi-item drop reports a roll that did not fit as not
// reached. Once a slot is free the next kill gives the poem and moves the
// quest to cond 2. An owner-approved divergence (#5138) from a drop that
// reported the poem it could not give as held.
func TestQ163PoemThatDoesNotFitLeavesCondOne(t *testing.T) {
	t.Parallel()
	poems := []int32{1038, 1039, 1040, 1041}
	w := bootSeamQuest(t, 20, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q163Name, "<state>", val("STARTED")},
			journalRow{objID, q163Name, "<cond>", val("1")},
		)
		for _, id := range poems[:3] {
			srv.GiveItem(t, objID, id, 1)
		}
		srv.GiveItem(t, objID, q163Filler, 1)
	},
		x1Items(append(poems, q163Filler)),
		gameservertest.WithInventorySlots(4, 4),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q163OlMahum)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{q163OlMahum: script.KindHostile},
			[]script.Listing{{Path: q163Path}}, script.Catalog{q163Path: q163.New}),
		// Every roll succeeds.
		gameservertest.WithScriptRand(func(int) int { return 0 }),
	)
	c := onlineCharacter(t, w.srv, w.player)
	kill := func() {
		t.Helper()
		h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q163OlMahum), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
		killOnPlayerQueue(t, w.srv, w.player, h)
	}
	cond := func() string { return questRows(t, w.srv, w.player, q163Name)["<cond>"] }

	if c.RewardItemFits(poems[3], 1) {
		t.Fatal("the fourth poem fits before the kill, want a full inventory")
	}
	kill()
	if n := w.srv.PlayerItemCount(t, w.player, poems[3]); n != 0 {
		t.Fatalf("fourth poems after a kill with no free slot = %d, want 0", n)
	}
	if got := cond(); got != "1" {
		t.Fatalf("cond after a kill with no free slot = %q, want 1", got)
	}

	w.srv.RunScript(t, w.player, q163Name, func(sc *script.Script, p *script.Player) { sc.TakeItems(p, q163Filler, -1) })
	if !c.RewardItemFits(poems[3], 1) {
		t.Fatal("the fourth poem does not fit once a slot is free")
	}
	kill()
	if n := w.srv.PlayerItemCount(t, w.player, poems[3]); n != 1 {
		t.Fatalf("fourth poems after a kill with a free slot = %d, want 1", n)
	}
	if got := cond(); got != "2" {
		t.Fatalf("cond after a kill with a free slot = %q, want 2", got)
	}
}
