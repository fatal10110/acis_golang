package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestRequestAutoSoulShotTogglesKnownInventoryItem(t *testing.T) {
	const soulshotID int32 = 1463
	templates := item.NewTable([]*item.Template{{
		ID:            soulshotID,
		Name:          "Soulshot: No Grade",
		Kind:          item.KindEtcItem,
		Duration:      -1,
		Stackable:     true,
		DefaultAction: item.ActionSoulshot,
		EtcItem:       &item.EtcItemDetail{Type: item.EtcItemShot},
	}})
	shot := &item.Instance{ObjectID: 500, TemplateID: soulshotID, Count: 100, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{shot})
	gcl := &GameClientLink{}

	gcl.handleAutoSoulShot(live, clientpackets.RequestAutoSoulShot{ItemID: soulshotID, Type: 1})

	if !live.AutoSoulShotEnabled(soulshotID) {
		t.Fatal("auto soulshot was not enabled")
	}
	if len(capture.frames) != 2 {
		t.Fatalf("enable frames = %x, want ExAutoSoulShot and SystemMessage", capture.frames)
	}
	assertExAutoSoulShotFrame(t, capture.frames[0], soulshotID, true)
	assertSystemMessageItemFrame(t, capture.frames[1], serverpackets.SystemMessageUseOfItemWillBeAuto, soulshotID)

	capture.frames = nil
	gcl.handleAutoSoulShot(live, clientpackets.RequestAutoSoulShot{ItemID: soulshotID, Type: 0})

	if live.AutoSoulShotEnabled(soulshotID) {
		t.Fatal("auto soulshot is still enabled")
	}
	if len(capture.frames) != 2 {
		t.Fatalf("disable frames = %x, want ExAutoSoulShot and SystemMessage", capture.frames)
	}
	assertExAutoSoulShotFrame(t, capture.frames[0], soulshotID, false)
	assertSystemMessageItemFrame(t, capture.frames[1], serverpackets.SystemMessageAutoUseOfItemCancelled, soulshotID)
}

func TestRequestAutoSoulShotIgnoresMissingOrFishingShots(t *testing.T) {
	const fishingShotID int32 = 6535
	templates := item.NewTable([]*item.Template{{
		ID:            fishingShotID,
		Name:          "Fishing Shot",
		Kind:          item.KindEtcItem,
		Duration:      -1,
		Stackable:     true,
		DefaultAction: item.ActionFishingShot,
		EtcItem:       &item.EtcItemDetail{Type: item.EtcItemShot},
	}})
	fishingShot := &item.Instance{ObjectID: 501, TemplateID: fishingShotID, Count: 100, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{fishingShot})
	gcl := &GameClientLink{}

	gcl.handleAutoSoulShot(live, clientpackets.RequestAutoSoulShot{ItemID: 999999, Type: 1})
	gcl.handleAutoSoulShot(live, clientpackets.RequestAutoSoulShot{ItemID: fishingShotID, Type: 1})

	if live.AutoSoulShotEnabled(fishingShotID) {
		t.Fatal("fishing shot was enabled for auto use")
	}
	if len(capture.frames) != 0 {
		t.Fatalf("frames = %x, want none", capture.frames)
	}
}

func assertExAutoSoulShotFrame(t *testing.T, frame []byte, itemID int32, enabled bool) {
	t.Helper()
	if len(frame) != 11 || frame[0] != serverpackets.OpcodeExtended {
		t.Fatalf("ExAutoSoulShot frame = %x", frame)
	}
	r := wire.NewReader(frame[1:])
	if second := r.ReadUint16(); second != serverpackets.OpcodeExAutoSoulShot {
		t.Fatalf("extended opcode = %#x, want %#x", second, serverpackets.OpcodeExAutoSoulShot)
	}
	if got := r.ReadInt32(); got != itemID {
		t.Fatalf("item id = %d, want %d", got, itemID)
	}
	wantEnabled := int32(0)
	if enabled {
		wantEnabled = 1
	}
	if got := r.ReadInt32(); got != wantEnabled {
		t.Fatalf("enabled = %d, want %d", got, wantEnabled)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read ExAutoSoulShot: %v", err)
	}
}

func assertSystemMessageItemFrame(t *testing.T, frame []byte, messageID int, itemID int32) {
	t.Helper()
	if len(frame) != 17 || frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("SystemMessage frame = %x", frame)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if got := r.ReadInt32(); got != 1 {
		t.Fatalf("param count = %d, want 1", got)
	}
	if got := r.ReadInt32(); got != serverpackets.SystemMessageParamItemName {
		t.Fatalf("param type = %d, want item name", got)
	}
	if got := r.ReadInt32(); got != itemID {
		t.Fatalf("item id = %d, want %d", got, itemID)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}
