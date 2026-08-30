package manager

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestNpcSpawnCreatesPrivateMinion(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "privates.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<npcmaker name="maker" territory="field" maximumNpcs="1">
		<npc id="1" total="1" pos="10;20;0;123"><privates><private id="2" weight="7" respawn="3sec"/></privates></npc>
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
	partyAI := commons.NewStatSet()
	partyAI.Set("Party_Type", 2)
	npcs, err := NewNpcs(NewSpawns(table, nil), npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "Monster", HPMax: 100, RunSpeed: 100, AIParams: partyAI},
		{ID: 2, TemplateID: 2, Type: "Monster", HPMax: 100, RunSpeed: 100},
	}), fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker)
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	if got, want := npcs.LiveCount(), 2; got != want {
		t.Fatalf("LiveCount() = %d, want %d", got, want)
	}
	master, ok := state.Object(1)
	if !ok {
		t.Fatal("master missing")
	}
	child, ok := state.Object(2)
	if !ok {
		t.Fatal("private missing")
	}
	masterHostile := master.(*npc.Hostile)
	if got := child.(*npc.Hostile).Master(); got != masterHostile {
		t.Fatalf("private master = %p, want %p", got, master)
	}
	respawnPrivate := npcs.RespawnHook(2)
	if respawnPrivate == nil {
		t.Fatal("private RespawnHook() = nil, want a respawn scheduler")
	}
	if !child.(*npc.Hostile).Decay(state, respawnPrivate) {
		t.Fatal("private Decay() = false, want true")
	}
	if got := len(masterHostile.Minions()); got != 1 {
		t.Fatalf("master minions after private decay = %d, want 1 until respawn", got)
	}
	npcs.Respawn("maker#0#0/private/0")
	child, ok = state.Object(3)
	if !ok {
		t.Fatal("respawned private missing")
	}
	if got := child.(*npc.Hostile).Master(); got != masterHostile {
		t.Fatalf("respawned private master = %p, want %p", got, master)
	}
	if got := len(masterHostile.Minions()); got != 1 {
		t.Fatalf("master minions after private respawn = %d, want 1", got)
	}
	partyAI.Unset("Party_Type")
	npcs.spawnPrivates("skipped", npcs.slot["maker#0#0"].entry, masterHostile)
	if got := npcs.LiveCount(); got != 2 {
		t.Fatalf("LiveCount() after non-party private spawn = %d, want 2", got)
	}
	if !master.(*npc.Hostile).Decay(state, npcs.RespawnHook(1)) {
		t.Fatal("master Decay() = false, want true")
	}
	if _, ok := state.Object(3); ok {
		t.Fatal("private remains in world after master decay")
	}
	if got := npcs.LiveCount(); got != 0 {
		t.Fatalf("LiveCount() after master decay = %d, want 0", got)
	}
}
