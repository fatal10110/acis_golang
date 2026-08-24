package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// renameTo sends RequestChangePetName and returns every frame the flow
// answered.
func renameTo(t *testing.T, h *petWorld, name string) [][]byte {
	t.Helper()
	h.client.Send(encodeRequestChangePetName(name))
	return drainFrames(t, h.client)
}

// sysMessages filters a drained burst down to its SystemMessage frames.
func sysMessages(frames [][]byte) [][]byte {
	var out [][]byte
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeSystemMessage {
			out = append(out, frame)
		}
	}
	return out
}

// TestRenamePetAppliesAndPersists renames an unnamed pet: the pet's name
// changes in world state and the pets row is written immediately.
func TestRenamePetAppliesAndPersists(t *testing.T) {
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)

	frames := renameTo(t, h, "Fenrir")
	var sawRefresh bool
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodePetInfo {
			sawRefresh = true
			if _, name := readPetInfoName(t, frame); name != "Fenrir" {
				t.Fatalf("refreshed PetInfo name = %q, want Fenrir", name)
			}
		}
	}
	if !sawRefresh {
		t.Fatalf("rename frames = opcodes %x, want a refreshed PetInfo", frameOpcodes(frames))
	}
	if got := pet.Name(); got != "Fenrir" {
		t.Fatalf("actor Name() = %q, want Fenrir", got)
	}
	if state := h.savedPetState(t); state.Name != "Fenrir" {
		t.Fatalf("pets row name = %q, want Fenrir", state.Name)
	}
}

// TestRenamePetValidationOrder walks the reference's rejection gates: empty
// length first, then invalid pattern; both leave the pet unnamed.
func TestRenamePetValidationOrder(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	frames := sysMessages(renameTo(t, h, ""))
	if len(frames) != 1 {
		t.Fatalf("empty-name rejections = %d, want exactly one", len(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNamingCharnameUpTo16Chars)

	frames = sysMessages(renameTo(t, h, "Re x!"))
	if len(frames) != 1 {
		t.Fatalf("invalid-pattern rejections = %d, want exactly one", len(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNamingPetnameContainsInvalidChars)

	if _, ok, err := h.srv.Pets.Get(petCtx(), h.collarID); ok || err != nil {
		t.Fatalf("pets row after rejections: ok=%v err=%v, want none", ok, err)
	}
}

// TestRenamePetRejectsTakenName seeds another pet's row with the target
// name: uniqueness is global across pets, answered with its own message.
func TestRenamePetRejectsTakenName(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)
	if err := h.srv.Pets.Save(petCtx(), 999999, pet.State{Name: "Fenrir", Level: wolfLevel}); err != nil {
		t.Fatalf("seed taken name: %v", err)
	}

	frames := sysMessages(renameTo(t, h, "Fenrir"))
	if len(frames) != 1 {
		t.Fatalf("taken-name rejections = %d, want exactly one", len(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNamingAlreadyInUseByAnotherPet)
}

// TestRenamePetNPCNameCollisionIsSilent covers the npc-name collision:
// naming a pet after an NPC template rejects silently, before any packet.
func TestRenamePetNPCNameCollisionIsSilent(t *testing.T) {
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)

	frames := renameTo(t, h, "Wolf")
	if len(frames) != 0 {
		t.Fatalf("npc-name collision frames = %d, want silent rejection", len(frames))
	}
	if got := pet.Name(); got != "Wolf" {
		t.Fatalf("Name() = %q after collision attempt, want unchanged", got)
	}
	if _, ok, err := h.srv.Pets.Get(petCtx(), h.collarID); ok || err != nil {
		t.Fatalf("pets row written by rejected rename: ok=%v err=%v", ok, err)
	}
}

// TestAlreadyNamedPetCannotBeRenamedAgain pins the once-only naming rule:
// a named pet rejects any further rename request with its own message.
func TestAlreadyNamedPetCannotBeRenamedAgain(t *testing.T) {
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	renameTo(t, h, "Fenrir")

	frames := sysMessages(renameTo(t, h, "Rex"))
	if len(frames) != 1 {
		t.Fatalf("second-rename rejections = %d, want exactly one", len(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNamingYouCannotSetNameOfThePet)
	if got := pet.Name(); got != "Fenrir" {
		t.Fatalf("Name() = %q, want unchanged Fenrir", got)
	}
}
