package network

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
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
	if current := binary.LittleEndian.Uint32(got[5:9]); current != 10_000 {
		t.Fatalf("gauge currentTime = %d, want 10000", current)
	}
	if duration := binary.LittleEndian.Uint32(got[9:13]); duration != 10_000 {
		t.Fatalf("gauge maxTime = %d, want 10000", duration)
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
	if current := binary.LittleEndian.Uint32(capture.frames[0][5:9]); current != 120_000 {
		t.Fatalf("breath gauge currentTime = %d, want 120000", current)
	}
	if duration := binary.LittleEndian.Uint32(capture.frames[0][9:13]); duration != 120_000 {
		t.Fatalf("breath gauge maxTime = %d, want 120000", duration)
	}
	if current := binary.LittleEndian.Uint32(capture.frames[1][5:9]); current != 0 {
		t.Fatalf("exit gauge currentTime = %d, want 0", current)
	}
	if duration := binary.LittleEndian.Uint32(capture.frames[1][9:13]); duration != 0 {
		t.Fatalf("exit gauge maxTime = %d, want 0", duration)
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

func TestZoneRevalidationSerializesClientAndServerPositions(t *testing.T) {
	state := world.New()
	live := newTestLivePlayer(t, 103, &frameCapture{})
	live.zoneActor = &liveZoneActor{live: live}
	live.SetWorld(state)
	state.Spawn(live, 0, 0, 0, 0)
	state.AddPlayer(live)

	entered := make(chan struct{})
	releaseEnter := make(chan struct{})
	water := zone.NewWater(1, zone.NewCuboid(1000, 2000, -100, 100, -100, 100))
	water.OnEnter(func(zone.Actor) {
		close(entered)
		<-releaseEnter
	})
	zones := zone.NewIndex()
	zones.Add(water)
	link := &GameClientLink{world: state, zones: zones}
	live.SetZoneRevalidator(func(previous location.Location) { link.revalidateZones(live, previous) })

	clientDone := make(chan struct{})
	go func() {
		link.updateLivePlayerPosition(live, location.Location{X: 1500}, 0)
		close(clientDone)
	}()
	<-entered

	serverDone := make(chan struct{})
	go func() {
		live.SyncPosition(location.Location{})
		close(serverDone)
	}()
	select {
	case <-serverDone:
		t.Fatal("server position sync interleaved with zone entry")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseEnter)
	<-clientDone
	<-serverDone

	if water.Inside(live.zoneActor) {
		t.Fatal("outside player remains in water zone")
	}
	if live.zoneActor.ZoneFlags().Has(zone.FlagWater) {
		t.Fatal("outside player remains flagged as swimming")
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

// TestTaskEffectsDrownDoesNotBreakInFlightCast pins WaterTaskManager.java:53
// (player.reduceCurrentHp(hp, player, false, false, null)) against
// PlayerStatus.reduceHp (PlayerStatus.java:96-193): calcCastBreak is only
// invoked from attack handlers (Pdam/Mdam/Blow/CreatureAttack/etc.), never
// from reduceHp, so a drowning tick never rolls to interrupt an in-progress
// cast. Drown must go through the DOT-style HP path (ReduceHPByDOT), not the
// normal-hit path (ReduceHP, which runs breakCastOnDamage).
func TestTaskEffectsDrownDoesNotBreakInFlightCast(t *testing.T) {
	live := newTestLivePlayer(t, 100, &frameCapture{})
	state := world.New()
	state.AddPlayer(live)

	gcl := &GameClientLink{log: zerolog.Nop()}
	controller := gcl.castController(live)
	castingDef := modelskill.Definition{ID: 9, Level: 1, HitTime: 5000, StaticHitTime: true, StaticReuse: true}
	if _, err := controller.Start(time.Now(), skillCastObject(live), castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	NewTaskEffects(state).Drown(live)

	if !controller.CastingNow() {
		t.Fatal("CastingNow() = false after drowning damage, want the in-flight cast to survive")
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
	link := &GameClientLink{world: state, inventory: invops.NewService(nil)}
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
	if len(capture.frames) != 3 || capture.frames[0][0] != serverpackets.OpcodeUserInfo || capture.frames[2][0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("expiry frames = %x, want UserInfo, SystemMessage, InventoryUpdate", capture.frames)
	}
	msg := capture.frames[1]
	if msg[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want %#x", msg[0], serverpackets.OpcodeSystemMessage)
	}
	if id := binary.LittleEndian.Uint32(msg[1:5]); id != serverpackets.SystemMessageRemainingManaIsNow0 {
		t.Fatalf("message id = %d, want %d", id, serverpackets.SystemMessageRemainingManaIsNow0)
	}
	if itemID := binary.LittleEndian.Uint32(msg[13:17]); itemID != uint32(shadow.ID) {
		t.Fatalf("message item id = %d, want %d", itemID, shadow.ID)
	}
}

func TestShadowItemExpiryWaitsForDetach(t *testing.T) {
	state := world.New()
	live := newTestLivePlayer(t, 101, &frameCapture{})
	state.Spawn(live, 0, 0, 0, 0)
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	shadows, err := task.NewShadowItems(effects)
	if err != nil {
		t.Fatalf("NewShadowItems() error = %v", err)
	}
	link := &GameClientLink{world: state, inventory: invops.NewService(nil), shadowItems: shadows, log: zerolog.Nop()}
	inst := &item.Instance{ObjectID: 201, TemplateID: 30, Count: 1}
	tmpl := &item.Template{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: 0, Weapon: &item.WeaponDetail{Type: item.WeaponSword}}
	inv := live.Inventory()
	inv.Add(inst)
	inv.EquipItem(inst, tmpl)

	expiryStarted := make(chan struct{})
	releaseExpiry := make(chan struct{})
	effects.SetShadowItemExpiry(func(actor *livePlayer, expired *item.Instance) {
		close(expiryStarted)
		<-releaseExpiry
		link.ExpireShadowItem(actor, expired)
	})
	shadows.Track(live.ObjectID(), inst, tmpl)
	tickDone := make(chan struct{})
	go func() {
		shadows.Tick()
		close(tickDone)
	}()
	<-expiryStarted

	detachDone := make(chan struct{})
	go func() {
		link.detachLivePlayer(context.Background(), live)
		close(detachDone)
	}()
	select {
	case <-detachDone:
		t.Fatal("detach completed while shadow expiry was admitted")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseExpiry)
	<-tickDone
	<-detachDone

	if inv.ItemByObjectID(inst.ObjectID) != nil {
		t.Fatal("expired shadow item remains in inventory")
	}
	if shadows.Tracked(inst) {
		t.Fatal("expired shadow item remains tracked")
	}
}
