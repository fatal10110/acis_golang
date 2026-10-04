package combat

import (
	"cmp"
	"slices"
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
// drop. Both rolls are adena, but each category's roll lands as its own
// ground stack (Monster.doItemDrop drops per category), so a kill drops the
// currency stack and then the DROP stack, never one merged stack.
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
		want []int
	}{
		{"unsown", nil, []int{dropMonsterAdena, seededDropItemAdena}},
		{"sown", &manor.Seed{CropID: 5073, Level: 10}, []int{dropMonsterAdena}},
		{"sown with an alternative seed", &manor.Seed{CropID: 5818, Level: 10, Alternative: true}, []int{dropMonsterAdena, seededDropItemAdena}},
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

			// Object ids are allocated in drop order, so sorting by them
			// recovers the category order the stacks landed in.
			drops := groundDrops(srv)
			slices.SortFunc(drops, func(a, b item.GroundSnapshot) int { return cmp.Compare(a.ObjectID, b.ObjectID) })
			counts := make([]int, 0, len(drops))
			for _, d := range drops {
				if d.TemplateID != item.AdenaID {
					t.Fatalf("ground drops = %+v, want only adena", drops)
				}
				counts = append(counts, d.Count)
			}
			if !slices.Equal(counts, tc.want) {
				t.Fatalf("ground adena stacks = %v, want %v in category order", counts, tc.want)
			}
			if !monster.SpoilPool().Sweepable() {
				t.Fatal("spoil pool empty, want the spoil roll whatever the seed")
			}
		})
	}
}
