package pets

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPetStrikeBroadcastsSkillUseThenLaunched pins a summon cast's observer
// packets: MagicSkillUse from the pet onto its target when the cast starts,
// then MagicSkillLaunched naming that target once the launch comes due.
func TestPetStrikeBroadcastsSkillUseThenLaunched(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	started := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed")
	use, ok := firstOpcode(started, serverpackets.OpcodeMagicSkillUse)
	if !ok {
		t.Fatalf("strike start sent no MagicSkillUse: opcodes %x", frameOpcodes(started))
	}
	r := wire.NewReader(use[1:])
	caster, target, skill, level, hitTime, reuse := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != petActor.ObjectID() || target != hostile.ObjectID() || skill != wolfStrikeSkill || level != 1 ||
		hitTime != wolfStrikeHitTime || reuse != 60_000 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d/%d hit %d reuse %d, want %d %d %d/1 %d 60000",
			caster, target, skill, level, hitTime, reuse, petActor.ObjectID(), hostile.ObjectID(), wolfStrikeSkill, wolfStrikeHitTime)
	}

	h.srv.Advance(t, wolfStrikeLaunch)
	launched := readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	r = wire.NewReader(launched[len(launched)-1][1:])
	caster, skill, level = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	count := r.ReadInt32()
	if caster != petActor.ObjectID() || skill != wolfStrikeSkill || level != 1 || count != 1 {
		t.Fatalf("MagicSkillLaunched = caster %d skill %d/%d targets %d, want %d %d/1 1",
			caster, skill, level, count, petActor.ObjectID(), wolfStrikeSkill)
	}
	if got := r.ReadInt32(); got != hostile.ObjectID() {
		t.Fatalf("MagicSkillLaunched target = %d, want %d", got, hostile.ObjectID())
	}
	h.srv.AdvanceUntil(t, "strike landing on the monster", func() bool { return hostile.HP() < float64(hostile.MaxHP()) })
	drainUntilQuiet(t, h.client)
}

// TestPetDespawnMidCastCancelsBeforePetDelete unsummons the pet while its
// strike is casting: observers see MagicSkillCanceled for the pet before
// its owner reads PetDelete and DeleteObject.
func TestPetDespawnMidCastCancelsBeforePetDelete(t *testing.T) {
	t.Parallel()
	h, petActor, _ := bootWolfStriker(t)
	startWolfStrike(t, h)

	done := make(chan struct{})
	if !petActor.Queue().Post(func() { petActor.Unsummon(); close(done) }) {
		t.Fatal("post unsummon: queue closed")
	}
	<-done
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeDeleteObject, "DeleteObject")
	opcodes := frameOpcodes(frames)
	canceled := slices.Index(opcodes, serverpackets.OpcodeMagicSkillCanceled)
	petDelete := slices.Index(opcodes, serverpackets.OpcodePetDelete)
	if canceled < 0 || petDelete < 0 || canceled > petDelete {
		t.Fatalf("despawn opcodes = %x, want MagicSkillCanceled (%#x) before PetDelete (%#x) and DeleteObject",
			opcodes, serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodePetDelete)
	}
	if n := wire.NewReader(frames[canceled][1:]).ReadInt32(); n != petActor.ObjectID() {
		t.Fatalf("MagicSkillCanceled object = %d, want pet %d", n, petActor.ObjectID())
	}
	if slices.Contains(opcodes[canceled+1:], serverpackets.OpcodeMagicSkillCanceled) {
		t.Fatalf("despawn opcodes = %x, want one MagicSkillCanceled", opcodes)
	}
}

// sightGeo is passable movement geo whose line of sight can be cut
// mid-scenario, standing in for a wall rising between two actors.
type sightGeo struct {
	gameservertest.Geo
	blind atomic.Bool
}

func (g *sightGeo) CanSeeActor(int, int, int, float64, int, int, int, float64) bool {
	return !g.blind.Load()
}

// TestPetStrikeLaunchFailsWithoutLineOfSight cuts the pet's line of sight
// to its target after the strike starts. The launch refuses it: the owner
// reads CANT_SEE_TARGET, observers see MagicSkillCanceled and never
// MagicSkillLaunched, and the monster keeps its HP.
func TestPetStrikeLaunchFailsWithoutLineOfSight(t *testing.T) {
	t.Parallel()
	strike := wolfStrike()
	strike.Radius = 150 // the launch sight gate applies to a skill with a radius
	geo := &sightGeo{}
	h, petActor, hostile := bootWolfStrikerWith(t, strike, gameservertest.WithGeo(geo))
	startWolfStrike(t, h)
	geo.blind.Store(true)

	h.srv.Advance(t, wolfStrikeLaunch)
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillCanceled, "MagicSkillCanceled")
	if n := wire.NewReader(frames[len(frames)-1][1:]).ReadInt32(); n != petActor.ObjectID() {
		t.Fatalf("MagicSkillCanceled object = %d, want pet %d", n, petActor.ObjectID())
	}
	if len(frames) < 2 {
		t.Fatalf("launch abort opcodes = %x, want CANT_SEE_TARGET before MagicSkillCanceled", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[len(frames)-2], serverpackets.SystemMessageCantSeeTarget)

	h.srv.Advance(t, wolfStrikeHitTime*time.Millisecond)
	rest := drainFrames(t, h.client)
	if slices.Contains(frameOpcodes(append(frames, rest...)), serverpackets.OpcodeMagicSkillLaunched) {
		t.Fatalf("occluded launch sent MagicSkillLaunched: opcodes %x", frameOpcodes(append(frames, rest...)))
	}
	if hp, full := hostile.HP(), float64(hostile.MaxHP()); hp != full {
		t.Fatalf("monster HP = %v after an occluded launch, want untouched %v", hp, full)
	}
}
