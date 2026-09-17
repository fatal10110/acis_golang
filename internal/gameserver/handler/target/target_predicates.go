package target

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func skillRadius(skill *modelskill.Definition) int {
	if skill == nil {
		return 0
	}
	return skill.Radius
}

func sameCreature(a, b Actor) bool {
	return a != nil && b != nil && a.ObjectID() == b.ObjectID()
}

// isPlayable reports a player-controlled actor: a player or its summon.
func isPlayable(a Actor) bool { return a.Kind().Playable() }

// isAttackable reports an attackable NPC.
func isAttackable(a Actor) bool { return a.Kind() == actor.KindNPC }

func areaCanAffect(caster, creature Actor) bool {
	if isPlayable(caster) && (isAttackable(creature) || isPlayable(creature)) {
		return creature.AttackableWithoutForceBy(caster)
	}
	if isAttackable(caster) && isPlayable(creature) {
		return creature.AttackableBy(caster)
	}
	return false
}

func auraCanAffect(caster, creature Actor) bool {
	if areaCanAffect(caster, creature) {
		return true
	}
	return caster.Folk() && isPlayable(creature)
}

func validUndeadSingleTarget(creature Actor) bool {
	if creature == nil || creature.Dead() || !creature.Undead() {
		return false
	}
	if isAttackable(creature) {
		return creature.MonsterKind()
	}
	if isPlayable(creature) {
		_, ok := ownerOf(creature)
		return ok && !creature.IsPet()
	}
	return false
}

func corpseTooOld(creature Actor) bool {
	deadline, ok := creature.CorpseDeadline()
	if !ok {
		return false
	}
	corpseTime := creature.CorpseTime()
	if corpseTime <= 0 {
		return false
	}
	cutoff := deadline.Add(-corpseTime / 2)
	return !time.Now().Before(cutoff)
}

func corpseAgeBypass(creature Actor) bool {
	return creature.Spoiled() || creature.Seeded()
}

func summonOf(creature Actor) (Actor, bool) {
	summon, ok := creature.Summon()
	return summon, ok && summon != nil
}

// ownerOf returns the player controlling a summon.
func ownerOf(creature Actor) (Actor, bool) {
	owner, ok := creature.Owner()
	if !ok {
		return nil, false
	}
	// Summon owners are players, which are always skill actors.
	player, ok := owner.(Actor)
	return player, ok
}

func creatureLocation(creature Actor) location.Location {
	if creature == nil {
		return location.Location{}
	}
	x, y, z := creature.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func creatureOrientedLocation(creature Actor) location.OrientedLocation {
	if creature == nil {
		return location.OrientedLocation{}
	}
	return location.OrientedLocation{Location: creatureLocation(creature), Heading: creature.Heading()}
}
