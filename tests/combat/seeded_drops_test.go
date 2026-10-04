package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Monster.calculateRewards (Monster.java:471-482). A monster
// sown with a seed that is not an alternative one skips its DROP
// categories; its CURRENCY, SPOIL and HERB categories still roll. An
// alternative seed blocks nothing.

// seededDropItemAdena is the guaranteed DROP-category roll of
// seededDropTemplate, beside its dropMonsterAdena currency roll.
const seededDropItemAdena = 7

// seededDropTemplate is dropMonsterTemplate plus a guaranteed ordinary item
// drop. Both rolls are adena, so a kill drops one stack holding whatever
// rolled.
func seededDropTemplate() *npc.Template {
	tmpl := dropMonsterTemplate()
	tmpl.Drops = append(tmpl.Drops, item.DropCategory{
		Kind: item.DropNormal, Chance: 100,
		Drops: []item.Drop{{ItemID: item.AdenaID, Min: seededDropItemAdena, Max: seededDropItemAdena, Chance: 100}},
	})
	return tmpl
}

func TestSeededMonsterDrops(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		seed *manor.Seed
		want int
	}{
		{"unsown", nil, dropMonsterAdena + seededDropItemAdena},
		{"sown", &manor.Seed{CropID: 5073, Level: 10}, dropMonsterAdena},
		{"sown with an alternative seed", &manor.Seed{CropID: 5818, Level: 10, Alternative: true}, dropMonsterAdena + seededDropItemAdena},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			monster := srv.SpawnHostileNPCTemplateAt(t, seededDropTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			if tc.seed != nil && !monster.SeedState().Sow(objID, *tc.seed) {
				t.Fatal("fresh monster already sown")
			}
			if !monster.SpoilPool().Mark(objID) {
				t.Fatal("spoil mark refused on a fresh monster")
			}
			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatal("player missing from world state")
			}
			player, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("player %T is not an online character", obj)
			}

			if !monster.TakeDamage(1_000_000, player) {
				t.Fatal("lethal hit did not kill the monster")
			}

			drops := groundDrops(srv)
			if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != tc.want {
				t.Fatalf("ground drops = %+v, want one stack of %d adena", drops, tc.want)
			}
			if !monster.SpoilPool().Sweepable() {
				t.Fatal("spoil pool empty, want the spoil roll whatever the seed")
			}
		})
	}
}
