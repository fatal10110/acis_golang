package manager

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// despawnFixture is a population of two maker wolves, a database-tracked
// boss and nothing else, over a respawn tracker that never fires on its
// own.
type despawnFixture struct {
	npcs      *Npcs
	state     *world.State
	decay     *task.Decay
	respawn   *task.Respawn
	queues    *sim.Inline
	templates *npc.Table
}

const despawnSpawnlist = `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<npcmaker name="wolves" territory="field" maximumNpcs="2">
		<npc id="1" total="2" pos="10;20;0;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="lair" territory="field" maximumNpcs="1">
		<npc id="2" total="1" pos="50;50;0;0" respawn="60sec" dbName="boss_db"/>
	</npcmaker>
</list>`

func despawnTable(t *testing.T, body string) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "despawn.xml"), body)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	return table
}

func newDespawnFixture(t *testing.T, states map[string]*spawn.State) *despawnFixture {
	t.Helper()
	f := &despawnFixture{state: world.New(), queues: npcQueues()}
	f.decay, _ = task.NewDecay(nopDecayEffects{}, time.Now)
	f.respawn, _ = task.NewRespawn(nopRespawnEffects{}, time.Now)
	walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, time.Now, f.state)
	f.templates = npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "Monster", Name: "Wolf", HPMax: 100, RunSpeed: 100, AIParams: commons.NewStatSet()},
		{ID: 2, TemplateID: 2, Type: "Monster", Name: "Boss", HPMax: 100, RunSpeed: 100, AIParams: commons.NewStatSet()},
		{ID: 3, TemplateID: 3, Type: "ChristmasTree", Name: "Tree", HPMax: 100, AIParams: commons.NewStatSet()},
	})
	var err error
	f.npcs, err = NewNpcs(NewSpawns(despawnTable(t, despawnSpawnlist), states), f.templates, fakeGeo{}, f.state, &sequentialIDs{}, f.decay, f.respawn,
		task.NewAI(f.state, zerolog.Nop()), task.NewPositionUpdates(f.state), item.NewTable(nil), &recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(),
		nil, actorcast.EffectHandlers{}, walker, nil, effect.Env{Activity: task.NewEffects()}, f.queues)
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	return f
}

// npcObjects returns the NPCs in the world.
func (f *despawnFixture) npcObjects() []world.Tracked {
	var out []world.Tracked
	for _, obj := range f.state.Objects() {
		switch obj.(type) {
		case *npc.Hostile, *npc.Folk, *npc.Decoration, *npc.EffectPoint:
			out = append(out, obj)
		}
	}
	return out
}

func (f *despawnFixture) hostile(t *testing.T, npcID int) *npc.Hostile {
	t.Helper()
	for _, obj := range f.npcObjects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == npcID {
			return h
		}
	}
	t.Fatalf("no npc %d in the world", npcID)
	return nil
}

// hostiles returns every hostile of the template in the world.
func (f *despawnFixture) hostiles(npcID int) []*npc.Hostile {
	var out []*npc.Hostile
	for _, obj := range f.npcObjects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == npcID {
			out = append(out, h)
		}
	}
	return out
}

