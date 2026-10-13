package effect

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// funcCondition builds the Condition gate for one stat func from its own
// direct predicate (a func element's child, e.g. <add ...><using
// .../></add>) and/or the <cond> block attached to its enclosing <for>/
// <effect> group, requiring both when both are present. The attached block
// is tested against owner, the skill that owns the func (nil for an item's
// func); the func's own predicate against the skill the stat calculation
// runs for. A predicate that holds no condition (see conditions.Compile)
// gates nothing. Returns (nil, nil) when neither gates, matching every
// unconditional stat func today.
func funcCondition(direct *modelskill.Condition, attach *modelskill.ConditionClause, owner conditions.Skill) (Condition, error) {
	var g conditionGate
	if attach != nil {
		c, err := conditions.Compile(attach.Root)
		if err != nil {
			return nil, err
		}
		g.attach, g.owner = c, owner
	}
	if direct != nil {
		c, err := conditions.Compile(*direct)
		if err != nil {
			return nil, err
		}
		g.own = c
	}
	if g.attach == nil && g.own == nil {
		return nil, nil
	}
	return g, nil
}

// conditionGate adapts built conditions into a Condition: it resolves
// effector (as passed to Calculator.Calc) to a conditions.Actor. In every
// Calc call site today, effector is the calculation-only wrapper
// (characterStatActor/hostileStatActor/summonStatActor) around a creature,
// which also implements conditions.Actor (see
// model/actor/player/character_conditions.go,
// model/actor/npc/hostile_conditions.go, model/actor/summon/conditions.go),
// so this always resolves to that same owner, matching this package's doc:
// a stat func is gated by its owner alone. A test that aborts (it reached a
// <not> around no condition, see conditions.Evaluate) does not hold.
type conditionGate struct {
	// attach is the enclosing block's <cond>, tested with owner.
	attach conditions.Condition
	owner  conditions.Skill
	// own is the func's own condition, tested with the calculation's skill.
	own conditions.Condition
}

func (g conditionGate) Test(effector stat.Actor, skill conditions.Skill) bool {
	actor, ok := effector.(conditions.Actor)
	if !ok {
		return false
	}
	if g.attach != nil && !g.attach.Test(actor, actor, g.owner) {
		return false
	}
	return g.own == nil || g.own.Test(actor, actor, skill)
}

// Admits reports whether tmpl's attached <cond> (the <cond> sibling gating
// the whole <effect> block) holds for effector applying meta's effect to
// effected, tested with meta as the skill under test. An effect whose
// attached condition fails is never created. A template with no attached
// condition, or one that holds no condition, always admits. A condition
// that does not compile, a nil effector, or a test that aborts does not.
// effected may lack a condition view, failing every target-side test.
func Admits(effector, effected Actor, meta Skill, tmpl modelskill.EffectTemplate) bool {
	if tmpl.AttachCondition == nil {
		return true
	}
	cond, err := conditions.Compile(tmpl.AttachCondition.Root)
	if err != nil {
		return false
	}
	if cond == nil {
		return true
	}
	caster := conditions.ActorOf(effector)
	if caster == nil {
		return false
	}
	return cond.Test(caster, conditions.ActorOf(effected), conditions.SkillOf(meta.Stat))
}
