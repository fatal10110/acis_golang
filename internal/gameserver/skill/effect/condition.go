package effect

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// funcCondition builds the Condition gate for one stat func from its own
// direct predicate (a func element's child, e.g. <add ...><using
// .../></add>), tested on every calculation against the calculation's
// target and skill. A predicate that holds no condition (see
// conditions.Compile) gates nothing. Returns (nil, nil) when it does not
// gate, matching every unconditional stat func today.
func funcCondition(direct *modelskill.Condition) (Condition, error) {
	if direct == nil {
		return nil, nil
	}
	c, err := conditions.Compile(*direct)
	if err != nil || c == nil {
		return nil, err
	}
	return conditionGate{c}, nil
}

// attachCondition compiles the <cond> block attached to a func's enclosing
// <for>/<effect> group, or nil when there is none or it holds no condition.
func attachCondition(attach *modelskill.ConditionClause) (conditions.Condition, error) {
	if attach == nil {
		return nil, nil
	}
	return conditions.Compile(attach.Root)
}

// conditionGate adapts a built condition into a Condition: it resolves
// effector (as passed to Calculator.Calc) to a conditions.Actor. In every
// Calc call site, effector is the calculation-only wrapper
// (characterStatActor/hostileStatActor/summonStatActor) around a creature,
// which also implements conditions.Actor (see
// model/actor/player/character_conditions.go,
// model/actor/npc/hostile_conditions.go, model/actor/summon/conditions.go).
// A test that aborts (it reached a <not> around no condition, see
// conditions.Evaluate) does not hold.
type conditionGate struct{ cond conditions.Condition }

func (g conditionGate) Test(effector stat.Actor, effected conditions.Actor, skill conditions.Skill) bool {
	actor, ok := effector.(conditions.Actor)
	if !ok {
		return false
	}
	return g.cond.Test(actor, effected, skill)
}

// attaches reports whether m's attached <cond> holds, tested with effector
// and effected (nil for none) and the skill owning m. A func whose attached
// condition fails is not attached at all, so the outcome stays fixed until
// the func is attached again.
func (m Mod) attaches(effector, effected conditions.Actor) bool {
	return m.attach == nil || (effector != nil && m.attach.Test(effector, effected, m.attachSkill))
}

// Attachable returns the mods of fns, attached to owner outside an effect
// list, whose attached <cond> holds: a skill's funcs are tested against no
// target, an item's against owner itself. An effect's funcs are tested by
// its list each time the effect activates. fns itself is returned when
// every mod attaches.
func Attachable(fns []Mod, owner conditions.Actor) []Mod {
	return keepMods(fns, func(m Mod) bool {
		var effected conditions.Actor
		if m.Owner.item.Tmpl != nil {
			effected = owner
		}
		return m.attaches(owner, effected)
	})
}

// attachableFuncs is e's funcs whose attached condition holds for e's
// effector applying it to its effected.
func (e *Effect) attachableFuncs() []Mod {
	effector, effected := conditions.ActorOf(e.Effector), conditions.ActorOf(e.Effected)
	return keepMods(e.Funcs, func(m Mod) bool { return m.attaches(effector, effected) })
}

// keepMods returns the mods of fns keep accepts, fns itself when it
// accepts every one.
func keepMods(fns []Mod, keep func(Mod) bool) []Mod {
	for i, m := range fns {
		if keep(m) {
			continue
		}
		kept := append(make([]Mod, 0, len(fns)-1), fns[:i]...)
		for _, rest := range fns[i+1:] {
			if keep(rest) {
				kept = append(kept, rest)
			}
		}
		return kept
	}
	return fns
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
