package manager

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// A nil sink factory silently disables every client broadcast for the actors
// a manager spawns. Broadcasts report no error to detect it by, so the wiring
// gap has to be loud at construction instead.

func TestNewNpcsWithoutSinkFactoryWarns(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "empty.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<npcmaker name="maker" territory="field" maximumNpcs="1">
		<npc id="1" total="1" pos="10;20;0;123"/>
	</npcmaker>
</list>`)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	state := world.New()
	decay, _ := task.NewDecay(nopDecayEffects{}, time.Now)
	respawn, _ := task.NewRespawn(nopRespawnEffects{}, time.Now)
	walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, time.Now, state)

	var logs bytes.Buffer
	if _, err := newStartedNpcs(NpcsConfig{
		Spawns:    NewSpawns(table, nil),
		Templates: npc.NewTable([]*npc.Template{{ID: 1, TemplateID: 1, Type: "Monster", HPMax: 100, RunSpeed: 100}}),
		Geo:       fakeGeo{},
		World:     state,
		IDs:       &sequentialIDs{},
		Decay:     decay,
		Respawn:   respawn,
		AI:        task.NewAI(state, zerolog.Nop()),
		Positions: task.NewPositionUpdates(state),
		Items:     item.NewTable(nil),
		Ground:    &recordingGround{},
		Now:       time.Now,
		Log:       zerolog.New(&logs),
		Walker:    walker,
		Effects:   effect.Env{Activity: task.NewEffects()},
		Queues:    npcQueues(),
		Makers:    testMakers(),
	}); err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	if !strings.Contains(logs.String(), "no NPC event sink factory") {
		t.Fatalf("log = %q, want a nil-sink-factory warning", logs.String())
	}
}

func TestNewWorldObjectsRequiresSinkFactory(t *testing.T) {
	doors, err := door.NewTable(nil)
	if err != nil {
		t.Fatalf("door.NewTable(): %v", err)
	}
	statics, err := staticobject.NewTable(nil)
	if err != nil {
		t.Fatalf("staticobject.NewTable(): %v", err)
	}
	doorTimers, err := task.NewDoor(doorTimerRecorder{}, time.Now)
	if err != nil {
		t.Fatalf("NewDoor: %v", err)
	}
	geo, _, _ := newDoorGeo(t)

	if _, err := NewWorldObjects(doors, statics, &worldObjectIDs{next: 1000}, geo, world.New(), doorTimers, task.NewDoorRegen(nil), nil, zerolog.Nop()); err == nil {
		t.Fatal("NewWorldObjects() with no door sink factory error = nil, want a wiring error")
	}
}

// discardDoorSinks gives every door a sink that drops its events, for tests
// that do not watch door broadcasts.
func discardDoorSinks(*door.Object) event.Sink { return discardSink{} }

type discardSink struct{}

func (discardSink) Emit(event.Event) {}

// npcQueues runs NPC work on a virtual clock no test advances: timers armed
// on its queues never fire.
func npcQueues() *sim.Inline { return sim.NewInline(time.Unix(0, 0)) }

// Without an effect activity registry no NPC effect would ever expire.
func TestNewNpcsRejectsMissingEffectRegistry(t *testing.T) {
	state := world.New()
	decay, _ := task.NewDecay(nopDecayEffects{}, time.Now)
	respawn, _ := task.NewRespawn(nopRespawnEffects{}, time.Now)
	walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, time.Now, state)

	_, err := newStartedNpcs(NpcsConfig{
		Spawns:    NewSpawns(&spawn.Table{}, nil),
		Templates: npc.NewTable(nil),
		Geo:       fakeGeo{},
		World:     state,
		IDs:       &sequentialIDs{},
		Decay:     decay,
		Respawn:   respawn,
		AI:        task.NewAI(state, zerolog.Nop()),
		Positions: task.NewPositionUpdates(state),
		Items:     item.NewTable(nil),
		Ground:    &recordingGround{},
		Now:       time.Now,
		Log:       zerolog.Nop(),
		Walker:    walker,
		Effects:   effect.Env{},
		Queues:    npcQueues(),
		Makers:    testMakers(),
	})
	if err == nil || !strings.Contains(err.Error(), "effect activity registry") {
		t.Fatalf("NewNpcs() error = %v, want missing effect activity registry", err)
	}
}
