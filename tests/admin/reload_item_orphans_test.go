package admin

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Templates only the booted item table defines; //reload item drops them.
const (
	orphanEtcID    int32 = 65010
	orphanWeaponID int32 = 65011
)

// orphanItemTable is the shared item catalog, plus orphanEtcID and
// orphanWeaponID (a copy of the sword) when withOrphans is set.
func orphanItemTable(withOrphans bool) *item.Table {
	templates := gameservertest.ItemTemplates().All()
	if withOrphans {
		sword, _ := gameservertest.ItemTemplates().Get(swordID)
		weapon := *sword
		weapon.ID, weapon.Name = orphanWeaponID, "Reloaded Away Sword"
		templates = append(templates,
			&item.Template{ID: orphanEtcID, Name: "Reloaded Away", Kind: item.KindEtcItem, Duration: -1, EtcItem: &item.EtcItemDetail{}},
			&weapon,
		)
	}
	return item.NewTable(templates)
}

// TestReloadedAwayItemsStayHeldAndSkipped reloads an item table that no
// longer defines two items an online player holds, one carried and one
// worn. The player's inventory still opens, listing what it can and leaving
// those two out; nothing deletes or rewrites their rows, across a relog that
// restores the inventory without them; and once a reload brings their
// templates back, the next login hands both back.
func TestReloadedAwayItemsStayHeldAndSkipped(t *testing.T) {
	t.Parallel()
	items := orphanItemTable(true)
	withOrphans := true
	srv, objID := bootAdmin(t, adminLevel,
		gameservertest.WithItemTemplates(items),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithCapturedLog(),
		gameservertest.WithDataReloads(network.DataReloads{Items: func() error {
			withOrphans = !withOrphans
			items.Replace(orphanItemTable(withOrphans))
			return nil
		}}),
	)
	gm := srv.Client
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	carried := srv.GiveItem(t, objID, orphanEtcID, 1)
	worn := srv.GiveItem(t, objID, orphanWeaponID, 1)
	row := itemRow(t, srv, objID, worn)
	row.Location, row.LocationData = item.LocationPaperdoll, itemcontainer.RHand
	if err := srv.Items.Update(context.Background(), row); err != nil {
		t.Fatalf("wear the sword: %v", err)
	}

	listed := heldObjects(t, enterWorldBurst(t, gm))
	if !slices.Contains(listed, carried) || !slices.Contains(listed, worn) {
		t.Fatalf("login ItemList = %v, want %d and %d listed while their templates are loaded", listed, carried, worn)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload item")), "Items' templates have been reloaded.")

	frames := exchange(t, gm, wire.NewPacketWriter(clientpackets.OpcodeRequestItemList).Bytes())
	listed = heldObjects(t, frames)
	if !slices.Contains(listed, adena) || slices.Contains(listed, carried) || slices.Contains(listed, worn) {
		t.Fatalf("ItemList after the reload = %v, want adena %d listed and the reloaded-away %d and %d left out", listed, adena, carried, worn)
	}

	gm.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	if frame := readUntil(t, gm, serverpackets.OpcodeCharSelectInfo); frame == nil {
		t.Fatal("no CharSelectInfo after the restart")
	}
	srv.FlushItems(t)
	assertOrphanRows(t, srv, objID, carried, worn)

	listed = heldObjects(t, enterWorldBurst(t, gm))
	if !slices.Contains(listed, adena) || slices.Contains(listed, carried) || slices.Contains(listed, worn) {
		t.Fatalf("relog ItemList = %v, want adena %d listed and the reloaded-away %d and %d left out", listed, adena, carried, worn)
	}
	srv.FlushItems(t)
	assertOrphanRows(t, srv, objID, carried, worn)

	// A reload that defines them again: the next login restores both.
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload item")), "Items' templates have been reloaded.")
	gm.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	readUntil(t, gm, serverpackets.OpcodeCharSelectInfo)
	listed = heldObjects(t, enterWorldBurst(t, gm))
	if !slices.Contains(listed, carried) || !slices.Contains(listed, worn) {
		t.Fatalf("ItemList once the templates are back = %v, want %d and %d listed again", listed, carried, worn)
	}
}

// enterWorldBurst selects slot 0 and enters the world, returning every frame
// the entry sent.
func enterWorldBurst(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	return c.ReadQueued()
}

// readUntil reads frames until one carries opcode and returns it.
func readUntil(t *testing.T, c *testsupport.ScriptedClient, opcode byte) []byte {
	t.Helper()
	for range 40 {
		if frame := c.Read(); frame[0] == opcode {
			return frame
		}
	}
	t.Fatalf("no frame with opcode %#x within 40 frames", opcode)
	return nil
}

// heldObjects returns the object ids the one ItemList among frames lists.
func heldObjects(t *testing.T, frames [][]byte) []int32 {
	t.Helper()
	var list []byte
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeItemList {
			if list != nil {
				t.Fatal("two ItemList frames, want one")
			}
			list = frame
		}
	}
	if list == nil {
		t.Fatalf("no ItemList among %x", testsupport.FrameOpcodes(frames))
	}
	r := wire.NewReader(list[1:])
	r.ReadUint16() // show window
	n := r.ReadUint16()
	ids := make([]int32, 0, n)
	for range n {
		r.ReadUint16() // category
		ids = append(ids, r.ReadInt32())
		r.ReadInt32() // item id
		r.ReadInt32() // count
		r.ReadUint16()
		r.ReadUint16()
		r.ReadUint16() // equipped
		r.ReadInt32()  // slot
		r.ReadUint16()
		r.ReadUint16()
		r.ReadInt32() // augmentation
		r.ReadInt32() // mana
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read ItemList: %v", err)
	}
	if r.Remaining() != 0 {
		t.Fatalf("ItemList has %d bytes past its %d rows", r.Remaining(), n)
	}
	return ids
}

// assertOrphanRows checks the carried and worn rows are still ownerID's,
// with their item ids, counts and places unchanged.
func assertOrphanRows(t *testing.T, srv *gameservertest.Server, ownerID, carried, worn int32) {
	t.Helper()
	if row := itemRow(t, srv, ownerID, carried); row.TemplateID != orphanEtcID || row.Count != 1 || row.Location != item.LocationInventory {
		t.Fatalf("carried row = %+v, want item %d count 1 in the inventory", *row, orphanEtcID)
	}
	if row := itemRow(t, srv, ownerID, worn); row.TemplateID != orphanWeaponID || row.Count != 1 || row.Location != item.LocationPaperdoll || row.LocationData != itemcontainer.RHand {
		t.Fatalf("worn row = %+v, want item %d count 1 in the right hand", *row, orphanWeaponID)
	}
}
