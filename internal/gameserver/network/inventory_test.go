package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkAutoSoulShotInGame(t *testing.T) {
	const soulshotID int32 = 1463
	c, chars, items, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   510,
		TemplateID: soulshotID,
		OwnerID:    objID,
		Count:      100,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestAutoSoulShot(soulshotID, 1))
	reply := c.read()
	assertExAutoSoulShotFrame(t, reply, soulshotID, true)
	reply = c.read()
	assertSystemMessageItemFrame(t, reply, serverpackets.SystemMessageUseOfItemWillBeAuto, soulshotID)

	c.send(encodeRequestAutoSoulShot(soulshotID, 0))
	reply = c.read()
	assertExAutoSoulShotFrame(t, reply, soulshotID, false)
	reply = c.read()
	assertSystemMessageItemFrame(t, reply, serverpackets.SystemMessageAutoUseOfItemCancelled, soulshotID)
}

func TestGameClientLinkDropItemInGame(t *testing.T) {
	c, chars, items, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   500,
		TemplateID: item.AdenaID,
		OwnerID:    objID,
		Count:      100,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestDropItem(500, 40, location.Location{X: 10, Y: 20, Z: 30}))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("drop inventory opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
	r := wire.NewReader(reply[1:])
	if count := r.ReadUint16(); count != 1 {
		t.Fatalf("InventoryUpdate count = %d, want 1", count)
	}
	if state := r.ReadUint16(); state != 2 {
		t.Fatalf("InventoryUpdate state = %d, want modified (2, per ItemState.ordinal)", state)
	}
	r.ReadUint16()
	if got := r.ReadInt32(); got != 500 {
		t.Fatalf("InventoryUpdate object id = %d, want 500", got)
	}
	r.ReadInt32()
	if got := r.ReadInt32(); got != 60 {
		t.Fatalf("InventoryUpdate count = %d, want 60", got)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeDropItem {
		t.Fatalf("drop broadcast opcode = %#x, want DropItem (%#x)", reply[0], serverpackets.OpcodeDropItem)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("DropItem dropper id = %d, want %d", got, objID)
	}
	groundID := r.ReadInt32()
	if groundID == 500 {
		t.Fatalf("DropItem ground object id reused source stack id %d", groundID)
	}
	if got := r.ReadInt32(); got != item.AdenaID {
		t.Fatalf("DropItem item id = %d, want adena", got)
	}
	x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if x != 10 || y != 20 || z != 30 {
		t.Fatalf("DropItem location = (%d,%d,%d), want (10,20,30)", x, y, z)
	}
	if stackable := r.ReadInt32(); stackable != 1 {
		t.Fatalf("DropItem stackable = %d, want 1", stackable)
	}
	if got := r.ReadInt32(); got != 40 {
		t.Fatalf("DropItem count = %d, want 40", got)
	}

	if _, ok := state.Object(groundID); !ok {
		t.Fatalf("world.Object(%d) missing for dropped item", groundID)
	}
}

func TestGameClientLinkDestroyItemInGame(t *testing.T) {
	c, chars, items, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   501,
		TemplateID: 20,
		OwnerID:    objID,
		Count:      5,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestDestroyItem(501, 2))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("destroy inventory opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
	r := wire.NewReader(reply[1:])
	if count := r.ReadUint16(); count != 1 {
		t.Fatalf("InventoryUpdate count = %d, want 1", count)
	}
	if state := r.ReadUint16(); state != 2 {
		t.Fatalf("InventoryUpdate state = %d, want modified (2, per ItemState.ordinal)", state)
	}
	r.ReadUint16()
	if got := r.ReadInt32(); got != 501 {
		t.Fatalf("InventoryUpdate object id = %d, want 501", got)
	}
	r.ReadInt32()
	if got := r.ReadInt32(); got != 3 {
		t.Fatalf("InventoryUpdate count = %d, want 3", got)
	}
}

func TestGameClientLinkCrystallizeItemInGame(t *testing.T) {
	c, chars, items, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	chars.updateCharacter(t, objID, func(ch *player.Character) {
		ch.SetSkillLevel(248, 1)
	})
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   502,
		TemplateID: 30,
		OwnerID:    objID,
		Count:      1,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestCrystallizeItem(502, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("crystallize message opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageItemCrystallized {
		t.Fatalf("SystemMessage id = %d, want crystallized", id)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("SystemMessage params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
		t.Fatalf("SystemMessage param type = %d, want item name", typ)
	}
	if got := r.ReadInt32(); got != 30 {
		t.Fatalf("SystemMessage item id = %d, want 30", got)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("crystallize inventory opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
	r = wire.NewReader(reply[1:])
	if count := r.ReadUint16(); count != 2 {
		t.Fatalf("InventoryUpdate count = %d, want 2", count)
	}
	if state := r.ReadUint16(); state != 3 {
		t.Fatalf("source update state = %d, want removed (3, per ItemState.ordinal)", state)
	}
	r.ReadUint16()
	if got := r.ReadInt32(); got != 502 {
		t.Fatalf("source update object id = %d, want 502", got)
	}
	if got := r.ReadInt32(); got != 30 {
		t.Fatalf("source update item id = %d, want 30", got)
	}
	if got := r.ReadInt32(); got != 1 {
		t.Fatalf("source update count = %d, want 1", got)
	}
	skipInventoryRemainder(r)

	if state := r.ReadUint16(); state != 1 {
		t.Fatalf("crystal update state = %d, want added (1, per ItemState.ordinal)", state)
	}
	r.ReadUint16()
	if got := r.ReadInt32(); got == 0 {
		t.Fatal("crystal update object id = 0, want allocated id")
	}
	if got := r.ReadInt32(); got != item.CrystalD.ItemID() {
		t.Fatalf("crystal update item id = %d, want D crystal", got)
	}
	if got := r.ReadInt32(); got != 10 {
		t.Fatalf("crystal update count = %d, want 10", got)
	}
}

func TestGameClientLinkCrystallizeItemSkillTooLow(t *testing.T) {
	c, chars, items, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   503,
		TemplateID: 30,
		OwnerID:    objID,
		Count:      1,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestCrystallizeItem(503, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("skill-low opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageCrystallizeLevelTooLow {
		t.Fatalf("SystemMessage id = %d, want crystallize level too low", id)
	}
	if params := r.ReadInt32(); params != 0 {
		t.Fatalf("SystemMessage params = %d, want 0", params)
	}

	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeItemList {
		t.Fatalf("post-skill-low opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
	}
}

func TestGameClientLinkRequestPackageItemListSendsSendableInventory(t *testing.T) {
	c, _, _, _ := newLinkedGameClientWithSkillsSeed(t, nil, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		for _, inst := range []item.Instance{
			{ObjectID: 500, TemplateID: item.AdenaID, OwnerID: objID, Count: 100, Location: item.LocationInventory, ManaLeft: -1},
			{ObjectID: 501, TemplateID: 20, OwnerID: objID, Count: 3, Location: item.LocationInventory, ManaLeft: -1},
			{ObjectID: 502, TemplateID: 30, OwnerID: objID, Count: 1, Location: item.LocationPaperdoll, LocationData: 7, ManaLeft: -1},
			{ObjectID: 503, TemplateID: 20, OwnerID: objID, Count: 9, Location: item.LocationWarehouse, ManaLeft: -1},
		} {
			if err := items.Create(context.Background(), objID, inst); err != nil {
				t.Fatalf("seed item %d: %v", inst.ObjectID, err)
			}
		}
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestPackageSendableItemList(200))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePackageSendableList {
		t.Fatalf("package item list opcode = %#x, want PackageSendableList (%#x)", reply[0], serverpackets.OpcodePackageSendableList)
	}
	r := wire.NewReader(reply[1:])
	if objectID := r.ReadInt32(); objectID != 200 {
		t.Fatalf("PackageSendableList object id = %d, want 200", objectID)
	}
	if adena := r.ReadInt32(); adena != 100 {
		t.Fatalf("PackageSendableList adena = %d, want 100", adena)
	}
	if count := r.ReadInt32(); count != 2 {
		t.Fatalf("PackageSendableList count = %d, want 2 sendable carried items", count)
	}
	if category, objectID, itemID, count := r.ReadUint16(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); category != uint16(item.CategoryMoneyOrEtcItem) || objectID != 500 || itemID != item.AdenaID || count != 100 {
		t.Fatalf("first package item = (%d,%d,%d,%d), want adena object 500 count 100", category, objectID, itemID, count)
	}
	skipPackageSendableRemainder(r)
	if category, objectID, itemID, count := r.ReadUint16(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); category != uint16(item.CategoryMoneyOrEtcItem) || objectID != 501 || itemID != 20 || count != 3 {
		t.Fatalf("second package item = (%d,%d,%d,%d), want potion object 501 count 3", category, objectID, itemID, count)
	}
	if r.Err() != nil {
		t.Fatalf("parse PackageSendableList: %v", r.Err())
	}
}
