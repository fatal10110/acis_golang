package npcs

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The NPC-side operations of the finishing lane of the script-facing API
// freeze (#3748), driven through a booted world as a hook calls them.

// quakePayload is the Earthquake payload, opcode first: the point, the
// intensity, the duration and whether an NPC shakes it.
func quakePayload(at location.Location, intensity, duration int32, npc bool) []byte {
	out := []byte{serverpackets.OpcodeEarthquake}
	flag := int32(0)
	if npc {
		flag = 1
	}
	for _, v := range []int32{int32(at.X), int32(at.Y), int32(at.Z), intensity, duration, flag} {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// TestNPCEarthquakeShakesKnownPlayers pins npc.broadcastPacket(new
// Earthquake(npc, intensity, duration, true)) (Antharas, Baium): every
// player that knows the hostile NPC gets one Earthquake centred where the
// NPC stands, marked as an NPC's, each its own copy; an NPC no player knows
// shakes no one. A civilian NPC has no earthquake.
func TestNPCEarthquakeShakesKnownPlayers(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	near := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Antharas"), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	far := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Distant"), location.Location{X: w.at.X + 40_000, Y: w.at.Y, Z: w.at.Z})
	folk := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30868), location.Location{X: w.at.X, Y: w.at.Y + 40, Z: w.at.Z})
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)

	w.direct(t, func(_ *script.Player) {
		script.NPCOf(near).Earthquake(20, 10)
		script.NPCOf(far).Earthquake(40, 10)
	})
	x, y, z := near.Position()
	want := quakePayload(location.Location{X: x, Y: y, Z: z}, 20, 10, true)
	quakes := map[string][]byte{}
	for name, c := range map[string]*testsupport.ScriptedClient{"first": w.first, "second": w.second} {
		got := pick(w.srv.ReadQueued(t, c), serverpackets.OpcodeEarthquake)
		if len(got) != 1 || !bytes.Equal(got[0], want) {
			t.Fatalf("%s: Earthquake frames % x, want one % x", name, got, want)
		}
		quakes[name] = got[0]
	}
	quakes["first"][len(quakes["first"])-1] ^= 0xff
	if !bytes.Equal(quakes["second"], want) {
		t.Fatalf("second's Earthquake changed with first's: % x", quakes["second"])
	}

	if w.srv.RunScript(t, w.firstID, "Director", func(_ *script.Script, _ *script.Player) {
		script.NPCOf(folk).Earthquake(20, 10)
	}) {
		t.Fatal("a civilian NPC's earthquake did not abort the invocation")
	}
}

// TestScriptInRangeMeasuresBodies pins MathUtil.checkIfInRange(range, a, b,
// includeZ): the distance between the two positions against the range plus
// both collision radii, on the ground plane or in 3D; -1 is unlimited.
func TestScriptInRangeMeasuresBodies(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	mob := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Measured"), location.Location{X: w.at.X + 300, Y: w.at.Y + 400, Z: w.at.Z + 200})
	obj, ok := w.srv.State.Player(w.firstID)
	if !ok {
		t.Fatal("the first player is not in the world")
	}
	body, ok := obj.(interface{ CollisionRadius() float64 })
	if !ok {
		t.Fatalf("player %T has no collision radius", obj)
	}
	px, py, pz := w.srv.PlayerPosition(t, w.firstID)
	mx, my, mz := mob.Position()
	if mz == pz {
		t.Fatalf("fixture: the monster stands at the player's height %d", mz)
	}
	radii := body.CollisionRadius() + mob.CollisionRadius()
	dx, dy, dz := float64(mx-px), float64(my-py), float64(mz-pz)
	least := func(d float64) int { return int(math.Ceil(d - radii)) }
	flat, full := least(math.Hypot(dx, dy)), least(math.Sqrt(dx*dx+dy*dy+dz*dz))

	w.direct(t, func(p *script.Player) {
		n := script.NPCOf(mob)
		for _, tc := range []struct {
			r        int
			includeZ bool
			want     bool
		}{
			{flat, false, true},
			{flat - 1, false, false},
			{full, true, true},
			{full - 1, true, false},
			{flat, true, false},
			{-1, true, true},
		} {
			if got := script.InRange(tc.r, p, n, tc.includeZ); got != tc.want {
				t.Errorf("InRange(%d, includeZ %v) = %v, want %v", tc.r, tc.includeZ, got, tc.want)
			}
		}
		if script.InRange(-1, p, nil, true) {
			t.Error("InRange of nothing = true")
		}
	})
}

