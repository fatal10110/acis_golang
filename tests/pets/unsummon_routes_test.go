package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRestartTakesPetOutOfWorldAndAllowsResummon covers the logout path: the
// pet leaves with its owner, observers are told it is gone, its row keeps
// the live state, and the owner can call it again on the next login instead
// of being refused because a ghost still holds the summon slot.
func TestRestartTakesPetOutOfWorldAndAllowsResummon(t *testing.T) {
	// No character-select reuse delay: the relog below follows at once.
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithReuseDelays(0, 0)})
	pet, _ := h.spawnWolf(t)
	petID := pet.ObjectID()

	h.srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, h.client)

	pet.SetHP(37)
	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")

	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("world still tracks the owner's summon after restart")
	}
	if _, ok := h.srv.State.Object(petID); ok {
		t.Fatal("pet still on the grid after its owner left")
	}
	if !sawDeleteObject(drainFrames(t, observer), petID) {
		t.Fatalf("observer got no DeleteObject for pet %d", petID)
	}
	if got := h.savedPetState(t).CurHP; got != 37 {
		t.Fatalf("saved pet HP after restart = %v, want 37", got)
	}

	startInWorld(t, h.client)
	again, _ := h.spawnWolf(t)
	if got := again.HP(); got != 37 {
		t.Fatalf("resummoned pet HP = %v, want the saved 37", got)
	}
}

// TestHostileUnsummonRoutesSettleThePet covers the despawns no owner
// packet drives: Erase (UnSummon with the owner, as the disabler calls it)
// and the anti-summon signet (Unsummon). Each must save the pets row from
// the live state, lift the collar, and hand the pet's items back, exactly
// as the owner's own return command does.
func TestHostileUnsummonRoutesSettleThePet(t *testing.T) {
	routes := []struct {
		name string
		do   func(*summon.Actor)
	}{
		{"erase", func(a *summon.Actor) { a.UnSummon(a.SummonOwner()) }},
		{"signet", func(a *summon.Actor) { a.Unsummon() }},
	}
	for _, tc := range routes {
		t.Run(tc.name, func(t *testing.T) {
			h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 40})
			pet, _ := h.spawnWolf(t)
			h.giveToPet(t, h.seededItem(t, item.AdenaID), 40)
			pet.SetHP(37)
			pet.AddExpAndSp(1, 0)
			pet.TickPet(h.srv.State)
			exp, fed := pet.Exp(), pet.Fed()
			if fed == wolfMaxMeal {
				t.Fatal("precondition: the feed tick left the gauge full")
			}
			setCollarEnchant(t, h, 0)

			tc.do(pet)
			readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")

			if _, ok := h.srv.State.Summon(h.ownerID); ok {
				t.Fatal("world still tracks the summon")
			}
			saved := h.savedPetState(t)
			if saved.CurHP != 37 || saved.Exp != exp || saved.Fed != fed {
				t.Fatalf("saved pet = hp %v exp %d fed %d, want hp 37 exp %d fed %d", saved.CurHP, saved.Exp, saved.Fed, exp, fed)
			}
			if got := h.liveCollarEnchant(t); got != wolfLevel {
				t.Fatalf("live collar enchant = %d, want pet level %d", got, wolfLevel)
			}
			if got := h.persistedCollarEnchant(t); got != wolfLevel {
				t.Fatalf("persisted collar enchant = %d, want pet level %d", got, wolfLevel)
			}
			if got := h.ownerItemCount(t, item.AdenaID); got != 40 {
				t.Fatalf("owner adena after despawn = %d, want the pet's 40 back", got)
			}
		})
	}
}

// TestHostileUnsummonLeavesDeadPetAlone keeps the rule that a dead summon is
// not unsummoned: its corpse stays, and nothing is settled for it.
func TestHostileUnsummonLeavesDeadPetAlone(t *testing.T) {
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	killPet(t, h, pet)

	pet.UnSummon(pet.SummonOwner())
	pet.Unsummon()

	if got, ok := h.srv.State.Summon(h.ownerID); !ok || got.ObjectID() != pet.ObjectID() {
		t.Fatal("hostile unsummon took a dead pet out of the world")
	}
}

// TestRestartWithDeadPetReturnsItemsAndKeepsItDead covers a dead pet leaving
// with its owner: its container is keyed by an object id no later summon
// reuses, so its items must come back to the owner, and its row must record
// the death rather than leave an older, living save to restore from.
func TestRestartWithDeadPetReturnsItemsAndKeepsItDead(t *testing.T) {
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithReuseDelays(0, 0)},
		seedItem{TemplateID: item.AdenaID, Count: 40})
	pet, _ := h.spawnWolf(t)
	h.giveToPet(t, h.seededItem(t, item.AdenaID), 40)
	killPet(t, h, pet)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")

	if got := h.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("owner adena after restart = %d, want the dead pet's 40 back", got)
	}
	if got := h.savedPetState(t).CurHP; got != 0 {
		t.Fatalf("saved dead pet HP = %v, want 0", got)
	}

	startInWorld(t, h.client)
	again, _ := h.spawnWolf(t)
	if got := again.HP(); got != 0 {
		t.Fatalf("resummoned pet HP = %v, want the dead pet's 0", got)
	}
}

// killPet lands lethal damage on pet from its owner and waits for the death.
func killPet(t *testing.T, h *petWorld, pet *summon.Actor) {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}
	pet.ReduceHP(pet.HP()+1, obj.(attackable.Combatant), modelskill.Definition{})
	h.srv.AdvanceUntil(t, "pet dead", pet.Dead)
	drainUntilQuiet(t, h.client)
}

func sawDeleteObject(frames [][]byte, objectID int32) bool {
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeDeleteObject && wire.NewReader(f[1:]).ReadInt32() == objectID {
			return true
		}
	}
	return false
}

// setCollarEnchant puts the owner's collar out of step with the pet, so a
// despawn that settles the pet has something to lift.
func setCollarEnchant(t *testing.T, h *petWorld, level int) {
	t.Helper()
	inv := h.ownerInventory(t)
	inv.SetEnchantLevel(inv.ItemByObjectID(h.collarID), level)
	drainUntilQuiet(t, h.client)
}
