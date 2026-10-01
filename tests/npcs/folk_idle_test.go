package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// moveTypeRunning decodes a ChangeMoveType frame's run flag.
func moveTypeRunning(fr npcFrame) bool { return fr.body().ReadInt32() == 1 }

// npcInfoRunning decodes an NpcInfo frame's run flag.
func npcInfoRunning(fr npcFrame) bool {
	r := fr.body()
	for range 2 + 3 + 1 + 1 + 2 + 8 { // template id .. walk speeds
		r.ReadInt32()
	}
	for range 4 { // move and attack speed multipliers, collision radius and height
		r.ReadFloat64()
	}
	for range 3 { // right hand, chest, left hand
		r.ReadInt32()
	}
	r.ReadUint8()
	return r.ReadUint8() == 1
}

// assertWalkStanceShown fails unless frames are exactly ChangeMoveType to
// walk, then NpcInfo with the run flag 0, and f walks.
func assertWalkStanceShown(t *testing.T, what string, frames []npcFrame, f *npc.Folk) {
	t.Helper()
	want := []byte{serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeNPCInfo}
	if got := npcOpcodes(frames); string(got) != string(want) {
		t.Fatalf("%s: frames about the NPC = %#x, want ChangeMoveType then NpcInfo", what, got)
	}
	if moveTypeRunning(frames[0]) || npcInfoRunning(frames[1]) || f.Running() {
		t.Fatalf("%s: ChangeMoveType running %v, NpcInfo running %v, NPC running %v; want walk throughout",
			what, moveTypeRunning(frames[0]), npcInfoRunning(frames[1]), f.Running())
	}
}

// TestStandingFolkIdlesToWalkStance pins NpcAI.runAI's update tick for a
// civilian NPC with nothing to do: spawned running (Npc.onSpawn), it does
// nothing on its first AI tick, switches to its walk stance on the next
// (thinkIdle's forceWalkStance: ChangeMoveType, then Npc.setWalkOrRun's
// NpcInfo with the run flag 0), and stays walking after.
func TestStandingFolkIdlesToWalkStance(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	f := w.spawnFolk(t, folkTemplate("Folk", 30100), 80)
	if !f.Running() {
		t.Fatal("civilian NPC spawned walking, want run stance")
	}

	if frames := aboutFolk(w.tickAI(t), f); len(frames) != 0 {
		t.Fatalf("frames about the NPC on its first AI tick = %#x, want none", npcOpcodes(frames))
	}
	assertWalkStanceShown(t, "second AI tick", aboutFolk(w.tickAI(t), f), f)
	for i := range 3 {
		if frames := aboutFolk(w.tickAI(t), f); len(frames) != 0 {
			t.Fatalf("frames about the idle NPC on tick %d after = %#x, want none", i+1, npcOpcodes(frames))
		}
	}
}

// TestHitFolkWalksAgainOnNextIdleTick pins thinkIdle after a hit forced the
// run stance (Npc.reduceCurrentHp's setWalkOrRun(true)): the next AI tick
// finds nothing to do and switches the NPC back to its walk stance.
func TestHitFolkWalksAgainOnNextIdleTick(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	f := w.spawnFolk(t, folkTemplate("Folk", 30100), 80)
	w.settleAI(t)
	if f.Running() {
		t.Fatal("settled NPC running, want walk")
	}

	f.TakeDamage(10, w.onlineCharacter(t))
	drainUntilQuiet(t, w.c)
	if !f.Running() {
		t.Fatal("hit NPC walking, want run")
	}
	assertWalkStanceShown(t, "idle tick after the hit", aboutFolk(w.tickAI(t), f), f)
}

// TestRouteWalkerFolkNeverIdles pins a route walker's MOVE_ROUTE desire,
// which never leaves its queue (Walkers): its AI never idles it, so one
// that walks its route running keeps running.
func TestRouteWalkerFolkNeverIdles(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	a := location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 1000, Y: w.at.Y, Z: w.at.Z}
	f, _ := w.srv.SpawnRouteFolkNPCAt(t, walkerTemplate(), a, walkerRoutes(a, b), false)
	drainUntilQuiet(t, w.c)

	for i := range 5 {
		for _, fr := range aboutFolk(w.tickAI(t), f) {
			if fr.opcode == serverpackets.OpcodeChangeMoveType {
				t.Fatalf("AI tick %d showed the route walker's stance change", i+1)
			}
		}
	}
	if !f.Running() {
		t.Fatal("route walker idled to its walk stance, want it still running")
	}
}

