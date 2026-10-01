package manager

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// spawnDecay is the production corpse-decay effect over npcs: the decayed
// actor leaves the world and its spawn slot arms its respawn.
type spawnDecay struct {
	state *world.State

	mu   sync.Mutex
	npcs *Npcs
}

func (d *spawnDecay) Decay(actor task.DecayActor) {
	d.mu.Lock()
	npcs := d.npcs
	d.mu.Unlock()
	obj, ok := d.state.Object(actor.ObjectID())
	if !ok {
		return
	}
	if corpse, ok := obj.(interface {
		Decay(*world.State, func()) bool
	}); ok {
		corpse.Decay(d.state, npcs.RespawnHook(actor.ObjectID()))
	}
}

// dueSlots records the spawn slots whose respawn deadline passed.
type dueSlots struct {
	mu   sync.Mutex
	keys []string
}

func (d *dueSlots) Respawn(key string) {
	d.mu.Lock()
	d.keys = append(d.keys, key)
	d.mu.Unlock()
}

func (d *dueSlots) take() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	keys := d.keys
	d.keys = nil
	return keys
}

// TestMortalFolkDecaysAndRespawnsThroughItsSlot follows Npc.doDie and
// Npc.onDecay for a civilian NPC spawned from the spawn table: a mortal one
// that dies registers its corpse for the template corpse time, the decay
// takes it out of the world and arms its slot's respawn, and the respawn
// places a new NPC under the same slot. An undying one in the same maker
// keeps 1 HP.
func TestMortalFolkDecaysAndRespawnsThroughItsSlot(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "folk.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<npcmaker name="maker" territory="field" maximumNpcs="2">
		<npc id="1" total="1" pos="10;20;0;123" respawn="60sec"/>
		<npc id="2" total="1" pos="40;20;0;0" respawn="60sec"/>
	</npcmaker>
</list>`)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	state := world.New()
	start := time.Unix(1_700_000_000, 0)
	var clockMu sync.Mutex
	now := start
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	effects := &spawnDecay{state: state}
	decay, _ := task.NewDecay(effects, clock)
	due := &dueSlots{}
	respawn, _ := task.NewRespawn(due, clock)
	walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, clock, state)
	queues := npcQueues()
	npcs, err := NewNpcs(NewSpawns(table, nil), npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "Folk", Name: "Huge Cursed Pig", Level: 1, HPMax: 100, CorpseTime: 7},
		{ID: 2, TemplateID: 2, Type: "Merchant", Name: "Lector", Level: 1, HPMax: 100, CorpseTime: 7, Undying: true},
	}), fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, clock, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, effect.Env{Activity: task.NewEffects()}, queues)
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	effects.mu.Lock()
	effects.npcs = npcs
	effects.mu.Unlock()
	if got := npcs.LiveCount(); got != 2 {
		t.Fatalf("LiveCount() = %d, want both civilian NPCs tracked", got)
	}
	folkAt := func(id int32) *npc.Folk {
		t.Helper()
		obj, ok := state.Object(id)
		if !ok {
			t.Fatalf("npc %d missing from the world", id)
		}
		f, ok := obj.(*npc.Folk)
		if !ok {
			t.Fatalf("npc %d is %T, want a civilian NPC", id, obj)
		}
		return f
	}
	pig, merchant := folkAt(1), folkAt(2)

	if merchant.TakeDamage(1_000_000, nil) || merchant.Dead() || merchant.HP() != 1 {
		t.Fatalf("undying merchant after a lethal hit: dead %v, HP %v; want alive at 1 HP", merchant.Dead(), merchant.HP())
	}
	if decay.Tracked(merchant) {
		t.Fatal("the undying merchant's corpse was registered for decay")
	}

	if !pig.TakeDamage(1_000_000, nil) || !pig.Dead() || pig.HP() != 0 {
		t.Fatalf("mortal pig after a lethal hit: dead %v, HP %v; want dead at 0 HP", pig.Dead(), pig.HP())
	}
	deadline, ok := decay.Deadline(pig)
	if want := start.Add(7 * time.Second); !ok || !deadline.Equal(want) {
		t.Fatalf("pig corpse decay deadline = %v, %v; want %v", deadline, ok, want)
	}
	if got, ok := pig.CorpseDeadline(); !ok || !got.Equal(deadline) || !pig.HasCorpse() {
		t.Fatalf("pig corpse deadline = %v, %v (corpse %v); want the decay task's %v", got, ok, pig.HasCorpse(), deadline)
	}

	clockMu.Lock()
	now = start.Add(6 * time.Second)
	clockMu.Unlock()
	if err := decay.Tick(); err != nil {
		t.Fatal(err)
	}
	queues.Run()
	if _, ok := state.Object(1); !ok {
		t.Fatal("pig corpse decayed before its corpse time")
	}

	clockMu.Lock()
	now = start.Add(7 * time.Second)
	clockMu.Unlock()
	if err := decay.Tick(); err != nil {
		t.Fatal(err)
	}
	queues.Run()
	if _, ok := state.Object(1); ok {
		t.Fatal("pig corpse still in the world after its corpse time")
	}
	if pig.HasCorpse() {
		t.Fatal("decayed pig still reports a corpse")
	}
	if got := npcs.LiveCount(); got != 1 {
		t.Fatalf("LiveCount() after the decay = %d, want 1", got)
	}
	const key = "maker#0#0"
	setNow := func(at time.Time) {
		clockMu.Lock()
		now = at
		clockMu.Unlock()
	}
	setNow(start.Add(7*time.Second + time.Minute - time.Second))
	respawn.Tick()
	if keys := due.take(); len(keys) != 0 || !respawn.Tracked(key) {
		t.Fatalf("pig slot respawned at %v before its 60s delay: %v", keys, respawn.Tracked(key))
	}
	setNow(start.Add(7*time.Second + time.Minute))
	respawn.Tick()
	if keys := due.take(); len(keys) != 1 || keys[0] != key {
		t.Fatalf("respawned slots after the delay = %v, want [%s]", keys, key)
	}

	npcs.Respawn(key)
	queues.Run()
	if got := npcs.LiveCount(); got != 2 {
		t.Fatalf("LiveCount() after the respawn = %d, want 2", got)
	}
	var reborn *npc.Folk
	for _, o := range state.Objects() {
		if f, ok := o.(*npc.Folk); ok && f.NpcID() == 1 {
			reborn = f
		}
	}
	if reborn == nil || reborn == pig || reborn.Dead() || reborn.CurrentHP() != reborn.MaxHP() {
		t.Fatalf("respawned pig = %v, want a new live pig at full HP", reborn)
	}
	if x, y, _ := reborn.Position(); x != 10 || y != 20 {
		t.Fatalf("respawned pig at (%d, %d), want its spawn point (10, 20)", x, y)
	}
}
