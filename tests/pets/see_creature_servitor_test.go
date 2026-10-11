package pets

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestServitorSummonTellsNearbyMonsters pins the see-creature re-fire of a
// servitor skill: the servitor's arrival tells each bound monster with it in
// sight range, and a monster beyond that range hears nothing. The monsters
// are off the AI task, so no tick scan runs.
func TestServitorSummonTellsNearbyMonsters(t *testing.T) {
	t.Parallel()
	calls := &seeCalls{names: map[int32]string{}}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCatSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", NpcID: catNPCID, SummonTotalLifeTime: 1_200_000,
			StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{catTemplate()})),
		gameservertest.WithSkills(skills),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{seeWatcherID: script.KindHostile},
			[]script.Listing{{Path: "ai.SeeWatcher"}},
			script.Catalog{"ai.SeeWatcher": func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{seeWatcherID}, Hooks: script.Hooks{OnSeeCreature: calls.see}}
			}}),
	)
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, summonCatSkillID, 1); err != nil {
		t.Fatalf("seed known skill %d: %v", summonCatSkillID, err)
	}
	startInWorld(t, srv.Client)

	x, y, z := srv.PlayerPosition(t, ownerID)
	watcher := func(name string, at location.Location) {
		t.Helper()
		tmpl := gameservertest.MovingHostileTemplate("Monster")
		tmpl.ID, tmpl.TemplateID = int(seeWatcherID), int(seeWatcherID)
		m := srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
		calls.mu.Lock()
		calls.names[m.ObjectID()] = name
		calls.mu.Unlock()
	}
	watcher("near", location.Location{X: x + 200, Y: y, Z: z})
	watcher("far", location.Location{X: x + 2000, Y: y, Z: z})
	drainUntilQuiet(t, srv.Client)
	calls.want(t, "before the servitor is summoned")

	srv.Client.Send(encodeRequestMagicSkillUse(int32(summonCatSkillID)))
	srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := srv.State.Summon(ownerID)
		if !ok {
			return false
		}
		_, ok = obj.(*summon.Actor)
		return ok
	})
	srv.Settle(t)
	drainUntilQuiet(t, srv.Client)
	calls.want(t, "when the servitor is summoned", "near summon")
}
