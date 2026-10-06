package manager

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// groundGeo puts the ground 7 units below any point.
type groundGeo struct{ fakeGeo }

func (groundGeo) Height(_, _, z int) int16 { return int16(z - 7) }

// fixedSpawnFixture boots a population with one maker spawn of npc 1 and
// templates for a monster (1), a merchant (2) and a castle artifact (3).
func fixedSpawnFixture(t *testing.T) (*Npcs, *world.State, *sim.Inline) {
	t.Helper()
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "maker.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<npcmaker name="field_maker" territory="field" maximumNpcs="1">
		<npc id="1" total="1" pos="10;20;0;123" respawn="60sec"/>
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
	queues := npcQueues()
	npcs, err := NewNpcs(NewSpawns(table, nil), npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "Monster", Name: "Wolf", HPMax: 100, RunSpeed: 100},
		{ID: 2, TemplateID: 2, Type: "Merchant", Name: "Lector", HPMax: 100},
		{ID: 3, TemplateID: 3, Type: "HolyThing", Name: "Artifact", HPMax: 100},
	}), groundGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, effect.Env{Activity: task.NewEffects()}, queues, testMakers())
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	return npcs, state, queues
}

// TestSpawnFixedPlacesStandaloneSpawns pins AdminSpawn's //spawn through
// Spawn.setLoc and doSpawn: the NPC stands at the point's x/y on the ground
// height below it, facing the given heading, and wanders around that point;
// a civilian NPC is placed the same way. The spawn reports itself
// standalone with that point, unlike a maker's. A template no live NPC
// models places nothing.
func TestSpawnFixedPlacesStandaloneSpawns(t *testing.T) {
	npcs, state, _ := fixedSpawnFixture(t)
	templates := npcs.templates
	tmpl := func(id int) *npc.Template {
		t.Helper()
		got, ok := templates.Get(id)
		if !ok {
			t.Fatalf("template %d missing", id)
		}
		return got
	}

	if err := npcs.SpawnFixed(tmpl(1), 500, 600, 70, 1234); err != nil {
		t.Fatalf("SpawnFixed(monster) error: %v", err)
	}
	obj, ok := state.Object(2)
	if !ok {
		t.Fatal("fixed monster missing from the world")
	}
	wolf := obj.(*npc.Hostile)
	if x, y, z := wolf.Position(); x != 500 || y != 600 || z != 63 || wolf.Heading() != 1234 {
		t.Fatalf("fixed monster at %d,%d,%d heading %d; want 500,600,63 heading 1234", x, y, z, wolf.Heading())
	}
	if !wolf.Instance.HasHome || wolf.Instance.Home != (location.Location{X: 500, Y: 600, Z: 63}) || wolf.Instance.Maker != nil {
		t.Fatalf("fixed monster home = %v (%v), maker %v; want its spawn point and no maker", wolf.Instance.Home, wolf.Instance.HasHome, wolf.Instance.Maker)
	}
	rec, ok := npcs.SpawnOf(2)
	if want := (SpawnRecord{Fixed: true, At: location.Location{X: 500, Y: 600, Z: 63}, Heading: 1234}); !ok || rec != want {
		t.Fatalf("SpawnOf(fixed) = %+v, %v; want %+v", rec, ok, want)
	}
	if rec, ok := npcs.SpawnOf(1); !ok || rec != (SpawnRecord{Maker: "field_maker"}) {
		t.Fatalf("SpawnOf(maker spawn) = %+v, %v; want the maker", rec, ok)
	}

	if err := npcs.SpawnFixed(tmpl(2), -10, -20, 0, 5); err != nil {
		t.Fatalf("SpawnFixed(merchant) error: %v", err)
	}
	if obj, ok := state.Object(3); !ok {
		t.Fatal("fixed merchant missing from the world")
	} else if _, isFolk := obj.(*npc.Folk); !isFolk {
		t.Fatalf("fixed merchant is %T, want a civilian NPC", obj)
	}

	if err := npcs.SpawnFixed(tmpl(3), 0, 0, 0, 0); !errors.Is(err, ErrNotPlaceable) {
		t.Fatalf("SpawnFixed(artifact) error = %v, want ErrNotPlaceable", err)
	}
	if got := npcs.LiveCount(); got != 3 {
		t.Fatalf("LiveCount() = %d, want the maker spawn and two fixed spawns", got)
	}
	if got := npcs.SkippedNonCombatCount(); got != 0 {
		t.Fatalf("SkippedNonCombatCount() = %d, want the refused spawn left out of the boot count", got)
	}
}

// TestDeleteFixedRemovesWithoutRespawn pins //delete through Spawn.doDelete
// and SpawnManager.deleteSpawn: only a standalone spawn's NPC is removed,
// at once and for good, hostile or civilian; a maker's NPC is refused and
// stays. A standalone spawn whose NPC dies never respawns either
// (Spawn.onDecay schedules nothing).
func TestDeleteFixedRemovesWithoutRespawn(t *testing.T) {
	npcs, state, queues := fixedSpawnFixture(t)
	wolfTmpl, _ := npcs.templates.Get(1)
	merchantTmpl, _ := npcs.templates.Get(2)
	for _, spawn := range []*npc.Template{wolfTmpl, merchantTmpl, wolfTmpl} {
		if err := npcs.SpawnFixed(spawn, 100, 100, 0, 0); err != nil {
			t.Fatalf("SpawnFixed(%d) error: %v", spawn.ID, err)
		}
	}

	if npcs.DeleteFixed(1) {
		t.Fatal("DeleteFixed(maker spawn) = true, want refused")
	}
	queues.Run()
	if _, ok := state.Object(1); !ok {
		t.Fatal("the refused maker spawn left the world")
	}

	for _, id := range []int32{2, 3} {
		if !npcs.DeleteFixed(id) {
			t.Fatalf("DeleteFixed(%d) = false, want removed", id)
		}
	}
	queues.Run()
	for _, id := range []int32{2, 3} {
		if _, ok := state.Object(id); ok {
			t.Fatalf("deleted npc %d still in the world", id)
		}
		if _, ok := npcs.SpawnOf(id); ok {
			t.Fatalf("deleted npc %d still has a spawn", id)
		}
	}
	if npcs.DeleteFixed(2) {
		t.Fatal("second DeleteFixed(2) = true, want nothing left to delete")
	}

	obj, _ := state.Object(4)
	if !obj.(*npc.Hostile).Decay(state, npcs.RespawnHook(4)) {
		t.Fatal("fixed monster Decay() = false, want true")
	}
	if got := npcs.LiveCount(); got != 1 {
		t.Fatalf("LiveCount() = %d, want only the maker spawn", got)
	}
	npcs.mu.Lock()
	slots := len(npcs.slot)
	npcs.mu.Unlock()
	if slots != 1 {
		t.Fatalf("spawn slots = %d, want only the maker's left", slots)
	}
	for _, key := range []string{"fixed#1", "fixed#2", "fixed#3"} {
		if npcs.respawn.Tracked(key) {
			t.Fatalf("respawn armed for %s, want none", key)
		}
	}
}
