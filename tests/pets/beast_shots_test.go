package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBeastSoulshotChargesPetAndConsumes uses a beast soulshot from the
// item window with a pet out: PET_USES_S1 announces it, MagicSkillUse shows
// the charge cast by the pet itself, one per-hit unit leaves the stack, and
// the pet reports itself charged.
func TestBeastSoulshotChargesPetAndConsumes(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: beastSoulshotID, Count: 10})
	petActor, _ := h.spawnWolf(t)
	shotID := h.seededItem(t, beastSoulshotID)

	h.client.Send(encodeUseItem(shotID, false))
	assertSystemMessageID(t, mustRead(t, h.client, "PET_USES_S1"), serverpackets.SystemMessagePetUsesS1)
	frame := mustRead(t, h.client, "charge MagicSkillUse")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "charge MagicSkillUse")
	r := wire.NewReader(frame[1:])
	caster, target, skill, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != petActor.ObjectID() || target != petActor.ObjectID() || skill != 2033 || level != 1 {
		t.Fatalf("charge cast = %d/%d skill %d level %d, want pet-cast self 2033/1", caster, target, skill, level)
	}
	if !petActor.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = false after beast soulshot use")
	}

	h.srv.InventoryUpdates.Tick()
	drainFrames(t, h.client)
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), h.ownerID)
	if err != nil {
		t.Fatalf("list owner items: %v", err)
	}
	count := 0
	for _, row := range rows {
		if row.TemplateID == 6645 {
			count += row.Count
		}
	}
	if count != 9 {
		t.Fatalf("beast soulshot stack = %d, want 9 after charging a 1-per-hit pet", count)
	}
}

// TestBeastSoulshotWithoutSummonRejectsWithoutConsuming answers
// PETS_ARE_NOT_AVAILABLE_AT_THIS_TIME when no summon is out and consumes
// nothing.
func TestBeastSoulshotWithoutSummonRejectsWithoutConsuming(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: beastSoulshotID, Count: 10})
	shotID := h.seededItem(t, beastSoulshotID)

	h.client.Send(encodeUseItem(shotID, false))
	frame := mustRead(t, h.client, "no-summon rejection")
	assertSystemMessageID(t, frame, serverpackets.SystemMessagePetsNotAvailableAtThisTime)
	drainUntilQuiet(t, h.client)

	if got := h.ownerItemCount(t, 6645); got != 10 {
		t.Fatalf("beast soulshot stack = %d, want untouched 10", got)
	}
}

// TestPetAttackConsumesChargedBeastSoulshot drives the combat side: with a
// charged pet attacking a targeted monster, the first landed hit consumes
// the charge and its stack unit.
func TestPetAttackConsumesChargedBeastSoulshot(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: beastSoulshotID, Count: 10})
	petActor, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)
	shotID := h.seededItem(t, beastSoulshotID)

	h.client.Send(encodeUseItem(shotID, false))
	drainFrames(t, h.client)
	if !petActor.SoulshotCharged() {
		t.Fatal("pet not charged before the attack")
	}

	// Target the monster, then command the pet to attack it.
	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUse(16, false))

	h.srv.AdvanceUntil(t, "pet hit consuming the charged soulshot", func() bool {
		return !petActor.SoulshotCharged()
	})
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), h.ownerID)
	if err != nil {
		t.Fatalf("list owner items: %v", err)
	}
	count := 0
	for _, row := range rows {
		if row.TemplateID == 6645 {
			count += row.Count
		}
	}
	if count != 9 {
		t.Fatalf("beast soulshot stack = %d, want 9 after one charged hit", count)
	}
	drainUntilQuiet(t, h.client)
}

// TestPetAttackRechargesAutoBeastSoulshots pins CreatureAttack.onHitTimer
// (CreatureAttack.java:144-145) with Summon.rechargeShots
// (Summon.java:408-435): a pet's hit spends its charge and then charges it
// again from its owner's auto-use beast soulshots, through the same handler
// a direct use runs (PET_USES_S1, then the charge MagicSkillUse cast by the
// pet), consuming the pet's per-hit count. Once the stack is gone the
// auto-use entry is dropped silently.
func TestPetAttackRechargesAutoBeastSoulshots(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: beastSoulshotID, Count: 2})
	petActor, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)
	// The wolf would kill the fixture monster outright; blocked hits still
	// land, spending the charge each time.
	hostile.SetInvul(true)
	shotID := h.seededItem(t, beastSoulshotID)

	// Charge the pet by hand, then turn auto use on: the first hit spends
	// that charge and the recharge takes the last shot.
	h.client.Send(encodeUseItem(shotID, false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestAutoSoulShot(beastSoulshotID, 1))
	drainFrames(t, h.client)
	live, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from the world")
	}
	owner, ok := live.(interface{ AutoSoulShotEnabled(int32) bool })
	if !ok {
		t.Fatalf("owner %T exposes no auto-shot state", live)
	}
	if !petActor.SoulshotCharged() || !owner.AutoSoulShotEnabled(beastSoulshotID) {
		t.Fatal("pet not charged, or auto beast soulshots off, before the attack")
	}

	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUse(16, false))

	// The second landed hit finds the stack gone and drops auto use.
	h.srv.AdvanceUntil(t, "auto beast soulshots dropped with the spent stack", func() bool {
		return !owner.AutoSoulShotEnabled(beastSoulshotID)
	})
	frames := drainFrames(t, h.client)

	uses, casts := 0, 0
	for _, frame := range frames {
		r := wire.NewReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() == serverpackets.SystemMessagePetUsesS1 {
				uses++
			}
		case serverpackets.OpcodeMagicSkillUse:
			if caster, target, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster == petActor.ObjectID() && target == caster && skill == 2033 {
				if casts == uses {
					t.Fatal("charge MagicSkillUse ahead of its PET_USES_S1")
				}
				casts++
			}
		case serverpackets.OpcodeExtended:
			if r.ReadUint16() == serverpackets.OpcodeExAutoSoulShot {
				t.Fatal("ExAutoSoulShot sent; a spent stack drops auto use silently")
			}
		}
	}
	if uses != 1 || casts != 1 {
		t.Fatalf("recharges in combat: PET_USES_S1 = %d, charge MagicSkillUse = %d; want 1, 1", uses, casts)
	}
	if petActor.SoulshotCharged() {
		t.Fatal("pet still charged: the second hit did not spend the first hit's recharge")
	}
	if got := h.ownerItemCount(t, 6645); got != 0 {
		t.Fatalf("beast soulshot stack = %d, want 0 after the recharge", got)
	}
}

