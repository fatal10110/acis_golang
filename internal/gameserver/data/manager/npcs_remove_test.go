package manager

import (
	"path/filepath"
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

// DeleteMe on a spawned NPC goes through Npcs.Remove on the NPC's own
// queue: it cancels a pending corpse decay, takes the NPC and its privates
// out of the world with the respawn hook's cleanup, and arms the slot's
// respawn.
func TestNpcDeleteMeRemovesThroughRespawnHook(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "remove.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<npcmaker name="maker" territory="field" maximumNpcs="1">
		<npc id="1" total="1" pos="10;20;0;123" respawn="60sec"><privates><private id="2" weight="7" respawn="3sec"/></privates></npc>
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
	partyAI := npc.AIParams{"Party_Type": "2"}
	queues := npcQueues()
	npcs, err := NewNpcs(NewSpawns(table, nil), npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "Chest", HPMax: 100, RunSpeed: 100, AIParams: partyAI},
		{ID: 2, TemplateID: 2, Type: "Monster", HPMax: 100, RunSpeed: 100},
	}), fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, effect.Env{Activity: task.NewEffects()}, queues, testMakers())
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	const key = "maker#0#0"
	if got := npcs.LiveCount(); got != 2 {
		t.Fatalf("LiveCount() = %d, want 2", got)
	}
	obj, ok := state.Object(1)
	if !ok {
		t.Fatal("chest missing")
	}
	chest := obj.(*npc.Hostile)
	if _, ok := state.Object(2); !ok {
		t.Fatal("private missing")
	}
	decay.Add(chest, time.Minute)
	if respawn.Tracked(key) {
		t.Fatal("slot respawn armed before removal")
	}

	chest.DeleteMe()
	queues.Run()

	if _, ok := state.Object(1); ok {
		t.Fatal("removed chest still in the world")
	}
	if _, ok := state.Object(2); ok {
		t.Fatal("private still in the world after its master's removal")
	}
	if len(chest.Minions()) != 0 {
		t.Fatalf("removed master still lists %d minions", len(chest.Minions()))
	}
	if !chest.Decayed() {
		t.Fatal("removed chest not decayed")
	}
	if decay.Tracked(chest) {
		t.Fatal("pending decay not cancelled by the removal")
	}
	if got := npcs.LiveCount(); got != 0 {
		t.Fatalf("LiveCount() after removal = %d, want 0", got)
	}
	if !respawn.Tracked(key) {
		t.Fatal("slot respawn not armed by the removal")
	}
}
