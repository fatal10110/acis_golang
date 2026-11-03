package pets

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// hallTreasureBoxID is AzitWateringTreasureBox's NPC, a Rainbow Springs
// treasure box.
const hallTreasureBoxID = int32(35595)

// TestTreasureBoxCountsOnlyAnUnarmedPetsBlows ports the attacked hook of
// AzitWateringTreasureBox for a pet's blows: the box is invulnerable, and
// its script takes a playable's hit only when nothing is in its right hand.
// The wolf's hit of 22 takes 22 of the box's hit points while the wolf
// carries its weapon unequipped, and none once the owner has it put the
// weapon on.
func TestTreasureBoxCountsOnlyAnUnarmedPetsBlows(t *testing.T) {
	t.Parallel()
	for _, armed := range []bool{false, true} {
		t.Run(fmt.Sprintf("armed %v", armed), func(t *testing.T) {
			t.Parallel()
			wolf := wolfTemplate()
			wolf.ID = armedWolfNPCID
			box := &npc.Template{
				ID: int(hallTreasureBoxID), TemplateID: int(hallTreasureBoxID), Type: "Monster", Name: "Treasure Chest", Level: 1, HPMax: 1000,
				AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 10, CollisionHeight: 20,
			}
			catalog := script.Catalog{
				"script.ai.individual.DefaultNpc":                defaultnpc.New,
				"script.ai.siegablehall.AzitWateringTreasureBox": hall.TreasureBox,
			}
			list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.siegablehall.AzitWateringTreasureBox"}}
			h, pet, weaponID := bootArmedWolf(t,
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, box})),
				gameservertest.WithNpcSpawns(nil),
				gameservertest.WithNPCScripts(map[int32]script.NPCKind{hallTreasureBoxID: script.KindHostile}, list, catalog))
			if armed {
				h.sendAndTick(t, encodeRequestPetUseItem(weaponID))
				if pet.HeldItemTypeMask() == 0 {
					t.Fatal("setup: the wolf holds no weapon after putting it on")
				}
			}

			x, y, z := h.srv.PlayerPosition(t, h.ownerID)
			spawned, err := h.srv.NpcSpawns.AddSpawn(hallTreasureBoxID, x+100, y, z, 0, false, 0)
			if err != nil {
				t.Fatalf("spawn the box: %v", err)
			}
			h.srv.Settle(t)
			chest := spawned.(*npc.Hostile)
			if !chest.Invul() {
				t.Fatal("the box is not invulnerable as it appears")
			}
			full := chest.HP()

			runOnPetQueue(t, pet, func() { chest.TakeDamage(22, pet) })
			h.srv.Settle(t)
			want := full - 22
			if armed {
				want = full
			}
			if got := chest.HP(); got != want || chest.Dead() {
				t.Fatalf("box HP after the wolf's hit = %v (dead %v), want %v", got, chest.Dead(), want)
			}
		})
	}
}
