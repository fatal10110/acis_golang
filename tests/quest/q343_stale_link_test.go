package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q343"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	q343Name = "Q343_UnderTheShadowOfTheIvoryTower"
	q343Path = "quest." + q343Name
	cemaID   = 30834
	nebulite = 4364
)

// TestQ343CemaBuyBackWithNoOrb: the "receive money" link of Cema's page is
// offered only to a player holding orbs, so no scenario can reach her answer
// to one with none (the runner drops an unoffered link). A player who loses
// the orbs while the page is open still sends it: Cema refuses with 30834-08a
// and pays nothing.
func TestQ343CemaBuyBackWithNoOrb(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, 40, chatPages(shippedQuestPages(t, q343Name), cemaID),
		func(srv *gameservertest.Server, objID int32) {
			insertJournal(t, srv.DB,
				journalRow{objID, q343Name, "<state>", val("STARTED")},
				journalRow{objID, q343Name, "<cond>", val("1")},
				journalRow{objID, q343Name, "memoState", val("1")},
				journalRow{objID, q343Name, "memoStateEx", val("1")},
			)
			srv.GiveItem(t, objID, nebulite, 3)
		},
		x1Items([]int32{nebulite}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", cemaID)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{cemaID: script.KindFolk},
			[]script.Listing{{Path: q343Path}}, script.Catalog{q343Path: q343.New}),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "cema", cemaID, 40, 0, 0)

	wantLines(t, "window", w.talk(t, "cema"), w.page(t, "cema", q343Name, "30834-07.htm"))

	w.srv.RunScript(t, w.player, q343Name, func(sc *script.Script, p *script.Player) { sc.TakeItems(p, nebulite, -1) })
	got := w.send(t, encodeBypass("Quest "+q343Name+" 30834-08.htm"))
	wantLines(t, "no orb", got, w.page(t, "cema", q343Name, "30834-08a.htm"))
	if n := w.srv.PlayerItemCount(t, w.player, 57); n != 0 {
		t.Fatalf("adena after the refusal = %d, want 0", n)
	}
}
