package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// folkHealSkill is a heal a civilian NPC casts on one target, with a
	// hit long enough to stop it.
	folkHealSkill = modelskill.ID(9301)
	// folkCastHitTime is folkHealSkill's hit time.
	folkCastHitTime = 1500 * time.Millisecond
	// outweighsRoute is a cast desire weight heavier than a route walker's
	// wish to walk its route (1,000,000), which it outweighs.
	outweighsRoute = 2_000_000
)

// folkHealRef is folkHealSkill at level 1.
var folkHealRef = modelskill.Ref{ID: folkHealSkill, Level: 1}

// folkHealSkills is the skill data holding folkHealSkill at castRange.
func folkHealSkills(castRange int) *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{{
		ID: folkHealSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, CastRange: castRange, SkillType: "HEAL", Power: 1,
		HitTime: int(folkCastHitTime / time.Millisecond), StaticHitTime: true, StaticReuse: true,
	}})
}

// blindGeo is passable geodata through which no actor sees another.
type blindGeo struct{ gameservertest.Geo }

func (blindGeo) CanSeeActor(int, int, int, float64, int, int, int, float64) bool { return false }

// spawnCastingWalker spawns the fixture route walker at at, in walk
// stance, with the cast runtime over skills, walking a route over nodes.
func (w *folkWorld) spawnCastingWalker(t *testing.T, at location.Location, skills *modelskill.Table, nodes ...route.WalkerLocation) *npc.Folk {
	t.Helper()
	f, _ := w.srv.SpawnRouteFolkNPC(t, gameservertest.RouteFolkSpawn{
		Template: walkerTemplate(), At: at, Routes: walkerRoute(nodes...), WalkMode: true, Skills: skills,
	})
	return f
}

// queueCastDesire asks f to cast ref at target with weight, the way a
// script does, and waits for f's queue to take it.
func queueCastDesire(t *testing.T, f *npc.Folk, target attackable.Combatant, ref modelskill.Ref, weight float64) {
	t.Helper()
	f.AddCastDesire(target, ref, weight)
	done := make(chan struct{})
	if !f.Queue().Post(func() { close(done) }) {
		t.Fatal("post to folk queue: queue closed")
	}
	<-done
}

// aboutFolk keeps the frames about f, in order.
func aboutFolk(frames [][]byte, f *npc.Folk) []npcFrame {
	var kept []npcFrame
	for _, frame := range frames {
		if len(frame) >= 5 && objectFrame(frame, frame[0], f.ObjectID()) {
			kept = append(kept, npcFrame{opcode: frame[0], raw: frame})
		}
	}
	return kept
}

// standingOn reports whether f stands on at.
func standingOn(f *npc.Folk, at location.Location) func() bool {
	return func() bool { return !f.IsMoving() && folkAt(f) == at }
}

// TestRouteWalkerFolkClosesInToCastThenResumesItsRoute pins
// CreatureAI.thinkCast for a movable civilian NPC whose cast target stands
// out of the skill's reach: it switches to its run stance (ChangeMoveType,
// then its info again) before it walks toward the target
// (CreatureMove.maybeStartOffensiveFollow), casts once in reach on a later
// tick, and once the cast is over goes back to its route from the node
// nearest to where it stands (NpcAI.moveToNextPoint off the route).
func TestRouteWalkerFolkClosesInToCastThenResumesItsRoute(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	a := location.Location{X: w.at.X + 400, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 400, Y: w.at.Y + 300, Z: w.at.Z}
	f := w.spawnCastingWalker(t, a, folkHealSkills(100),
		route.WalkerLocation{Location: a, DelayMillis: int(nodeDelay / time.Millisecond)}, route.WalkerLocation{Location: b})
	w.srv.AdvanceUntil(t, "walker standing on its delayed node", standingOn(f, a))
	drainUntilQuiet(t, w.c)

	queueCastDesire(t, f, w.onlineCharacter(t), folkHealRef, outweighsRoute)
	frames := aboutFolk(w.tickAI(t), f)
	want := []byte{serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeNPCInfo, serverpackets.OpcodeMoveToLocation}
	if got := npcOpcodes(frames); len(got) < len(want) || string(got[:len(want)]) != string(want) {
		t.Fatalf("frames about the walker on the AI tick = %#x, want %#x first", got, want)
	}
	if dest, origin := moveLeg(frames[2]); dest != w.at || origin != a {
		t.Fatalf("walk toward the cast target = %v from %v, want %v from %v", dest, origin, w.at, a)
	}
	for _, fr := range frames {
		if fr.opcode == serverpackets.OpcodeMagicSkillUse {
			t.Fatalf("walker cast out of reach (frames %#x)", npcOpcodes(frames))
		}
	}
	if !f.Running() {
		t.Fatal("walker closing in on its target in walk stance, want run")
	}

	w.srv.AdvanceUntil(t, "walker in reach of its target", func() bool { return !f.IsMoving() })
	drainUntilQuiet(t, w.c)
	stoppedAt := folkAt(f)
	if d := stoppedAt.Distance2D(w.at); d >= 100+8+8 {
		t.Fatalf("walker stopped %v away from its target, want within the cast's reach", d)
	}

	frames = aboutFolk(w.tickAI(t), f)
	if len(framesOf(frames, serverpackets.OpcodeMagicSkillUse)) != 1 {
		t.Fatalf("frames about the walker in reach = %#x, want its MagicSkillUse", npcOpcodes(frames))
	}

	w.srv.Advance(t, folkCastHitTime)
	frames = readNpcFramesUntil(t, w.c, f, "walk back onto the route", func(fr npcFrame) bool {
		return fr.opcode == serverpackets.OpcodeMoveToLocation
	})
	if dest, origin := moveLeg(frames[len(frames)-1]); dest != a || origin != stoppedAt {
		t.Fatalf("walk after the cast = %v from %v, want the nearest node %v from %v", dest, origin, a, stoppedAt)
	}
}

