package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: RequestDestroyItem.java:54-58 refuses an item only when it is
// !isDestroyable() (or a configured cursed weapon); isHeroItem() picks the
// refusal's message and refuses nothing by itself. The shipped hero
// circlet (heroCircletID) is destroyable; the Infinity Blade
// (infinityBladeID) is is_destroyable=false.

// bootShippedHeroItems boots a character holding one of each shipped hero
// item, on the suite's item fixtures plus their shipped templates.
func bootShippedHeroItems(t *testing.T) (*gameservertest.Server, int32, map[int32]int32) {
	t.Helper()
	datapack.Require(t)
	_, shipped := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{heroCircletID, infinityBladeID} {
		tmpl, ok := shipped.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	objID := srv.SoleObjectID(t)
	held := map[int32]int32{
		heroCircletID:   srv.GiveItem(t, objID, heroCircletID, 1),
		infinityBladeID: srv.GiveItem(t, objID, infinityBladeID, 1),
	}
	startInWorld(t, srv.Client)
	return srv, objID, held
}

// TestDestroyDestroyableHeroCirclet destroys the hero circlet, whose
// template leaves it destroyable: S1_DISAPPEARED names it, the
// InventoryUpdate removes it, and its row is gone.
func TestDestroyDestroyableHeroCirclet(t *testing.T) {
	t.Parallel()
	srv, objID, held := bootShippedHeroItems(t)
	c := srv.Client
	circlet := held[heroCircletID]

	c.Send(encodeRequestDestroyItem(circlet, 1))
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageS1Disappeared, heroCircletID)
	srv.InventoryUpdates.Tick()
	waitForInventoryUpdate(t, c, circlet)

	srv.FlushItems(t)
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.ObjectID == circlet {
			t.Fatalf("destroyed circlet row still saved: %+v", inst)
		}
	}
	assertItemCount(t, srv, objID, held[infinityBladeID], 1)
}

// TestDestroyUndestroyableHeroWeaponIsRefused: a hero weapon the template
// keeps from being destroyed still reads HERO_WEAPONS_CANT_DESTROYED alone
// and stays held.
func TestDestroyUndestroyableHeroWeaponIsRefused(t *testing.T) {
	t.Parallel()
	srv, objID, held := bootShippedHeroItems(t)
	c := srv.Client
	blade := held[infinityBladeID]

	c.Send(encodeRequestDestroyItem(blade, 1))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageHeroWeaponsCantDestroyed)
	assertNoFrame(t, c, "after the hero weapon refusal")
	assertItemCount(t, srv, objID, blade, 1)
}
