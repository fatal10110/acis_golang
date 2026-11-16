package admin

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A game master's end of a cursed weapon whose ground item a pickup holds
// never arms the picker: the pickup gets the weapon's item, which ends
// with it (#5257, follow-up of the owner-approved #5149 fix).

// heldAkamanah drops Akamanah at the game master's feet and holds its
// ground item the way a pickup's click does.
func heldAkamanah(t *testing.T, srv *gameservertest.Server, gmID int32) *grounditem.Item {
	t.Helper()
	drop, ok := srv.CursedWeapons.RollDrop(func(int) int { return 0 })
	if !ok || drop.ItemID != akamanahID {
		t.Fatalf("drop = %d %v, want Akamanah", drop.ItemID, ok)
	}
	x, y, z := srv.PlayerPosition(t, gmID)
	srv.SeedGroundItem(t, 0, akamanahID, 1, x, y, z)
	var ground *grounditem.Item
	for _, obj := range srv.State.Objects() {
		if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == akamanahID {
			ground = g
		}
	}
	if ground == nil {
		t.Fatal("Akamanah not on the ground")
	}
	if !srv.CursedWeapons.PlaceOnGround(drop, ground, location.Location{X: x, Y: y, Z: z}) {
		t.Fatal("PlaceOnGround refused the drop")
	}
	if !ground.Claim() {
		t.Fatal("the fresh drop refused its first claim")
	}
	return ground
}

// pickUpDeadAkamanah lets the held pickup go on and take the ground item,
// then checks the picker is not armed and keeps no copy of the weapon.
func pickUpDeadAkamanah(t *testing.T, srv *gameservertest.Server, gmID int32, ground *grounditem.Item) {
	t.Helper()
	gm := srv.Client
	if _, lying := srv.State.Object(ground.ObjectID()); !lying {
		t.Fatal("the end took the held ground item off the ground")
	}
	// A raw Release models the held claim's end; a production pickup that
	// took nothing lets go through the weapon instead, which takes the
	// ended weapon's item out of the world.
	ground.Release()
	gm.Send(encodeAction(ground.ObjectID()))
	srv.AdvanceUntil(t, "weapon picked up", func() bool {
		_, lying := srv.State.Object(ground.ObjectID())
		return !lying
	})
	srv.SettlePickup(t, gmID)
	settle(t, gm)

	picker := onlineCharacter(t, srv, gmID)
	if picker.CursedWeaponID() != 0 || picker.Karma() != 0 || picker.Inventory().ItemByTemplateID(akamanahID) != nil {
		t.Fatalf("picker: weapon %d karma %d, Akamanah held %v; want no weapon, 0 and none",
			picker.CursedWeaponID(), picker.Karma(), picker.Inventory().ItemByTemplateID(akamanahID) != nil)
	}
	if skillID, ok := srv.CursedWeapons.Skill(akamanahID); ok && picker.HasSkill(int(skillID)) {
		t.Fatal("picker learned Akamanah's skill")
	}
	if active := srv.CursedWeapons.Active(); len(active) != 0 {
		t.Fatalf("weapons out = %+v, want none", active)
	}
	srv.FlushItems(t)
	ctx := context.Background()
	var items, rows int
	if err := srv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE item_id = ?", akamanahID).Scan(&items); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if err := srv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM cursed_weapons").Scan(&rows); err != nil {
		t.Fatalf("count cursed_weapons: %v", err)
	}
	if items != 0 || rows != 0 {
		t.Fatalf("stored Akamanah items %d, cursed weapon rows %d; want none", items, rows)
	}
}

// TestAdminRemoveCursedWeaponHeldByPickup: //cw remove of Akamanah while a
// pickup holds its ground item.
func TestAdminRemoveCursedWeaponHeldByPickup(t *testing.T) {
	t.Parallel()
	srv, gmID := bootCursedAdmin(t, 0)
	gm := srv.Client
	enterWorld(t, gm)
	ground := heldAkamanah(t, srv, gmID)

	frames := readUntilPage(t, gm, exchange(t, gm, encodeBuildCmd("cw remove 8689")))
	if indexOfItemMessage(frames, serverpackets.SystemMessageS1HasDisappeared, akamanahID) < 0 {
		t.Fatal("//cw remove did not announce Akamanah gone")
	}
	pickUpDeadAkamanah(t, srv, gmID, ground)
}

// TestAdminReloadCursedWeaponHeldByPickup: //reload cw while a pickup holds
// Akamanah's ground item; the reloaded table still has Akamanah.
func TestAdminReloadCursedWeaponHeldByPickup(t *testing.T) {
	t.Parallel()
	fresh := cursedWeaponTable(t, zaricheID, akamanahID)
	srv, gmID := bootCursedAdmin(t, 0, gameservertest.WithDataReloads(network.DataReloads{
		CursedWeapons: func() (*entity.CursedWeaponTable, error) { return fresh, nil },
	}))
	gm := srv.Client
	enterWorld(t, gm)
	ground := heldAkamanah(t, srv, gmID)

	frames := exchange(t, gm, encodeBuildCmd("reload cw"))
	frames = append(frames, settle(t, gm)...)
	if indexOfItemMessage(frames, serverpackets.SystemMessageS1HasDisappeared, akamanahID) < 0 {
		t.Fatal("//reload cw did not announce Akamanah gone")
	}
	pickUpDeadAkamanah(t, srv, gmID, ground)
}
