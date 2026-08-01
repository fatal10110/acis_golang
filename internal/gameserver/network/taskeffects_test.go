package network

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
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

func TestWaterZoneMovementUsesBreathStatAndClearsGaugeOnExit(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 100, capture)
	live.zoneActor = &liveZoneActor{live: live}
	state.Spawn(live, 0, 0, 0, 0)
	state.AddPlayer(live)
	live.AddStatFuncs([]basefunc.Func{basefunc.NewMul(nil, stat.Breath, 2, nil)})

	effects := NewTaskEffects(state)
	water, err := task.NewWater(effects, time.Now)
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, zone.NewCuboid(1000, 2000, -100, 100, -100, 100)))
	link := &GameClientLink{world: state, zones: zones, water: water}
	link.wireWaterZones()

	link.updateLivePlayerPosition(live, location.Location{X: 1500}, 0)
	link.updateLivePlayerPosition(live, location.Location{}, 0)

	if len(capture.frames) != 2 {
		t.Fatalf("water-zone frames = %d, want 2", len(capture.frames))
	}
	if duration := binary.LittleEndian.Uint32(capture.frames[0][9:13]); duration != 120_000 {
		t.Fatalf("breath gauge duration = %d, want 120000", duration)
	}
	if duration := binary.LittleEndian.Uint32(capture.frames[1][9:13]); duration != 0 {
		t.Fatalf("exit gauge duration = %d, want 0", duration)
	}
}

func TestWaterZoneMovementAcrossRegionKeepsCountdown(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 101, capture)
	live.zoneActor = &liveZoneActor{live: live}
	boundary := world.MinX + (world.MaxX-world.MinX+1)/world.RegionsX
	state.Spawn(live, boundary-200, 0, 0, 0)
	state.AddPlayer(live)

	water, err := task.NewWater(NewTaskEffects(state), time.Now)
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, zone.NewCuboid(boundary-100, boundary+100, -100, 100, -100, 100)))
	link := &GameClientLink{world: state, zones: zones, water: water}
	link.wireWaterZones()

	link.updateLivePlayerPosition(live, location.Location{X: boundary - 1}, 0)
	link.updateLivePlayerPosition(live, location.Location{X: boundary + 1}, 0)

	if len(capture.frames) != 1 {
		t.Fatalf("cross-region water frames = %d, want 1", len(capture.frames))
	}
}

func TestWaterZoneServerPositionSyncStartsCountdown(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 102, capture)
	live.zoneActor = &liveZoneActor{live: live}
	live.SetWorld(state)
	state.Spawn(live, 0, 0, 0, 0)
	state.AddPlayer(live)

	water, err := task.NewWater(NewTaskEffects(state), time.Now)
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, zone.NewCuboid(1000, 2000, -100, 100, -100, 100)))
	link := &GameClientLink{world: state, zones: zones, water: water}
	link.wireWaterZones()
	live.SetZoneRevalidator(func(previous location.Location) { link.revalidateZones(live, previous) })

	live.SyncPosition(location.Location{X: 1500})

	if len(capture.frames) != 1 {
		t.Fatalf("server-sync water frames = %d, want 1", len(capture.frames))
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