// TestStandingFolkWaitsForCastTargetOutOfReach pins the other half of
// CreatureMove.maybeStartOffensiveFollow: a civilian NPC that cannot walk
// neither casts at a target out of reach nor moves; it keeps the desire and
// waits.
func TestStandingFolkWaitsForCastTargetOutOfReach(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	at := location.Location{X: w.at.X + 400, Y: w.at.Y, Z: w.at.Z}
	f := w.srv.SpawnCastingFolkNPCAt(t, folkTemplate("Folk", 30100), at, folkHealSkills(100))
	drainUntilQuiet(t, w.c)

	queueCastDesire(t, f, w.onlineCharacter(t), folkHealRef, outweighsRoute)
	for range 2 {
		if frames := aboutFolk(w.tickAI(t), f); len(frames) != 0 {
			t.Fatalf("frames about the standing NPC = %#x, want none", npcOpcodes(frames))
		}
	}
	if folkAt(f) != at || f.IsMoving() {
		t.Fatalf("standing NPC at %v moving=%v, want still on %v", folkAt(f), f.IsMoving(), at)
	}
}

// TestRouteWalkerFolkFollowsCastTargetOutOfSight pins
// CreatureMove.maybeStartOffensiveFollow's sight branch: a movable civilian
// NPC with its target in reach but out of sight walks toward the target
// instead of casting through the obstacle. Off its route meanwhile, the end
// of its node delay does not walk it on (NpcAI.moveToNextPoint off a
// MOVE_ROUTE intention).
func TestRouteWalkerFolkFollowsCastTargetOutOfSight(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	a := location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 60, Y: w.at.Y + 300, Z: w.at.Z}
	f, walker := w.srv.SpawnRouteFolkNPC(t, gameservertest.RouteFolkSpawn{
		Template: walkerTemplate(), At: a, WalkMode: true, Skills: folkHealSkills(600),
		Routes: walkerRoute(route.WalkerLocation{Location: a, DelayMillis: int(nodeDelay / time.Millisecond)}, route.WalkerLocation{Location: b}),
		Geo:    blindGeo{},
	})
	w.srv.AdvanceUntil(t, "walker standing on its delayed node", standingOn(f, a))
	drainUntilQuiet(t, w.c)

	queueCastDesire(t, f, w.onlineCharacter(t), folkHealRef, outweighsRoute)
	frames := aboutFolk(w.tickAI(t), f)
	if len(framesOf(frames, serverpackets.OpcodeMagicSkillUse)) != 0 {
		t.Fatalf("walker cast at a target out of sight (frames %#x)", npcOpcodes(frames))
	}
	moves := framesOf(frames, serverpackets.OpcodeMoveToLocation)
	if len(moves) != 1 {
		t.Fatalf("frames about the walker = %#x, want one walk toward its target", npcOpcodes(frames))
	}
	if dest, origin := moveLeg(moves[0]); dest != w.at || origin != a {
		t.Fatalf("walk = %v from %v, want toward the target %v from %v", dest, origin, w.at, a)
	}

	// Off its route, the node delay running out walks it nowhere.
	w.srv.AdvanceUntil(t, "walker done walking toward its target", func() bool { return !f.IsMoving() })
	w.srv.Advance(t, nodeDelay)
	walker.Tick()
	if dests, _ := folkMoves(drainFrames(t, w.c), f); len(dests) != 0 {
		t.Fatalf("walker off its route walked to %v when its node delay ran out, want nowhere", dests)
	}
}

