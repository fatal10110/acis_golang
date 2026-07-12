package manager

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// staticGeo is a minimal move.Geo that never blocks movement, avoiding a
// real geo engine setup for tests that don't exercise pathing.
type staticGeo struct{}

func (staticGeo) CanMove(int, int, int, int, int, int) bool { return true }
func (staticGeo) Height(_, _, z int) int16                  { return int16(z) }

var neutralRates = item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

func monsterTemplate(id int) *npc.Template {
	return &npc.Template{
		ID: id, Type: "Monster", Level: 1,
		HPMax: 100, PAtk: 100, PDef: 10, DEX: 30, CritRate: 4,
		AtkSpd: 300, RunSpeed: 100, CollisionRadius: 10,
		BaseAttackRange: 40, CorpseTime: 7,
	}
}

// testHarness wires the same Decay/Respawn effects indirection cmd/gameserver
// is expected to use: both tasks need to call back into the not-yet-built
// Npcs, so their effects are built with a settable hook, then pointed at
// Npcs once it exists.
type testHarness struct {
	state   *world.State
	decay   *task.Decay
	respawn *task.Respawn
	ai      *task.AI
	npcs    *Npcs
	now     time.Time
}

type hookedDecayEffects struct {
	state *world.State
	hook  func(id int32) func()
}

func (e *hookedDecayEffects) Decay(actor task.DecayActor) {
	obj, ok := e.state.Object(actor.ObjectID())
	if !ok {
		return
	}
	var respawn func()
	if e.hook != nil {
		respawn = e.hook(actor.ObjectID())
	}
	if d, ok := obj.(interface {
		Decay(*world.State, func()) bool
	}); ok {
		d.Decay(e.state, respawn)
	}
}

type hookedRespawnEffects struct{ hook func(key string) }

func (e *hookedRespawnEffects) Respawn(key string) {
	if e.hook != nil {
		e.hook(key)
	}
}

func newHarness(t *testing.T, spawns *Spawns, templates *npc.Table) *testHarness {
	t.Helper()

	h := &testHarness{state: world.New(), now: time.UnixMilli(0)}
	nowFn := func() time.Time { return h.now }

	decayEffects := &hookedDecayEffects{state: h.state}
	decay, err := task.NewDecay(decayEffects, nowFn)
	if err != nil {
		t.Fatalf("NewDecay: %v", err)
	}
	respawnEffects := &hookedRespawnEffects{}
	respawn, err := task.NewRespawn(respawnEffects, nowFn)
	if err != nil {
		t.Fatalf("NewRespawn: %v", err)
	}
	ai := task.NewAI()

	ids := &sequentialIDs{}
	items := item.NewTable(nil)
	ground := &recordingGround{}

	npcs, err := NewNpcs(spawns, templates, staticGeo{}, h.state, ids, decay, respawn, ai, items, ground, neutralRates, nowFn, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewNpcs: %v", err)
	}
	decayEffects.hook = npcs.RespawnHook
	respawnEffects.hook = npcs.Respawn

	h.decay, h.respawn, h.ai, h.npcs = decay, respawn, ai, npcs
	return h
}

func fixedPositionEntry(npcID int32, total int, x, y, z, heading int, dbName string) spawn.Entry {
	return spawn.Entry{
		NPCID:  npcID,
		Total:  total,
		DBName: dbName,
		Positions: []spawn.Position{
			{Location: location.Location{X: x, Y: y, Z: z}, Heading: heading},
		},
	}
}

func testMakerSet(name string, maximumNPCs int) *commons.StatSet {
	set := commons.NewStatSetWithCapacity(2)
	set.Set("name", name)
	set.Set("maximumNpcs", maximumNPCs)
	return set
}

func testTerritory() *spawn.Territory {
	return &spawn.Territory{
		Name: "t1", MinZ: -100, MaxZ: 100,
		Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 0, Y: 100}},
	}
}

func TestPickPositionSingleKeepsDeclaredHeading(t *testing.T) {
	positions := []spawn.Position{{Location: location.Location{X: 1, Y: 2, Z: 3}, Heading: 999}}
	got := pickPosition(positions)
	if got.Heading != 999 || got.Location.X != 1 {
		t.Fatalf("pickPosition(single) = %+v, want the declared entry unchanged", got)
	}
}

func TestPickPositionWeightedDistributionAndRandomHeading(t *testing.T) {
	positions := []spawn.Position{
		{Location: location.Location{X: 1}, Chance: 20, Heading: 111},
		{Location: location.Location{X: 2}, Chance: 30, Heading: 222},
		{Location: location.Location{X: 3}, Chance: 50, Heading: 333},
	}

	const trials = 20000
	counts := map[int]int{}
	headingRandomized := false
	for i := 0; i < trials; i++ {
		pos := pickPosition(positions)
		counts[pos.Location.X]++
		if pos.Location.X == 1 && pos.Heading != 111 {
			headingRandomized = true
		}
	}

	if !headingRandomized {
		t.Fatal("pickPosition(weighted) never randomized heading, want the declared heading discarded")
	}

	// Roughly matches the declared 20/30/50 split; generous tolerance
	// keeps this non-flaky while still catching a badly broken roll.
	wantFrac := map[int]float64{1: 0.20, 2: 0.30, 3: 0.50}
	for x, want := range wantFrac {
		got := float64(counts[x]) / trials
		if got < want-0.03 || got > want+0.03 {
			t.Fatalf("pickPosition(weighted) x=%d frequency = %.3f, want ~%.2f", x, got, want)
		}
	}
}

