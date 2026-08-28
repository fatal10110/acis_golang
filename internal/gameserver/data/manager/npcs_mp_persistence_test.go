package manager

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// nopDecayEffects and nopRespawnEffects satisfy task.Decay/task.Respawn's
// effect interfaces without doing anything: this test's spawn stays alive
// throughout, so neither corpse decay nor a respawn ever fires.
type nopDecayEffects struct{}

func (nopDecayEffects) Decay(task.DecayActor) {}

type nopRespawnEffects struct{}

func (nopRespawnEffects) Respawn(string) {}

// bootMPTestNpcs builds one Npcs instance around a single database-tracked
// "mp_test" spawn entry (npc id 1, 100 max HP, 50 max MP), backed by spawns.
// Each call gets its own world/AI/id-allocator dependencies, so it can stand
// in for a fresh server boot while spawns carries persisted state across.
func bootMPTestNpcs(t *testing.T, spawns *Spawns, templates *npc.Table) (*Npcs, *world.State) {
	t.Helper()
	state := world.New()
	ids := &sequentialIDs{}
	decay, err := task.NewDecay(nopDecayEffects{}, time.Now)
	if err != nil {
		t.Fatalf("NewDecay() error: %v", err)
	}
	respawnTask, err := task.NewRespawn(nopRespawnEffects{}, time.Now)
	if err != nil {
		t.Fatalf("NewRespawn() error: %v", err)
	}
	ai := task.NewAI(state, zerolog.Nop())
	positions := task.NewPositionUpdates(state)
	items := item.NewTable(nil)

	npcs, err := NewNpcs(spawns, templates, fakeGeo{}, state, ids, decay, respawnTask, ai, positions, items,
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{})
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	return npcs, state
}

// mpTestTemplate returns the single npc template ("id 1") the fixture's
// database-tracked spawn entry resolves against: 100 max HP, 50 max MP.
func mpTestTemplate() *npc.Table {
	return npc.NewTable([]*npc.Template{{
		ID:         1,
		TemplateID: 1,
		Type:       "Monster",
		HPMax:      100,
		MPMax:      50,
		RunSpeed:   100,
		AIParams:   commons.NewStatSet(),
	}})
}

// mpTestSpawnFixture writes a spawnlist with one database-tracked entry
// ("mp_test") inside a walkable territory, matching the shape
// TestNewSpawnsCreatesMissingStateRows already uses.
func mpTestSpawnFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "20_20.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="100" y="0"/>
		<node x="100" y="100"/>
		<node x="0" y="100"/>
	</territory>
	<npcmaker name="maker" territory="field" maximumNpcs="1">
		<npc id="1" total="1" dbName="mp_test"/>
	</npcmaker>
</list>`)
	return dir
}

// TestNpcSpawnRestoresPersistedMPAcrossRestart pins issue #1942: a
// database-tracked spawn's saved spawn_data.current_mp must round-trip
// through a live Hostile and back, the same way current_hp already does
// (aCis SpawnData.java:181-194 saves both; ASpawn.java:460-468 restores both
// together through setHpMp). Before the fix, npcs_respawn.go always wrote
// mp=0 at sync and npcs_spawn.go always restored full MP at boot.
func TestNpcSpawnRestoresPersistedMPAcrossRestart(t *testing.T) {
	dir := mpTestSpawnFixture(t)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	templates := mpTestTemplate()

	seed := &spawn.State{Name: "mp_test", Status: spawn.StatusAlive, CurrentHP: 80, CurrentMP: 30}
	spawns := NewSpawns(table, map[string]*spawn.State{"mp_test": seed})

	npcs1, state1 := bootMPTestNpcs(t, spawns, templates)
	if got, want := npcs1.LiveCount(), 1; got != want {
		t.Fatalf("first boot LiveCount() = %d, want %d", got, want)
	}
	obj, ok := state1.Object(1)
	if !ok {
		t.Fatal("first boot: spawned npc object id 1 not found")
	}
	hostile, ok := obj.(*npc.Hostile)
	if !ok {
		t.Fatalf("first boot: object id 1 is %T, want *npc.Hostile", obj)
	}
	if got, want := hostile.CurrentMP(), 30; got != want {
		t.Fatalf("restored CurrentMP() = %d, want %d (persisted spawn_data.current_mp)", got, want)
	}

	// Spend MP the way a hostile-NPC cast does (cast/controller.go's charge
	// path), then sync live state back into spawns the way shutdown does.
	if spent := hostile.ReduceMP(10); spent != 10 {
		t.Fatalf("ReduceMP(10) = %v, want 10", spent)
	}
	npcs1.SyncPersistedState()

	persisted, ok := spawns.State("mp_test")
	if !ok {
		t.Fatal("spawns.State(mp_test) missing after SyncPersistedState")
	}
	if got, want := persisted.CurrentMP, 20; got != want {
		t.Fatalf("persisted CurrentMP after sync = %d, want %d", got, want)
	}

	// Second boot: same spawns store (as a restarted process would reload),
	// fresh world/id/task dependencies.
	npcs2, state2 := bootMPTestNpcs(t, spawns, templates)
	obj2, ok := state2.Object(1)
	if !ok {
		t.Fatal("second boot: spawned npc object id 1 not found")
	}
	hostile2, ok := obj2.(*npc.Hostile)
	if !ok {
		t.Fatalf("second boot: object id 1 is %T, want *npc.Hostile", obj2)
	}
	if got, want := hostile2.CurrentMP(), 20; got != want {
		t.Fatalf("restart-restored CurrentMP() = %d, want %d", got, want)
	}
	if got, want := npcs2.LiveCount(), 1; got != want {
		t.Fatalf("second boot LiveCount() = %d, want %d", got, want)
	}
}
