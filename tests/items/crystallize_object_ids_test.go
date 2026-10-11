package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestCrystallizeOntoHeldCrystalsTakesNoObjectID: crystals joining the
// stack the player holds take no object id from the allocator, while the
// first crystals of their grade take exactly one.
func TestCrystallizeOntoHeldCrystalsTakesNoObjectID(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		held     int32 // D crystals held before
		wantIDs  int
		wantRows int
	}{
		{name: "held stack", held: 5, wantIDs: 0, wantRows: 1},
		{name: "new stack", wantIDs: 1, wantRows: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
			c := srv.Client
			objID := srv.SoleObjectID(t)
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, 248, 3); err != nil {
				t.Fatalf("grant crystallize skill: %v", err)
			}
			weapon := srv.GiveItem(t, objID, 30, 1)
			if tt.held > 0 {
				srv.GiveItem(t, objID, item.CrystalD.ItemID(), tt.held)
			}
			startInWorld(t, c)

			before := srv.IssuedObjectIDs()
			c.Send(encodeRequestCrystallizeItem(weapon, 1))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSystemMessage, "crystallized SystemMessage")
			assertPickedUpCrystals(t, c.Read(), item.CrystalD.ItemID(), 10)
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "crystallize UserInfo")

			if got := srv.IssuedObjectIDs() - before; got != tt.wantIDs {
				t.Fatalf("object ids spent by the crystallize = %d, want %d", got, tt.wantIDs)
			}
			srv.FlushItems(t)
			rows, count := 0, 0
			for _, inst := range persistedItems(t, srv, objID) {
				if inst.TemplateID == item.CrystalD.ItemID() {
					rows++
					count += inst.Count
				}
			}
			if rows != tt.wantRows || count != int(tt.held)+10 {
				t.Fatalf("crystal rows = %d holding %d, want %d holding %d", rows, count, tt.wantRows, tt.held+10)
			}
		})
	}
}
