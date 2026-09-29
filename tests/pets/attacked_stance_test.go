package pets

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// avoidSpots are the offsets from the owner of the twelve spots an attacked
// summon steps aside to: Circle(owner, 70).getEquidistantPoints(12) run on a
// JVM (SummonMove.avoidAttack).
var avoidSpots = [12][2]int{
	{70, 0},
	{60, 34},
	{35, 60},
	{0, 70},
	{-34, 60},
	{-60, 35},
	{-70, 0},
	{-60, -34},
	{-35, -60},
	{0, -70},
	{34, -60},
	{60, -35},
}

// avoidSpot is the spot index a pet's fixed roll picks.
const avoidSpot = 4

// ownerKey looks a player up in the stance tracker, which reads only the id.
type ownerKey struct{ id int32 }

func (k ownerKey) ObjectID() int32 { return k.id }
func (ownerKey) Queue() *sim.Queue { return nil }

// frameIndex finds the first frame with opcode op whose leading object id is
// id, or -1.
func frameIndex(frames [][]byte, op byte, id int32) int {
	for i, f := range frames {
		if f[0] == op && len(f) >= 5 && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
			return i
		}
	}
	return -1
}

// frameCount counts the frames with opcode op whose leading object id is id.
func frameCount(frames [][]byte, op byte, id int32) int {
	n := 0
	for _, f := range frames {
		if f[0] == op && len(f) >= 5 && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
			n++
		}
	}
	return n
}

// petNextToMonster boots an owner with a spawned wolf and a stationary
// attacking monster next to the wolf. The wolf's rolls always pick
// avoidSpot and never break a stun.
func petNextToMonster(t *testing.T) (*petWorld, *summon.Actor, *gameservertest.AttackingHostile) {
	t.Helper()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithAttackStanceClock(time.Now)})
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	setSummonRoll(t, h.srv, h.ownerID, pet, func(int) int { return avoidSpot })
	x, y, z := pet.Position()
	monster := h.srv.SpawnAttackingHostileNPCAt(t, location.Location{X: x + 20, Y: y, Z: z})
	drainUntilQuiet(t, h.client)
	return h, pet, monster
}

// assertSteppedAside asserts frames carry the pet's MoveToLocation to
// avoidSpot around the owner.
func assertSteppedAside(t *testing.T, h *petWorld, pet *summon.Actor, frames [][]byte) {
	t.Helper()
	i := frameIndex(frames, serverpackets.OpcodeMoveToLocation, pet.ObjectID())
	if i < 0 {
		t.Fatal("pet never stepped aside: no MoveToLocation")
	}
	ox, oy, _ := h.srv.PlayerPosition(t, h.ownerID)
	r := wire.NewReader(frames[i][5:])
	x, y := r.ReadInt32(), r.ReadInt32()
	want := avoidSpots[avoidSpot]
	if int(x) != ox+want[0] || int(y) != oy+want[1] {
		t.Fatalf("pet step-aside destination = (%d, %d), want owner (%d, %d) + %v", x, y, ox, oy, want)
	}
}

// TestMonsterHitOnPetPutsOwnerInStanceAndPetStepsAside pins SummonAI's
// ATTACKED reaction (SummonAI.java:70-75): the owner, not yet in stance,
// enters it with AutoAttackStart for the pet and then for the owner
// (SummonAI.startAttackStance, :235-244), and the following pet near its
// now fighting owner steps aside to a spot around the owner
// (SummonMove.avoidAttack).
func TestMonsterHitOnPetPutsOwnerInStanceAndPetStepsAside(t *testing.T) {
	t.Parallel()
	h, pet, monster := petNextToMonster(t)

	monster.DoAttack(t, pet)
	frames := drainFrames(t, h.client)
	petStart := frameIndex(frames, serverpackets.OpcodeAutoAttackStart, pet.ObjectID())
	ownerStart := frameIndex(frames, serverpackets.OpcodeAutoAttackStart, h.ownerID)
	if petStart < 0 || ownerStart < petStart {
		t.Fatalf("AutoAttackStart frames: pet at %d, owner at %d; want the pet's, then the owner's", petStart, ownerStart)
	}
	if n, m := frameCount(frames, serverpackets.OpcodeAutoAttackStart, pet.ObjectID()), frameCount(frames, serverpackets.OpcodeAutoAttackStart, h.ownerID); n != 1 || m != 1 {
		t.Fatalf("AutoAttackStart count: pet %d, owner %d; want 1 each", n, m)
	}
	if !h.srv.AttackStance.InAttackStance(ownerKey{id: h.ownerID}) {
		t.Fatal("owner not in the stance tracker after its pet was hit")
	}
	assertSteppedAside(t, h, pet, frames)

	// A second hit, the owner already in stance, broadcasts no new stance.
	h.srv.AdvanceUntil(t, "pet step-aside done", func() bool { return !pet.IsMoving() })
	drainUntilQuiet(t, h.client)
	monster.DoAttack(t, pet)
	frames = drainFrames(t, h.client)
	if n := frameCount(frames, serverpackets.OpcodeAutoAttackStart, pet.ObjectID()) + frameCount(frames, serverpackets.OpcodeAutoAttackStart, h.ownerID); n != 0 {
		t.Fatalf("AutoAttackStart frames on a hit inside the stance = %d, want 0", n)
	}
}