func TestNewNpcsSpawnsFixedTotalIntoWorldState(t *testing.T) {
	tmpl := monsterTemplate(100)
	templates := npc.NewTable([]*npc.Template{tmpl})

	entry := fixedPositionEntry(100, 3, 1000, 2000, 0, 12345, "")
	maker, err := spawn.NewMaker(testMakerSet("maker1", 100), []*spawn.Territory{testTerritory()}, nil, []spawn.Entry{entry}, nil)
	if err != nil {
		t.Fatalf("NewMaker: %v", err)
	}
	table, err := spawn.NewTable([]*spawn.Territory{testTerritory()}, []*spawn.Maker{maker})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	spawns := NewSpawns(table, nil)

	h := newHarness(t, spawns, templates)

	if got := h.npcs.LiveCount(); got != 3 {
		t.Fatalf("LiveCount() = %d, want 3", got)
	}
	if got := h.npcs.DeferredCount(); got != 0 {
		t.Fatalf("DeferredCount() = %d, want 0", got)
	}

	found := 0
	for _, obj := range h.state.Objects() {
		hostile, ok := obj.(*npc.Hostile)
		if !ok {
			continue
		}
		x, y, z := hostile.Position()
		if x != 1000 || y != 2000 || z != 0 || hostile.Heading() != 12345 {
			t.Fatalf("spawned hostile at (%d,%d,%d,%d), want (1000,2000,0,12345)", x, y, z, hostile.Heading())
		}
		found++
	}
	if found != 3 {
		t.Fatalf("found %d live hostiles tracked in world state, want 3", found)
	}
}

func TestNewNpcsSkipsEntryWithoutExplicitPositions(t *testing.T) {
	tmpl := monsterTemplate(101)
	templates := npc.NewTable([]*npc.Template{tmpl})

	entry := spawn.Entry{NPCID: 101, Total: 5} // no Positions: territory-random, out of scope
	maker, err := spawn.NewMaker(testMakerSet("maker1", 100), []*spawn.Territory{testTerritory()}, nil, []spawn.Entry{entry}, nil)
	if err != nil {
		t.Fatalf("NewMaker: %v", err)
	}
	table, err := spawn.NewTable([]*spawn.Territory{testTerritory()}, []*spawn.Maker{maker})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	spawns := NewSpawns(table, nil)

	h := newHarness(t, spawns, templates)

	if got := h.npcs.LiveCount(); got != 0 {
		t.Fatalf("LiveCount() = %d, want 0 for a positionless entry", got)
	}
	if got := h.npcs.DeferredCount(); got != 1 {
		t.Fatalf("DeferredCount() = %d, want 1", got)
	}
}

func TestNewNpcsRestoresDeadDBNameEntryWithoutInstantiating(t *testing.T) {
	tmpl := monsterTemplate(102)
	templates := npc.NewTable([]*npc.Template{tmpl})

	entry := fixedPositionEntry(102, 1, 500, 500, 0, 0, "boss_1")
	entry.RespawnDelay = 30 * time.Minute
	maker, err := spawn.NewMaker(testMakerSet("maker1", 100), []*spawn.Territory{testTerritory()}, nil, []spawn.Entry{entry}, nil)
	if err != nil {
		t.Fatalf("NewMaker: %v", err)
	}
	table, err := spawn.NewTable([]*spawn.Territory{testTerritory()}, []*spawn.Maker{maker})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	state := spawn.NewState("boss_1")
	state.SetRespawn(10*time.Minute, time.UnixMilli(0)) // still 10 minutes out at boot
	spawns := NewSpawns(table, map[string]*spawn.State{"boss_1": state})

	h := newHarness(t, spawns, templates)

	if got := h.npcs.LiveCount(); got != 0 {
		t.Fatalf("LiveCount() = %d, want 0 for a still-dead persisted spawn", got)
	}
	if got := h.npcs.RestoredDeadCount(); got != 1 {
		t.Fatalf("RestoredDeadCount() = %d, want 1", got)
	}

	// Advance past the persisted deadline and let the respawn task fire.
	h.now = h.now.Add(11 * time.Minute)
	h.respawn.Tick()

	if got := h.npcs.LiveCount(); got != 1 {
		t.Fatalf("LiveCount() after respawn deadline = %d, want 1", got)
	}
}

