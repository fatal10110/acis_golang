package network

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestTaskEffectsWaterSendsCyanGauge(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 100, capture)
	state.AddPlayer(live)

	water, err := task.NewWater(NewTaskEffects(state), time.Now)
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}
	water.Add(live, 10*time.Second)

	if len(capture.frames) != 1 {
		t.Fatalf("captured frames = %d, want 1", len(capture.frames))
	}
	got := capture.frames[0]
	if got[0] != serverpackets.OpcodeSetupGauge {
		t.Fatalf("opcode = %#x, want %#x", got[0], serverpackets.OpcodeSetupGauge)
	}
	if color := binary.LittleEndian.Uint32(got[1:5]); color != uint32(serverpackets.GaugeCyan) {
		t.Fatalf("gauge color = %d, want %d", color, serverpackets.GaugeCyan)
	}
	if duration := binary.LittleEndian.Uint32(got[9:13]); duration != 10_000 {
		t.Fatalf("gauge duration = %d, want 10000", duration)
	}
}

func TestTaskEffectsDrownDamagesAndNotifiesLivePlayer(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 100, capture)
	state.AddPlayer(live)

	NewTaskEffects(state).Drown(live)

	if live.CurrentHP() >= 100 {
		t.Fatalf("current HP = %d, want drowning damage", live.CurrentHP())
	}
	if len(capture.frames) == 0 {
		t.Fatal("drowning sent no client frame")
	}
	got := capture.frames[len(capture.frames)-1]
	if got[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want %#x", got[0], serverpackets.OpcodeSystemMessage)
	}
	if message := binary.LittleEndian.Uint32(got[1:5]); message != serverpackets.SystemMessageDrownDamage {
		t.Fatalf("message = %d, want %d", message, serverpackets.SystemMessageDrownDamage)
	}
}

func TestTaskEffectsShadowItemExpiryDestroysAndUpdatesLivePlayer(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 100, capture)
	state.AddPlayer(live)

	inst := &item.Instance{ObjectID: 200, TemplateID: 30, Count: 1}
	shadow := &item.Template{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: 0, Weapon: &item.WeaponDetail{Type: item.WeaponSword}}
	inv := live.Inventory()
	inv.Add(inst)
	inv.EquipItem(inst, shadow)

	effects := NewTaskEffects(state)
	link := &GameClientLink{world: state}
	effects.SetShadowItemExpiry(link.ExpireShadowItem)
	updates := wireInventoryUpdates(link, live)
	updates.Tick()
	resetCapture(capture)

	items, err := task.NewShadowItems(effects)
	if err != nil {
		t.Fatalf("NewShadowItems() error = %v", err)
	}
	items.Track(live.ObjectID(), inst, shadow)
	items.Tick()
	updates.Tick()

	if inv.ItemByObjectID(inst.ObjectID) != nil {
		t.Fatal("expired shadow item remains in inventory")
	}
	if len(capture.frames) != 2 || capture.frames[0][0] != serverpackets.OpcodeUserInfo || capture.frames[1][0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("expiry frames = %x, want UserInfo then InventoryUpdate", capture.frames)
	}
}
