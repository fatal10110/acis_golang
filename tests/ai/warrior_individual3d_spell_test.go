package ai

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootIndividual3dSpells is bootIndividual3d with the character knowing
// two self buffs, neither offensive: aggroBuffID, carrying aggro points,
// and plainBuffID, carrying none.
func bootIndividual3dSpells(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	db := sqltest.SharedDB(t)
	buff := func(id modelskill.ID, aggro int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: aggro,
		}
	}
	defs := []modelskill.Definition{buff(aggroBuffID, 100), buff(plainBuffID, 0)}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	return bootIndividual3d(t, raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			for _, d := range defs {
				if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(d.ID), 1); err != nil {
					t.Fatalf("seed skill %d: %v", d.ID, err)
				}
			}
		},
	}, templates...)
}

// TestPartyLeaderPatrolStonePetrifiesAlliedCaster ports
// PartyLeaderPatrolPhysicalSpecialStoneAggressive's see-spell hook for a
// Ketra leader that sees the character cast a self buff. A buff with
// aggro points cast by a character with no alliance, or a buff without
// aggro points cast by a Ketra ally, goes on to the parent hooks, which
// want no petrify. A buff with aggro points cast by a Ketra ally makes the
// leader want to petrify the caster (4515, weight 1000000) and leaves it
// no hate.
func TestPartyLeaderPatrolStonePetrifiesAlliedCaster(t *testing.T) {
	t.Parallel()
	tmpl := ind3dTemplate(ind3dKetraID, nil, 0, false)
	tmpl.Clans, tmpl.ClanRange = []string{"ketra_orc_clan"}, 500
	w := bootIndividual3dSpells(t, tmpl)
	ch := ind3Character(t, w)
	stone := w.ind3dSpawn(t, ind3dKetraID, 150, 0)

	for _, row := range []struct {
		standing int
		skill    modelskill.ID
	}{{0, aggroBuffID}, {1, plainBuffID}} {
		ch.SetVarkaKetraAlliance(row.standing)
		castOwnSkill(w.warriorWorld, row.skill)
		w.srv.Advance(t, 2*time.Second)
		w.srv.ReadQueued(t, w.srv.Client)
		if d, ok := castDesireOf(stone, petrifyID); ok {
			t.Fatalf("standing %d, buff %d: the leader wants %+v, want no petrify", row.standing, row.skill, d)
		}
	}

	w.clearDesires(t, stone)
	castOwnSkill(w.warriorWorld, aggroBuffID)
	w.srv.AdvanceUntil(t, "the petrify desire", func() bool {
		_, ok := castDesireOf(stone, petrifyID)
		return ok
	})
	if d, _ := castDesireOf(stone, petrifyID); d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("petrify desire = %+v, want on the caster, weight 1000000", d)
	}
	if got := stone.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the allied caster = %v, want none", got)
	}
}

// TestQuestPartyLeaderIgnoresSpells ports QuestPartyLeader's see-spell
// hook. An NPC made to attack the character, which it hates most, sees it
// cast a self buff with aggro points. A PartyLeaderWarrior beside it adds
// hate of the caster through its parent's see-spell hook; the tree's hate
// only decays.
func TestQuestPartyLeaderIgnoresSpells(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		id    int32
		gains bool
	}{{"tree", ind3dQuestID, false}, {"party leader", leaderID, true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3dSpells(t, ind3dTemplate(row.id, nil, 0, false))
			n := w.ind3dSpawn(t, row.id, 150, 0)
			w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.PlayerOf(w.player), 100) })
			w.ticks(t, 1)
			if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack {
				t.Fatalf("setup: intention = %v, want attack", got)
			}
			before := n.AggroHate(w.player)
			castOwnSkill(w.warriorWorld, aggroBuffID)
			w.srv.Advance(t, 2*time.Second)
			w.srv.ReadQueued(t, w.srv.Client)
			if got := n.AggroHate(w.player); (got > before) != row.gains {
				t.Fatalf("hate after the buff = %v from %v, want a gain %v", got, before, row.gains)
			}
		})
	}
}
