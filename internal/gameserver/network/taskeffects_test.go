package network

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
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

// mustCuboid panics on error, only for fixed test-literal coordinates known
// valid at compile time.
func mustCuboid(x1, x2, y1, y2, z1, z2 int) zone.Form {
	f, err := zone.NewCuboid(x1, x2, y1, y2, z1, z2)
	if err != nil {
		panic(err)
	}
	return f
}

// TestResolveIsGMUsesAccessLevelsIsGMFlag pins liveZoneActor.GM() to
// accessLevels.xml's isGM attribute (Player.isGM() / AccessLevel.isGm() in
// the reference) rather than treating every positive access level as GM:
// levels 1-6 (Chat Moderator .. Head GM) are not GM, only 7-8 (Admin,
// Master) are.
func TestResolveIsGMUsesAccessLevelsIsGMFlag(t *testing.T) {
	levels := []admin.AccessLevel{
		{Level: 0, IsGM: false},
		{Level: 3, Name: "Head GM", IsGM: false},
		{Level: 7, Name: "Admin", IsGM: true},
		{Level: 8, Name: "Master", IsGM: true},
	}
	data, err := admin.NewData(levels, nil)
	if err != nil {
		t.Fatalf("admin.NewData() error: %v", err)
	}

	cases := []struct {
		accessLevel int
		want        bool
	}{
		{accessLevel: 0, want: false},
		{accessLevel: 3, want: false},
		{accessLevel: 7, want: true},
		{accessLevel: 8, want: true},
		{accessLevel: 99, want: false}, // undefined level: not found, not GM.
	}
	for _, c := range cases {
		if got := resolveIsGM(data, c.accessLevel); got != c.want {
			t.Errorf("resolveIsGM(data, %d) = %v, want %v", c.accessLevel, got, c.want)
		}
	}

	if resolveIsGM(nil, 7) {
		t.Error("resolveIsGM(nil, 7) = true, want false")
	}

	live := &livePlayer{isGM: true}
	actor := &liveZoneActor{live: live}
	if !actor.GM() {
		t.Error("liveZoneActor.GM() = false, want true when live.isGM is true")
	}
}

func TestPvPZoneMembershipTracksZoneTransitions(t *testing.T) {
	state := world.New()
	live := newTestLivePlayer(t, 100, &frameCapture{})
	live.zoneActor = &liveZoneActor{live: live}
	state.Spawn(live, 125, 0, 0, 0)
	state.AddPlayer(live)

	zones := zone.NewIndex()
	zones.Add(zone.NewArena(1, mustCuboid(100, 200, -100, 100, -100, 100)))
	zones.Add(zone.NewArena(2, mustCuboid(150, 250, -100, 100, -100, 100)))
	zones.Add(zone.NewPeace(3, mustCuboid(175, 225, -100, 100, -100, 100)))
	link := &GameClientLink{world: state, zones: zones, log: zerolog.Nop()}

	live.zoneActor.revalidate(zones)
	if !live.InPvPZone() {
		t.Fatal("spawning in a PvP zone did not set membership")
	}

	link.updateLivePlayerPosition(live, location.Location{X: 175}, 0)
	if live.InPvPZone() {
		t.Fatal("overlapping peace zone did not suppress PvP membership")
	}

	link.updateLivePlayerPosition(live, location.Location{X: 226}, 0)
	if !live.InPvPZone() {
		t.Fatal("leaving overlapping peace zone cleared PvP membership")
	}

	link.updateLivePlayerPosition(live, location.Location{X: 500}, 0)
	if live.InPvPZone() {
		t.Fatal("leaving all PvP zones retained membership")
	}

	link.updateLivePlayerPosition(live, location.Location{X: 125}, 0)
	link.detachLivePlayer(context.Background(), live)
	if live.InPvPZone() {
		t.Fatal("detach retained PvP-zone membership")
	}
}

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
	zones.Add(zone.NewWater(1, mustCuboid(1000, 2000, -100, 100, -100, 100)))
	link := &GameClientLink{world: state, zones: zones, water: water, playerConfig: PlayerConfig{AllowWater: true}}
	link.wireWaterZones()

	link.updateLivePlayerPosition(live, location.Location{X: 1500}, 0)
	link.updateLivePlayerPosition(live, location.Location{}, 0)

	if len(capture.frames) != 4 {
		t.Fatalf("water-zone frames = %d, want 4 (UserInfo+gauge on enter, UserInfo+gauge on exit)", len(capture.frames))
	}
	gauges := framesWithOpcode(capture.frames, serverpackets.OpcodeSetupGauge)
	if len(gauges) != 2 {
		t.Fatalf("gauge frames = %d, want 2", len(gauges))
	}
	if current := binary.LittleEndian.Uint32(gauges[0][5:9]); current != 120_000 {
		t.Fatalf("breath gauge currentTime = %d, want 120000", current)
	}
	if duration := binary.LittleEndian.Uint32(gauges[0][9:13]); duration != 120_000 {
		t.Fatalf("breath gauge maxTime = %d, want 120000", duration)
	}
	if current := binary.LittleEndian.Uint32(gauges[1][5:9]); current != 0 {
		t.Fatalf("exit gauge currentTime = %d, want 0", current)
	}
	if duration := binary.LittleEndian.Uint32(gauges[1][9:13]); duration != 0 {
		t.Fatalf("exit gauge maxTime = %d, want 0", duration)
	}
	userInfos := framesWithOpcode(capture.frames, serverpackets.OpcodeUserInfo)
	if len(userInfos) != 2 {
		t.Fatalf("UserInfo frames = %d, want 2 (broadcastUserInfo on enter and exit)", len(userInfos))
	}
}

