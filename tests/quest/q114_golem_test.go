package quest

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q114"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Resurrection of an Old Manager's guardian (Q114_ResurrectionOfAnOldManager
// onAdvEvent "32047-15a.htm", onDecayed, onMyDying) and the pile of stones'
// screen message (onTalk at cond 17). A scenario cannot show a script-spawned
// NPC appearing, speaking from out of sight or leaving, nor an
// ExShowScreenMessage (#4020 item 5), so these Boot tests pin them.
const (
	q114Name      = "Q114_ResurrectionOfAnOldManager"
	q114Path      = "quest.Q114_ResurrectionOfAnOldManager"
	q114Wendy     = 32047
	q114Stone     = 32046
	q114Golem     = 27318
	q114Detector  = 8090
	q114Detector2 = 8091
)

// q114GolemAt is where Wendy wakes her guardian.
var q114GolemAt = [3]int{96977, -110625, -3322}

// q114World boots a level 49 character whose Resurrection of an Old
// Manager is at cond, holding one of each of holds, standing where Wendy
// wakes her guardian.
func q114World(t *testing.T, cond int, holds ...int32) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q114Name, map[string][]string{
		"32047-14a.htm": {"32047-15a.htm"},
		"32047-15a.htm": nil,
		"32047-16a.htm": nil,
		"32047-19a.htm": nil,
	}), q114Wendy, q114Stone)
	w := bootSeamQuest(t, 49, pages, func(srv *gameservertest.Server, objID int32) {
		if _, err := srv.DB.Exec(`UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?`,
			q114GolemAt[0], q114GolemAt[1], q114GolemAt[2], objID); err != nil {
			t.Fatal(err)
		}
		insertJournal(t, srv.DB,
			journalRow{objID, q114Name, "<state>", val("STARTED")},
			journalRow{objID, q114Name, "<cond>", val(strconv.Itoa(cond))})
		for _, id := range holds {
			srv.GiveItem(t, objID, id, 1)
		}
	},
		x1Items([]int32{q114Detector, q114Detector2}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q114Golem)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q114Wendy: script.KindFolk, q114Stone: script.KindFolk, q114Golem: script.KindHostile},
			[]script.Listing{{Path: q114Path}},
			script.Catalog{q114Path: q114.New},
		),
	)
	w.roles[w.player] = "player"
	return w
}

// wakeGuardian asks Wendy to wake her guardian and renders the answer.
func (w *dialogWorld) wakeGuardian(t *testing.T) []string {
	t.Helper()
	wantQ114Page(t, "Wendy's talk", w.talk(t, "wendy"), "32047-14a.htm")
	return w.send(t, encodeBypass("Quest "+q114Name+" 32047-15a.htm"))
}

// q114Page is the start of the rendered stand-in page file of Wendy.
func q114Page(file string) string {
	return `S NpcHtmlMessage obj=wendy item=0 html="<html><body>` + file
}

// wantQ114Page fails unless got shows Wendy's stand-in page file.
func wantQ114Page(t *testing.T, step string, got []string, file string) {
	t.Helper()
	if !slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, q114Page(file)) }) {
		t.Fatalf("%s: got %q, want the page %s", step, got, file)
	}
}

