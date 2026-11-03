package effect

import (
	"iter"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/funcs"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// StatMods is a fixed set of Mods grouped by Stat, each group in the order
// a Calculator reaches by adding them one at a time. It is never changed
// once built, so any number of actors compute from one set and start their
// Calculators from it (see NewCalculator): a Calculator publishes a fresh
// chain on every change and leaves the group it started from intact. A nil
// *StatMods holds no Mods.
type StatMods struct {
	groups []statGroup
}

type statGroup struct {
	stat stat.Stat
	mods []Mod
}

// GroupStatMods groups fns by Stat, or returns nil when fns is empty.
func GroupStatMods(fns []Mod) *StatMods {
	if len(fns) == 0 {
		return nil
	}
	g := &StatMods{}
	for _, m := range fns {
		i := slices.IndexFunc(g.groups, func(grp statGroup) bool { return grp.stat == m.Stat })
		if i < 0 {
			g.groups = append(g.groups, statGroup{stat: m.Stat})
			i = len(g.groups) - 1
		}
		g.groups[i].mods = insertMod(g.groups[i].mods, m)
	}
	return g
}

// mods returns s's group, nil when it has none.
func (g *StatMods) mods(s stat.Stat) []Mod {
	if g == nil {
		return nil
	}
	for _, grp := range g.groups {
		if grp.stat == s {
			return grp.mods
		}
	}
	return nil
}

// Calc runs s's group around builtin (nil for none) for actor from base,
// against target for skill (nil for none), as a Calculator holding just
// them would.
func (g *StatMods) Calc(s stat.Stat, builtin funcs.Func, actor stat.Actor, base float64, target conditions.Actor, skill conditions.Skill) float64 {
	return calcChain(g.mods(s), builtin, actor, base, target, skill)
}

// NewCalculator returns a Calculator for s that starts with s's group and
// builtin as its finalize step.
func (g *StatMods) NewCalculator(s stat.Stat, builtin funcs.Func) Calculator {
	return Calculator{mods: g.mods(s), builtin: builtin}
}

// StatsOwnedBy yields each Stat whose group holds a Mod of owner.
func (g *StatMods) StatsOwnedBy(owner ModOwner) iter.Seq[stat.Stat] {
	return func(yield func(stat.Stat) bool) {
		if g == nil {
			return
		}
		for _, grp := range g.groups {
			if slices.ContainsFunc(grp.mods, func(m Mod) bool { return m.Owner.is(owner) }) && !yield(grp.stat) {
				return
			}
		}
	}
}
