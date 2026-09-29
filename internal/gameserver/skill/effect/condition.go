package effect

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// funcCondition builds the Condition gate for one stat func from
// its own direct predicate (a func element's child, e.g. <add ...><using
// .../></add>) and/or the <cond> block attached to its enclosing <for>/
// <effect> group, ANDing both when both are present. Returns (nil, nil)
// when neither is set, matching every unconditional stat func today.
func funcCondition(direct *modelskill.Condition, attach *modelskill.ConditionClause) (Condition, error) {
	var conds []conditions.Condition
	if attach != nil {
		c, err := conditions.Compile(attach.Root)
		if err != nil {
			return nil, err
		}
		conds = append(conds, c)
	}
	if direct != nil {
		c, err := conditions.Compile(*direct)
		if err != nil {
			return nil, err
		}
		conds = append(conds, c)
	}
	switch len(conds) {
	case 0:
		return nil, nil
	case 1:
		return conditionGate{conds[0]}, nil
	default:
		return conditionGate{&conditions.And{Conditions: conds}}, nil
	}
}

// conditionGate adapts one built conditions.Condition into a
// Condition: it resolves effector (as passed to Func.Calc) to a
// conditions.Actor. In every Calc call site today, effector is the
// calculation-only wrapper (characterStatActor/hostileStatActor/
// summonStatActor) around a creature, which also implements
// conditions.Actor (see model/actor/player/character_conditions.go,
// model/actor/npc/hostile_conditions.go, model/actor/summon/conditions.go),
// so this always resolves to that same owner, matching this package's doc:
// a stat func is gated by its owner alone.
type conditionGate struct{ cond conditions.Condition }

func (g conditionGate) Test(effector stat.Actor) bool {
	actor, ok := effector.(conditions.Actor)
	if !ok {
		return false
	}
	return g.cond.Test(actor, actor, nil)
}

// andCond combines two optional Condition gates, either of which
// may be nil, into one that requires both (when both are set) or whichever
// one is set (when only one is).
func andCond(a, b Condition) Condition {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return bothCond{a, b}
	}
}

type bothCond struct{ a, b Condition }

func (c bothCond) Test(effector stat.Actor) bool {
	return c.a.Test(effector) && c.b.Test(effector)
}
