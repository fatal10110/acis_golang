package manager

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// testLog is the logger the live hostiles built by tests share.
var testLog = zerolog.Nop()

// spawnAllocCeiling bounds the heap allocations one standalone hostile spawn
// makes, its live wiring, settings and slot included.
const spawnAllocCeiling = 26

// TestSpawnAllocations bounds the allocations of one hostile's spawn: its
// shared settings, logger and wiring add no allocation of their own.
func TestSpawnAllocations(t *testing.T) {
	state := world.New()
	clock := func() time.Time { return time.Unix(1000, 0) }
	decay, _ := task.NewDecay(nopDecayEffects{}, clock)
	respawn, _ := task.NewRespawn(&respawnInto{}, clock)
	walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, clock, state)
	npcs, err := NewNpcs(NewSpawns(&spawn.Table{}, nil), npc.NewTable([]*npc.Template{
		{ID: 2, TemplateID: 2, Type: "Monster", HPMax: 100, RunSpeed: 100},
	}), fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, clock, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, effect.Env{Activity: task.NewEffects()}, npcQueues(), testMakers())
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	i := 0
	allocs := testing.AllocsPerRun(200, func() {
		i++
		if _, err := npcs.AddSpawn(2, (i%200)*50, (i/200)*50, 0, 0, false, 0); err != nil {
			t.Fatalf("AddSpawn() error: %v", err)
		}
	})
	t.Logf("allocations per hostile spawn: %.1f", allocs)
	if allocs > spawnAllocCeiling {
		t.Fatalf("allocations per hostile spawn = %.1f, want at most %d", allocs, spawnAllocCeiling)
	}

	queues := npcQueues()
	inst := &npc.Instance{ObjectID: 1, Template: &npc.Template{ID: 9001, Type: "Monster", RunSpeed: 100}, HasHome: true, Home: location.Location{}}
	positions := task.NewPositionUpdates(state)
	wiring := testing.AllocsPerRun(200, func() {
		if _, _, err := newLiveHostile(inst, 100, fakeGeo{}, positions, &testLog, nil, actorcast.EffectHandlers{}, walker, 20, 0, nil, effect.Env{}, queues.NewQueue("npc")); err != nil {
			t.Fatalf("newLiveHostile() error: %v", err)
		}
	})
	t.Logf("allocations per live hostile build: %.1f", wiring)
}
