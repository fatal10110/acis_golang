package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door/doorhit"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// A door is hit by every offensive skill as a creature is: its landing roll
// runs and a failure is answered as on a creature, but a success lands
// nothing, since a door holds no effects. It never reflects a skill, carries
// no shield and takes no lethal strike.

// doorLands rolls def's landing against door d as against any creature. ok
// is false when the roll has no input to run on.
func doorLands(caster Creature, d *door.Object, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (succeeded, ok bool) {
	in, ok := doorhit.Of(d).SkillSuccessInput(caster, def, bss, shield)
	if !ok {
		return false, false
	}
	return formulas.SkillSucceeds(formulas.SkillSuccessRate(in), landingRoll(caster)), true
}

// disableDoor is a disabler's step on door d. The types that roll report a
// failure to a player caster, ROOT, STUN, SLEEP, PARALYZE and MUTE naming
// the skill at level 1 and BETRAY, AGGREDUCE_CHAR and ERASE at the cast
// level; CONFUSION tells a player caster a door is no target; every other
// type does nothing to a door.
func disableDoor(cast Cast, d *door.Object, skillType string, bsps bool) {
	if d.Dead() {
		return
	}
	_, player := asPlayer(cast.Caster)
	level := cast.Skill.Level
	switch skillType {
	case "ROOT", "STUN", "SLEEP", "PARALYZE", "MUTE":
		level = 1
	case "BETRAY", "AGGREDUCE_CHAR", "ERASE":
	case "CONFUSION":
		if player {
			cast.record(InvalidTargetMessage{})
		}
		return
	default:
		return
	}
	succeeded, ok := doorLands(cast.Caster, d, cast.Skill, bsps, formulas.ShieldFailed)
	if ok && !succeeded && player {
		appendResisted(cast.resisted, d, cast.Skill, level, false)
	}
}

// continuousOnDoor is a continuous skill def's step on door d: an offensive
// or debuff skill rolls to land and a failure is answered ATTACK_FAILED;
// anything else acts on the door with nothing to show.
func continuousOnDoor(cast Cast, d *door.Object, def modelskill.Definition, bsps bool, result *Result) {
	if !def.Offensive && !def.Debuff {
		return
	}
	if succeeded, ok := doorLands(cast.Caster, d, def, bsps, formulas.ShieldFailed); !ok || !succeeded {
		result.AttackFailed++
		result.record(AttackFailedMessage{})
	}
}

// manaDamageDoor is a MANADAM cast's step on door d: a cast the magic
// affected roll misses is reported missed; otherwise the effect roll runs,
// its failure reported resisted at the cast level, and a player caster
// hears its opponent's MP fell by what the drain took of the none a door
// has.
func manaDamageDoor(cast Cast, d *door.Object, result *Result) {
	if d.Dead() {
		return
	}
	in, ok := creature.ResolveManaDamageInput(cast.Caster, doorhit.Of(d), 0, cast.Skill)
	if !ok {
		return
	}
	if !in.Affected {
		result.ManaDamageMissed++
		result.record(ManaDamageMissedMessage{})
		return
	}
	if len(cast.Skill.Effects) > 0 {
		succeeded, ok := doorLands(cast.Caster, d, cast.Skill, blessedSpiritshotCharged(cast.Caster), formulas.ShieldFailed)
		if ok && !succeeded {
			appendResisted(result, d, cast.Skill, cast.Skill.Level, true)
		}
	}
	// The drain takes the damage, capped at the door's MP, which is 0.
	mp := formulas.ManaDamage(in)
	if mp > 0 {
		mp = 0
	}
	if _, player := asPlayer(cast.Caster); player {
		result.OpponentMPReduced = append(result.OpponentMPReduced, commons.TruncInt32Sat(mp))
		result.record(OpponentMPReducedMessage{MP: commons.TruncInt32Sat(mp)})
	}
}
