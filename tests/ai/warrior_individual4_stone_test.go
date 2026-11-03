package ai

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPartyPrivateCastDDMagicCurseStonePetrifiesAlliedCaster ports
// PartyPrivateCastDDMagicCurseStone's see-spell hook for a Ketra private
// of no party that sees the character cast a self buff. A buff with aggro
// points cast by a character with no alliance, or a buff without aggro
// points cast by a Ketra ally, goes on to the parent hooks, which want no
// petrify. A buff with aggro points cast by a Ketra ally makes the private
// want to petrify the caster (4515, weight 1000000) and leaves it no hate.
func TestPartyPrivateCastDDMagicCurseStonePetrifiesAlliedCaster(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	buff := func(id modelskill.ID, aggro int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: aggro,
		}
	}
	defs := []modelskill.Definition{buff(aggroBuffID, 100), buff(plainBuffID, 0)}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	tmpl := ind4Template(ind4StoneID, false)
	tmpl.Clans, tmpl.ClanRange = []string{"ketra_orc_clan"}, 500
	w := bootIndividual4(t, raidBoot{
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
	}, tmpl)
	ch := ind3Character(t, w)
	stone := w.spawn(t, ind4StoneID, 150, 0)
	w.srv.InstallNPCCastSeam(t, stone, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true)}))

	for _, row := range []struct {
		standing int
		skill    modelskill.ID
	}{{0, aggroBuffID}, {1, plainBuffID}} {
		ch.SetVarkaKetraAlliance(row.standing)
		castOwnSkill(w.warriorWorld, row.skill)
		w.srv.Advance(t, 2*time.Second)
		w.srv.ReadQueued(t, w.srv.Client)
		if d, ok := castDesireOf(stone, petrifyID); ok {
			t.Fatalf("standing %d, buff %d: the private wants %+v, want no petrify", row.standing, row.skill, d)
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
