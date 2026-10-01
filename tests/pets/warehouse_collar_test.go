package pets

import (
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: WarehouseDepositList lists PcInventory.getAvailableItems
// (PcInventory.java:228-237 -> ItemInstance.isAvailable, :555-563), which
// leaves out the summoned pet's collar; SendWarehouseDepositList.java:88-90
// drops a deposit naming it through Player.checkItemManipulation
// (Player.java:2076-2078) before any fee is taken, and
// RequestPackageSendableItemList offers the same available items.

const (
	whKeeperID = 30005
	whPotionID = 20
	whAdena    = 1000
)

// depositableCollarItems is the fixture catalog with the wolf collar
// depositable, as the datapack's collars are.
func depositableCollarItems() *item.Table {
	all := gameservertest.ItemTemplates().All()
	for _, tmpl := range all {
		if tmpl.ID == wolfCollarID {
			tmpl.Depositable = true
		}
	}
	return item.NewTable(all)
}

func encodeItemRow(opcode byte, objectID, count int32) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(1)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

// listedObjects reads the object ids a WarehouseDepositList (augmentation
// set) or PackageSendableList offers, after the header bytes skip.
func listedObjects(t *testing.T, frame []byte, header int, wideCount, augmentation bool) []int32 {
	t.Helper()
	r := wire.NewReader(frame[1+header:])
	n := 0
	if wideCount {
		n = int(r.ReadInt32())
	} else {
		n = int(r.ReadUint16())
	}
	var out []int32
	for ; n > 0; n-- {
		r.ReadUint16()
		out = append(out, r.ReadInt32())
		r.ReadInt32()
		r.ReadInt32()
		r.ReadUint16()
		r.ReadUint16()
		r.ReadInt32()
		for range 3 {
			r.ReadUint16()
		}
		r.ReadInt32()
		if augmentation {
			r.ReadInt64()
		}
	}
	if r.Err() != nil || r.Remaining() != 0 {
		t.Fatalf("item list has %d trailing bytes (err %v)", r.Remaining(), r.Err())
	}
	return out
}

// TestSummonedPetCollarStaysOutOfTheWarehouse calls out the wolf and opens
// a keeper's deposit window: it lists the potion and adena but not the
// collar, a deposit naming the collar leaves it with its owner and takes no
// fee, and the package window leaves it out too. Once the wolf is returned
// the collar is deposited for the fee.
func TestSummonedPetCollarStaysOutOfTheWarehouse(t *testing.T) {
	t.Parallel()
	pages := map[string]string{
		"warehouse/30005.htm": `<html><body><a action="bypass -h npc_%objectId%_DepositP">Deposit</a></body></html>`,
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithItemTemplates(depositableCollarItems()),
		gameservertest.WithHTMLPages(pages),
	}, seedItem{TemplateID: whPotionID, Count: 5}, seedItem{TemplateID: item.AdenaID, Count: whAdena})
	potion, adena := h.seededItem(t, whPotionID), h.seededItem(t, item.AdenaID)
	h.spawnWolf(t)

	x, y, z := h.character(t).Position()
	f := h.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("WarehouseKeeper", whKeeperID), location.Location{X: x + 50, Y: y, Z: z})
	drainUntilQuiet(t, h.client)
	for range 2 { // select, then talk
		h.client.Send(encodeAction(f.ObjectID(), int32(x), int32(y), int32(z), false))
		drainUntilQuiet(t, h.client)
	}

	h.client.Send(encodeBypass("npc_" + strconv.Itoa(int(f.ObjectID())) + "_DepositP"))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeWarehouseDepositList, "WarehouseDepositList")
	listed := listedObjects(t, frames[len(frames)-1], 2+4, false, true)
	if slices.Contains(listed, h.collarID) || !slices.Contains(listed, potion) || !slices.Contains(listed, adena) {
		t.Fatalf("deposit window lists %v, want the potion and adena but not the collar %d", listed, h.collarID)
	}
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeItemRow(clientpackets.OpcodeSendWarehouseDeposit, h.collarID, 1))
	requireNoFrame(t, h.client, "deposit of the summoned pet's collar")
	inv := h.ownerInventory(t)
	if inv.ItemByObjectID(h.collarID) == nil || inv.Adena() != whAdena {
		t.Fatalf("after depositing the summoned pet's collar: collar held %v, adena %d; want kept and no fee", inv.ItemByObjectID(h.collarID) != nil, inv.Adena())
	}

	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPackageItemList)
	w.WriteInt32(h.ownerID)
	h.client.Send(w.Bytes())
	frames = readUntilOpcode(t, h.client, serverpackets.OpcodePackageSendableList, "PackageSendableList")
	if sendable := listedObjects(t, frames[len(frames)-1], 8, true, false); slices.Contains(sendable, h.collarID) || !slices.Contains(sendable, potion) {
		t.Fatalf("package window offers %v, want the potion but not the collar %d", sendable, h.collarID)
	}

	h.returnPet(t)
	h.client.Send(encodeItemRow(clientpackets.OpcodeSendWarehouseDeposit, h.collarID, 1))
	drainUntilQuiet(t, h.client)
	if inv.ItemByObjectID(h.collarID) != nil || inv.Adena() != whAdena-30 {
		t.Fatalf("after depositing the returned pet's collar: collar held %v, adena %d; want stored for 30", inv.ItemByObjectID(h.collarID) != nil, inv.Adena())
	}
}
