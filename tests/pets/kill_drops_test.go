package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const dropMonsterAdena = 10

// dropMonsterTemplate is a monster with one guaranteed currency drop.
func dropMonsterTemplate() *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Drops: []item.DropCategory{
			{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: dropMonsterAdena, Max: dropMonsterAdena, Chance: 100}}},
		},
	}
}

// TestPetKillDropsLootProtectedToOwner pins the drop receiver of a pet kill
// to the pet's owner, the pet's acting player, whether the pet deals the
// whole lethal hit or only a final hit too small to count as reward damage.
func TestPetKillDropsLootProtectedToOwner(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*npc.Hostile, attackable.Combatant) bool{
		"lethal hit": func(monster *npc.Hostile, pet attackable.Combatant) bool {
			return monster.TakeDamage(1_000_000, pet)
		},
		"final one-point hit": func(monster *npc.Hostile, pet attackable.Combatant) bool {
			monster.TakeDamage(int(monster.CurrentHP())-1, nil)
			return monster.TakeDamage(1, pet)
		},
	}
	for name, kill := range cases {
		t.Run(name, func(t *testing.T) {
			h := bootOwnerWithCollar(t)
			pet, _ := h.spawnWolf(t)
			monster := h.srv.SpawnHostileNPCTemplateAt(t, dropMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, h.client)

			if !kill(monster, pet) {
				t.Fatal("lethal hit did not kill the monster")
			}

			drops := h.srv.GroundItems.Snapshots(func(int32) bool { return false })
			if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != dropMonsterAdena {
				t.Fatalf("ground drops = %+v, want one stack of %d adena", drops, dropMonsterAdena)
			}
			if drops[0].OwnerID != h.ownerID {
				t.Fatalf("drop protected to %d, want the pet owner %d", drops[0].OwnerID, h.ownerID)
			}
		})
	}
}
