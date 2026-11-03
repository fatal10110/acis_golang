package cast

import (
	"slices"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// spellSeenRadius is how far from its caster NPCs see a player's skill.
const spellSeenRadius = 1000

// spellWatcher is an NPC whose scripts react to the skills players use: on
// it, or around it.
type spellWatcher interface {
	SeeSpell(caster attackable.Combatant, sk modelskill.Ref, targets []skilltarget.Actor, isPet bool)
	Spelled(caster attackable.Combatant, sk modelskill.Ref)
}

// knownScanner is a creature that visits the creatures it knows around it.
type knownScanner interface {
	ForEachKnownCombatantInRadius(radius int, fn func(attackable.Combatant))
}

// actingPlayer returns the player acting through caster: caster itself when
// it is a player, its owner when it is a summon, with isPet set; ok is false
// for any other caster.
func actingPlayer(caster skilltarget.Actor) (player skilltarget.Actor, isPet, ok bool) {
	switch caster.Kind() {
	case actor.KindPlayer:
		return caster, false, true
	case actor.KindSummon:
		owner, found := caster.Owner()
		if !found || owner == nil {
			return nil, false, false
		}
		player, ok = owner.(skilltarget.Actor)
		return player, true, ok
	}
	return nil, false, false
}

// spelled reports player's skill def, when it is not offensive, to each
// NPC among targets it landed on.
func spelled(player skilltarget.Actor, def modelskill.Definition, targets []skilltarget.Actor) {
	who, ok := player.(attackable.Combatant)
	if !ok || def.Offensive {
		return
	}
	ref := modelskill.Ref{ID: def.ID, Level: def.Level}
	for _, t := range targets {
		if n, ok := t.(spellWatcher); ok {
			n.Spelled(who, ref)
		}
	}
}

// seeSpell reports player's skill def, cast by caster (the player itself
// or, when isPet, its summon) on targets, doors included, to every NPC within
// spellSeenRadius of caster that sees it (see spellUnseen). The hooks run
// once the scan is done.
func seeSpell(caster any, player skilltarget.Actor, def modelskill.Definition, targets []skilltarget.Actor, isPet bool) {
	who, ok := player.(attackable.Combatant)
	if !ok {
		return
	}
	scan, ok := caster.(knownScanner)
	if !ok {
		return
	}
	var watchers []spellWatcher
	scan.ForEachKnownCombatantInRadius(spellSeenRadius, func(c attackable.Combatant) {
		if n, ok := c.(spellWatcher); ok && !spellUnseen(player, def, targets, c) {
			watchers = append(watchers, n)
		}
	})
	if len(watchers) == 0 {
		return
	}
	seen := slices.Clone(targets)
	ref := modelskill.Ref{ID: def.ID, Level: def.Level}
	for _, n := range watchers {
		n.SeeSpell(who, ref, seen, isPet)
	}
}

// spellUnseen reports whether npc does not see player's skill def on
// targets: a skill on one target is unseen when that target is the
// player's summon, and, by the target itself, when the skill is neither
// offensive nor a debuff.
func spellUnseen(player skilltarget.Actor, def modelskill.Definition, targets []skilltarget.Actor, npc attackable.Combatant) bool {
	if len(targets) != 1 {
		return false
	}
	only := targets[0].ObjectID()
	if summon, ok := player.Summon(); ok && summon != nil && summon.ObjectID() == only {
		return true
	}
	return !def.Offensive && !def.Debuff && npc.ObjectID() == only
}
