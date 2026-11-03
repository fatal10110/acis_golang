package ai

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ind3Stone is a fixture PartyLeaderCastDDMagicStoneAggressive of id in
// the faction clan, listing its DD magic by type.
func ind3Stone(id int32, faction string) *npc.Template {
	tmpl := ind3Template(id, nil, false)
	tmpl.Clans, tmpl.ClanRange = []string{faction}, 500
	return tmpl
}

// ind3Character returns the scenario's online character.
func ind3Character(t *testing.T, w *ind1World) *player.Character {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character is %T, not an online character", obj)
	}
	return ch
}

// TestPartyLeaderCastDDMagicStonePetrifiesAllies ports
// PartyLeaderCastDDMagicStoneAggressive's see-creature, attacked and
// clan-attacked hooks for a Ketra and a Varka leader. A character allied
// with the leader's faction is not attacked on sight; its hit makes the
// leader want to petrify it (4515, weight 1000000 from its attacked hook
// and as much again from its own clan call, which the hit makes first to
// itself), stop hating it and cast no DD magic. A character with no
// alliance is attacked on sight with weight 200.
func TestPartyLeaderCastDDMagicStonePetrifiesAllies(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		id       int32
		faction  string
		standing int
	}{{"ketra", ind3KetraStoneID, "ketra_orc_clan", 1}, {"varka", ind3VarkaStoneID, "varka_silenos_clan", -1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3(t, ind3Stone(row.id, row.faction))
			ch := ind3Character(t, w)
			ch.SetVarkaKetraAlliance(row.standing)

			allied := w.spawn(t, row.id, 150, 0)
			w.srv.InstallNPCCastSeam(t, allied, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true), ind1Skill(ind3DDMagicSkill, 2000, false)}))
			w.ticks(t, 2)
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of an ally it sees = %v, want none", got)
			}
			w.hit(t, allied, 10)
			if d, ok := castDesireOf(allied, petrifyID); !ok || d.Weight != 2000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("petrify desire = %+v (%v), want on the character, weight 2000000", d, ok)
			}
			if d, ok := castDesireOf(allied, ind3DDMagicSkill); ok {
				t.Fatalf("DD magic desire after an ally's hit = %+v, want none", d)
			}
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of the ally that hit it = %v, want none", got)
			}

			ch.SetVarkaKetraAlliance(0)
			open := w.spawn(t, row.id, 0, 150)
			w.ticks(t, 2)
			if got := open.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of a character with no alliance it sees = %v, want 200", got)
			}
		})
	}
}

// TestPartyLeaderCastDDMagicStonePetrifiesAlliedCaster ports
// PartyLeaderCastDDMagicStoneAggressive's see-spell hook for a Ketra leader
// that sees the character cast a self buff. A buff with aggro points cast
// by a character with no alliance, or a buff without aggro points cast by
// a Ketra ally, goes on to the parent hooks, which want no petrify. A buff
// with aggro points cast by a Ketra ally makes the leader want to petrify
// the caster (4515, weight 1000000) and leaves it no hate.
func TestPartyLeaderCastDDMagicStonePetrifiesAlliedCaster(t *testing.T) {
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
	w := bootIndividual3With(t, raidBoot{
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
	}, ind3Stone(ind3KetraStoneID, "ketra_orc_clan"))
	ch := ind3Character(t, w)
	stone := w.spawn(t, ind3KetraStoneID, 150, 0)
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
			t.Fatalf("standing %d, buff %d: the leader wants %+v, want no petrify", row.standing, row.skill, d)
		}
	}

	w.onQueue(t, stone, func() { stone.AI().Desires().Clear() })
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
