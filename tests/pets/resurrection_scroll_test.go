package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: ScrollsOfResurrection.useItem (ScrollsOfResurrection.java:19-94)
// checks the selected creature, then casts each of the scroll's skills
// through tryToCast(target, skill) with no modifiers and no item object id:
// an ordinary cast that sends USE_S1, sends no ExUseSharedGroupItem, and
// whose skill's own consume item (2179 consumes 6387,
// aCis_datapack/data/xml/skills/2100-2199.xml:868-880) pays for it.
// TargetCorpsePet then decides living vs dead non-pet vs dead pet.

const petScrollSkillID = 2179

// petScrollSkill is skill 2179 as the datapack defines it.
func petScrollSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: petScrollSkillID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpsePet, SkillType: "RESURRECT", Power: 100,
		CastRange: 400, EffectRange: 600, HitTime: 15000, StaticHitTime: true,
		ItemConsumeID: int(gameservertest.PetResurrectionScrollID), ItemConsumeCount: 1,
	}
}

// petScrollTable is the pet fixture's skills, the power-100 resurrection,
// and the pet resurrection scroll's skill.
func petScrollTable(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
		{
			ID: petResurrectSkillID, Level: 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT",
			CastRange: 400, HitTime: 500, StaticHitTime: true, Power: 100,
		},
		petScrollSkill(),
	}), gamesql.NewCharacterSkillStore(db))
}

// liveItemCount is how many units of templateID objID's live inventory
// holds.
func liveItemCount(t *testing.T, srv *gameservertest.Server, objID, templateID int32) int {
	t.Helper()
	inst := srv.PlayerInventory(t, objID).ItemByTemplateID(templateID)
	if inst == nil {
		return 0
	}
	return inst.CountValue()
}

// selectObject has c's player select objectID and drains the answer.
func selectObject(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, playerID, objectID int32) {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, playerID)
	c.Send(encodeAction(objectID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, c)
}

// assertOnlyFrame asserts c's next frame is the static system message id
// and that nothing follows it.
func assertOnlyFrame(t *testing.T, c *testsupport.ScriptedClient, messageID int, what string) {
	t.Helper()
	assertStaticSystemMessage(t, mustRead(t, c, what), messageID)
	if extra := c.ReadWithTimeout(300 * time.Millisecond); extra != nil {
		t.Fatalf("%s: extra opcode %#x, want none", what, extra[0])
	}
}

// TestPetScrollRevivesOwnDeadPet: the owner's scroll on its own dead pet
// casts 2179 as an ordinary skill (MagicSkillUse, USE_S1, gauge, with no
// shared-reuse packet), the skill's consume item takes the scroll, and the
// pet stands up once the 15 s cast lands.
func TestPetScrollRevivesOwnDeadPet(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithSkills(petScrollTable(t))},
		seedItem{TemplateID: gameservertest.PetResurrectionScrollID, Count: 2})
	scroll := h.seeded[gameservertest.PetResurrectionScrollID][0]
	wolf, _ := h.spawnWolf(t)
	killPet(t, h, wolf)
	selectObject(t, h.srv, h.client, h.ownerID, wolf.ObjectID())

	h.client.Send(encodeUseItem(scroll, false))
	frame := mustRead(t, h.client, "scroll MagicSkillUse")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "scroll MagicSkillUse")
	r := wire.NewReader(frame[1:])
	if caster, target, skill, level, hit, reuse := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != h.ownerID || target != wolf.ObjectID() || skill != petScrollSkillID || level != 1 || hit != 15000 || reuse != 0 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d/%d hit %d reuse %d, want %d/%d %d/1 15000/0",
			caster, target, skill, level, hit, reuse, h.ownerID, wolf.ObjectID(), petScrollSkillID)
	}
	assertSystemMessageSkill(t, mustRead(t, h.client, "USE_S1"), serverpackets.SystemMessageUseS1, petScrollSkillID, 1)
	assertFrameOpcode(t, mustRead(t, h.client, "SetupGauge"), serverpackets.OpcodeSetupGauge, "SetupGauge")
	if got := liveItemCount(t, h.srv, h.ownerID, gameservertest.PetResurrectionScrollID); got != 1 {
		t.Fatalf("scrolls after the cast started = %d, want 1", got)
	}

	h.srv.Advance(t, 15500*time.Millisecond)
	h.srv.AdvanceUntil(t, "pet revived", func() bool { return !wolf.Dead() })
	h.srv.InventoryUpdates.Tick()
	for _, f := range drainFrames(t, h.client) {
		if f[0] == serverpackets.OpcodeExtended && wire.NewReader(f[1:]).ReadUint16() == serverpackets.OpcodeExUseSharedGroupItem {
			t.Fatal("scroll cast sent ExUseSharedGroupItem, want none for a cast no item carries")
		}
	}
	if got := h.ownerItemCount(t, gameservertest.PetResurrectionScrollID); got != 1 {
		t.Fatalf("persisted scrolls = %d, want 1", got)
	}
}

