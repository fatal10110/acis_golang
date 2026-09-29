package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Player.cleanup unsummons the summon (Player.java:6279-6283);
// Pet.unSummon keeps a dead pet's inventory and leaves it in World._pets
// (Pet.java:340-356), and Summon.unSummon returns early for a dead summon.
// Player.restore relinks World.getPet(objectId) to the new Player
// (Player.java:4145-4151): setSummon(pet), pet.setOwner(player). aCis
// revision in the outer repo.

// TestReturningOwnerRevivesItsPetCorpse logs the owner out with a dead wolf
// carrying adena and back in before the corpse decays. The corpse is the new
// session's pet: the owner's resurrection revives it, and returning the
// revived wolf hands its adena to the new session and saves it alive.
func TestReturningOwnerRevivesItsPetCorpse(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	srv := bootPets(t,
		gameservertest.WithSkills(petResurrectionTable(t)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{penaltyWolfTemplate(), treeTemplate()})),
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
	)
	decay.attach(srv.State)
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, petResurrectSkillID, 1); err != nil {
		t.Fatalf("seed resurrection: %v", err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	if err := srv.Pets.Save(petCtx(), collarID, pet.State{
		Level: wolfLevel, Exp: 510, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: wolfMaxMeal,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	adenaID := srv.GiveItem(t, ownerID, item.AdenaID, 40)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)

	wolf, _ := h.spawnWolf(t)
	h.giveToPet(t, adenaID, 40)
	killPet(t, h, wolf)
	h.relogOwner(t)

	if obj, ok := h.srv.State.Summon(h.ownerID); !ok || obj.ObjectID() != wolf.ObjectID() {
		t.Fatal("returning owner's summon slot does not hold its pet's corpse")
	}
	if wolf.OwnerLeft() {
		t.Fatal("pet corpse still answers to the session that left it")
	}
	if got := petItemCount(wolf, item.AdenaID); got != 40 {
		t.Fatalf("corpse carries %d adena after its owner came back, want 40", got)
	}

	castResurrection(t, h.srv, h.client, h.ownerID, wolf.ObjectID())
	frames := drainFrames(t, h.client)
	if wolf.Dead() {
		t.Fatal("returning owner's resurrection left its pet's corpse dead")
	}
	if frameIndex(frames, serverpackets.OpcodeRevive, wolf.ObjectID()) < 0 {
		t.Fatal("returning owner never saw its pet revive")
	}
	decay.passAndTick(t, h.srv, 1201*time.Second)
	if obj, ok := h.srv.State.Summon(h.ownerID); !ok || obj.ObjectID() != wolf.ObjectID() {
		t.Fatal("revived pet decayed at its old corpse deadline")
	}

	h.returnPet(t)
	if inst := h.ownerInventory(t).ItemByTemplateID(item.AdenaID); inst == nil || inst.Snapshot().Count != 40 {
		t.Fatal("returning the revived pet did not hand its 40 adena to the new session")
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("owner adena rows after the return = %d, want 40", got)
	}
	if got := h.collarItemCount(t, item.AdenaID); got != 0 {
		t.Fatalf("adena still saved under the collar after the return = %d, want none", got)
	}
	if got := h.savedPetState(t).CurHP; got <= 0 {
		t.Fatalf("returned pet saved with HP %v, want it saved alive", got)
	}
}