// TestScriptRemoveMoveToDesire pins WarriorRunAwayToClan's
// getDesireQueue().removeIf(MOVE_TO at the flee point): the move desire to
// that point goes, a move desire elsewhere stays.
func TestScriptRemoveMoveToDesire(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	mob := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Runner"), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	flee := script.Loc{X: w.at.X + 9000, Y: w.at.Y, Z: w.at.Z}
	other := script.Loc{X: w.at.X - 9000, Y: w.at.Y, Z: w.at.Z}
	movesTo := func(l script.Loc) bool {
		return mob.AI().Desires().AnyMatch(func(d *ai.Desire) bool {
			return d.Kind == ai.IntentionMoveTo && d.Location == location.Location{X: l.X, Y: l.Y, Z: l.Z}
		})
	}
	w.direct(t, func(_ *script.Player) {
		n := script.NPCOf(mob)
		n.AddMoveToDesire(flee, 50)
		n.AddMoveToDesire(other, 10)
		if !movesTo(flee) || !movesTo(other) {
			t.Fatalf("fixture: move desires flee %v, other %v", movesTo(flee), movesTo(other))
		}
		n.RemoveMoveToDesire(flee)
		if movesTo(flee) || !movesTo(other) {
			t.Errorf("after the removal: flee desire %v, other %v; want only the other", movesTo(flee), movesTo(other))
		}
	})
}

// TestScriptShowMultisell pins MultisellData.separateAndSend(list, player,
// npc, false) as Q628 calls it: the list's pages as the merchant prepares
// them, the same as its own multisell link shows, after which the player
// exchanges from it; a list the NPC may not offer shows nothing, and a
// hostile NPC offers none.
func TestScriptShowMultisell(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 1000}}, nil,
		gameservertest.WithScripts([]script.Listing{{Path: "quest.Director"}}, script.Catalog{
			"quest.Director": func() script.Script { return script.Script{} },
		}))
	folk := w.spawnFolk(t, folkTemplate("Folk", msFolkID), 60)
	mob := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Mob"), location.Location{X: w.at.X - 60, Y: w.at.Y, Z: w.at.Z})
	drainFrames(t, w.c)
	direct := func(fn func(p *script.Player)) bool {
		return w.srv.RunScript(t, w.player, "Director", func(_ *script.Script, p *script.Player) { fn(p) })
	}

	want := pick(w.open(t, w.merchant, "multisell 9001"), serverpackets.OpcodeMultiSellList)
	if len(want) != 1 {
		t.Fatalf("fixture: the link showed %d pages", len(want))
	}
	if !direct(func(p *script.Player) { p.ShowMultisell(script.NPCOf(w.merchant), "9001") }) {
		t.Fatal("ShowMultisell panicked")
	}
	got := drainFrames(t, w.c)
	if len(got) != 1 || !bytes.Equal(got[0], want[0]) {
		t.Fatalf("script multisell frames % x, want the link's page % x alone", got, want[0])
	}
	assertMessages(t, w.choose(t, "9001", 2, 3),
		msg(serverpackets.SystemMessageS2S1Disappeared, item.AdenaID, 30),
		msg(serverpackets.SystemMessageEarnedS2S1S, msPotionID, 15),
		traded)

	if !direct(func(p *script.Player) { p.ShowMultisell(script.NPCOf(folk), "9001") }) {
		t.Fatal("ShowMultisell at a folk panicked")
	}
	if got := drainFrames(t, w.c); len(got) != 0 {
		t.Fatalf("a list the folk may not offer sent % x", got)
	}
	if direct(func(p *script.Player) { p.ShowMultisell(script.NPCOf(mob), "9002") }) {
		t.Fatal("a hostile NPC's multisell did not abort the invocation")
	}
}
