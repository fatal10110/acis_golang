package quest

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	q501Kalis    = 30759
	q501Poison   = modelskill.ID(4082)
	q501Antidote = 3872
	q501HerbOel  = 3834
	q501Voucher  = 3873
	q501Potion   = 3889
)

// q501PoisonSkill is the poison Kalis asks the leader to be under: a buff that
// holds for the whole test.
func q501PoisonSkill() gameservertest.Option {
	return gameservertest.WithScriptSkills(modelskill.Definition{
		ID: q501Poison, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", CastRange: 900, HitTime: 500,
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 3000, Icon: true, StackType: "q501_poison", StackOrder: 1}},
	})
}

// poison lands the poison on the leader the way a Box of Athrea's cast does
// and waits for it to stand.
func (w *q501World) poison(t *testing.T) {
	t.Helper()
	if !w.srv.RunScript(t, w.player, q501Name, func(s *script.Script, p *script.Player) {
		s.ApplyEffects(modelskill.Ref{ID: q501Poison, Level: 1}, p, p)
	}) {
		t.Fatal("the poison hook panicked")
	}
	w.srv.AdvanceUntil(t, "the poison landed on the leader", func() bool {
		var held bool
		w.srv.RunScript(t, w.player, q501Name, func(_ *script.Script, p *script.Player) { held = p.HasEffect(int32(q501Poison)) })
		return held
	})
	w.srv.ReadQueued(t, w.srv.Client)
}

// TestQ501KalisJudgesTheLeaderUnderThePoison: the leader at cond 3 under the
// poison who holds the four herbs and has spent every flag gets the antidote
// paid for the voucher (cond 4); one flag still set resets the quest to cond 1
// with nothing kept; a missing herb only turns the leader away. Without the
// poison the reset needs no herb check (the scenarios).
func TestQ501KalisJudgesTheLeaderUnderThePoison(t *testing.T) {
	t.Parallel()
	herbs := map[int32]int32{q501Antidote: 1, q501HerbOfHarit: 1, q501HerbOfVanor: 1, q501HerbOel: 1, q501BloodOfEva: 1}
	t.Run("antidote", func(t *testing.T) {
		t.Parallel()
		w := q501Boot(t, true, 3, nil, herbs, q501PoisonSkill())
		w.spawnFolk(t, "kalis", q501Kalis, 40, 0, 0)
		w.poison(t)
		w.shows(t, "talk", w.talk(t, "kalis"), "kalis", "30759-08.htm")
		for _, id := range []int32{q501Antidote, q501HerbOfHarit, q501HerbOfVanor, q501HerbOel, q501BloodOfEva} {
			w.held(t, id, 0)
		}
		w.held(t, q501Voucher, 1)
		w.held(t, q501Potion, 1)
		if vars := w.vars(t); vars["<cond>"] != "4" || vars["<state>"] != "STARTED" {
			t.Fatalf("leader's rows after the antidote: %v", vars)
		}
		w.shows(t, "talk again", w.talk(t, "kalis"), "kalis", "30759-11.htm")
	})
	t.Run("a flag still set", func(t *testing.T) {
		t.Parallel()
		w := q501Boot(t, true, 3, map[string]string{"3833": "true"}, herbs, q501PoisonSkill())
		w.spawnFolk(t, "kalis", q501Kalis, 40, 0, 0)
		w.poison(t)
		w.shows(t, "talk", w.talk(t, "kalis"), "kalis", "30759-09.htm")
		w.held(t, q501Voucher, 0)
		w.held(t, q501Potion, 0)
		vars := w.vars(t)
		if len(vars) != 2 || vars["<cond>"] != "1" || vars["<state>"] != "STARTED" {
			t.Fatalf("leader's rows after the reset: %v", vars)
		}
	})
	t.Run("a herb missing", func(t *testing.T) {
		t.Parallel()
		short := map[int32]int32{q501Antidote: 1, q501HerbOfHarit: 1, q501HerbOfVanor: 1, q501BloodOfEva: 1}
		w := q501Boot(t, true, 3, nil, short, q501PoisonSkill())
		w.spawnFolk(t, "kalis", q501Kalis, 40, 0, 0)
		w.poison(t)
		w.shows(t, "talk", w.talk(t, "kalis"), "kalis", "30759-10.htm")
		w.held(t, q501Voucher, 0)
		w.held(t, q501HerbOfHarit, 1)
		if vars := w.vars(t); vars["<cond>"] != "3" {
			t.Fatalf("leader's rows after the refusal: %v", vars)
		}
	})
}
