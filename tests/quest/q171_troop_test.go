package quest

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q171"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Acts of Evil's Ol Mahum support troop (Q171_ActsOfEvil.onMyDying,
// onDecayed). A scenario cannot show a script-spawned NPC appearing or
// leaving (#4020 item 5), so these Boot tests pin it.
const (
	q171Name      = "Q171_ActsOfEvil"
	q171Path      = "quest.Q171_ActsOfEvil"
	q171Archer    = 20496
	q171Troop     = 27190
	q171BladeMold = 4239
)

// q171World boots a level 27 character whose Acts of Evil is at cond 2,
// holding molds blade molds. Every blade mold drop lands; the support
// troop's roll of [0, 100) draws what roll holds.
func q171World(t *testing.T, molds int32, roll *atomic.Int32) *dialogWorld {
	t.Helper()
	w := bootSeamQuest(t, 27, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q171Name, "<state>", val("STARTED")},
			journalRow{objID, q171Name, "<cond>", val("2")})
		srv.GiveItem(t, objID, q171BladeMold, molds)
	},
		x1Items([]int32{q171BladeMold}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q171Archer), proofMonster(q171Troop)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q171Archer: script.KindHostile, q171Troop: script.KindHostile},
			[]script.Listing{{Path: q171Path}},
			script.Catalog{q171Path: q171.New},
		),
		gameservertest.WithScriptRand(func(n int) int {
			if n == 100 {
				return int(roll.Load())
			}
			return 0
		}),
	)
	w.roles[w.player] = "player"
	return w
}

// killArcher kills a fresh Turek orc archer beside the player and returns
// where it fell and the support troops then in the world.
func (w *dialogWorld) killArcher(t *testing.T) (location.Location, []*script.NPC) {
	t.Helper()
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q171Archer), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	x, y, z := h.Position()
	killOnPlayerQueue(t, w.srv, w.player, h)
	return location.Location{X: x, Y: y, Z: z}, npcsOf(w.srv, q171Troop)
}

// wantTroopAt fails unless troops is one support troop standing where the
// archer fell and hating the player most.
func (w *dialogWorld) wantTroopAt(t *testing.T, step string, troops []*script.NPC, at location.Location) {
	t.Helper()
	if len(troops) != 1 {
		t.Fatalf("%s: %d support troops, want 1", step, len(troops))
	}
	troop := troops[0]
	if loc := troop.SpawnLocation(); loc.X != at.X || loc.Y != at.Y {
		t.Fatalf("%s: the troop stands at (%d, %d), want where the archer fell (%d, %d)", step, loc.X, loc.Y, at.X, at.Y)
	}
	if h := troop.MostHated(); h == nil || h.ObjectID() != w.player {
		t.Fatalf("%s: the troop hates %v most, want the player", step, h)
	}
}

// TestQ171FifthMoldCallsTheSupportTroop: the archer whose drop makes five
// blade molds calls a support troop where it fell, attacking the player,
// with no roll; it leaves 200 s later without a word (an NPC is dead by
// the time its decayed hook runs). The sixth mold calls none.
func TestQ171FifthMoldCallsTheSupportTroop(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	roll.Store(99)
	w := q171World(t, 4, &roll)

	at, troops := w.killArcher(t)
	w.wantTroopAt(t, "fifth mold", troops, at)
	if n := w.srv.PlayerItemCount(t, w.player, q171BladeMold); n != 5 {
		t.Fatalf("blade molds %d after the kill, want 5", n)
	}
	troop := troops[0]
	w.roles[troop.ObjectID()] = "troop"

	// The troop comes with the archer's dying hook, inside the kill's 3.5 s:
	// 199 s after the kill it is still there, 204 s after it is gone.
	saysNone(t, "199 s after the kill", w.advance(t, 199*time.Second-3500*time.Millisecond))
	if n := len(npcsOf(w.srv, q171Troop)); n != 1 {
		t.Fatalf("%d support troops 199 s after the kill, want 1", n)
	}
	saysNone(t, "the despawn", w.advance(t, 5*time.Second))
	if n := len(npcsOf(w.srv, q171Troop)); n != 0 {
		t.Fatalf("%d support troops after the despawn, want 0", n)
	}

	if _, troops := w.killArcher(t); len(troops) != 0 {
		t.Fatalf("sixth mold: %d support troops, want 0", len(troops))
	}
}

// TestQ171TenthMoldRollsForTheSupportTroop: from ten blade molds on, each
// mold calls a support troop on a roll under 25 of 100: 25 calls none, 24
// calls one.
func TestQ171TenthMoldRollsForTheSupportTroop(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	roll.Store(25)
	w := q171World(t, 9, &roll)

	if _, troops := w.killArcher(t); len(troops) != 0 {
		t.Fatalf("tenth mold, roll 25: %d support troops, want 0", len(troops))
	}
	roll.Store(24)
	at, troops := w.killArcher(t)
	w.wantTroopAt(t, "eleventh mold, roll 24", troops, at)
	if n := w.srv.PlayerItemCount(t, w.player, q171BladeMold); n != 11 {
		t.Fatalf("blade molds %d after the kills, want 11", n)
	}
}
