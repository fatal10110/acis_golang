package manager

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// benchWorldSlots is about the number of spawn slots the shipped spawn list
// declares (the sum of its entries' totals).
const benchWorldSlots = 50_000

// BenchmarkMasterDecaySweep measures the slot sweeps a party leader's decay
// runs under the population lock, in a population of benchWorldSlots other
// spawns: the sweep of its gone script privates, and the drop of its taken
// privates' slots.
func BenchmarkMasterDecaySweep(b *testing.B) {
	state := world.New()
	clock := func() time.Time { return time.Unix(1000, 0) }
	decay, _ := task.NewDecay(nopDecayEffects{}, clock)
	respawn, _ := task.NewRespawn(&respawnInto{}, clock)
	walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, clock, state)
	npcs, err := NewNpcs(NewSpawns(&spawn.Table{}, nil), npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "Monster", HPMax: 100, RunSpeed: 100, Privates: []npc.PrivateEntry{{NpcID: 2, Weight: 3, RespawnDelay: 240 * time.Second}}},
		{ID: 2, TemplateID: 2, Type: "Monster", HPMax: 100, RunSpeed: 100},
	}), fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, clock, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, effect.Env{Activity: task.NewEffects()}, npcQueues(), testMakers())
	if err != nil {
		b.Fatalf("NewNpcs() error: %v", err)
	}
	for i := range benchWorldSlots {
		if _, err := npcs.AddSpawn(2, (i%200)*50, (i/200)*50, 0, 0, false, 0); err != nil {
			b.Fatalf("AddSpawn() error: %v", err)
		}
	}
	live, err := npcs.AddSpawn(1, 0, 0, 0, 0, false, 0)
	if err != nil {
		b.Fatalf("AddSpawn() error: %v", err)
	}
	master := live.(*npc.Hostile)
	if err := npcs.CreatePrivates(master); err != nil {
		b.Fatalf("CreatePrivates() error: %v", err)
	}
	if !master.SpawnedPrivates() {
		b.Fatal("the master spawned no private")
	}

	b.ResetTimer()
	for range b.N {
		npcs.dropGoneScriptPrivates(master)
		npcs.dropPrivateSlots(master, nil, nil)
	}
}
