package pets

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Reference: EffectList.updateEffectIcons (EffectList.java:778-864) builds,
// for a summon, a PartySpelled of its in-use icon effects and sends it to
// its owner's party, or to its owner alone outside one. PartySpelled
// (opcode 0xee) writes D type (1 pet, 2 servitor), D object id, D count,
// then per effect D skill id, H skill level, D remaining ms / 1000.
// Summon.sendPetInfosToOwner / updateAndBroadcastStatusAndInfos /
// sendInfo(owner) resend it after PetInfo, which clears it on the client
// (Summon.java:357-367, 589-604), unless the list never held an effect
// (EffectList.java:428-431).

// petPartySpelledBytes is the expected PartySpelled of a pet with one
// effect, written field by field from the reference layout.
func petPartySpelledBytes(petID, skillID int32, level uint16, seconds int32) []byte {
	b := []byte{0xee}
	b = binary.LittleEndian.AppendUint32(b, 1)
	b = binary.LittleEndian.AppendUint32(b, uint32(petID))
	b = binary.LittleEndian.AppendUint32(b, 1)
	b = binary.LittleEndian.AppendUint32(b, uint32(skillID))
	b = binary.LittleEndian.AppendUint16(b, level)
	return binary.LittleEndian.AppendUint32(b, uint32(seconds))
}

// samePetIcons reports whether got is want, a one-effect icon list, but
// for its seconds left, which may have ticked one lower before the list
// was read.
func samePetIcons(got, want []byte) bool {
	if len(got) != len(want) {
		return false
	}
	n := len(want) - 4
	secs, wantSecs := int32(binary.LittleEndian.Uint32(got[n:])), int32(binary.LittleEndian.Uint32(want[n:]))
	return bytes.Equal(got[:n], want[:n]) && (secs == wantSecs || secs == wantSecs-1)
}

// mightOn builds a 30 s P.Atk. buff (skill 1068 level 1) cast by pet on
// itself.
func mightOn(t *testing.T, pet *summon.Actor) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1068, Level: 1}, modelskill.EffectTemplate{
		Name: "Buff", Time: 30, Icon: true, StackType: "pa_up", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "pAtk", Value: 2}},
	})
	if err != nil {
		t.Fatalf("effect.New(might): %v", err)
	}
	e.Effector, e.Effected = pet, pet
	return e
}

// ofOpcodes keeps the frames whose opcode is in opcodes.
func ofOpcodes(frames [][]byte, opcodes ...byte) [][]byte {
	return slices.DeleteFunc(slices.Clone(frames), func(f []byte) bool { return !slices.Contains(opcodes, f[0]) })
}

// TestPetStatBuffShowsIconsOnce lands a P.Atk. buff on a partyless owner's
// pet, then removes it. Each stat change republishes the pet window
// (PetInfo, then PetStatusUpdate) from inside the effect list's pass, and
// the pass's own icon refresh follows: the owner gets PetInfo,
// PetStatusUpdate, PartySpelled, with no second PartySpelled. The
// reference's resend after PetInfo (Summon.updateAndBroadcastStatusAndInfos)
// re-enters EffectList.queueRunner while queueLock is held and sends
// nothing (EffectList.java:465-497). Before any effect, the pet's spawn sent
// no PartySpelled.
func TestPetStatBuffShowsIconsOnce(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, burst := h.spawnWolf(t)
	if _, ok := firstOpcode(burst, serverpackets.OpcodePartySpelled); ok {
		t.Fatalf("spawn of a pet that never held an effect sent PartySpelled: opcodes %x", frameOpcodes(burst))
	}
	drainUntilQuiet(t, h.client)

	want := []byte{serverpackets.OpcodePetInfo, serverpackets.OpcodePetStatusUpdate, serverpackets.OpcodePartySpelled}
	might := mightOn(t, pet)
	addToPet(t, pet, might)
	got := ofOpcodes(drainFrames(t, h.client), serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled, serverpackets.OpcodePetStatusUpdate)
	if !slices.Equal(frameOpcodes(got), want) {
		t.Fatalf("buff frames = %x, want PetInfo, PetStatusUpdate, PartySpelled (%x)", frameOpcodes(got), want)
	}
	if wantBytes := petPartySpelledBytes(pet.ObjectID(), 1068, 1, 30); !samePetIcons(got[2], wantBytes) {
		t.Fatalf("PartySpelled = %x, want %x", got[2], wantBytes)
	}

	onPetQueue(t, pet, func() { pet.EffectList().Remove(might) })
	got = ofOpcodes(drainFrames(t, h.client), serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled, serverpackets.OpcodePetStatusUpdate)
	if !slices.Equal(frameOpcodes(got), want) {
		t.Fatalf("removal frames = %x, want PetInfo, PetStatusUpdate, PartySpelled (%x)", frameOpcodes(got), want)
	}
	if n := binary.LittleEndian.Uint32(got[2][9:]); n != 0 {
		t.Fatalf("removal PartySpelled lists %d effects, want 0: %x", n, got[2])
	}
}

// TestPetRenameResendsIcons renames a buffed pet. The rename's PetInfo comes
// from outside any effect-list pass (RequestChangePetName ->
// Summon.sendPetInfosToOwner), so the icons it clears are resent right
// after it.
func TestPetRenameResendsIcons(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	addToPet(t, pet, mightOn(t, pet))
	drainUntilQuiet(t, h.client)

	got := ofOpcodes(renameTo(t, h, "Fenrir"), serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled)
	if want := []byte{serverpackets.OpcodePetInfo, serverpackets.OpcodePartySpelled}; !slices.Equal(frameOpcodes(got), want) {
		t.Fatalf("rename frames = %x, want PetInfo then PartySpelled (%x)", frameOpcodes(got), want)
	}
	if want := petPartySpelledBytes(pet.ObjectID(), 1068, 1, 30); !samePetIcons(got[1], want) {
		t.Fatalf("rename PartySpelled = %x, want %x", got[1], want)
	}
}

// TestPetIconsReachOwnersParty lands a buff on a partied owner's pet:
// every member, the owner and Mate, gets exactly one PartySpelled of the
// pet, from the effect list's icon pass.
func TestPetIconsReachOwnersParty(t *testing.T) {
	t.Parallel()
	h, wolf, mate, _ := partyPet(t, party.LootFindersKeepers)
	drainUntilQuiet(t, mate)

	addToPet(t, wolf, mightOn(t, wolf))
	want := petPartySpelledBytes(wolf.ObjectID(), 1068, 1, 30)
	for who, frames := range map[string][][]byte{"owner": drainFrames(t, h.client), "mate": drainFrames(t, mate)} {
		spelled := ofOpcodes(frames, serverpackets.OpcodePartySpelled)
		if len(spelled) != 1 {
			t.Fatalf("%s got %d PartySpelled of the pet, want 1: opcodes %x", who, len(spelled), frameOpcodes(frames))
		}
		if !samePetIcons(spelled[0], want) {
			t.Fatalf("%s PartySpelled = %x, want %x", who, spelled[0], want)
		}
	}
}
