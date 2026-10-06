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

// TestNpcSpawnInstallsAIConfig pins the npcs.properties target-selection
// switches reaching every spawned hostile: with GuardAttackAggroMob on, a
// spawned Guard picks a spawned aggressive monster; with the shipped
// default it does not.
func TestNpcSpawnInstallsAIConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  npc.AIConfig
		want bool
	}{
		{name: "shipped default", cfg: npc.DefaultAIConfig(), want: false},
		{name: "GuardAttackAggroMob on", cfg: npc.AIConfig{MobAggroInPeaceZone: true, GuardAttackAggroMob: true}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSpawnFixture(t, filepath.Join(dir, "guard.xml"), `
<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>
	<npcmaker name="guard" territory="field" maximumNpcs="1"><npc id="1" total="1" pos="10;10;0;0"/></npcmaker>
	<npcmaker name="mob" territory="field" maximumNpcs="1"><npc id="2" total="1" pos="50;50;0;0"/></npcmaker>
</list>`)
			table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("LoadSpawnlist() error: %v", err)
			}
			state := world.New()
			decay, _ := task.NewDecay(nopDecayEffects{}, time.Now)
			respawn, _ := task.NewRespawn(nopRespawnEffects{}, time.Now)
			walker, _ := task.NewWalker(nil, noRouteWalkerPath{}, time.Now, state)
			tmpl := func(id int, kind string, aggroRange int) *npc.Template {
				return &npc.Template{ID: id, TemplateID: id, Type: kind, Level: 11, HPMax: 100, RunSpeed: 100, AggroRange: aggroRange}
			}
			_, err = NewNpcsWithMaxBuffsAmount(NewSpawns(table, nil), npc.NewTable([]*npc.Template{tmpl(1, "Guard", 300), tmpl(2, "Monster", 300)}),
				fakeGeo{}, state, &sequentialIDs{}, decay, respawn, task.NewAI(state, zerolog.Nop()), task.NewPositionUpdates(state), item.NewTable(nil),
				&recordingGround{}, KillRewardConfig{}, time.Now, zerolog.Nop(), nil, actorcast.EffectHandlers{}, walker, nil, nil, 20, 30, 0,
				npc.DefaultRaidMultipliers(), tc.cfg, DefaultSpawnEvents(), effect.Env{Activity: task.NewEffects()}, npcQueues(), testMakers(), nil)
			if err != nil {
				t.Fatalf("NewNpcsWithMaxBuffsAmount() error: %v", err)
			}
			var guard, mob *npc.Hostile
			for _, obj := range state.Objects() {
				if h, ok := obj.(*npc.Hostile); ok {
					switch h.Instance.Template.ID {
					case 1:
						guard = h
					case 2:
						mob = h
					}
				}
			}
			if guard == nil || mob == nil {
				t.Fatal("guard or monster not spawned")
			}
			if got := guard.AutoAttackTargetValid(mob, 300, false); got != tc.want {
				t.Fatalf("guard targets the aggressive monster = %v, want %v", got, tc.want)
			}
		})
	}
}