// TestPetScrollTargetRefusals: a missing target and a living one answer
// INVALID_TARGET, a dead servitor S1_CANNOT_BE_USED naming 2179; each alone
// and with the scroll kept.
func TestPetScrollTargetRefusals(t *testing.T) {
	t.Parallel()
	t.Run("no target", func(t *testing.T) {
		t.Parallel()
		h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithSkills(petScrollTable(t))},
			seedItem{TemplateID: gameservertest.PetResurrectionScrollID, Count: 1})
		h.client.Send(encodeUseItem(h.seeded[gameservertest.PetResurrectionScrollID][0], false))
		assertOnlyFrame(t, h.client, serverpackets.SystemMessageInvalidTarget, "no-target scroll")
		if got := liveItemCount(t, h.srv, h.ownerID, gameservertest.PetResurrectionScrollID); got != 1 {
			t.Fatalf("scrolls = %d, want 1 kept", got)
		}
	})

	t.Run("living pet", func(t *testing.T) {
		t.Parallel()
		h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithSkills(petScrollTable(t))},
			seedItem{TemplateID: gameservertest.PetResurrectionScrollID, Count: 1})
		wolf, _ := h.spawnWolf(t)
		drainUntilQuiet(t, h.client)
		selectObject(t, h.srv, h.client, h.ownerID, wolf.ObjectID())
		h.client.Send(encodeUseItem(h.seeded[gameservertest.PetResurrectionScrollID][0], false))
		assertOnlyFrame(t, h.client, serverpackets.SystemMessageInvalidTarget, "living-pet scroll")
		if got := liveItemCount(t, h.srv, h.ownerID, gameservertest.PetResurrectionScrollID); got != 1 {
			t.Fatalf("scrolls = %d, want 1 kept", got)
		}
	})

	t.Run("dead servitor", func(t *testing.T) {
		t.Parallel()
		o := bootServitorOwnerWithItems(t, []modelskill.Definition{petScrollSkill()},
			[]seedItem{{TemplateID: gameservertest.PetResurrectionScrollID, Count: 1}})
		servitor := o.summonServitor(t)
		x, y, z := servitor.Position()
		o.client.Send(encodeAction(servitor.ObjectID(), int32(x), int32(y), int32(z), false))
		readUntilOpcode(t, o.client, serverpackets.OpcodeMyTargetSelected, "servitor selected")
		drainUntilQuiet(t, o.client)
		o.killServitor(t, servitor)

		o.client.Send(encodeUseItem(o.seeded[gameservertest.PetResurrectionScrollID][0], false))
		assertSystemMessageSkill(t, mustRead(t, o.client, "dead-servitor scroll"), serverpackets.SystemMessageS1CannotBeUsed, petScrollSkillID, 1)
		if extra := o.client.ReadWithTimeout(300 * time.Millisecond); extra != nil {
			t.Fatalf("dead-servitor scroll extra opcode = %#x, want none", extra[0])
		}
		if got := liveItemCount(t, o.srv, o.id, gameservertest.PetResurrectionScrollID); got != 1 {
			t.Fatalf("scrolls = %d, want 1 kept", got)
		}
	})
}

