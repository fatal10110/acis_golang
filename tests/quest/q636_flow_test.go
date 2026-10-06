package quest

import (
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q636"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Truth Beyond the Gate's NPCs, marks and gate zone
// (Q636_TruthBeyondTheGate.java; zone 100000 of ScriptZone.xml).
const (
	q636Name     = "Q636_TruthBeyondTheGate"
	eliyahID     = 31329
	flauronID    = 32010
	visitorMark  = 8064
	fadedMark    = 8065
	templeGateID = 100000
	gateMinX     = 200
	gateMaxX     = 400
	pickedUpS1   = serverpackets.SystemMessageYouPickedUpS1
	earnedItemS1 = serverpackets.SystemMessageEarnedItemS1
)

// q636Fixture boots Q636 behind Q001, with the two marks as the shipped
// stackable quest items and the gate as a script zone across the ground
// east of the character, which stands at (10, 20, 30).
func q636Fixture(t *testing.T) []gameservertest.Option {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{visitorMark, fadedMark} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Mark " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{Type: item.EtcItemQuest},
		})
	}
	form, err := zone.NewCuboid(gateMinX, gateMaxX, -200, 200, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewScript(templeGateID, form))
	return []gameservertest.Option{
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithZones(zones),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{eliyahID: script.KindFolk, flauronID: script.KindFolk, darinID: script.KindFolk},
			[]script.Listing{{Path: "quest.Q001_LettersOfLove"}, {Path: "quest.Q636_TruthBeyondTheGate"}},
			script.Catalog{"quest.Q001_LettersOfLove": quest001.New, "quest.Q636_TruthBeyondTheGate": q636.New},
		),
	}
}

func q636Pages() map[string]string {
	return chatPages(questPages(q636Name, map[string][]string{
		"31329-01.htm": nil, "31329-02.htm": {"31329-03.htm"}, "31329-05.htm": nil,
		"32010-01.htm": {"32010-02.htm"}, "32010-02.htm": nil, "32010-03.htm": nil,
	}), eliyahID, flauronID)
}

// TestQ636VisitorMarkFadesAtTheGate plays The Truth Beyond the Gate from a
// started state: Eliyah refuses a player below level 73 a new quest but
// answers the started one, Flauron's link gives the Visitor's Mark and
// completes the quest, and walking into the gate zone swaps the mark for a
// faded one: the mark goes silently, the faded mark is named as picked up.
// Entering the zone again, with no mark left, changes nothing. It runs on
// the inline executor and on the real pool.
func TestQ636VisitorMarkFadesAtTheGate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			visitorMarkFadesAtTheGate(t, tc.opts...)
		})
	}
}

func visitorMarkFadesAtTheGate(t *testing.T, opts ...gameservertest.Option) {
	w := bootSeamQuest(t, 20, q636Pages(), func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q636Name + ":STARTED"})
	}, append(q636Fixture(t), opts...)...)
	if w.at != (location.Location{X: 10, Y: 20, Z: 30}) {
		t.Fatalf("character stands at %+v, want (10, 20, 30)", w.at)
	}
	eliyah := w.spawnFolk(t, "eliyah", eliyahID, 0, 30, 0).ObjectID()
	flauron := w.spawnFolk(t, "flauron", flauronID, 0, -30, 0).ObjectID()

	if got := w.questWindow(t, eliyah); !shows(got, "31329-05.htm") {
		t.Fatalf("Eliyah's window = %q, want 31329-05.htm", got)
	}
	if got := w.questWindow(t, flauron); !shows(got, "32010-01.htm") {
		t.Fatalf("Flauron's window = %q, want 32010-01.htm", got)
	}
	got := w.bypass(t, "Quest "+q636Name+" 32010-02.htm")
	if want := []string{
		itemMessage(earnedItemS1, visitorMark),
		"S PlaySound type=0 file=ItemSound.quest_finish bind=0 obj=0 loc=0,0,0 delay=0",
	}; len(got) < 2 || !slices.Equal(got[:2], want) || !shows(got, "32010-02.htm") {
		t.Fatalf("Flauron's link = %q, want it to start %q and show 32010-02.htm", got, want)
	}
	if vars := w.srv.QuestVars(t, w.player, q636Name); vars["<state>"] != "COMPLETED" {
		t.Fatalf("Q636 after Flauron's link = %v, want completed", vars)
	}
	if got := w.questWindow(t, eliyah); !shows(got, "This quest has already been completed.") {
		t.Fatalf("Eliyah's window after completion = %q, want the completed page", got)
	}
	if n := w.srv.PlayerItemCount(t, w.player, visitorMark); n != 1 {
		t.Fatalf("Visitor's Marks = %d, want 1", n)
	}

	// Into the gate: the mark fades, with the picked-up line alone.
	lines := scriptLines(t, w.walkTo(t, location.Location{X: 300, Y: 20, Z: 30}))
	if want := []string{itemMessage(pickedUpS1, fadedMark)}; !slices.Equal(lines, want) {
		t.Fatalf("entering the gate sent %q, want %q", lines, want)
	}
	if v, f := w.srv.PlayerItemCount(t, w.player, visitorMark), w.srv.PlayerItemCount(t, w.player, fadedMark); v != 0 || f != 1 {
		t.Fatalf("marks after the gate = %d visitor, %d faded; want 0 and 1", v, f)
	}

	// Out and in again: no mark left to fade.
	w.walkTo(t, location.Location{X: 100, Y: 20, Z: 30})
	if lines := scriptLines(t, w.walkTo(t, location.Location{X: 300, Y: 20, Z: 30})); len(lines) != 0 {
		t.Fatalf("entering the gate again sent %q, want nothing", lines)
	}
	if f := w.srv.PlayerItemCount(t, w.player, fadedMark); f != 1 {
		t.Fatalf("faded marks after the second entry = %d, want 1", f)
	}
}

// TestQ636EliyahRefusesALowLevelPlayer: with no state, Eliyah's quest window
// creates the quest and answers 31329-01.htm below level 73.
func TestQ636EliyahRefusesALowLevelPlayer(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, 20, q636Pages(), nil, q636Fixture(t)...)
	eliyah := w.spawnFolk(t, "eliyah", eliyahID, 0, 30, 0).ObjectID()
	if got := w.questWindow(t, eliyah); !shows(got, "31329-01.htm") {
		t.Fatalf("Eliyah's window = %q, want 31329-01.htm", got)
	}
}
