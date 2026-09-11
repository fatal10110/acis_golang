package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func newEquipTestLivePlayer(t *testing.T, id int32, capture *testsupport.FrameCapture, templates *item.Table, items []*item.Instance) *livePlayer {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch := &player.Character{
		ID: id, Name: "Player", ClassID: 0, BaseClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		CharLevel: 1,
		Location:  location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	ch.SetResourceValues(player.Resources{MaxHP: 80, CurrentHP: 80, MaxMP: 30, CurrentMP: 30})
	ch.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(ch.ID, templates, items))
	ch.SetFrameSender(capture.Send)
	ch.SetBroadcastFrameSender(capture.Send)

	live, err := creature.NewLive(ch.Location, tmpl.RunSpeed, testGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live

	return &livePlayer{Character: ch, template: tmpl, items: items, visibilitySend: capture.Send}
}

func assertStaticSystemMessageFrame(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	if len(frame) != 9 || frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("SystemMessage frame = %x", frame)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("system message params = %d, want 0", got)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

func assertChooseInventoryItemFrame(t *testing.T, frame []byte, itemID int32) {
	t.Helper()
	if len(frame) != 5 || frame[0] != serverpackets.OpcodeChooseInventoryItem {
		t.Fatalf("ChooseInventoryItem frame = %x", frame)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != itemID {
		t.Fatalf("ChooseInventoryItem item id = %d, want %d", got, itemID)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read ChooseInventoryItem: %v", err)
	}
}

func assertEnchantResultFrame(t *testing.T, frame []byte, result serverpackets.EnchantResult) {
	t.Helper()
	if len(frame) != 5 || frame[0] != serverpackets.OpcodeEnchantResult {
		t.Fatalf("EnchantResult frame = %x", frame)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(result) {
		t.Fatalf("EnchantResult result = %d, want %d", got, result)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read EnchantResult: %v", err)
	}
}

func assertSystemMessageIDFrame(t *testing.T, frame []byte, want int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(want) {
		t.Fatalf("SystemMessage id = %d, want %d", got, want)
	}
}

func assertForceChargeMessage(t *testing.T, frame []byte, messageID int, charges int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("message opcode = %#x, want SystemMessage (%#x)", frame[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("message id = %d, want %d", got, messageID)
	}
	params := r.ReadInt32()
	if messageID == serverpackets.SystemMessageForceIncreasedToS1 {
		if params != 1 || r.ReadInt32() != serverpackets.SystemMessageParamNumber || r.ReadInt32() != charges {
			t.Fatalf("force-increased message params = %d, want one number %d", params, charges)
		}
		return
	}
	if params != 0 {
		t.Fatalf("force-max message params = %d, want 0", params)
	}
}