// newPetScrollScene is newPetRevivalScene with a healer who also carries
// a pet resurrection scroll.
func newPetScrollScene(t *testing.T) (*petRevivalScene, int32) {
	t.Helper()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithSkills(petScrollTable(t))})
	pet, _ := h.spawnWolf(t)
	healer := h.srv.SeedCharacterFor(t, "healer", "Healer", 5, 0)
	if err := h.srv.KnownSkills.SetKnownSkill(context.Background(), healer.ID, 0, petResurrectSkillID, 1); err != nil {
		t.Fatalf("seed resurrection: %v", err)
	}
	scroll := h.srv.GiveItem(t, healer.ID, gameservertest.PetResurrectionScrollID, 1)
	s := &petRevivalScene{h: h, pet: pet, healerID: healer.ID, healer: h.srv.DialClient(t, "healer", 1)}
	startInWorld(t, s.healer)
	drainUntilQuiet(t, s.healer)
	killPet(t, h, pet)
	drainUntilQuiet(t, s.healer)
	return s, scroll
}

// useScrollOn has the healer select objectID and use its scroll.
func (s *petRevivalScene) useScrollOn(t *testing.T, scroll, objectID int32) {
	t.Helper()
	selectObject(t, s.h.srv, s.healer, s.healerID, objectID)
	s.healer.Send(encodeUseItem(scroll, false))
}

// TestPetScrollRefusesOpenOffers: with a resurrection offer already open,
// the scroll answers the reference's refusal alone, casts nothing and keeps
// the scroll: the dead pet's own offer (RES_HAS_ALREADY_BEEN_PROPOSED), the
// owner's offer for itself (CANNOT_RES_PET2), and, on the dead owner, the
// pet's offer (CANNOT_RES_MASTER).
func TestPetScrollRefusesOpenOffers(t *testing.T) {
	t.Parallel()
	t.Run("pet offer open", func(t *testing.T) {
		t.Parallel()
		s, scroll := newPetScrollScene(t)
		s.resurrect(t, s.pet.ObjectID())
		readResurrectionOffer(t, s.h.client, "Healer")
		drainUntilQuiet(t, s.healer)

		s.useScrollOn(t, scroll, s.pet.ObjectID())
		assertOnlyFrame(t, s.healer, serverpackets.SystemMessageResHasAlreadyBeenProposed, "pet offer open")

		s.h.srv.MarkPlayerDead(t, s.h.ownerID)
		drainUntilQuiet(t, s.h.client)
		drainUntilQuiet(t, s.healer)
		s.useScrollOn(t, scroll, s.h.ownerID)
		assertOnlyFrame(t, s.healer, serverpackets.SystemMessageCannotResMaster, "dead owner with the pet's offer open")
		if got := liveItemCount(t, s.h.srv, s.healerID, gameservertest.PetResurrectionScrollID); got != 1 {
			t.Fatalf("healer scrolls = %d, want 1 kept", got)
		}
	})

	t.Run("owner offer open", func(t *testing.T) {
		t.Parallel()
		s, scroll := newPetScrollScene(t)
		s.h.srv.MarkPlayerDead(t, s.h.ownerID)
		drainUntilQuiet(t, s.h.client)
		drainUntilQuiet(t, s.healer)
		s.resurrect(t, s.h.ownerID)
		readResurrectionOffer(t, s.h.client, "Healer")
		drainUntilQuiet(t, s.healer)

		s.useScrollOn(t, scroll, s.pet.ObjectID())
		assertOnlyFrame(t, s.healer, serverpackets.SystemMessageCannotResPet2, "owner offer open")
		if got := liveItemCount(t, s.h.srv, s.healerID, gameservertest.PetResurrectionScrollID); got != 1 {
			t.Fatalf("healer scrolls = %d, want 1 kept", got)
		}
	})
}
