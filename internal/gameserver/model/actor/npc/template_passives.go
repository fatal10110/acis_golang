package npc

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// passiveCache keeps a template's passives compiled, for every NPC of the
// template to share. Compiling is idempotent, so concurrent spawns that
// both miss simply both compile and the last one stored stays.
type passiveCache struct {
	compiled atomic.Pointer[compiledPassives]
}

// compiledPassives is a template's passives compiled against the skill
// definitions they resolved to. It is never changed once built.
type compiledPassives struct {
	// defs is what each Passives entry resolved to, nil for none.
	defs []*skill.Definition
	// mods is every passive Mod in Passives order.
	mods []effect.Mod
	// byStat is mods grouped by Stat.
	byStat *effect.StatMods
}

// passiveMods returns t's passives compiled against lookup, nil for none.
// NPCs of one template share the result for as long as lookup resolves
// every passive to the definition it was compiled from, so a reloaded skill
// table compiles it anew for the NPCs spawned afterwards.
func (t *Template) passiveMods(lookup skillDefinitions) (*compiledPassives, error) {
	if lookup == nil || len(t.Passives) == 0 {
		return nil, nil
	}
	if t.passives != nil {
		if c := t.passives.compiled.Load(); c != nil && c.resolvedBy(lookup, t.Passives) {
			return c, nil
		}
	}
	defs := make([]*skill.Definition, len(t.Passives))
	for i, ref := range t.Passives {
		if def, ok := lookup.Definition(ref); ok {
			defs[i] = def
		}
	}
	mods, err := effect.TemplatePassiveMods(lookup, t.Passives)
	if err != nil {
		return nil, err
	}
	c := &compiledPassives{defs: defs, mods: mods, byStat: effect.GroupStatMods(mods)}
	if t.passives != nil {
		t.passives.compiled.Store(c)
	}
	return c, nil
}

// resolvedBy reports whether lookup resolves passives to the definitions c
// was compiled from.
func (c *compiledPassives) resolvedBy(lookup skillDefinitions, passives []skill.Ref) bool {
	if len(c.defs) != len(passives) {
		return false
	}
	for i, ref := range passives {
		def, ok := lookup.Definition(ref)
		if !ok {
			def = nil
		}
		if def != c.defs[i] {
			return false
		}
	}
	return true
}

// attached returns the passive Mods that attach to owner and the same Mods
// grouped by Stat: the template's shared group when every one attaches.
func (c *compiledPassives) attached(owner conditions.Actor) ([]effect.Mod, *effect.StatMods) {
	if c == nil {
		return nil, nil
	}
	kept := effect.Attachable(c.mods, owner)
	if len(kept) == len(c.mods) {
		return c.mods, c.byStat
	}
	return kept, effect.GroupStatMods(kept)
}
