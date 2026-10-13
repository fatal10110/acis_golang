package pets

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// Reference: Summon.doUnsummon (Summon.java:558-569) aborts the summon's
// actions, stops its regeneration, then stops all its effects before
// storing it and sending PetDelete. Each effect stopped refreshes the
// summon's icons (EffectList.queueRunner -> updateEffectIcons,
// EffectList.java:465-497, 778-864); Summon.stopAllEffects then sends the
// owner PetInfo and the icons again (Summon.java:376-382, 598-604), the
// icons only for a summon whose list ever held an effect
// (EffectList.java:428-431). A pet's stored row carries no effects
// (Pet.store, Pet.java:311-337).

// spelledCount is the number of icons a PartySpelled frame lists.
func spelledCount(t *testing.T, frame []byte) uint32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePartySpelled, "PartySpelled")
	return binary.LittleEndian.Uint32(frame[9:])
}

// TestReturnedBuffedPetStopsItsEffectsBeforePetDelete returns a pet holding
// a P.Atk. buff. The buff ends as the pet leaves, with its stat change
// undone: the owner sees its icon go (an empty PartySpelled), then the pet
// window and the empty icon list again, and only then PetDelete. The pet
// summoned again from its saved row holds no effect.
func TestReturnedBuffedPetStopsItsEffectsBeforePetDelete(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	basePAtk := pet.PAtk()
	might := mightOn(t, pet)
	addToPet(t, pet, might)
	drainUntilQuiet(t, h.client)
	if pet.PAtk() == basePAtk {
		t.Fatal("might did not raise the pet's P.Atk.")
	}

	h.client.Send(encodeRequestActionUse(19, false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	got := ofOpcodes(frames, serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled, serverpackets.OpcodePetDelete)
	want := []byte{serverpackets.OpcodePartySpelled, serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled, serverpackets.OpcodePetDelete}
	if !slices.Equal(frameOpcodes(got), want) {
		t.Fatalf("return frames = %x, want PartySpelled, PetInfo, PartySpelled, PetDelete (%x)", frameOpcodes(got), want)
	}
	for _, i := range []int{0, 2} {
		if n := spelledCount(t, got[i]); n != 0 {
			t.Fatalf("departure PartySpelled #%d lists %d icons, want 0: %x", i, n, got[i])
		}
	}
	if held := pet.EffectList().All(); len(held) != 0 {
		t.Fatalf("returned pet still holds %d effects", len(held))
	}
	if might.InUse() {
		t.Fatal("might still in use on the returned pet")
	}
	if got := pet.PAtk(); got != basePAtk {
		t.Fatalf("returned pet P.Atk. = %v, want %v with might's stat removed", got, basePAtk)
	}

	h.srv.AdvanceUntil(t, "pets row saved", func() bool {
		_, ok, err := h.srv.Pets.Get(petCtx(), h.collarID)
		return err == nil && ok
	})
	drainUntilQuiet(t, h.client)
	again, _ := h.spawnWolf(t)
	if again.EffectList().HasHeld() {
		t.Fatalf("re-summoned pet came back holding %d effects", len(again.EffectList().All()))
	}
}

// TestReturnedUnbuffedPetSendsPetInfoWithoutIcons returns a pet that never
// held an effect: stopping its effects still shows its owner the pet window
// before PetDelete, but no icon list.
func TestReturnedUnbuffedPetSendsPetInfoWithoutIcons(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestActionUse(19, false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	got := ofOpcodes(frames, serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled, serverpackets.OpcodePetDelete)
	if want := []byte{serverpackets.OpcodePetInfo, serverpackets.OpcodePetDelete}; !slices.Equal(frameOpcodes(got), want) {
		t.Fatalf("return frames = %x, want PetInfo then PetDelete (%x)", frameOpcodes(got), want)
	}
}

// TestReturnedBuffedPetClearsItsIconsForTheParty returns a partied owner's
// buffed pet: every member, the owner and Mate, sees the pet's icon list
// emptied.
func TestReturnedBuffedPetClearsItsIconsForTheParty(t *testing.T) {
	t.Parallel()
	h, wolf, mate, _ := partyPet(t, party.LootFindersKeepers)
	addToPet(t, wolf, mightOn(t, wolf))
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, mate)

	h.client.Send(encodeRequestActionUse(19, false))
	owner := readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	for who, frames := range map[string][][]byte{"owner": owner, "mate": drainFrames(t, mate)} {
		spelled := ofOpcodes(frames, serverpackets.OpcodePartySpelled)
		if len(spelled) != 2 {
			t.Fatalf("%s got %d PartySpelled of the departing pet, want 2: opcodes %x", who, len(spelled), frameOpcodes(frames))
		}
		for _, f := range spelled {
			if n := spelledCount(t, f); n != 0 {
				t.Fatalf("%s departure PartySpelled lists %d icons, want 0: %x", who, n, f)
			}
		}
	}
}

// TestBuffedServitorLifetimeEndsWithTrailingEmptyIcons runs a buffed
// servitor's lifetime out on a tick that shows the remaining time. The buff
// stops before PetDelete, emptying its icons, and after PetDelete the owner
// still reads the remaining time and then the servitor's empty icon list
// (Servitor.processLifeTime, Servitor.java:230-255, does not return after
// unSummon).
func TestBuffedServitorLifetimeEndsWithTrailingEmptyIcons(t *testing.T) {
	t.Parallel()
	def := lifetimeServitorSkill()
	def.SummonTotalLifeTime, def.ItemConsumeIDOT, def.ItemConsumeCountOT, def.ItemConsumeSteps = 2000, 0, 0, 0
	o := bootLifetimeServitorOwner(t, def)
	servitor := o.summonServitor(t)
	o.buffServitor(t, servitor)

	o.tickServitor(t, 4)
	frames := o.tickServitor(t, 1)
	assertTrailingRemainTime(t, frames, [2]int32{2000, -500}, true)
	gone := opcodeIndex(frames, serverpackets.OpcodePetDelete)
	before := ofOpcodes(frames[:gone], serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled)
	if want := []byte{serverpackets.OpcodePartySpelled, serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled}; !slices.Equal(frameOpcodes(before), want) {
		t.Fatalf("frames before PetDelete = %x, want PartySpelled, PetInfo, PartySpelled (%x)", frameOpcodes(before), want)
	}
	for _, f := range before {
		if f[0] == serverpackets.OpcodePartySpelled && spelledCount(t, f) != 0 {
			t.Fatalf("departure PartySpelled lists icons, want none: %x", f)
		}
	}
	if held := servitor.EffectList().All(); len(held) != 0 {
		t.Fatalf("departed servitor still holds %d effects", len(held))
	}
}
