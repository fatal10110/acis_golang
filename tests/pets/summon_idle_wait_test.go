package pets

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// spawnTankyHostile seeds the fixture monster with more HP than any test
// here can take off, so the pet never runs out of target.
func spawnTankyHostile(t *testing.T, h *petWorld) *npc.Hostile {
	t.Helper()
	return h.srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1e9,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
}

// sendPetMidSwing has the owner select hostile, commands the pet to attack
// it and advances until the pet's first swing is in flight.
func sendPetMidSwing(t *testing.T, h *petWorld, petActor *summon.Actor, hostile *npc.Hostile) {
	t.Helper()
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUse(petAttackAction, false))
	h.srv.AdvanceUntil(t, "the pet's first swing", petActor.IsAttackingNow)
	drainFrames(t, h.client)
}

// petAttacks counts the Attack broadcasts petActor makes in frames.
func petAttacks(frames [][]byte, petActor *summon.Actor) int {
	n := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeAttack && int32(binary.LittleEndian.Uint32(f[1:5])) == petActor.ObjectID() {
			n++
		}
	}
	return n
}

// advanceCollecting advances the clock by d in AI-sized steps, collecting
// the owner's frames along the way.
func advanceCollecting(t *testing.T, h *petWorld, d time.Duration) [][]byte {
	t.Helper()
	var frames [][]byte
	for passed := time.Duration(0); passed < d; passed += 100 * time.Millisecond {
		h.srv.Advance(t, 100*time.Millisecond)
		frames = append(frames, drainFrames(t, h.client)...)
	}
	return frames
}

// TestPetStopMidSwingIsWaitedOut pins PlayableAI.tryToIdle for a pet whose
// swing is in flight: the Stop only replaces the queued intention with an
// idle one, which is the same as nothing queued. The pet does not walk off
// mid-swing, its owner is told nothing (a summon's clientActionFailed is a
// no-op), and once the swing ends the pet keeps attacking the monster, as
// onEvtFinishedAttack does with nothing queued and a target it can keep
// attacking.
func TestPetStopMidSwingIsWaitedOut(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	hostile := spawnTankyHostile(t, h)
	sendPetMidSwing(t, h, petActor, hostile)

	h.client.Send(encodeRequestActionUse(petStopAction, false))
	h.handled(t)
	if n := countOpcode(drainFrames(t, h.client), serverpackets.OpcodeActionFailed); n != 0 {
		t.Fatalf("ActionFailed frames for the waited-out Stop = %d, want 0", n)
	}
	if petActor.Move().Moving() {
		t.Fatal("pet started walking mid-swing, want the Stop waited out")
	}
	if got := petActor.Intent(); got != summon.IntentAttackTarget {
		t.Fatalf("pet intent after the waited-out Stop = %v, want attack-target kept", got)
	}

	frames := advanceCollecting(t, h, 3*time.Second)
	if n := petAttacks(frames, petActor); n < 2 {
		t.Fatalf("pet swings after the Stop = %d, want it to keep attacking the monster", n)
	}
}

// TestPetStopMidSwingDropsQueuedStrike pins the other half of the wait: a
// strike commanded mid-swing is queued, the Stop that follows replaces it,
// and the swing ending leaves the pet attacking instead of casting it.
func TestPetStopMidSwingDropsQueuedStrike(t *testing.T) {
	t.Parallel()
	h, petActor, _ := bootWolfStrikerWith(t, wolfStrike(), gameservertest.WithAITask())
	hostile := spawnTankyHostile(t, h)
	sendPetMidSwing(t, h, petActor, hostile)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "queued strike ActionFailed")
	h.client.Send(encodeRequestActionUse(petStopAction, false))
	h.handled(t)

	frames := advanceCollecting(t, h, 3*time.Second)
	if n := countOpcode(frames, serverpackets.OpcodeMagicSkillUse); n != 0 {
		t.Fatalf("MagicSkillUse frames after the Stop = %d, want the queued strike dropped", n)
	}
	if n := petAttacks(frames, petActor); n == 0 {
		t.Fatal("pet stopped swinging, want it to keep attacking the monster")
	}
}

// TestPetStrikeQueuedMidSwingRunsAfterTheSwing pins
// PlayableAI.onEvtFinishedAttack running the queued intention: a strike
// commanded mid-swing starts as soon as the swing ends, not only once the
// monster is dead.
func TestPetStrikeQueuedMidSwingRunsAfterTheSwing(t *testing.T) {
	t.Parallel()
	h, petActor, _ := bootWolfStrikerWith(t, wolfStrike(), gameservertest.WithAITask())
	hostile := spawnTankyHostile(t, h)
	sendPetMidSwing(t, h, petActor, hostile)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	queued := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "queued strike ActionFailed")
	if hasOpcode(queued, serverpackets.OpcodeMagicSkillUse) {
		t.Fatalf("strike commanded mid-swing started at once: opcodes %x", frameOpcodes(queued))
	}

	frames := advanceCollecting(t, h, 4*time.Second)
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		// SummonAI.onEvtFinishedCasting resumes the attack the strike
		// replaced.
		if petAttacks(frames[i+1:], petActor) == 0 {
			t.Fatalf("pet did not go back to attacking after the strike: opcodes %x", frameOpcodes(frames[i+1:]))
		}
		return
	}
	t.Fatalf("queued strike never started after the swing: opcodes %x", frameOpcodes(frames))
}

// TestPetStopMidCastFollowsOwnerAfterTheCast pins the cast side of the
// wait: a Stop pressed while the pet casts leaves the cast to finish, and
// the cast ending sends the pet idle, which for a pet following its owner
// means following it again (SummonAI.onEvtFinishedCasting, thinkIdle).
func TestPetStopMidCastFollowsOwnerAfterTheCast(t *testing.T) {
	t.Parallel()
	h, petActor, _ := bootWolfStrikerWith(t, wolfStrike(), gameservertest.WithAITask())
	hostile := spawnTankyHostile(t, h)
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	cast := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed")
	if !hasOpcode(cast, serverpackets.OpcodeMagicSkillUse) {
		t.Fatalf("strike did not start: opcodes %x", frameOpcodes(cast))
	}
	h.client.Send(encodeRequestActionUse(petStopAction, false))
	h.handled(t)
	if petActor.Move().Moving() {
		t.Fatal("pet started walking mid-cast, want the Stop waited out")
	}

	h.srv.Advance(t, wolfStrikeHitTime*time.Millisecond)
	drainFrames(t, h.client)
	h.ownerWalksAway(t)
	h.requirePetCatchesUp(t, petActor, "pet following its owner once the cast ended")
}
