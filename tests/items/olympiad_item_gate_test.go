package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped items the Olympiad bars: a hero weapon the datapack also flags
// is_oly_restricted, a potion it flags, and the hero circlet, which carries
// no flag.
const (
	infinityBladeID int32 = 6611 // hero weapon, is_oly_restricted
	manaPotionID    int32 = 728  // potion, is_oly_restricted
	heroCircletID   int32 = 6842 // hero item
)

// bootOlympiadItems boots "Newbie", a hero when hero is set, with the
// shipped Olympiad-barred items.
func bootOlympiadItems(t *testing.T, hero bool) *gameservertest.Server {
	t.Helper()
	datapack.Require(t)
	_, shippedItems := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{infinityBladeID, manaPotionID, heroCircletID} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 40, 0),
		gameservertest.WithWantChars(1))
	if hero {
		if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET hero = 1 WHERE char_name = 'Newbie'"); err != nil {
			t.Fatalf("seed hero: %v", err)
		}
	}
	return srv
}

// TestOlympiadBarsRestrictedItems pins the Olympiad bar ahead of every item
// use condition: a competitor using an is_oly_restricted item or a hero
// item is told 1507 (cannot be equipped for the Olympiad) when the item can
// be worn and 1508 (not available for the Olympiad) otherwise; nothing is
// worn and nothing consumed. The bar comes before the items' own
// isHero="true" condition, which would answer 1518. Outside a match a hero
// wears the same weapon.
func TestOlympiadBarsRestrictedItems(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		hero     bool
		item     int32
		count    int32
		olympiad bool
		wantMsg  int
	}{
		{"hero's restricted weapon in a match", true, infinityBladeID, 1, true, serverpackets.SystemMessageItemCantBeEquippedForOlympiad},
		{"non-hero's hero circlet in a match", false, heroCircletID, 1, true, serverpackets.SystemMessageItemCantBeEquippedForOlympiad},
		{"restricted potion in a match", false, manaPotionID, 5, true, serverpackets.SystemMessageItemUnavailableForOlympiad},
		{"hero's restricted weapon outside a match", true, infinityBladeID, 1, false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootOlympiadItems(t, tt.hero)
			c, objID := srv.Client, srv.SoleObjectID(t)
			obj := srv.GiveItem(t, objID, tt.item, tt.count)
			startInWorld(t, c)
			drainUntilQuiet(t, c)
			srv.SetPlayerOlympiadMode(t, objID, tt.olympiad)

			c.Send(encodeUseItem(obj, false))
			want := item.LocationPaperdoll
			if tt.wantMsg != 0 {
				assertStaticSystemMessage(t, c.Read(), tt.wantMsg)
				barrier(t, c)
				want = item.LocationInventory
			} else {
				drainUntilQuiet(t, c)
			}
			srv.FlushItems(t)
			inst := mustFindItem(t, srv, objID, obj)
			if inst.Location != want || inst.Count != int(tt.count) {
				t.Fatalf("item %d after use = %v x%d, want %v x%d", tt.item, inst.Location, inst.Count, want, tt.count)
			}
		})
	}
}

// TestOlympiadLetsRestrictedItemComeOff pins that the bar only stops an
// item going on: a hero weapon a hero wore before the match comes off when
// used in it.
func TestOlympiadLetsRestrictedItemComeOff(t *testing.T) {
	t.Parallel()
	srv := bootOlympiadItems(t, true)
	c, objID := srv.Client, srv.SoleObjectID(t)
	blade := srv.GiveItem(t, objID, infinityBladeID, 1)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(blade, false))
	drainUntilQuiet(t, c)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, blade); inst.Location != item.LocationPaperdoll {
		t.Fatalf("weapon before the match = %v, want worn", inst.Location)
	}
	srv.SetPlayerOlympiadMode(t, objID, true)
	c.Send(encodeUseItem(blade, false))
	drainUntilQuiet(t, c)

	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, blade); inst.Location != item.LocationInventory {
		t.Fatalf("weapon after use in a match = %v, want back in the inventory", inst.Location)
	}
}