func TestNewNpcsRestoresAliveDBNameEntryAtPersistedHP(t *testing.T) {
	tmpl := monsterTemplate(103)
	templates := npc.NewTable([]*npc.Template{tmpl})

	entry := fixedPositionEntry(103, 1, 500, 500, 0, 0, "unique_1")
	maker, err := spawn.NewMaker(testMakerSet("maker1", 100), []*spawn.Territory{testTerritory()}, nil, []spawn.Entry{entry}, nil)
	if err != nil {
		t.Fatalf("NewMaker: %v", err)
	}
	table, err := spawn.NewTable([]*spawn.Territory{testTerritory()}, []*spawn.Maker{maker})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	state := spawn.NewState("unique_1")
	state.Status = spawn.StatusAlive
	state.CurrentHP = 42
	state.Location = location.Location{X: 777, Y: 888, Z: 1}
	state.Heading = 555
	spawns := NewSpawns(table, map[string]*spawn.State{"unique_1": state})

	h := newHarness(t, spawns, templates)

	if got := h.npcs.LiveCount(); got != 1 {
		t.Fatalf("LiveCount() = %d, want 1", got)
	}

	var hostile *npc.Hostile
	for _, obj := range h.state.Objects() {
		if hh, ok := obj.(*npc.Hostile); ok {
			hostile = hh
		}
	}
	if hostile == nil {
		t.Fatal("restored hostile not found in world state")
	}
	if got := hostile.CurrentHP(); got != 42 {
		t.Fatalf("restored CurrentHP() = %d, want 42 (persisted value)", got)
	}
	x, y, z := hostile.Position()
	if x != 777 || y != 888 || z != 1 || hostile.Heading() != 555 {
		t.Fatalf("restored position = (%d,%d,%d,%d), want (777,888,1,555)", x, y, z, hostile.Heading())
	}
}

// TestSpawnedNpcKillRewardDecayRespawnChainFiresEndToEnd drives a lethal
// attack between two NPCs the spawn runtime itself constructed (not hand-
// built test fakes), proving the full kill -> reward -> decay -> respawn
// wiring is reachable from a real spawn, extending #482's own end-to-end
// combat test up through this issue's orchestration layer.
func TestSpawnedNpcKillRewardDecayRespawnChainFiresEndToEnd(t *testing.T) {
	attackerTpl := &npc.Template{
		ID: 200, Type: "Monster", Level: 1,
		HPMax: 100, PAtk: 300, PDef: 10, DEX: 30, CritRate: 4,
		AtkSpd: 300, RunSpeed: 100, CollisionRadius: 10, BaseAttackRange: 40,
	}
	defenderTpl := &npc.Template{
		ID: 201, Type: "Monster", Level: 1,
		HPMax: 10, PAtk: 10, PDef: 1, DEX: 30, CollisionRadius: 10,
		RunSpeed:   100,
		CorpseTime: 5,
		Drops: []item.DropCategory{
			{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: 57, Min: 10, Max: 10, Chance: 100}}},
		},
	}
	templates := npc.NewTable([]*npc.Template{attackerTpl, defenderTpl})

	attackerEntry := fixedPositionEntry(200, 1, 1000, 1000, 0, 0, "")
	defenderEntry := fixedPositionEntry(201, 1, 1010, 1000, 0, 0, "")
	defenderEntry.RespawnDelay = time.Minute
	maker, err := spawn.NewMaker(testMakerSet("maker1", 100), []*spawn.Territory{testTerritory()}, nil, []spawn.Entry{attackerEntry, defenderEntry}, nil)
	if err != nil {
		t.Fatalf("NewMaker: %v", err)
	}
	table, err := spawn.NewTable([]*spawn.Territory{testTerritory()}, []*spawn.Maker{maker})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	spawns := NewSpawns(table, nil)

	h := newHarness(t, spawns, templates)
	if got := h.npcs.LiveCount(); got != 2 {
		t.Fatalf("LiveCount() = %d, want 2", got)
	}

	var attacker, defender *npc.Hostile
	for _, obj := range h.state.Objects() {
		hostile, ok := obj.(*npc.Hostile)
		if !ok {
			continue
		}
		if hostile.Instance.Template.ID == 200 {
			attacker = hostile
		} else {
			defender = hostile
		}
	}
	if attacker == nil || defender == nil {
		t.Fatal("attacker/defender not found among spawned npcs")
	}
	defenderID := defender.ObjectID()

	attacker.SetRollSource(func(int) int { return 0 }) // guarantee a hit for a deterministic test
	controller := attack.NewAttackable(attacker)
	controller.DoAttack(defender)

	if !defender.Dead() {
		t.Fatal("defender.Dead() = false after a lethal hit, want true")
	}

	h.now = h.now.Add(6 * time.Second) // past CorpseTime
	h.decay.Tick()

	if !defender.Decayed() {
		t.Fatal("defender.Decayed() = false after decay tick, want true")
	}
	if _, ok := h.state.Object(defenderID); ok {
		t.Fatal("defender is still tracked in world state after decay")
	}
	if got := h.npcs.LiveCount(); got != 1 {
		t.Fatalf("LiveCount() after decay = %d, want 1 (attacker only)", got)
	}

	h.now = h.now.Add(2 * time.Minute) // past RespawnDelay
	h.respawn.Tick()

	if got := h.npcs.LiveCount(); got != 2 {
		t.Fatalf("LiveCount() after respawn = %d, want 2", got)
	}
}
