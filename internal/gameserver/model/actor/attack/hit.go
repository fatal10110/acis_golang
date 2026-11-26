package attack

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// HitRoll is the attacker-side input of one physical hit roll that differs
// by actor kind.
type HitRoll struct {
	// Accuracy is the attacker's accuracy rating.
	Accuracy int
	// Night reports whether night time holds for the attacker.
	Night bool
	// CritRate is the attacker's critical rate against target, read only
	// once the hit lands. Nil reads it from the attacker's stats against
	// target.
	CritRate func(target conditions.Actor) float64
}

// ResolveHit resolves one physical swing of attacker at target: the hit
// roll, then for a landed hit the critical roll, the shield and the damage.
// Both rolls draw attacker.Roll(1000), the critical one only after the hit
// roll lands. A target with no formula stats is always missed.
func ResolveHit(attacker creature.FormulaActor, target attackable.Combatant, r HitRoll, split bool) Hit {
	hit := Hit{TargetID: target.ObjectID()}
	// Folding this needs the world known-list to hand out formula operands.
	// CreatureActor.ForEachKnownCombatantInRadius yields
	// attackable.Combatant, narrowed from world.Tracked, and world cannot
	// name creature.FormulaActor because creature already depends on world.
	// The pole and split paths draw their extra targets from that callback,
	// so the narrowing stays here, at the one point where a combatant
	// becomes a formula operand; see #2362.
	other, ok := target.(creature.FormulaActor)
	if !ok {
		hit.Miss = true
		return hit
	}

	evasion := other.Against(conditions.ActorOf(attacker), nil).Evasion()
	_, _, az := attacker.Position()
	_, _, tz := other.Position()
	behind, inFront := creature.AttackFacing(other, attacker)
	rate := formulas.HitRate(r.Accuracy, evasion, az-tz, r.Night, behind, inFront)
	if formulas.Missed(rate, attacker.Roll(1000)) {
		hit.Miss = true
		return hit
	}

	versus := conditions.ActorOf(other)
	var critRate float64
	if r.CritRate != nil {
		critRate = r.CritRate(versus)
	} else {
		critRate = attacker.Against(versus, nil).CriticalRate()
	}
	crit := formulas.CritSucceeds(critRate, attacker.Roll(1000))
	in, shield := creature.ResolvePhysicalAttackInput(attacker, other, crit)
	hit.Damage = creature.ApplyPhysicalAttackDamage(in, shield, split)
	hit.Crit = crit
	hit.Shield = shield
	return hit
}