// TestRouteWalkerFolkCastStopResumesFromNearestNode pins a route walker a
// cast stops (hit time over 50 ms, CreatureAI.thinkCast): it stops in place
// and casts, switches to its run stance on the third AI tick, and once the
// cast is over walks to the route node nearest to where it stopped, even
// the one behind it, then on along the route.
func TestRouteWalkerFolkCastStopResumesFromNearestNode(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	a := location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 60, Y: w.at.Y + 1000, Z: w.at.Z}
	f := w.spawnCastingWalker(t, a, folkHealSkills(600),
		route.WalkerLocation{Location: a}, route.WalkerLocation{Location: b})
	for range 100 {
		if dest, moving := f.MovingTo(); moving && dest == b && folkAt(f).Y > a.Y+100 {
			break
		}
		w.srv.TickPositions()
	}
	if folkAt(f).Y <= a.Y+100 {
		t.Fatalf("walker at %v, want well on its way to %v", folkAt(f), b)
	}
	drainUntilQuiet(t, w.c)

	queueCastDesire(t, f, w.onlineCharacter(t), folkHealRef, outweighsRoute)
	frames := aboutFolk(w.tickAI(t), f)
	if got := npcOpcodes(frames); len(got) < 2 || got[0] != serverpackets.OpcodeStopMove || len(framesOf(frames, serverpackets.OpcodeMagicSkillUse)) != 1 {
		t.Fatalf("frames about the walker = %#x, want StopMove then its MagicSkillUse", got)
	}
	stoppedAt := folkAt(f)
	if f.IsMoving() {
		t.Fatal("walker still walking while it casts")
	}
	// Every third AI tick an NPC acting on a cast desire runs
	// (NpcAI.runAI's update tick), its cast still in flight.
	if f.Running() {
		t.Fatal("walker running on its first AI tick, want walk stance until the third")
	}
	w.tickAI(t)
	frames = aboutFolk(w.tickAI(t), f)
	if got := npcOpcodes(frames); string(got) != string([]byte{serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeNPCInfo}) || !f.Running() {
		t.Fatalf("frames about the walker on the third AI tick = %#x running=%v, want ChangeMoveType then NpcInfo, running", got, f.Running())
	}

	w.srv.Advance(t, folkCastHitTime)
	frames = readNpcFramesUntil(t, w.c, f, "walk back onto the route", func(fr npcFrame) bool {
		return fr.opcode == serverpackets.OpcodeMoveToLocation
	})
	if dest, origin := moveLeg(frames[len(frames)-1]); dest != a || origin != stoppedAt {
		t.Fatalf("walk after the cast = %v from %v, want the nearest node %v from %v", dest, origin, a, stoppedAt)
	}
	readNpcFramesUntil(t, w.c, f, "walk on to the far node", func(fr npcFrame) bool { return isMoveTo(fr, b) })
}

// TestRouteWalkerFolkKeepsWalkingForLighterCastDesire pins the desire pick
// (NpcAI.runAI): a cast desire no heavier than the route walk's 1,000,000
// leaves the walker on its route.
func TestRouteWalkerFolkKeepsWalkingForLighterCastDesire(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	a := location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 60, Y: w.at.Y + 1000, Z: w.at.Z}
	f := w.spawnCastingWalker(t, a, folkHealSkills(600),
		route.WalkerLocation{Location: a}, route.WalkerLocation{Location: b})
	w.srv.AdvanceUntil(t, "walker on its way to the far node", func() bool {
		dest, moving := f.MovingTo()
		return moving && dest == b
	})
	drainUntilQuiet(t, w.c)

	queueCastDesire(t, f, w.onlineCharacter(t), folkHealRef, 1_000_000)
	if frames := aboutFolk(w.tickAI(t), f); len(frames) != 0 {
		t.Fatalf("frames about the walker = %#x, want none: the route outweighs the cast", npcOpcodes(frames))
	}
	if dest, moving := f.MovingTo(); !moving || dest != b {
		t.Fatalf("walker heading %v moving=%v, want still walking to %v", dest, moving, b)
	}
}

// framesOf keeps the frames of opcode.
func framesOf(frames []npcFrame, opcode byte) []npcFrame {
	var kept []npcFrame
	for _, fr := range frames {
		if fr.opcode == opcode {
			kept = append(kept, fr)
		}
	}
	return kept
}
