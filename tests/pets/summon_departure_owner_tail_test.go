package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// contractPaymentID is the servitor upkeep debuff its owner carries, which
// ends with the summon.
const contractPaymentID = 4140

// framesAfterPetDelete returns the frames the owner reads after the
// PetDelete a return command sends, once the server is quiet.
func framesAfterPetDelete(t *testing.T, h *petWorld) [][]byte {
	t.Helper()
	h.client.Send(encodeRequestActionUse(19, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	return drainFrames(t, h.client)
}

// TestReturnedPetDisablesOwnerSummonAutoShots returns a pet while its owner
// has beast soulshots on auto use: after PetDelete and the pet's
// DeleteObject the owner reads ExAutoSoulShot(6645, off), then
// AUTO_USE_OF_S1_CANCELLED naming the shot, and auto use is off.
func TestReturnedPetDisablesOwnerSummonAutoShots(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: beastSoulshotID, Count: 10})
	petActor, _ := h.spawnWolf(t)
	h.client.Send(encodeRequestAutoSoulShot(beastSoulshotID, 1))
	drainFrames(t, h.client)
	live, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from the world")
	}
	owner := live.(interface{ AutoSoulShotEnabled(int32) bool })
	if !owner.AutoSoulShotEnabled(beastSoulshotID) {
		t.Fatal("auto beast soulshots off before the return")
	}

	frames := framesAfterPetDelete(t, h)

	deleted, shot := -1, -1
	for i, frame := range frames {
		r := wire.NewReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeDeleteObject:
			if r.ReadInt32() == petActor.ObjectID() {
				deleted = i
			}
		case serverpackets.OpcodeExtended:
			if r.ReadUint16() != serverpackets.OpcodeExAutoSoulShot {
				continue
			}
			if shot >= 0 {
				t.Fatal("more than one ExAutoSoulShot after the return")
			}
			if id, state := r.ReadInt32(), r.ReadInt32(); id != beastSoulshotID || state != 0 {
				t.Fatalf("ExAutoSoulShot = item %d state %d, want %d off", id, state, beastSoulshotID)
			}
			shot = i
		}
	}
	if shot < 0 {
		t.Fatalf("no ExAutoSoulShot after PetDelete; frames %x", frameOpcodes(frames))
	}
	if deleted < 0 || deleted > shot {
		t.Fatalf("pet DeleteObject at %d, ExAutoSoulShot at %d: want the despawn first; frames %x", deleted, shot, frameOpcodes(frames))
	}
	if shot+1 >= len(frames) {
		t.Fatal("no AUTO_USE_OF_S1_CANCELLED after ExAutoSoulShot")
	}
	assertSystemMessageItem(t, frames[shot+1], serverpackets.SystemMessageAutoUseOfItemCancelled, beastSoulshotID)
	if owner.AutoSoulShotEnabled(beastSoulshotID) {
		t.Fatal("auto beast soulshots still on after the return")
	}
}

// TestReturnedPetStopsOwnerContractPayment returns a pet while its owner
// carries Contract Payment and another buff: after PetDelete the owner's
// icons refresh without Contract Payment, the other buff stays, and nothing
// is sent for auto shots the owner never enabled.
func TestReturnedPetStopsOwnerContractPayment(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}
	owner := obj.(interface {
		effect.Actor
		EffectList() *effect.List
	})
	payment, err := effect.New(effect.Skill{ID: contractPaymentID, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "DamOverTime", Time: 3600, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(contract payment): %v", err)
	}
	buff, err := effect.New(effect.Skill{ID: 1040, Level: 1}, modelskill.EffectTemplate{Name: "Buff", Time: 3600, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(buff): %v", err)
	}
	for _, e := range []*effect.Effect{payment, buff} {
		e.Effector, e.Effected = owner, owner
	}
	runOn(t, h.srv.PlayerQueue(t, h.ownerID), func() {
		owner.EffectList().Add(payment)
		owner.EffectList().Add(buff)
	})
	drainUntilQuiet(t, h.client)

	frames := framesAfterPetDelete(t, h)

	icons := -1
	for i, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeAbnormalStatusUpdate:
			icons = i
		case serverpackets.OpcodeExtended:
			if wire.NewReader(frame[1:]).ReadUint16() == serverpackets.OpcodeExAutoSoulShot {
				t.Fatal("ExAutoSoulShot sent for auto shots never enabled")
			}
		}
	}
	if icons < 0 {
		t.Fatalf("no AbnormalStatusUpdate after PetDelete; frames %x", frameOpcodes(frames))
	}
	r := wire.NewReader(frames[icons][1:])
	count := int(r.ReadUint16())
	ids := make([]int32, 0, count)
	for range count {
		id := r.ReadInt32()
		r.ReadUint16()
		r.ReadInt32()
		ids = append(ids, id)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AbnormalStatusUpdate: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1040 {
		t.Fatalf("owner icons after the return = %v, want [1040]", ids)
	}
	for _, e := range owner.EffectList().All() {
		if e.Skill.ID == contractPaymentID {
			t.Fatal("owner still holds Contract Payment after the return")
		}
	}
}
