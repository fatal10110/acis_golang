package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q224"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Test of Sagittarius's serpent demon Kadesh (Q224_TestOfSagittarius
// onMyDying): at cond 13 a Leto lizardman's death past 120 blood may call
// Kadesh on the player for 300 s, taking the blood, and Kadesh slain
// without the Crescent Moon bow in hand comes back. A scenario cannot show
// a script-spawned NPC appearing or leaving (#4020 item 5), so these Boot
// tests pin it; the scenarios play the blood drops and the bow's kill.
const (
	q224Name          = "Q224_TestOfSagittarius"
	q224Path          = "quest.Q224_TestOfSagittarius"
	q224LetoLizardman = 20577
	q224Kadesh        = 27090
	q224Blood         = 3306
	q224Talisman      = 3300
)

// q224World boots a level 39 character whose Test of Sagittarius is at
// cond 13, holding blood Leto lizardman's blood (0 for none). The draw of
// [0, 100) takes roll.
func q224World(t *testing.T, blood int32, roll int) *dialogWorld {
	t.Helper()
	w := bootSeamQuest(t, 39, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q224Name, "<state>", val("STARTED")},
			journalRow{objID, q224Name, "<cond>", val("13")})
		if blood != 0 {
			srv.GiveItem(t, objID, q224Blood, blood)
		}
	},
		x1Items([]int32{q224Blood, q224Talisman}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q224LetoLizardman), proofMonster(q224Kadesh)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q224LetoLizardman: script.KindHostile, q224Kadesh: script.KindHostile},
			[]script.Listing{{Path: q224Path}},
			script.Catalog{q224Path: q224.New},
		),
		gameservertest.WithScriptRand(func(n int) int {
			if n == 100 {
				return roll
			}
			return 0
		}),
	)
	w.roles[w.player] = "player"
	return w
}

// wantKadeshOnThePlayer fails unless exactly one Kadesh other than the
// object slain (and its respawn) stands, where the player does, with no
// offset.
func wantKadeshOnThePlayer(t *testing.T, w *dialogWorld, slain int32) {
	t.Helper()
	var kadesh []script.Loc
	for _, n := range npcsOf(w.srv, q224Kadesh) {
		if loc := n.SpawnLocation(); n.ObjectID() != slain && loc.X != w.at.X+40 {
			kadesh = append(kadesh, loc)
		}
	}
	if len(kadesh) != 1 {
		t.Fatalf("Kadesh called at %+v, want one", kadesh)
	}
	if loc := kadesh[0]; loc.X != w.at.X || loc.Y != w.at.Y || abs(loc.Z-w.at.Z) > 32 {
		t.Fatalf("Kadesh stands at %+v, want the player's %+v", loc, w.at)
	}
}

// TestQ224LizardmanBloodCallsKadesh: a Leto lizardman's death calls Kadesh
// when 5 times the blood held past 120 is over the draw of [0, 100): the
// before-battle sound, every blood taken, Kadesh on the player, gone 300 s
// later. Otherwise the death gives one more blood.
func TestQ224LizardmanBloodCallsKadesh(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		blood     int32
		roll      int
		wantCall  bool
		wantBlood int32
	}{
		{"121 blood, draw 4", 121, 4, true, 0},
		{"121 blood, draw 5", 121, 5, false, 122},
		{"120 blood, draw 0", 120, 0, false, 121},
		{"no blood, draw 0", 0, 0, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q224World(t, tc.blood, tc.roll)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q224LetoLizardman), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			killOnPlayerQueue(t, w.srv, w.player, h)
			sent := w.render(t, w.srv.ReadQueued(t, w.srv.Client))
			if n := w.srv.PlayerItemCount(t, w.player, q224Blood); n != int(tc.wantBlood) {
				t.Fatalf("blood %d after the kill, want %d", n, tc.wantBlood)
			}
			if !tc.wantCall {
				if n := len(npcsOf(w.srv, q224Kadesh)); n != 0 || slices.Contains(sent, q217BeforeBattle) {
					t.Fatalf("%d Kadesh, sent %q; want none and no sound", n, sent)
				}
				return
			}
			if !slices.Contains(sent, q217BeforeBattle) {
				t.Fatalf("sent %q, want the before-battle sound", sent)
			}
			wantKadeshOnThePlayer(t, w, 0)
			// Kadesh comes with the dying hook, inside the kill's 3.5 s.
			w.advance(t, 299*time.Second-3500*time.Millisecond)
			if n := len(npcsOf(w.srv, q224Kadesh)); n != 1 {
				t.Fatalf("%d Kadesh 299 s after the kill, want 1", n)
			}
			w.advance(t, 5*time.Second)
			if n := len(npcsOf(w.srv, q224Kadesh)); n != 0 {
				t.Fatalf("%d Kadesh after the despawn, want 0", n)
			}
		})
	}
}

// TestQ224KadeshWithoutTheBowComesBack: Kadesh slain by a player with no
// Crescent Moon bow in hand comes back on the player, and the test stays at
// cond 13 without the talisman.
func TestQ224KadeshWithoutTheBowComesBack(t *testing.T) {
	t.Parallel()
	w := q224World(t, 0, 0)
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q224Kadesh), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	killOnPlayerQueue(t, w.srv, w.player, h)
	wantKadeshOnThePlayer(t, w, h.ObjectID())
	if rows := questRows(t, w.srv, w.player, q224Name); rows["<cond>"] != "13" {
		t.Fatalf("rows %v, want cond 13", rows)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q224Talisman); n != 0 {
		t.Fatalf("talismans %d, want 0", n)
	}
}