// TestPetEvasionStepsAsideOnlyWhenOwnerFights pins SummonAI.onEvtEvaded
// (SummonAI.java:77-83): an evaded hit is no stance event, and the pet steps
// aside only while its owner is already in combat.
func TestPetEvasionStepsAsideOnlyWhenOwnerFights(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		inCombat bool
	}{
		{name: "owner in combat", inCombat: true},
		{name: "owner at peace"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, pet, monster := petNextToMonster(t)
			done := make(chan struct{})
			if !monster.Queue().Post(func() {
				defer close(done)
				monster.SetRollSource(func(int) int { return 999 })
			}) {
				t.Fatal("monster queue closed")
			}
			<-done
			if tt.inCombat {
				h.srv.SetPlayerInCombat(t, h.ownerID, true)
			}

			monster.DoAttack(t, pet)
			frames := drainFrames(t, h.client)
			if n := frameCount(frames, serverpackets.OpcodeAutoAttackStart, pet.ObjectID()) + frameCount(frames, serverpackets.OpcodeAutoAttackStart, h.ownerID); n != 0 {
				t.Fatalf("AutoAttackStart frames after an evaded hit = %d, want 0", n)
			}
			if !tt.inCombat {
				if frameIndex(frames, serverpackets.OpcodeMoveToLocation, pet.ObjectID()) >= 0 {
					t.Fatal("pet stepped aside with its owner out of combat")
				}
				return
			}
			assertSteppedAside(t, h, pet, frames)
		})
	}
}

// TestPetLandedHitPutsOwnerInStance pins the attacker side of
// SummonAI.startAttackStance (CreatureAttack.java:236-238): the pet's first
// landed hit shows AutoAttackStart for the pet and then for its owner, who
// never swung, and puts the owner in stance.
func TestPetLandedHitPutsOwnerInStance(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithAttackStanceClock(time.Now)})
	pet, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, h.client)
	setSummonRoll(t, h.srv, h.ownerID, pet, landNoCrit())

	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUse(petAttackAction, false))
	h.srv.AdvanceUntil(t, "pet hit landing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	frames := drainFrames(t, h.client)

	if frameIndex(frames, serverpackets.OpcodeAttack, h.ownerID) >= 0 {
		t.Fatal("owner swung itself, want only its pet attacking")
	}
	petStart := frameIndex(frames, serverpackets.OpcodeAutoAttackStart, pet.ObjectID())
	ownerStart := frameIndex(frames, serverpackets.OpcodeAutoAttackStart, h.ownerID)
	if petStart < 0 || ownerStart < petStart {
		t.Fatalf("AutoAttackStart frames: pet at %d, owner at %d; want the pet's, then the owner's", petStart, ownerStart)
	}
	if !h.srv.AttackStance.InAttackStance(ownerKey{id: h.ownerID}) {
		t.Fatal("owner not in the stance tracker after its pet's landed hit")
	}
}

// TestPetOffensiveSkillPutsOwnerInStance pins the caster side of
// SummonAI.startAttackStance at cast finalization (CreatureCast.java:308-309):
// the pet's offensive skill, once it finishes with its launch having named a
// target, shows AutoAttackStart for the pet and then for its owner, who cast
// nothing, and puts the owner in stance.
func TestPetOffensiveSkillPutsOwnerInStance(t *testing.T) {
	t.Parallel()
	strike := wolfStrike()
	strike.Power = 1
	h, petActor, _ := bootWolfStrikerWith(t, strike, gameservertest.WithAttackStanceClock(time.Now))
	startWolfStrike(t, h)
	h.srv.AdvanceUntil(t, "strike finish", func() bool { return !petActor.CastingNow() })
	frames := drainFrames(t, h.client)

	launched := frameIndex(frames, serverpackets.OpcodeMagicSkillLaunched, petActor.ObjectID())
	petStart := frameIndex(frames, serverpackets.OpcodeAutoAttackStart, petActor.ObjectID())
	ownerStart := frameIndex(frames, serverpackets.OpcodeAutoAttackStart, h.ownerID)
	if launched < 0 || petStart < launched || ownerStart < petStart {
		t.Fatalf("frames: launch at %d, pet AutoAttackStart at %d, owner's at %d; want the launch, then the pet's, then the owner's", launched, petStart, ownerStart)
	}
	if n, m := frameCount(frames, serverpackets.OpcodeAutoAttackStart, petActor.ObjectID()), frameCount(frames, serverpackets.OpcodeAutoAttackStart, h.ownerID); n != 1 || m != 1 {
		t.Fatalf("AutoAttackStart count: pet %d, owner %d; want 1 each", n, m)
	}
	if !h.srv.AttackStance.InAttackStance(ownerKey{id: h.ownerID}) {
		t.Fatal("owner not in the stance tracker after its pet's offensive skill")
	}
}
