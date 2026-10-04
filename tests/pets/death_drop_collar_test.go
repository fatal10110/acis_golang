package pets

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Catalog items the owner carries next to the wolf collar: a potion no
// list keeps and a tunic on the kept list.
const (
	deathDropPotionID int32 = 20
	deathDropTunicID  int32 = 40
)

// TestDeathDropKeepsSummonedPetCollar has the owner killed by a monster
// with every death-drop rate at 100 and only the tunic on the kept list.
// The potion drops and the tunic stays either way. The wolf collar, which
// no list keeps, drops when its pet is not out, but stays while the wolf
// is summoned: the item of a pet that is out never leaves its owner.
func TestDeathDropKeepsSummonedPetCollar(t *testing.T) {
	t.Parallel()
	every := player.DeathDropRates{Chance: 100, Equip: 100, EquipWeapon: 100, Item: 100, Limit: 10}
	for _, tt := range []struct {
		name       string
		summon     bool
		wantGround []int32
	}{
		{name: "pet not out", wantGround: []int32{deathDropPotionID, wolfCollarID}},
		{name: "pet summoned", summon: true, wantGround: []int32{deathDropPotionID}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootPets(t,
				gameservertest.WithItemTemplates(discardableCollarItems()),
				gameservertest.WithDeathDrop(player.DeathDropRules{Monster: every, Kept: []int32{deathDropTunicID}}),
			)
			ownerID := srv.SoleObjectID(t)
			// A monster kill costs items only from level 5 up.
			if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET level = 10 WHERE obj_Id = ?", ownerID); err != nil {
				t.Fatalf("raise owner level: %v", err)
			}
			h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, seeded: map[int32][]int32{}}
			h.collarID = srv.GiveItem(t, ownerID, wolfCollarID, 1)
			srv.GiveItem(t, ownerID, deathDropPotionID, 5)
			tunicID := srv.GiveItem(t, ownerID, deathDropTunicID, 1)
			startInWorld(t, h.client)
			if tt.summon {
				h.spawnWolf(t)
			}
			drainUntilQuiet(t, h.client)

			var killer attackable.Combatant = srv.SpawnHostileNPC(t)
			owner := h.character(t)
			done := make(chan struct{})
			if !srv.PlayerQueue(t, ownerID).Post(func() { owner.Kill(killer); close(done) }) {
				t.Fatal("post: queue closed")
			}
			<-done
			srv.Settle(t)

			var ground []int32
			for _, g := range srv.GroundItems.Snapshots(nil) {
				ground = append(ground, g.TemplateID)
			}
			slices.Sort(ground)
			want := slices.Clone(tt.wantGround)
			slices.Sort(want)
			if !slices.Equal(ground, want) {
				t.Fatalf("ground items = %v, want %v", ground, want)
			}
			inv := h.ownerInventory(t)
			if inv.ItemByObjectID(tunicID) == nil {
				t.Fatal("kept tunic left the owner's inventory")
			}
			if collar := inv.ItemByObjectID(h.collarID); tt.summon && collar == nil {
				t.Fatal("summoned wolf's collar left the owner's inventory")
			}
		})
	}
}
