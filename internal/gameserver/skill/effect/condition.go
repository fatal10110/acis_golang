package effect

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// funcCondition builds the basefunc.Condition gate for one stat func from
// its own direct predicate (a func element's child, e.g. <add ...><using
// .../></add>) and/or the <cond> block attached to its enclosing <for>/
// <effect> group, ANDing both when both are present. Returns (nil, nil)
// when neither is set, matching every unconditional stat func today.
func funcCondition(direct *modelskill.Condition, attach *modelskill.ConditionClause) (basefunc.Condition, error) {
	var conds []conditions.Condition
	if attach != nil {
		c, err := buildCondition(attach.Root)
		if err != nil {
			return nil, err
		}
		conds = append(conds, c)
	}
	if direct != nil {
		c, err := buildCondition(*direct)
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
// basefunc.Condition: it resolves effector/effected (as passed to Func.Calc)
// to a conditions.Actor, preferring effected — every Calc call site in this
// codebase passes the stat func's owner (a *player.Character, NPC, or
// summon) as effected, and effector as a thinner calculation-only wrapper
// around the same owner — falling back to effector when effected doesn't
// implement it. Both roles then test as that one owner, matching this
// package's doc: a stat func is gated by its owner alone.
type conditionGate struct{ cond conditions.Condition }

func (g conditionGate) Test(effector, effected, skill any) bool {
	actor, ok := effected.(conditions.Actor)
	if !ok {
		actor, ok = effector.(conditions.Actor)
	}
	if !ok {
		return false
	}
	return g.cond.Test(actor, actor, nil)
}

// buildCondition converts one parsed skill.Condition node to a runnable
// conditions.Condition, supporting exactly the tags the shipped datapack
// uses on a stat func: using, player, and, or, not, game (see issue #1499).
func buildCondition(node modelskill.Condition) (conditions.Condition, error) {
	switch strings.ToLower(node.Kind) {
	case "using":
		mask := item.ParseWornKindMask(node.Attrs["kind"])
		return conditions.UsingItemType{Mask: int(mask)}, nil
	case "player":
		return buildPlayerCondition(node.Attrs)
	case "game":
		return buildGameCondition(node.Attrs)
	case "and":
		return buildLogic(node.Children, true)
	case "or":
		return buildLogic(node.Children, false)
	case "not":
		if len(node.Children) != 1 {
			return nil, fmt.Errorf("skill: not: want exactly one child condition, got %d", len(node.Children))
		}
		child, err := buildCondition(node.Children[0])
		if err != nil {
			return nil, err
		}
		return conditions.Not{Condition: child}, nil
	default:
		return nil, fmt.Errorf("skill: unsupported condition %q", node.Kind)
	}
}

func buildLogic(children []modelskill.Condition, and bool) (conditions.Condition, error) {
	if and {
		g := &conditions.And{}
		for _, ch := range children {
			c, err := buildCondition(ch)
			if err != nil {
				return nil, err
			}
			g.Add(c)
		}
		return g, nil
	}
	g := &conditions.Or{}
	for _, ch := range children {
		c, err := buildCondition(ch)
		if err != nil {
			return nil, err
		}
		g.Add(c)
	}
	return g, nil
}

// buildPlayerCondition resolves one <player .../> element's single
// attribute to the matching conditions.Condition. Only the attributes the
// shipped datapack's stat funcs actually use are wired; an unrecognized
// attribute is a load-time error rather than a silent no-op.
func buildPlayerCondition(attrs map[string]string) (conditions.Condition, error) {
	if v, ok := attrs["hp"]; ok {
		pct, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("skill: player hp: %w", err)
		}
		return conditions.Hp{Percent: pct}, nil
	}
	if v, ok := attrs["moving"]; ok {
		return conditions.PlayerState{Check: conditions.StateMoving, Required: parseBool(v)}, nil
	}
	if v, ok := attrs["running"]; ok {
		return conditions.PlayerState{Check: conditions.StateRunning, Required: parseBool(v)}, nil
	}
	if v, ok := attrs["resting"]; ok {
		return conditions.PlayerState{Check: conditions.StateResting, Required: parseBool(v)}, nil
	}
	if v, ok := attrs["flying"]; ok {
		return conditions.PlayerState{Check: conditions.StateFlying, Required: parseBool(v)}, nil
	}
	if v, ok := attrs["behind"]; ok {
		return conditions.PlayerState{Check: conditions.StateBehind, Required: parseBool(v)}, nil
	}
	if v, ok := attrs["front"]; ok {
		return conditions.PlayerState{Check: conditions.StateFront, Required: parseBool(v)}, nil
	}
	return nil, fmt.Errorf("skill: player: no recognized attribute in %v", attrs)
}

// buildGameCondition resolves one <game .../> element's single attribute.
func buildGameCondition(attrs map[string]string) (conditions.Condition, error) {
	if v, ok := attrs["night"]; ok {
		return conditions.GameTime{Clock: gameClockRef{}, Night: parseBool(v)}, nil
	}
	if v, ok := attrs["chance"]; ok {
		pct, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("skill: game chance: %w", err)
		}
		return conditions.GameChance{Percent: pct}, nil
	}
	return nil, fmt.Errorf("skill: game: no recognized attribute in %v", attrs)
}

// andCond combines two optional basefunc.Condition gates, either of which
// may be nil, into one that requires both (when both are set) or whichever
// one is set (when only one is).
func andCond(a, b basefunc.Condition) basefunc.Condition {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return bothCond{a, b}
	}
}

type bothCond struct{ a, b basefunc.Condition }

func (c bothCond) Test(effector, effected, skill any) bool {
	return c.a.Test(effector, effected, skill) && c.b.Test(effector, effected, skill)
}

func parseBool(v string) bool {
	return strings.EqualFold(v, "true") || v == "1"
}

// gameClock is the boot-wired day/night source <game night=.../> reads;
// SetGameClock installs it once at server startup (see cmd/gameserver),
// before any character can log in and reach a conditional stat func.
var gameClock conditions.NightSource

// SetGameClock installs clock as the source GameTime conditions read.
func SetGameClock(clock conditions.NightSource) { gameClock = clock }

// gameClockRef defers to the package-level gameClock at Test time rather
// than at condition-build time, since skill definitions build once at data
// load — before SetGameClock necessarily runs — while conditions are tested
// per stat recalculation, long after boot wiring completes.
type gameClockRef struct{}

func (gameClockRef) IsNight() bool {
	if gameClock == nil {
		return false
	}
	return gameClock.IsNight()
}
