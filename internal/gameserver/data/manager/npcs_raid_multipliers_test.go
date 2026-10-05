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

// TestNpcSpawnAppliesRaidMultipliersToRaidRelatedHostiles pins the
// npcs.properties raid multipliers reaching every raid-related spawn: a raid
// boss and its Monster-family private finalize P.Def from the doubled base,
// while a non-Monster private of the same boss and an unrelated monster keep
// the raw base. Level 11 makes the level mod exactly 1.
func TestNpcSpawnAppliesRaidMultipliersToRaidRelatedHostiles(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "raid.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<npcmaker name="boss" territory="field" maximumNpcs="1">
		<npc id="1" total="1" pos="10;20;0;123"><privates><private id="2" weight="7" respawn="3sec"/><private id="3" weight="7" respawn="3sec"/></privates></npc>
	</npcmaker>
	<npcmaker name="field" territory="field" maximumNpcs="1">
		<npc id="4" total="1" pos="50;50;0;0"/>
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
	tmpl := func(id int, kind string) *npc.Template {
		return &npc.Template{ID: id, TemplateID: id, Type: kind, Level: 11, PDef: 51, HPMax: 100, RunSpeed: 100}
	}
	boss := tmpl(1, "RaidBoss")
	boss.AIParams = partyAI
	_, err = NewNpcsWithMaxBuffsAmount(NewSpawns(table, nil), npc.NewTable([]*npc.Template{boss, tmpl(2, "Monster"), tmpl(3, "Guard"), tmpl(4, "Monster")}),
		fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
		&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, nil, 20, 30, 0,
		npc.RaidMultipliers{Defence: 2, HPRegen: 1, MPRegen: 1}, npc.DefaultAIConfig(), DefaultSpawnEvents(), effect.Env{Activity: task.NewEffects()}, npcQueues())
	if err != nil {
		t.Fatalf("NewNpcsWithMaxBuffsAmount() error: %v", err)
	}
	pDef := map[bool]float64{true: 102, false: 51}
	seen, raid := 0, 0
	for _, obj := range state.Objects() {
		h, ok := obj.(*npc.Hostile)
		if !ok {
			continue
		}
		seen++
		if h.RaidRelated() {
			raid++
		}
		if got, want := h.PDef(), pDef[h.RaidRelated()]; got != want {
			t.Errorf("object %d (template %d, raid related %v) PDef() = %v, want %v", h.ObjectID(), h.Instance.Template.ID, h.RaidRelated(), got, want)
		}
	}
	if seen != 4 || raid != 2 {
		t.Fatalf("spawned hostiles = %d (%d raid related), want 4 (2): boss, two privates and a field monster", seen, raid)
	}
}
