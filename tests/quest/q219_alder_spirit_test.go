package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q219"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Testimony of Fate's Alder spirit (Q219_TestimonyOfFate onTalk of Kaira at
// cond 9 and of the spirit at cond 10): Kaira's second visit calls the
// spirit on the player, with no offset and no time to stay, and the spirit's
// dialog leaves with it. A scenario cannot show a script-spawned NPC
// appearing or leaving (#4020), so this Boot test pins it; the scenarios play
// the rest of the quest with the cond seeded.
const (
	q219Name        = "Q219_TestimonyOfFate"
	q219Path        = "quest.Q219_TestimonyOfFate"
	q219Kaira       = 30476
	q219AlderSpirit = 30613
	q219AlderSkull1 = 3184
	q219AlderSkull2 = 3185
)

func TestQ219KairaCallsTheAlderSpiritOnThePlayer(t *testing.T) {
	t.Parallel()
	pages := chatPages(questPages(q219Name, map[string][]string{
		"30476-09.htm": nil,
		"30613-01.htm": nil,
	}), q219Kaira, q219AlderSpirit)
	w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q219Name, "<state>", val("STARTED")},
			journalRow{objID, q219Name, "<cond>", val("9")})
		srv.GiveItem(t, objID, q219AlderSkull1, 1)
	},
		x1Items([]int32{q219AlderSkull1, q219AlderSkull2}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("Folk", q219Kaira), gameservertest.FolkTemplate("Folk", q219AlderSpirit),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q219Kaira: script.KindFolk, q219AlderSpirit: script.KindFolk},
			[]script.Listing{{Path: q219Path}},
			script.Catalog{q219Path: q219.New},
		),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "kaira", q219Kaira, 40, 0, 0)

	if n := len(npcsOf(w.srv, q219AlderSpirit)); n != 0 {
		t.Fatalf("%d Alder spirits before Kaira's call, want 0", n)
	}
	if got := w.talk(t, "kaira"); !shows(got, "30476-09.htm") {
		t.Fatalf("Kaira: got %q, want 30476-09.htm", got)
	}
	if rows := questRows(t, w.srv, w.player, q219Name); rows["<cond>"] != "10" {
		t.Fatalf("rows %v, want cond 10", rows)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q219AlderSkull1); n != 0 {
		t.Fatalf("%d first Alder skulls after the call, want 0", n)
	}
	spirits := npcsOf(w.srv, q219AlderSpirit)
	if len(spirits) != 1 {
		t.Fatalf("%d Alder spirits after Kaira's call, want 1", len(spirits))
	}
	spirit := spirits[0]
	if loc := spirit.SpawnLocation(); loc.X != w.at.X || loc.Y != w.at.Y || abs(loc.Z-w.at.Z) > 32 {
		t.Fatalf("the spirit stands at %+v, want the player's %+v", loc, w.at)
	}

	// The call has no time to stay: the spirit is still there much later.
	w.advance(t, 30*time.Minute)
	if n := len(npcsOf(w.srv, q219AlderSpirit)); n != 1 {
		t.Fatalf("%d Alder spirits 30 minutes after the call, want 1", n)
	}

	w.roles[spirit.ObjectID()], w.npcs["spirit"] = "spirit", spirit.ObjectID()
	if got := w.talk(t, "spirit"); !shows(got, "30613-01.htm") {
		t.Fatalf("spirit: got %q, want 30613-01.htm", got)
	}
	if rows := questRows(t, w.srv, w.player, q219Name); rows["<cond>"] != "11" {
		t.Fatalf("rows %v, want cond 11", rows)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q219AlderSkull2); n != 1 {
		t.Fatalf("%d second Alder skulls, want 1", n)
	}
	if n := len(npcsOf(w.srv, q219AlderSpirit)); n != 0 {
		t.Fatalf("%d Alder spirits after the dialog, want 0", n)
	}
}