// movableFolkTemplate is the fixture civilian template left movable, as
// the shipped templates are unless they set canMove="false"
// (NpcTemplate's canMove default).
func movableFolkTemplate() *npc.Template {
	tmpl := folkTemplate("Folk", 30100)
	tmpl.CanMove = true
	return tmpl
}

// TestMovableFolkClosesInToCastThenIdles pins Npc.isMovementDisabled for a
// civilian NPC with no route: only its template's canMove keeps it in
// place. Its cast target out of reach, it switches to its run stance and
// walks toward the target (CreatureMove.maybeStartOffensiveFollow), casts
// once in reach, and once the cast is over and nothing is left to do goes
// back to its walk stance at once (NpcAI.onEvtFinishedCasting's runAI,
// doIdleIntention).
func TestMovableFolkClosesInToCastThenIdles(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	at := location.Location{X: w.at.X + 400, Y: w.at.Y, Z: w.at.Z}
	f := w.srv.SpawnCastingFolkNPCAt(t, movableFolkTemplate(), at, folkHealSkills(100))
	w.settleAI(t)
	drainUntilQuiet(t, w.c)

	queueCastDesire(t, f, w.onlineCharacter(t), folkHealRef, outweighsRoute)
	frames := aboutFolk(w.tickAI(t), f)
	want := []byte{serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeNPCInfo, serverpackets.OpcodeMoveToLocation}
	if got := npcOpcodes(frames); string(got) != string(want) {
		t.Fatalf("frames about the NPC on the AI tick = %#x, want %#x", got, want)
	}
	if !moveTypeRunning(frames[0]) || !npcInfoRunning(frames[1]) {
		t.Fatal("NPC closing in on its target in walk stance, want run")
	}
	if dest, origin := moveLeg(frames[2]); dest != w.at || origin != at {
		t.Fatalf("walk toward the cast target = %v from %v, want %v from %v", dest, origin, w.at, at)
	}

	w.srv.AdvanceUntil(t, "NPC in reach of its target", func() bool { return !f.IsMoving() })
	drainUntilQuiet(t, w.c)
	if d := folkAt(f).Distance2D(w.at); d >= 100+8+8 {
		t.Fatalf("NPC stopped %v away from its target, want within the cast's reach", d)
	}
	frames = aboutFolk(w.tickAI(t), f)
	if len(framesOf(frames, serverpackets.OpcodeMagicSkillUse)) != 1 {
		t.Fatalf("frames about the NPC in reach = %#x, want its MagicSkillUse", npcOpcodes(frames))
	}

	w.srv.Advance(t, folkCastHitTime)
	frames = readNpcFramesUntil(t, w.c, f, "walk stance after the cast", func(fr npcFrame) bool {
		return fr.opcode == serverpackets.OpcodeNPCInfo
	})
	idle := frames[len(frames)-2:]
	assertWalkStanceShown(t, "cast end", idle, f)
}

// TestMovableFolkFollowsCastTargetOutOfSight pins
// CreatureMove.maybeStartOffensiveFollow's sight branch for a civilian NPC
// with no route: with its target in reach but out of sight it walks toward
// the target instead of casting through the obstacle.
func TestMovableFolkFollowsCastTargetOutOfSight(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	at := location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z}
	// No route data: the spawner's walker finds no route for the NPC.
	f, _ := w.srv.SpawnRouteFolkNPC(t, gameservertest.RouteFolkSpawn{
		Template: movableFolkTemplate(), At: at, Skills: folkHealSkills(600), Geo: blindGeo{},
	})
	w.settleAI(t)
	drainUntilQuiet(t, w.c)

	queueCastDesire(t, f, w.onlineCharacter(t), folkHealRef, outweighsRoute)
	frames := aboutFolk(w.tickAI(t), f)
	if len(framesOf(frames, serverpackets.OpcodeMagicSkillUse)) != 0 || len(framesOf(frames, serverpackets.OpcodeMoveToPawn)) != 0 {
		t.Fatalf("NPC cast or turned at a target out of sight (frames %#x)", npcOpcodes(frames))
	}
	moves := framesOf(frames, serverpackets.OpcodeMoveToLocation)
	if len(moves) != 1 {
		t.Fatalf("frames about the NPC = %#x, want one walk toward its target", npcOpcodes(frames))
	}
	if dest, origin := moveLeg(moves[0]); dest != w.at || origin != at {
		t.Fatalf("walk = %v from %v, want toward the target %v from %v", dest, origin, w.at, at)
	}
}