func framesWithOpcode(frames [][]byte, opcode byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if len(f) > 0 && f[0] == opcode {
			out = append(out, f)
		}
	}
	return out
}

func TestWaterZoneMovementNoOpWhenAllowWaterDisabled(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 102, capture)
	live.zoneActor = &liveZoneActor{live: live}
	state.Spawn(live, 0, 0, 0, 0)
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	water, err := task.NewWater(effects, time.Now)
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, mustCuboid(1000, 2000, -100, 100, -100, 100)))
	link := &GameClientLink{world: state, zones: zones, water: water, playerConfig: PlayerConfig{AllowWater: false}}
	link.wireWaterZones()

	link.updateLivePlayerPosition(live, location.Location{X: 1500}, 0)
	link.updateLivePlayerPosition(live, location.Location{}, 0)

	if len(capture.frames) != 0 {
		t.Fatalf("water-zone frames = %d, want 0 when AllowWater is disabled", len(capture.frames))
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
	zones.Add(zone.NewWater(1, mustCuboid(boundary-100, boundary+100, -100, 100, -100, 100)))
	link := &GameClientLink{world: state, zones: zones, water: water, playerConfig: PlayerConfig{AllowWater: true}}
	link.wireWaterZones()

	link.updateLivePlayerPosition(live, location.Location{X: boundary - 1}, 0)
	link.updateLivePlayerPosition(live, location.Location{X: boundary + 1}, 0)

	if len(capture.frames) != 2 {
		t.Fatalf("cross-region water frames = %d, want 2 (UserInfo+gauge)", len(capture.frames))
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
	zones.Add(zone.NewWater(1, mustCuboid(1000, 2000, -100, 100, -100, 100)))
	link := &GameClientLink{world: state, zones: zones, water: water, playerConfig: PlayerConfig{AllowWater: true}}
	link.wireWaterZones()
	live.SetZoneRevalidator(func(previous location.Location) { link.revalidateZones(live, previous) })

	live.SyncPosition(location.Location{X: 1500})

	if len(capture.frames) != 2 {
		t.Fatalf("server-sync water frames = %d, want 2 (UserInfo+gauge)", len(capture.frames))
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
	water := zone.NewWater(1, mustCuboid(1000, 2000, -100, 100, -100, 100))
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

func TestTaskEffectsSavePersistsOnlineLivePlayer(t *testing.T) {
	state := world.New()
	chars := newFakeCharStore()
	items := newFakeItemStore()
	roster := gamemanager.NewRoster(chars, items, nil, testTemplates(t), testItemTemplates(), npc.NewTable(nil), &sequentialIDs{next: 100}, gamemanager.DefaultDeleteAfter, time.Now)
	live := newTestLivePlayer(t, 101, &frameCapture{})
	if err := chars.Create(context.Background(), live.Character); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, zerolog.Nop())
	autosave, err := task.NewAutosave(effects, time.Now)
	if err != nil {
		t.Fatalf("NewAutosave() error = %v", err)
	}
	autosave.Add(live)

	effects.Save(live)

	if got := chars.saves(live.ObjectID()); got != 1 {
		t.Fatalf("saves(%d) = %d, want 1", live.ObjectID(), got)
	}
}

// TestTaskEffectsSaveStillPersistsMidDetachLivePlayer pins Save to still
// running for a session that started detaching but hasn't yet left world
// state: detachLivePlayer itself never persists level/exp/sp/HP-CP-MP
// (issue #1198 is still open), so an autosave tick landing in that window
// is the only thing that would catch it. Skipping here, as an earlier
// revision did, silently dropped that state instead.
func TestTaskEffectsSaveStillPersistsMidDetachLivePlayer(t *testing.T) {
	state := world.New()
	chars := newFakeCharStore()
	items := newFakeItemStore()
	roster := gamemanager.NewRoster(chars, items, nil, testTemplates(t), testItemTemplates(), npc.NewTable(nil), &sequentialIDs{next: 100}, gamemanager.DefaultDeleteAfter, time.Now)
	live := newTestLivePlayer(t, 101, &frameCapture{})
	if err := chars.Create(context.Background(), live.Character); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	state.AddPlayer(live)
	live.shadowExpiryMu.Lock()
	live.detaching = true
	live.shadowExpiryMu.Unlock()

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, zerolog.Nop())

	effects.Save(live)

	if got := chars.saves(live.ObjectID()); got != 1 {
		t.Fatalf("saves(%d) = %d, want 1 for a mid-detach session", live.ObjectID(), got)
	}
}

// TestTaskEffectsSaveSkipsRemovedLivePlayer pins the real "gone" gate:
// once RemovePlayer has actually run (world.State no longer resolves the
// actor), Save has nothing to save against and must not look one up.
func TestTaskEffectsSaveSkipsRemovedLivePlayer(t *testing.T) {
	state := world.New()
	chars := newFakeCharStore()
	items := newFakeItemStore()
	roster := gamemanager.NewRoster(chars, items, nil, testTemplates(t), testItemTemplates(), npc.NewTable(nil), &sequentialIDs{next: 100}, gamemanager.DefaultDeleteAfter, time.Now)
	live := newTestLivePlayer(t, 101, &frameCapture{})

	effects := NewTaskEffects(state)
	effects.SetAutosave(roster, zerolog.Nop())

	effects.Save(live)

	if got := chars.saves(live.ObjectID()); got != 0 {
		t.Fatalf("saves(%d) = %d, want 0 once removed from world state", live.ObjectID(), got)
	}
}

func TestTaskEffectsSaveWithoutRosterIsNoop(t *testing.T) {
	state := world.New()
	live := newTestLivePlayer(t, 101, &frameCapture{})
	state.AddPlayer(live)

	effects := NewTaskEffects(state)
	// SetAutosave never called: roster stays nil.
	effects.Save(live)
}