// DespawnAll is SpawnManager.despawn plus World.deleteVisibleNpcSpawns:
// every NPC leaves, the maker's, the standalone and the one no spawn
// placed; a respawn already armed is cancelled, a corpse decaying meanwhile
// arms none, and the database-tracked row is reset so the next save drops
// it.
func TestDespawnAllRemovesEveryNpcForGood(t *testing.T) {
	f := newDespawnFixture(t, nil)
	// The maker's wolves, taken before the standalone wolf of the same
	// template joins them: world iteration order cannot tell them apart.
	makerWolves := f.hostiles(1)
	if len(makerWolves) != 2 {
		t.Fatalf("maker wolves = %d, want 2", len(makerWolves))
	}
	wolf, _ := f.templates.Get(1)
	if err := f.npcs.SpawnFixed(wolf, 70, 70, 0, 0); err != nil {
		t.Fatalf("SpawnFixed() error: %v", err)
	}
	tree, _ := f.templates.Get(3)
	inst, _ := npc.NewInstance(1000, tree)
	decoration, err := npc.NewDecoration(inst, "")
	if err != nil {
		t.Fatalf("NewDecoration() error: %v", err)
	}
	decoration.Attach(npc.DecorationRuntime{World: f.state})
	decoration.Spawn(80, 80, 0, 0)
	if got := len(f.npcObjects()); got != 5 {
		t.Fatalf("npcs in the world = %d, want 5", got)
	}
	if state, _ := f.npcs.Spawns().State("boss_db"); state.Status != spawn.StatusAlive {
		t.Fatalf("boss row status = %d, want alive", state.Status)
	}

	// One wolf is removed: its slot's respawn is armed.
	first := makerWolves[0]
	first.DeleteMe()
	f.queues.Run()
	if !f.respawn.Tracked("wolves#0#0") && !f.respawn.Tracked("wolves#0#1") {
		t.Fatal("removed wolf armed no respawn")
	}
	// The other wolf's corpse decays while the despawn runs: its respawn
	// hook resolved first, the arming lands after.
	second := makerWolves[1]
	arm := f.npcs.RespawnHook(second.ObjectID())
	if arm == nil {
		t.Fatal("maker wolf resolved no respawn hook")
	}
	boss := f.hostile(t, 2)
	f.decay.Add(boss, time.Minute)

	if got := f.npcs.DespawnAll(); got != 4 {
		t.Fatalf("DespawnAll() = %d, want 4 npcs deleted", got)
	}
	arm()
	f.queues.Run()

	if got := f.npcObjects(); len(got) != 0 {
		t.Fatalf("npcs left in the world: %d", len(got))
	}
	for _, key := range []string{"wolves#0#0", "wolves#0#1", "boss_db"} {
		if f.respawn.Tracked(key) {
			t.Fatalf("respawn of %s still armed", key)
		}
	}
	if f.decay.Tracked(boss) {
		t.Fatal("boss corpse decay not cancelled")
	}
	if got := f.npcs.LiveCount(); got != 0 {
		t.Fatalf("LiveCount() = %d, want 0", got)
	}
	if state, _ := f.npcs.Spawns().State("boss_db"); state.Status != spawn.StatusUninitialized {
		t.Fatalf("boss row status = %d, want uninitialized", state.Status)
	}
	// A respawn firing late finds no spawn left.
	f.npcs.Respawn("wolves#0#0")
	f.npcs.Respawn("boss_db")
	f.queues.Run()
	if got := f.npcObjects(); len(got) != 0 {
		t.Fatalf("a late respawn placed %d npcs", len(got))
	}
}

// A respawn firing while everything is despawned either lands before the
// despawn, which then deletes it, or finds its spawn gone: no NPC is left
// and no respawn stays armed, whichever runs first.
func TestDespawnAllRacingRespawn(t *testing.T) {
	for range 30 {
		f := newDespawnFixture(t, nil)
		f.hostile(t, 1).DeleteMe()
		f.queues.Run()
		key := "wolves#0#0"
		if !f.respawn.Tracked(key) {
			key = "wolves#0#1"
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			f.npcs.Respawn(key)
		}()
		go func() {
			defer wg.Done()
			f.npcs.DespawnAll()
		}()
		wg.Wait()
		f.queues.Run()

		if got := f.npcObjects(); len(got) != 0 {
			t.Fatalf("npcs left in the world: %d", len(got))
		}
		if f.respawn.Tracked(key) || f.npcs.LiveCount() != 0 {
			t.Fatalf("respawn armed %v, live %d; want neither", f.respawn.Tracked(key), f.npcs.LiveCount())
		}
	}
}

// RespawnAll is the respawn half of SpawnManager.reload: the new spawn
// list's on-start makers spawn as at boot, a database-tracked spawn still
// dead stays dead with its respawn armed, and a respawn armed for a slot of
// the old list never fires on the new one.
func TestRespawnAllSpawnsTheNewListAnew(t *testing.T) {
	f := newDespawnFixture(t, nil)
	f.hostile(t, 1).DeleteMe()
	f.queues.Run()
	f.npcs.DespawnAll()
	f.queues.Run()

	dead := spawn.NewState("boss_db")
	dead.SetRespawn(time.Hour, time.Now())
	fresh := NewSpawns(despawnTable(t, despawnSpawnlist), map[string]*spawn.State{"boss_db": dead})
	f.npcs.RespawnAll(fresh)
	f.queues.Run()

	if f.npcs.Spawns() != fresh {
		t.Fatal("Spawns() is not the new spawn list")
	}
	wolves, bosses := 0, 0
	for _, obj := range f.npcObjects() {
		switch obj.(*npc.Hostile).Instance.Template.ID {
		case 1:
			wolves++
		case 2:
			bosses++
		}
	}
	if wolves != 2 || bosses != 0 {
		t.Fatalf("wolves %d, bosses %d; want 2 wolves and the dead boss waiting", wolves, bosses)
	}
	if !f.respawn.Tracked("boss_db@1") {
		t.Fatal("dead boss's respawn not armed on the new slot")
	}
	// The old list's slot keys name no spawn of the new one.
	f.npcs.Respawn("wolves#0#0")
	f.npcs.Respawn("boss_db")
	f.queues.Run()
	if got := len(f.npcObjects()); got != 2 {
		t.Fatalf("npcs = %d after stale respawns, want the 2 wolves", got)
	}
	if got := f.npcs.LiveCount(); got != 2 {
		t.Fatalf("LiveCount() = %d, want 2", got)
	}
}
