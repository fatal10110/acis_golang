package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestBeastSoulshotChargesPetAndConsumes uses a beast soulshot from the
// item window with a pet out: PET_USES_S1 announces it, MagicSkillUse shows
// the charge cast by the pet itself, one per-hit unit leaves the stack, and
// the pet reports itself charged.
func TestBeastSoulshotChargesPetAndConsumes(t *testing.T) {
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

	waitFor(t, "pet hit consuming the charged soulshot", func() bool {
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
