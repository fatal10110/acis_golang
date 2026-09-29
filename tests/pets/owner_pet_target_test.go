package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const ownerPetSkillID = 5200

func ownerPetSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: ownerPetSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOwnerPet,
		SkillType: "HEAL", Power: 10, CastRange: 600, HitTime: 1000, ReuseDelay: 60_000,
		StaticHitTime: true, StaticReuse: true,
	}
}

// bootOwnerPetWolf summons a wolf that knows an OWNER_PET skill and returns
// it with its owner as a combatant.
func bootOwnerPetWolf(t *testing.T) (*petWorld, *summon.Actor, attackable.Combatant) {
	t.Helper()
	wolf := wolfTemplate()
	wolf.Skills = map[int]int{ownerPetSkillID: 1}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		ownerPetSkill(),
	}), gamesql.NewCharacterSkillStore(db))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skills),
	})
	petActor, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	return h, petActor, obj.(attackable.Combatant)
}

// assertSummonCastRejected reads the owner's rejection message, then the
// pet's MoveToPawn toward the refused target, and nothing else: no cast
// starts.
func assertSummonCastRejected(t *testing.T, h *petWorld, petActor *summon.Actor, targetID int32, message func([]byte)) {
	t.Helper()
	message(mustRead(t, h.client, "rejection SystemMessage"))
	frame := mustRead(t, h.client, "pet MoveToPawn")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToPawn, "pet MoveToPawn")
	r := wire.NewReader(frame[1:])
	if mover, target := r.ReadInt32(), r.ReadInt32(); mover != petActor.ObjectID() || target != targetID {
		t.Fatalf("MoveToPawn = %d toward %d, want pet %d toward %d", mover, target, petActor.ObjectID(), targetID)
	}
	if extra := h.client.ReadWithTimeout(300 * time.Millisecond); extra != nil {
		t.Fatalf("rejected summon cast sent extra frame %#x", extra[0])
	}
}

// TestOwnerPetSkillTargetsLiveOwner resolves an OWNER_PET skill cast by a
// live pet to its owner, and the cast on a living owner starts.
func TestOwnerPetSkillTargetsLiveOwner(t *testing.T) {
	t.Parallel()
	h, petActor, owner := bootOwnerPetWolf(t)
	def := ownerPetSkill()

	handler, ok := skilltarget.NewRegistry(nil).Handler(modelskill.TargetOwnerPet)
	if !ok {
		t.Fatal("no OWNER_PET target handler")
	}
	final := handler.FinalTarget(petActor, nil, &def)
	if final == nil || final.ObjectID() != h.ownerID {
		t.Fatalf("OWNER_PET final target of a live pet = %v, want owner %d", final, h.ownerID)
	}

	runOnPetQueue(t, petActor, func() { petActor.TryUseSkill(ownerPetSkillID, owner, false) })
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillUse, "pet MagicSkillUse")
	r := wire.NewReader(frames[len(frames)-1][1:])
	if caster, target := r.ReadInt32(), r.ReadInt32(); caster != petActor.ObjectID() || target != h.ownerID {
		t.Fatalf("MagicSkillUse caster/target = %d/%d, want pet %d onto owner %d", caster, target, petActor.ObjectID(), h.ownerID)
	}
	drainUntilQuiet(t, h.client)
}

// TestOwnerPetSkillOnDeadOwnerIsRefusedByName casts the OWNER_PET skill once
// the owner has died: the owner reads S1_CANNOT_BE_USED naming the skill,
// the pet turns toward the owner, and no cast starts.
func TestOwnerPetSkillOnDeadOwnerIsRefusedByName(t *testing.T) {
	t.Parallel()
	h, petActor, owner := bootOwnerPetWolf(t)
	h.srv.MarkPlayerDead(t, h.ownerID)
	drainUntilQuiet(t, h.client)

	runOnPetQueue(t, petActor, func() { petActor.TryUseSkill(ownerPetSkillID, owner, false) })
	assertSummonCastRejected(t, h, petActor, h.ownerID, func(frame []byte) {
		assertSystemMessageSkill(t, frame, serverpackets.SystemMessageS1CannotBeUsed, ownerPetSkillID, 1)
	})
}

// TestOwnerPetSkillAimedElsewhereCastsOnOwner aims the OWNER_PET skill at a
// monster. The cast intention stores the skill's final target
// (Intention.updateAsCast -> getFinalTarget), which for OWNER_PET is the
// owner, so the pet casts on its owner whatever was selected.
func TestOwnerPetSkillAimedElsewhereCastsOnOwner(t *testing.T) {
	t.Parallel()
	h, petActor, _ := bootOwnerPetWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, h.client)

	runOnPetQueue(t, petActor, func() { petActor.TryUseSkill(ownerPetSkillID, hostile, false) })
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillUse, "pet MagicSkillUse")
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeSystemMessage {
			t.Fatalf("OWNER_PET cast aimed at a monster was refused: opcodes %x", frameOpcodes(frames))
		}
	}
	r := wire.NewReader(frames[len(frames)-1][1:])
	if caster, target := r.ReadInt32(), r.ReadInt32(); caster != petActor.ObjectID() || target != h.ownerID {
		t.Fatalf("MagicSkillUse caster/target = %d/%d, want pet %d onto owner %d", caster, target, petActor.ObjectID(), h.ownerID)
	}
	drainUntilQuiet(t, h.client)
}

// assertSystemMessageSkill checks a SystemMessage carrying one skill-name
// parameter.
func assertSystemMessageSkill(t *testing.T, frame []byte, messageID int, skillID, level int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	id, params, typ, gotSkill, gotLevel := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
	if id != int32(messageID) || params != 1 || typ != serverpackets.SystemMessageParamSkillName || gotSkill != skillID || gotLevel != level {
		t.Fatalf("SystemMessage = id %d params %d type %d skill %d/%d, want id %d with skill %d/%d",
			id, params, typ, gotSkill, gotLevel, messageID, skillID, level)
	}
}