// beastSpiritshotID is the plain beast spiritshot (BeastSpiritShots handler,
// charge skill 2008).
const beastSpiritshotID = int32(6646)

// magicOwnerPetSkillID is a magic OWNER_PET heal the wolf casts on its
// owner; being magic it spends a spiritshot charge.
const magicOwnerPetSkillID = 5201

// TestPetMagicCastRechargesAutoBeastSpiritshots pins
// CreatureCast.onMagicFinalizer (rechargeShots(skill.useSoulShot(),
// skill.useSpiritShot())) with Summon.rechargeShots (Summon.java:408-435)
// for the spiritshot arm: once a pet's magic cast finishes, its owner's
// auto-use beast spiritshots charge it through the same handler a direct use
// runs (PET_USES_S1, then the charge MagicSkillUse cast by the pet),
// consuming the pet's per-cast spiritshot count.
func TestPetMagicCastRechargesAutoBeastSpiritshots(t *testing.T) {
	t.Parallel()
	const spsCount = 2
	wolf := wolfTemplate()
	levels := make(map[int]npc.PetLevelStats, len(wolf.Pet.Levels))
	for lvl, stats := range wolf.Pet.Levels {
		stats.SPSCount = spsCount
		levels[lvl] = stats
	}
	wolf.Pet.Levels = levels
	wolf.Skills = map[int]int{magicOwnerPetSkillID: 1}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		// Not static-reuse: a static-reuse skill writes its spent charge
		// back as still charged.
		{
			ID: magicOwnerPetSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOwnerPet,
			SkillType: "HEAL", Magic: true, Power: 10, CastRange: 600, HitTime: 1000, ReuseDelay: 60_000,
			StaticHitTime: true,
		},
	}), gamesql.NewCharacterSkillStore(db))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skills),
	}, seedItem{TemplateID: beastSpiritshotID, Count: 5})
	petActor, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)

	// Charge the pet by hand, then turn auto use on: the heal's hit spends
	// that charge and the finalizer charges it again from the stack.
	h.client.Send(encodeUseItem(h.seededItem(t, beastSpiritshotID), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeRequestAutoSoulShot(beastSpiritshotID, 1))
	drainUntilQuiet(t, h.client)
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from the world")
	}
	owner, ok := obj.(interface {
		attackable.Combatant
		AutoSoulShotEnabled(int32) bool
	})
	if !ok || !owner.AutoSoulShotEnabled(beastSpiritshotID) || !petActor.SpiritshotCharged() {
		t.Fatalf("owner %T: pet not charged, or auto beast spiritshots off, before the cast", obj)
	}
	before := h.ownerItemCount(t, beastSpiritshotID)

	runOnPetQueue(t, petActor, func() { petActor.TryUseSkill(magicOwnerPetSkillID, owner, false) })
	h.srv.AdvanceUntil(t, "the pet's heal finishing", func() bool { return !petActor.CastingNow() })
	frames := drainFrames(t, h.client)
	if !petActor.SpiritshotCharged() {
		t.Fatal("pet uncharged after its magic cast: the finalizer did not recharge it")
	}

	launched, uses, casts := false, 0, 0
	for _, frame := range frames {
		r := wire.NewReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillLaunched:
			launched = true
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() == serverpackets.SystemMessagePetUsesS1 {
				if !launched {
					t.Fatal("PET_USES_S1 ahead of the cast's MagicSkillLaunched")
				}
				uses++
			}
		case serverpackets.OpcodeMagicSkillUse:
			if caster, target, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster == petActor.ObjectID() && target == caster && skill == 2008 {
				if casts == uses {
					t.Fatal("charge MagicSkillUse ahead of its PET_USES_S1")
				}
				casts++
			}
		}
	}
	if uses != 1 || casts != 1 {
		t.Fatalf("recharge after the magic cast: PET_USES_S1 = %d, charge MagicSkillUse = %d; want 1, 1", uses, casts)
	}
	if got := h.ownerItemCount(t, beastSpiritshotID); got != before-spsCount {
		t.Fatalf("beast spiritshot stack = %d, want %d after one %d-per-cast recharge", got, before-spsCount, spsCount)
	}
}
