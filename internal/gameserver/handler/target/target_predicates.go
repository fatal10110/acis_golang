package target

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func skillRadius(skill *modelskill.Definition) int {
	if skill == nil {
		return 0
	}
	return skill.Radius
}

func sameCreature(a, b Creature) bool {
	return a != nil && b != nil && a.ObjectID() == b.ObjectID()
}

func canSee(origin, target Creature) bool {
	checker, ok := origin.(SightChecker)
	return !ok || checker.CanSeeTarget(target)
}

func areaCanAffect(caster, creature Creature) bool {
	if caster.Category().Has(CategoryPlayable) && creature.Category()&(CategoryAttackable|CategoryPlayable) != 0 {
		return attackableWithoutForceBy(creature, caster)
	}
	if caster.Category().Has(CategoryAttackable) && creature.Category().Has(CategoryPlayable) {
		return attackableBy(creature, caster)
	}
	return false
}

func auraCanAffect(caster, creature Creature) bool {
	if areaCanAffect(caster, creature) {
		return true
	}
	return caster.Category().Has(CategoryFolk) && creature.Category().Has(CategoryPlayable)
}

func attackableBy(creature, caster Creature) bool {
	rules, ok := creature.(AttackRules)
	return ok && rules.AttackableBy(caster)
}

func attackableWithoutForceBy(creature, caster Creature) bool {
	rules, ok := creature.(AttackRules)
	return ok && rules.AttackableWithoutForceBy(caster)
}

func validUndeadSingleTarget(creature Creature) bool {
	if creature == nil || creature.Dead() || !isUndead(creature) {
		return false
	}
	if creature.Category().Has(CategoryAttackable) {
		return true
	}
	if creature.Category().Has(CategoryPlayable) {
		_, ok := ownerOf(creature)
		return ok
	}
	return false
}

func isUndead(creature Creature) bool {
	undead, ok := creature.(UndeadTarget)
	return ok && undead.Undead()
}

func hasCorpse(creature Creature) bool {
	corpse, ok := creature.(CorpseTarget)
	return ok && corpse.HasCorpse()
}

func corpseTooOld(creature Creature) bool {
	target, ok := creature.(CorpseDeadlineTarget)
	if !ok {
		return false
	}
	deadline, ok := target.CorpseDeadline()
	if !ok {
		return false
	}
	corpseTime := target.CorpseTime()
	if corpseTime <= 0 {
		return false
	}
	cutoff := deadline.Add(-corpseTime / 2)
	return !time.Now().Before(cutoff)
}

func corpseAgeBypass(creature Creature) bool {
	if spoiled, ok := creature.(SpoiledCorpse); ok && spoiled.Spoiled() {
		return true
	}
	seeded, ok := creature.(SeededCorpse)
	return ok && seeded.Seeded()
}

func inPeaceZone(creature Creature) bool {
	zoner, ok := creature.(PeaceZoner)
	return ok && zoner.InPeaceZone()
}

func summonOf(creature Creature) (Creature, bool) {
	summoner, ok := creature.(Summoner)
	if !ok {
		return nil, false
	}
	summon, ok := summoner.Summon()
	return summon, ok && summon != nil
}

func ownerOf(creature Creature) (Creature, bool) {
	owned, ok := creature.(OwnedCreature)
	if !ok {
		return nil, false
	}
	owner, ok := owned.Owner()
	return owner, ok && owner != nil
}

func creatureLocation(creature Creature) location.Location {
	if creature == nil {
		return location.Location{}
	}
	x, y, z := creature.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func creatureOrientedLocation(creature Creature) location.OrientedLocation {
	if creature == nil {
		return location.OrientedLocation{}
	}
	return location.OrientedLocation{Location: creatureLocation(creature), Heading: creature.Heading()}
}
