package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestDestroyItem.java:44-58 checks the count (an out-of-range
// count reads CANNOT_DESTROY_NUMBER_INCORRECT, several units of a
// non-stackable item are dropped silently), then refuses an item that is
// !isDestroyable() or CursedWeaponManager.isCursed(itemId) with
// HERO_WEAPONS_CANT_DESTROYED for a hero item and CANNOT_DISCARD_THIS_ITEM
// otherwise — before the equipped item is taken off. isCursed is a lookup
// in the configured cursedWeapons.xml table, not a template flag.

const destroyCursedHeroID = 6611 // first hero weapon id

// destroyCursedBoot boots a player against the shared catalog plus a
// destroyable hero weapon, with cursed weapons configured for ids.
func destroyCursedBoot(t *testing.T, ids ...int32) *gameservertest.Server {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	for _, tmpl := range templates {
		if tmpl.ID == 30 {
			hero := *tmpl
			hero.ID = destroyCursedHeroID
			hero.Name = "Hero Sword"
			templates = append(templates, &hero)
			break
		}
	}
	weapons := make([]entity.CursedWeapon, 0, len(ids))
	for _, id := range ids {
		weapons = append(weapons, entity.CursedWeapon{ItemID: id})
	}
	table, err := entity.NewCursedWeaponTable(weapons)
	if err != nil {
		t.Fatalf("NewCursedWeaponTable() error: %v", err)
	}
	return gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCursedWeapons(table),
	)
}

// TestDestroyCursedWeaponIsRefused: a configured cursed weapon whose
// template is destroyable is refused with CANNOT_DISCARD_THIS_ITEM (a hero
// one with HERO_WEAPONS_CANT_DESTROYED), stays held, and a worn one stays
// worn.
func TestDestroyCursedWeaponIsRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		templateID int32
		equip      bool
		wantID     int
	}{
		{name: "held", templateID: 30, wantID: serverpackets.SystemMessageCannotDiscardThisItem},
		{name: "worn", templateID: 30, equip: true, wantID: serverpackets.SystemMessageCannotDiscardThisItem},
		{name: "hero", templateID: destroyCursedHeroID, wantID: serverpackets.SystemMessageHeroWeaponsCantDestroyed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := destroyCursedBoot(t, tc.templateID)
			c := srv.Client
			objID := srv.SoleObjectID(t)
			weapon := srv.GiveItem(t, objID, tc.templateID, 1)
			startInWorld(t, c)
			if tc.equip {
				c.Send(encodeUseItem(weapon, false))
				drainUntilQuiet(t, c)
			}

			c.Send(encodeRequestDestroyItem(weapon, 1))
			assertStaticSystemMessage(t, c.Read(), tc.wantID)
			assertNoFrameFor(t, c, 300*time.Millisecond, "after the cursed-weapon refusal")

			assertItemCount(t, srv, objID, weapon, 1)
			if tc.equip && !mustFindItem(t, srv, objID, weapon).Equipped() {
				t.Fatal("refused destroy took the worn cursed weapon off")
			}
		})
	}
}

// TestDestroyCursedWeaponCountChecksComeFirst: an out-of-range count on a
// configured cursed weapon still reads CANNOT_DESTROY_NUMBER_INCORRECT
// rather than the cursed-weapon refusal.
func TestDestroyCursedWeaponCountChecksComeFirst(t *testing.T) {
	t.Parallel()
	srv := destroyCursedBoot(t, 30)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeRequestDestroyItem(sword, 0))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotDestroyNumberIncorrect)
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the count refusal")

	assertItemCount(t, srv, objID, sword, 1)
}

// TestDestroyUnconfiguredItemBesideCursedTable: with cursed weapons
// configured, an item outside the table is destroyed as before.
func TestDestroyUnconfiguredItemBesideCursedTable(t *testing.T) {
	t.Parallel()
	srv := destroyCursedBoot(t, 30)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potions := srv.GiveItem(t, objID, 20, 5)
	startInWorld(t, c)

	c.Send(encodeRequestDestroyItem(potions, 3))
	var messages [][]byte
	for _, f := range collectUntilQuiet(t, c) {
		if f[0] == serverpackets.OpcodeSystemMessage {
			messages = append(messages, f)
		}
	}
	if len(messages) != 1 {
		t.Fatalf("destroy sent %d SystemMessages, want 1", len(messages))
	}
	assertGrantMessage(t, messages[0], serverpackets.SystemMessageS2S1Disappeared, itemNameParam(20), itemNumberParam(3))
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, potions, 2)
}
