package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestDeadPetItemsSavedUnderOwnerWhileCorpseWaits gives the wolf adena, kills
// it and takes its owner back to character select. The corpse keeps its items
// until it decays, and their row is saved under the owner, at PET: what a
// restart then finds in the table. No row is saved under the collar.
func TestDeadPetItemsSavedUnderOwnerWhileCorpseWaits(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 100})
	wolf, _ := h.spawnWolf(t)
	h.giveToPet(t, h.seededItem(t, item.AdenaID), 30)
	killPet(t, h, wolf)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	if got := petItemCount(wolf, item.AdenaID); got != 30 {
		t.Fatalf("waiting corpse carries %d adena, want 30", got)
	}
	if got := h.petRowCount(t, item.AdenaID); got != 30 {
		t.Fatalf("adena rows under the owner at a pet location = %d, want 30", got)
	}
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), h.collarID)
	if err != nil {
		t.Fatalf("list collar rows: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows saved under the collar = %+v, want none", rows)
	}
}

// TestLeftPetRowsStayOutOfOwnerAndNextPet starts from the rows a restart
// leaves behind a pet that still carried items (a corpse waiting for its
// owner, or a crash): owner_id is the owner, loc PET and PET_EQUIP. Neither
// the owner's login nor the next pet called out of the collar loads them, and
// the rows stay as they were.
func TestLeftPetRowsStayOutOfOwnerAndNextPet(t *testing.T) {
	t.Parallel()
	srv := bootPets(t)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	left := []item.Instance{
		{ObjectID: srv.NewObjectID(), TemplateID: item.AdenaID, OwnerID: ownerID, Count: 40, Location: item.LocationPet},
		{ObjectID: srv.NewObjectID(), TemplateID: 30, OwnerID: ownerID, Count: 1, Location: item.LocationPetEquip},
	}
	for _, inst := range left {
		if err := srv.Items.Create(context.Background(), ownerID, inst); err != nil {
			t.Fatalf("seed left pet row: %v", err)
		}
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)

	inv := h.ownerInventory(t)
	for _, inst := range left {
		if inv.ItemByObjectID(inst.ObjectID) != nil {
			t.Fatalf("owner's login restored the left pet row %d", inst.ObjectID)
		}
	}
	wolf, _ := h.spawnWolf(t)
	if items := wolf.PetInventory().Items(); len(items) != 0 {
		t.Fatalf("wolf called out carries %d items, want none", len(items))
	}
	h.returnPet(t)

	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), ownerID)
	if err != nil {
		t.Fatalf("list owner rows: %v", err)
	}
	for _, want := range left {
		found := false
		for _, row := range rows {
			if row.ObjectID == want.ObjectID {
				found = true
				if row.Location != want.Location || row.Count != want.Count || row.OwnerID != ownerID {
					t.Fatalf("left pet row %d = %+v, want it unchanged", want.ObjectID, row)
				}
			}
		}
		if !found {
			t.Fatalf("left pet row %d is gone", want.ObjectID)
		}
	}
}