// TestQ114WendyWakesOneGuardianAtATime: Wendy wakes her guardian beside the
// pile of stones, which calls the player out by name and attacks it; while
// it stands she wakes no other, and once it leaves after 300 s, dead by the
// time its decayed hook runs and so without a word, she wakes another. A
// guardian killed at cond 10 moves the quest to cond 11 with its last line.
func TestQ114WendyWakesOneGuardianAtATime(t *testing.T) {
	t.Parallel()
	w := q114World(t, 10)
	w.spawnFolk(t, "wendy", q114Wendy, 40, 0, 0)
	// The guardian beats the player for the whole 300 s it stands; its
	// random hits would now and then kill the player before the third
	// call, so the player takes no damage from them.
	p, ok := playerCombatant(t, w.srv, w.player).(interface{ SetInvul(bool) bool })
	if !ok {
		t.Fatal("the player cannot be made invulnerable")
	}
	p.SetInvul(true)

	got := w.wakeGuardian(t)
	g := w.spawned(t, "golem", q114Golem)
	// The call renders before the guardian is named: it reads as its object id.
	wantLines(t, "first call", got, say(strconv.Itoa(int(g.ObjectID())), q114Golem, 11450, "Seeker"))
	wantQ114Page(t, "first call", got, "32047-15a.htm")
	if loc := g.SpawnLocation(); abs(loc.X-q114GolemAt[0]) > 100 || abs(loc.Y-q114GolemAt[1]) > 100 || abs(loc.Z-q114GolemAt[2]) > 100 {
		t.Fatalf("the guardian stands at %+v, want within 100 of %v", loc, q114GolemAt)
	}
	if h := g.MostHated(); h == nil || h.ObjectID() != w.player {
		t.Fatalf("the guardian hates %v most, want the player", h)
	}
	first := g.ObjectID()

	wantQ114Page(t, "second call", w.wakeGuardian(t), "32047-19a.htm")
	if n := len(npcsOf(w.srv, q114Golem)); n != 1 {
		t.Fatalf("%d guardians after the second call, want 1", n)
	}

	saysNone(t, "299 s after the call", w.advance(t, 299*time.Second))
	if n := len(npcsOf(w.srv, q114Golem)); n != 1 {
		t.Fatalf("%d guardians 299 s after the call, want 1", n)
	}
	saysNone(t, "the despawn", w.advance(t, 2*time.Second))
	if n := len(npcsOf(w.srv, q114Golem)); n != 0 {
		t.Fatalf("%d guardians after the despawn, want 0", n)
	}

	if w.srv.PlayerDead(t, w.player) {
		t.Fatal("the player died while the guardian stood")
	}

	got = w.wakeGuardian(t)
	g = w.spawned(t, "golem", q114Golem)
	if g.ObjectID() == first {
		t.Fatal("the third call kept the first guardian")
	}
	wantLines(t, "third call", got, say(strconv.Itoa(int(g.ObjectID())), q114Golem, 11450, "Seeker"))
	wantQ114Page(t, "third call", got, "32047-15a.htm")

	killOnPlayerQueue(t, w.srv, w.player, hostileNPC(t, w.srv, g))
	wantLines(t, "the kill", w.render(t, w.srv.ReadQueued(t, w.srv.Client)),
		"S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0",
		say("golem", q114Golem, 11452))
	wantQ114Page(t, "Wendy after the kill", w.talk(t, "wendy"), "32047-16a.htm")
}

// TestQ114StonesShowTheDetectorsFind: at cond 17 the pile of stones swaps
// the detector for the reacting one and shows the player the stones' find
// at the top center of its screen for 4.5 s, with no page.
func TestQ114StonesShowTheDetectorsFind(t *testing.T) {
	t.Parallel()
	w := q114World(t, 17, q114Detector)
	stone := w.spawnFolk(t, "stone", q114Stone, 40, 0, 0)

	w.interact(t, stone.ObjectID())
	w.srv.Client.Send(encodeBypass("npc_" + strconv.Itoa(int(stone.ObjectID())) + "_Quest"))
	frames := w.srv.ReadQueued(t, w.srv.Client)

	screen := []byte{serverpackets.OpcodeExtended, 0x38, 0x00}
	screen = int32s(screen, 1, -1, 2, 0, 0, 0, 0, 0, 4500, 0)
	screen = utf16z(screen, script.NpcString(11453))
	var screens int
	for _, f := range frames {
		switch {
		case bytes.Equal(f, screen):
			screens++
		case f[0] == serverpackets.OpcodeNpcHtmlMessage:
			t.Fatalf("the stones showed a page % x", f)
		}
	}
	if screens != 1 {
		t.Fatalf("%d screen messages of the stones' find, want 1", screens)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q114Detector); n != 0 {
		t.Fatalf("detectors %d, want 0", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q114Detector2); n != 1 {
		t.Fatalf("reacting detectors %d, want 1", n)
	}
	if cond := questRows(t, w.srv, w.player, q114Name)["<cond>"]; cond != "18" {
		t.Fatalf("cond %q after the stones, want 18", cond)
	}
}
