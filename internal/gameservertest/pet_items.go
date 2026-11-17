package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
)

// GivePetItem adds count units of templateID to the active pet of the online
// player ownerID, on the owner's queue, where the pet's inventory is driven,
// and waits for it. The item is saved as one the pet picked up is: under its
// owner, at PET. A pet is called out empty, so this is how a suite starts
// from a pet already carrying items, past what a give or a pickup would
// accept. It returns the object id of the stack that holds them.
func (s *Server) GivePetItem(tb testing.TB, ownerID, templateID, count int32) int32 {
	tb.Helper()
	c := s.onlineCharacter(tb, ownerID)
	obj, ok := s.State.Summon(ownerID)
	if !ok {
		tb.Fatalf("player %d has no active summon", ownerID)
	}
	pet, ok := obj.(*summon.Actor)
	if !ok || pet.PetInventory() == nil {
		tb.Fatalf("summon of player %d = %T is not a pet", ownerID, obj)
	}
	objectID := s.NewObjectID()
	done := make(chan int32, 1)
	if !c.Queue().Post(func() {
		inst := pet.PetInventory().AddNew(templateID, int(count), objectID)
		if inst == nil {
			done <- 0
			return
		}
		done <- inst.Snapshot().ObjectID
	}) {
		tb.Fatalf("player %d's queue is closed", ownerID)
	}
	id := <-done
	if id == 0 {
		tb.Fatalf("pet of player %d refused %d of item %d", ownerID, count, templateID)
	}
	return id
}
