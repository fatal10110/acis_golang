package pets

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: PcInventory.getSellableItems (PcInventory.java:242-245) leaves
// the summoned pet's collar out of the sell window, and RequestSellItem.java:68
// skips it through Player.checkItemManipulation (Player.java:2076-2078).

const (
	sellMerchantID = 30001
	sellPotionID   = 20
)

// sellableCollarItems is the fixture catalog with the wolf collar (price
// 200) and the potion (price 40) sellable.
func sellableCollarItems() *item.Table {
	all := gameservertest.ItemTemplates().All()
	for _, tmpl := range all {
		switch tmpl.ID {
		case wolfCollarID:
			tmpl.Sellable, tmpl.ReferencePrice = true, 200
		case sellPotionID:
			tmpl.Sellable, tmpl.ReferencePrice = true, 40
		}
	}
	return item.NewTable(all)
}

func encodeBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

func encodeRequestSellItem(objectID, itemID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSellItem)
	w.WriteInt32(0)
	w.WriteInt32(1)
	w.WriteInt32(objectID)
	w.WriteInt32(itemID)
	w.WriteInt32(count)
	return w.Bytes()
}

// sellListObjects reads the object ids a SellList frame offers.
func sellListObjects(t *testing.T, frame []byte) []int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSellList, "SellList")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // adena
	r.ReadInt32()
	var out []int32
	for n := int(r.ReadUint16()); n > 0; n-- {
		r.ReadUint16()
		out = append(out, r.ReadInt32())
		for range 2 {
			r.ReadInt32()
		}
		r.ReadUint16()
		r.ReadUint16()
		r.ReadInt32()
		for range 3 {
			r.ReadUint16()
		}
		r.ReadInt32()
	}
	return out
}

// TestSummonedPetCollarIsNotSold calls out the wolf and opens a merchant's
// sell window: it offers the potion but not the collar, and a sale naming
// the collar leaves it with its owner, unpaid. Once the wolf is returned the
// collar sells for half its price.
func TestSummonedPetCollarIsNotSold(t *testing.T) {
	t.Parallel()
	pages := map[string]string{
		"merchant/30001.htm": `<html><body><a action="bypass -h npc_%objectId%_Sell">Sell</a></body></html>`,
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithItemTemplates(sellableCollarItems()),
		gameservertest.WithHTMLPages(pages),
	}, seedItem{TemplateID: sellPotionID, Count: 5})
	potion := h.seededItem(t, sellPotionID)
	h.spawnWolf(t)

	x, y, z := h.character(t).Position()
	f := h.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Merchant", sellMerchantID), location.Location{X: x + 50, Y: y, Z: z})
	drainUntilQuiet(t, h.client)
	for range 2 { // select, then talk
		h.client.Send(encodeAction(f.ObjectID(), int32(x), int32(y), int32(z), false))
		drainUntilQuiet(t, h.client)
	}

	h.client.Send(encodeBypass("npc_" + strconv.Itoa(int(f.ObjectID())) + "_Sell"))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeSellList, "SellList")
	if offered := sellListObjects(t, frames[len(frames)-1]); len(offered) != 1 || offered[0] != potion {
		t.Fatalf("sell window offers %v, want only the potion %d", offered, potion)
	}
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestSellItem(h.collarID, wolfCollarID, 1))
	requireNoFrame(t, h.client, "sale of the summoned pet's collar")
	inv := h.ownerInventory(t)
	if inv.ItemByObjectID(h.collarID) == nil || inv.Adena() != 0 {
		t.Fatalf("after selling the summoned pet's collar: collar held %v, adena %d; want kept and unpaid", inv.ItemByObjectID(h.collarID) != nil, inv.Adena())
	}

	h.returnPet(t)
	h.client.Send(encodeRequestSellItem(h.collarID, wolfCollarID, 1))
	drainUntilQuiet(t, h.client)
	if inv.ItemByObjectID(h.collarID) != nil || inv.Adena() != 100 {
		t.Fatalf("after selling the returned pet's collar: collar held %v, adena %d; want sold for 100", inv.ItemByObjectID(h.collarID) != nil, inv.Adena())
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 0 {
		t.Fatalf("owner collar rows = %d, want the sold collar gone", got)
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 100 {
		t.Fatalf("owner adena rows = %d, want 100", got)
	}
}
