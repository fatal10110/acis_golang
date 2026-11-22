package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The cancel family rolls against a rate built from the candidate effect's
// period. These scenarios pin the caster's roll and set the rate a hair above
// or below it, so a cast strips the candidate or keeps it only if the period
// reaching the formula is the effect's own: a constant in its place moves the
// rate across the roll in at least one case of every table.
const (
	cancelCasterLevel = 5
	cancelSkillID     = 1701
	cancelCandidateID = 9701
)

// cancelCase is one candidate effect period, the roll the caster's cancel
// draws against it, and whether the effect is stripped.
type cancelCase struct {
	name     string
	period   int
	roll     int
	stripped bool
}

type cancelWorld struct {
	srv   *gameservertest.Server
	objID int32
}

func bootCancelWorld(t *testing.T, def modelskill.Definition) cancelWorld {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", cancelCasterLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	objID := srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), 1)
	startInWorld(t, srv.Client)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if pc.Level() != cancelCasterLevel {
			t.Errorf("caster level = %d, want %d", pc.Level(), cancelCasterLevel)
		}
	})
	return cancelWorld{srv: srv, objID: objID}
}

// addCandidate lands an effect of the given period and skill on the caster
// and returns it.
func (w cancelWorld) addCandidate(t *testing.T, sk effect.Skill, tmpl modelskill.EffectTemplate) *effect.Effect {
	t.Helper()
	sk.ID, sk.Level = cancelCandidateID, 1
	e, err := effect.New(sk, tmpl)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", tmpl.Name, err)
	}
	onPlayerQueue(t, w.srv, w.objID, func(pc *player.Character) {
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	return e
}

// castPinned has the caster draw roll for every die of one cast of the cancel
// skill, then reports whether e is still on its effect list; a surviving
// effect is removed so the next case starts clean.
func (w cancelWorld) castPinned(t *testing.T, roll int, e *effect.Effect) (stripped bool) {
	t.Helper()
	onPlayerQueue(t, w.srv, w.objID, func(pc *player.Character) {
		pc.SetRollSource(func(int) int { return roll })
	})
	w.srv.Client.Send(encodeRequestMagicSkillUse(cancelSkillID, false, false))
	drainUntilQuiet(t, w.srv.Client)
	onPlayerQueue(t, w.srv, w.objID, func(pc *player.Character) {
		stripped = !slices.Contains(pc.EffectList().All(), e)
		pc.EffectList().Remove(e)
	})
	return stripped
}

func buffTemplate(period int) modelskill.EffectTemplate {
	return modelskill.EffectTemplate{Name: "Buff", Count: 1, Time: period, Icon: true, StackType: "cancel_probe", StackOrder: 1}
}

// TestCancelSkillRateReadsEffectPeriod casts a CANCEL skill (power 30, same
// level as its target) at a caster-held buff: the rate is 30 plus the buff's
// period over 120 seconds, so a 3600 s buff is stripped by any roll under 60
// and a 240 s buff by any roll under 32.
func TestCancelSkillRateReadsEffectPeriod(t *testing.T) {
	t.Parallel()
	w := bootCancelWorld(t, modelskill.Definition{
		ID: cancelSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 0, StaticHitTime: true, SkillType: "CANCEL", Power: 30, MagicLevel: cancelCasterLevel,
	})
	for _, tc := range []cancelCase{
		{name: "long buff, roll just under 60", period: 3600, roll: 59, stripped: true},
		{name: "long buff, roll at 60", period: 3600, roll: 60, stripped: false},
		{name: "short buff, roll just under 32", period: 240, roll: 31, stripped: true},
		{name: "short buff, roll at 32", period: 240, roll: 32, stripped: false},
	} {
		e := w.addCandidate(t, effect.Skill{}, buffTemplate(tc.period))
		if got := w.castPinned(t, tc.roll, e); got != tc.stripped {
			t.Errorf("%s: stripped = %v, want %v", tc.name, got, tc.stripped)
		}
	}
}

// TestCancelEffectRateReadsEffectPeriod is the same table for the Cancel
// effect a skill carries (its power is not marked as rolled, so the effect
// always lands and only the strip rolls): the rate is the magic-level difference (0 here)
// plus the buff's period over 120 seconds plus the effect power, 30.
func TestCancelEffectRateReadsEffectPeriod(t *testing.T) {
	t.Parallel()
	w := bootCancelWorld(t, modelskill.Definition{
		ID: cancelSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 0, StaticHitTime: true, SkillType: "BUFF", MagicLevel: 20,
		Effects: []modelskill.EffectTemplate{{Name: "Cancel", EffectPower: 30}},
	})
	for _, tc := range []cancelCase{
		{name: "long buff, roll just under 60", period: 3600, roll: 59, stripped: true},
		{name: "long buff, roll at 60", period: 3600, roll: 60, stripped: false},
		{name: "short buff, roll just under 32", period: 240, roll: 31, stripped: true},
		{name: "short buff, roll at 32", period: 240, roll: 32, stripped: false},
	} {
		e := w.addCandidate(t, effect.Skill{MagicLevel: 20}, buffTemplate(tc.period))
		if got := w.castPinned(t, tc.roll, e); got != tc.stripped {
			t.Errorf("%s: stripped = %v, want %v", tc.name, got, tc.stripped)
		}
	}
}

// TestCancelDebuffRateReadsRemainingTime casts a CancelDebuff skill (magic
// level 40) at a held 7200 s debuff of magic level 20: the rate is 40 plus
// the debuff's remaining seconds over 1200, 46 while it is fresh and 45 once
// 1200 s of its period have passed.
func TestCancelDebuffRateReadsRemainingTime(t *testing.T) {
	t.Parallel()
	w := bootCancelWorld(t, modelskill.Definition{
		ID: cancelSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 0, StaticHitTime: true, SkillType: "BUFF", MagicLevel: 40,
		Effects: []modelskill.EffectTemplate{{Name: "CancelDebuff"}},
	})
	if !w.srv.DrivesClock() {
		t.Skip("moving an effect's period needs the driven clock")
	}
	debuff := effect.Skill{MagicLevel: 20, Debuff: true, CanBeDispelled: true}
	tmpl := modelskill.EffectTemplate{Name: "Debuff", Count: 1, Time: 7200, Icon: true, StackType: "cancel_probe", StackOrder: 1}

	for _, tc := range []struct {
		name     string
		elapsed  time.Duration
		roll     int
		stripped bool
	}{
		{name: "fresh, roll just under 46", elapsed: 0, roll: 45, stripped: true},
		{name: "fresh, roll at 46", elapsed: 0, roll: 46, stripped: false},
		{name: "1200 s in, roll at 45", elapsed: 1200 * time.Second, roll: 45, stripped: false},
		{name: "1200 s in, roll just under 45", elapsed: 1200 * time.Second, roll: 44, stripped: true},
	} {
		e := w.addCandidate(t, debuff, tmpl)
		w.srv.Advance(t, tc.elapsed)
		if got := w.castPinned(t, tc.roll, e); got != tc.stripped {
			t.Errorf("%s: stripped = %v, want %v", tc.name, got, tc.stripped)
		}
	}
}
