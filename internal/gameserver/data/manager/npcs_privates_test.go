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

// TestNpcSpawnMarksRaidBossPrivatesRaidRelated: a Monster-family private a
// raid boss master's behavior creates is raid related (so lethal strikes
// and other raid gates skip it), on first spawn and on respawn; a
// non-Monster private of the same master is not.
func TestNpcSpawnMarksRaidBossPrivatesRaidRelated(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "privates.xml"), `
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
	npcs, err := NewNpcs(NewSpawns(table, nil), npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "RaidBoss", HPMax: 100, RunSpeed: 100},
		{ID: 2, TemplateID: 2, Type: "Monster", HPMax: 100, RunSpeed: 100},
		{ID: 3, TemplateID: 3, Type: "Guard", HPMax: 100, RunSpeed: 100},
	}), fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, effect.Env{Activity: task.NewEffects()}, npcQueues(), testMakers())
	if err != nil {
		t.Fatalf("NewNpcs() error: %v", err)
	}
	hostile := func(id int32) *npc.Hostile {
		t.Helper()
		obj, ok := state.Object(id)
		if !ok {
			t.Fatalf("object %d missing", id)
		}
		return obj.(*npc.Hostile)
	}
	for _, id := range []int{2, 3} {
		tmpl, _ := npcs.templates.Get(id)
		if _, err := npcs.createPrivate(hostile(1), tmpl, nil, 0, 0, [3]int32{}, 3*time.Second); err != nil {
			t.Fatalf("createPrivate(%d) error: %v", id, err)
		}
	}
	for id, want := range map[int32]bool{1: true, 2: true, 3: false} {
		if got := hostile(id).RaidRelated(); got != want {
			t.Fatalf("object %d (template %d) RaidRelated() = %v, want %v", id, hostile(id).Instance.Template.ID, got, want)
		}
	}
	npcs.mu.Lock()
	key := npcs.live[2]
	npcs.mu.Unlock()
	if !hostile(2).Decay(state, npcs.RespawnHook(2)) {
		t.Fatal("private Decay() = false, want true")
	}
	npcs.Respawn(key)
	if !hostile(4).RaidRelated() {
		t.Fatal("respawned raid private RaidRelated() = false, want true")
	}
}
