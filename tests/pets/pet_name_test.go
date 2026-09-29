package pets

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestRenamePetFailsClosedOnLookupError covers a uniqueness lookup that
// cannot answer: the name is treated as taken, so the owner gets the
// already-in-use rejection (RequestChangePetName.doesPetNameExist starts from
// true and keeps it when the query throws) and the pet stays unnamed.
func TestRenamePetFailsClosedOnLookupError(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithPetNameLookupError(errors.New("pets table unavailable")),
	})
	actor, _ := h.spawnWolf(t)

	frames := renameTo(t, h, "Fenrir")
	messages := sysMessages(frames)
	if len(messages) != 1 {
		t.Fatalf("lookup-failure replies = %d system messages, want exactly one; opcodes %x", len(messages), frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, messages[0], serverpackets.SystemMessageNamingAlreadyInUseByAnotherPet)
	if countOpcode(frames, serverpackets.OpcodePetInfo) != 0 {
		t.Fatalf("lookup failure refreshed PetInfo: opcodes %x", frameOpcodes(frames))
	}
	if actor.IsNamed() || actor.Name() != "Wolf" {
		t.Fatalf("pet after failed lookup: Name()=%q IsNamed()=%v, want unnamed Wolf", actor.Name(), actor.IsNamed())
	}
	if state, ok, err := h.srv.Pets.Get(petCtx(), h.collarID); err != nil || (ok && state.Name != "") {
		t.Fatalf("pets row after failed lookup: state=%+v ok=%v err=%v, want no saved name", state, ok, err)
	}
}

// TestUnnamedPetStoresNullAndRestoresFromNull pins the unnamed-pet round trip
// through the shared pets schema: an unnamed pet is saved with a NULL name,
// and a row carrying a NULL name summons the pet unnamed instead of leaving
// the collar unusable.
func TestUnnamedPetStoresNullAndRestoresFromNull(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	actor, _ := h.spawnWolf(t)
	actor.AddExpAndSp(100, 0)
	wantExp := actor.Exp()
	h.returnPet(t)
	h.srv.FlushPersistence(t)

	var name sql.NullString
	if err := h.srv.DB.QueryRowContext(petCtx(), `SELECT name FROM pets WHERE item_obj_id = ?`, h.collarID).Scan(&name); err != nil {
		t.Fatalf("read pets.name: %v", err)
	}
	if name.Valid {
		t.Fatalf("unnamed pet saved pets.name = %q, want NULL", name.String)
	}

	respawned, burst := h.spawnWolf(t)
	if respawned.IsNamed() || respawned.Name() != "Wolf" {
		t.Fatalf("pet restored from NULL name: Name()=%q IsNamed()=%v, want unnamed Wolf", respawned.Name(), respawned.IsNamed())
	}
	if got := respawned.Exp(); got != wantExp {
		t.Fatalf("pet restored from NULL name Exp() = %d, want saved %d", got, wantExp)
	}
	if countOpcode(burst, serverpackets.OpcodePetInfo) == 0 {
		t.Fatalf("respawn burst has no PetInfo: opcodes %x", frameOpcodes(burst))
	}

	// The restored pet can still be named: the NULL row is an unnamed pet.
	renameTo(t, h, "Fenrir")
	if state := h.savedPetState(t); state.Name != "Fenrir" {
		t.Fatalf("pets row name after naming restored pet = %q, want Fenrir", state.Name)
	}
}
