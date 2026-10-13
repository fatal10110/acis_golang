package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Player.mount(int, int) (Player.java:3542-3545) mounts the
// wyvern only once disarmWeapon(true) took the hands off, and
// disarmWeapon refuses a cursed weapon (Player.java:3476-3478): the
// collar's use mounts nothing and sends nothing.

// zaricheID is Demonic Sword Zariche, a cursed weapon.
const zaricheID int32 = 8190

// holderRiderItems is the wyvern rider's catalog with Zariche worn in both
// hands, as the datapack declares it.
func holderRiderItems() *item.Table {
	templates := wyvernRiderItems().All()
	for i, tmpl := range templates {
		if tmpl.ID == zaricheID {
			lr := *tmpl
			lr.Slot = item.SlotLRHand
			lr.Dropable, lr.Destroyable, lr.Tradable = false, false, false
			templates[i] = &lr
		}
	}
	return item.NewTable(templates)
}

// zaricheTable is Zariche as cursedWeapons.xml ships it.
func zaricheTable(t *testing.T) *entity.CursedWeaponTable {
	t.Helper()
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{
		ItemID: zaricheID, Skill: modelskill.Ref{ID: 3603, Level: 3}, Name: "Demonic Sword Zariche",
		DropRate: 1, Duration: 72, DurationLost: 24, DisappearChance: 50, StageKills: 10,
	}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	return table
}

// TestWyvernCollarRefusedToCursedHolder has the owner pick up Zariche, which
// makes it the weapon's holder, then use the wyvern collar: it stays on
// foot holding the weapon, and no packet answers the collar.
func TestWyvernCollarRefusedToCursedHolder(t *testing.T) {
	t.Parallel()
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), fedWyvernTemplate()})),
		gameservertest.WithRestartPoints(mountRestartTable()),
		gameservertest.WithSkills(wyvernRiderSkills(t)),
		gameservertest.WithItemTemplates(holderRiderItems()),
		gameservertest.WithCursedWeapons(zaricheTable(t)),
	)
	ownerID := srv.SoleObjectID(t)
	collar := srv.GiveItem(t, ownerID, wyvernCollarID, 1)
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID}

	x, y, z := srv.PlayerPosition(t, ownerID)
	srv.SeedGroundItem(t, 0, zaricheID, 1, x, y, z)
	drainUntilQuiet(t, h.client)
	var ground *grounditem.Item
	for _, obj := range srv.State.Objects() {
		if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == zaricheID {
			ground = g
		}
	}
	if ground == nil {
		t.Fatal("no Zariche on the ground")
	}
	h.client.Send(encodeAction(ground.ObjectID(), int32(x), int32(y), int32(z), false))
	srv.Settle(t)
	drainUntilQuiet(t, h.client)
	rider := h.character(t)
	if !rider.CursedWeaponEquipped() {
		t.Fatal("the owner does not hold Zariche after picking it up")
	}

	h.client.Send(encodeUseItem(collar, false))
	srv.Settle(t)
	requireNoFrame(t, h.client, "the wyvern collar of a cursed weapon holder")
	if rider.Mounted() {
		t.Fatal("the holder mounted the wyvern")
	}
	if inst := rider.Inventory().ItemByTemplateID(zaricheID); inst == nil || !inst.Equipped() {
		t.Fatal("Zariche left the holder's hands")
	}
	if worn := h.paperdollRows(t, zaricheID); !worn[zaricheID] {
		t.Fatal("Zariche's row left the paperdoll")
	}
}
