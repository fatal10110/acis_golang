package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestCollarUseSpawnsWolfBesideOwner drives the whole pet-spawn chain from
// the item window: the collar use casts SUMMON_CREATURE, and its Hit phase
// spawns a live wolf beside the owner — announced to the owner with
// SUMMON_A_PET, MagicSkillUse cast by the player, then PetInfo +
// PetItemList once the spawn lands — with no pets row written yet.
func TestCollarUseSpawnsWolfBesideOwner(t *testing.T) {
	h := bootOwnerWithCollar(t)

	pet, burst := h.spawnWolf(t)

	if !pet.IsPet() {
		t.Fatalf("summon IsPet() = false for NPCID %d", pet.NPCID())
	}
	if got := pet.NPCID(); got != wolfNPCID {
		t.Fatalf("pet NPCID() = %d, want %d", got, wolfNPCID)
	}
	if got := pet.Name(); got != "Wolf" {
		t.Fatalf("fresh pet Name() = %q, want template name", got)
	}
	if got := pet.Level(); got != wolfLevel {
		t.Fatalf("fresh pet Level() = %d, want template level %d", got, wolfLevel)
	}
	if pet.PetInventory() == nil {
		t.Fatal("spawned pet has no carried-item inventory")
	}
	if _, ok, err := h.srv.Pets.Get(petCtx(), h.collarID); ok || err != nil {
		t.Fatalf("pets row after first spawn: ok=%v err=%v, want none until a save point", ok, err)
	}

	// The owner's own spawn burst carries PetInfo naming the pet ahead of
	// the empty PetItemList.
	var sawItemList bool
	for _, frame := range burst {
		if frame[0] == serverpackets.OpcodePetInfo {
			summonType, name := readPetInfoName(t, frame)
			if name != "Wolf" {
				t.Fatalf("PetInfo name = %q, want Wolf", name)
			}
			if summonType != int32(pet.SummonType()) {
				t.Fatalf("PetInfo summon type = %d, want pet type %d", summonType, pet.SummonType())
			}
		}
		if frame[0] == serverpackets.OpcodePetItemList {
			sawItemList = true
		}
	}
	if !sawItemList {
		t.Fatalf("spawn burst missing PetItemList: opcodes %x", frameOpcodes(burst))
	}
	drainUntilQuiet(t, h.client)
}

// TestSecondCollarUseWhilePetActiveAnswersSummonOnlyOne re-uses the collar
// while the wolf is out: the cast runs again but the spawn is rejected with
// SUMMON_ONLY_ONE and the original wolf stays.
func TestSecondCollarUseWhilePetActiveAnswersSummonOnlyOne(t *testing.T) {
	h := bootOwnerWithCollar(t)
	first, _ := h.spawnWolf(t)

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET"), serverpackets.SystemMessageSummonAPet)
	frame := mustRead(t, h.client, "second collar MagicSkillUse")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")

	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeSystemMessage, "SUMMON_ONLY_ONE")
	assertStaticSystemMessage(t, frames[len(frames)-1], serverpackets.SystemMessageSummonOnlyOne)

	again, ok := h.srv.State.Summon(h.ownerID)
	if !ok {
		t.Fatal("original wolf despawned by rejected second summon")
	}
	if again.(*summon.Actor).ObjectID() != first.ObjectID() {
		t.Fatalf("active summon id = %d, want original %d", again.(*summon.Actor).ObjectID(), first.ObjectID())
	}
	drainUntilQuiet(t, h.client)
}

// TestUnsummonShortcutOnAPetStaysSilent pins the command split: action 52
// is the servitor unsummon shortcut; on a pet it resolves as ignored —
// silent, no despawn, no persistence write. Only the return command (19)
// sends a pet back into its collar.
func TestUnsummonShortcutOnAPetStaysSilent(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	h.client.Send(encodeRequestActionUse(52, false))
	frames := drainFrames(t, h.client)
	if len(frames) != 0 {
		t.Fatalf("unsummon action on a pet answered %d frames, want silence", len(frames))
	}
	if _, ok := h.srv.State.Summon(h.ownerID); !ok {
		t.Fatal("unsummon shortcut despawned a pet; only the return command may")
	}
	if _, ok, err := h.srv.Pets.Get(petCtx(), h.collarID); ok || err != nil {
		t.Fatalf("pets row written by ignored unsummon: ok=%v err=%v", ok, err)
	}
}

// TestReturnCommandDespawnsPetAndSavesRow sends the return shortcut
// (action 19): after the pet's StopMove, the owner gets PetDelete, world
// state drops the summon, and the collar's pets row persists full vitals.
func TestReturnCommandDespawnsPetAndSavesRow(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	h.returnPet(t)

	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("world still tracks the summon after return")
	}
	state := h.savedPetState(t)
	if state.Name != "" || state.Level != wolfLevel {
		t.Fatalf("saved state = %+v, want default (unnamed) level %d", state, wolfLevel)
	}
	if state.CurHP != wolfMaxHP || state.Fed != wolfMaxMeal {
		t.Fatalf("saved vitals = hp %v mp %v fed %d, want hp %v fed %d", state.CurHP, state.CurMP, state.Fed, wolfMaxHP, wolfMaxMeal)
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("owner collar count = %d, want 1 (the collar is never consumed)", got)
	}
}

// TestRespawnAfterSaveRestoresSavedName renames through the rename flow,
// returns the pet, then respawns: the restored wolf carries the persisted
// name instead of the template default.
func TestRespawnAfterSaveRestoresSavedName(t *testing.T) {
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)

	renameTo(t, h, "Fenrir")
	if got := pet.Name(); got != "Fenrir" {
		t.Fatalf("renamed actor Name() = %q, want Fenrir", got)
	}
	h.returnPet(t)

	respawned, burst := h.spawnWolf(t)
	if got := respawned.Name(); got != "Fenrir" {
		t.Fatalf("respawned pet Name() = %q, want restored Fenrir", got)
	}
	for _, frame := range burst {
		if frame[0] == serverpackets.OpcodePetInfo {
			if _, name := readPetInfoName(t, frame); name != "Fenrir" {
				t.Fatalf("respawn PetInfo name = %q, want Fenrir", name)
			}
		}
	}
	if state := h.savedPetState(t); state.Name != "Fenrir" {
		t.Fatalf("pets row name = %q, want Fenrir", state.Name)
	}
}
