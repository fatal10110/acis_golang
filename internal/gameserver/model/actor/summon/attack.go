package summon

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type physicalTarget interface {
	attackable.Combatant
	Position() (int, int, int)
	PDef() float64
	Evasion() int
}

func (a *Actor) AttackDisabled() bool   { return a.DenyAIAction() }
func (a *Actor) MovementDisabled() bool { return a.DenyAIAction() }

func (a *Actor) InAttackRange(target attackable.Combatant) bool {
	other, ok := target.(interface {
		Position() (int, int, int)
		CollisionRadius() float64
	})
	if !ok {
		return false
	}
	x, y, z := a.Position()
	tx, ty, tz := other.Position()
	distance := a.PhysicalAttackRange() + int(a.CollisionRadius()) + int(other.CollisionRadius())
	return location.In3DRange(x, y, z, tx, ty, tz, distance)
}

// LineOfSight reports whether two actors have a geodata-obstructed view.
type LineOfSight interface {
	CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool
}

// SetLineOfSight attaches the geodata query used for attack visibility.
func (a *Actor) SetLineOfSight(los LineOfSight) { a.los = los }

// CanSee reports whether target is visible through geodata, or permits the
// check when no query is attached (such as isolated domain tests).
func (a *Actor) CanSee(target attackable.Combatant) bool {
	if a.los == nil {
		return true
	}
	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return false
	}
	var height float64
	if target, ok := target.(interface{ CollisionHeight() float64 }); ok {
		height = target.CollisionHeight()
	}
	ox, oy, oz := a.Position()
	tx, ty, tz := other.Position()
	return a.los.CanSeeActor(ox, oy, oz, a.CollisionHeight(), tx, ty, tz, height)
}
func (a *Actor) AttackSpeed() int                { return int(a.PhysicalAttackSpeed()) }
func (a *Actor) WeaponReuseDelay() time.Duration { return 0 }
func (a *Actor) WeaponGrade() int                { return 0 }
func (a *Actor) InPeaceZone() bool               { return false }
func (a *Actor) Evasion() int                    { return int(a.EvasionRate()) }

func (a *Actor) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	hit := attack.Hit{Target: target, TargetID: target.ObjectID()}
	other, ok := target.(physicalTarget)
	if !ok {
		hit.Miss = true
		return hit
	}
	_, _, z := a.Position()
	_, _, targetZ := other.Position()
	if formulas.Missed(formulas.HitRate(int(a.Accuracy()), other.Evasion(), z-targetZ, false, false, true), a.Roll(1000)) {
		hit.Miss = true
		return hit
	}
	defence := other.PDef()
	if defence <= 0 {
		defence = 1
	}
	damage := formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: a.PAtk(), Defence: defence, PosMul: formulas.PosMul(false, true, false),
		ElementalMul: 1, RandomMul: creature.RandomDamageMultiplier(a, modelskill.Definition{}),
		RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, CritDamageMul: 1, CritDamagePosMul: 1, CritVulnMul: 1,
	})
	if split {
		damage /= 2
	}
	hit.Damage = int(damage)
	return hit
}

func (a *Actor) BroadcastAttack(snapshot attack.Snapshot) error {
	a.BroadcastFrame(serverpackets.FrameAttack(snapshot))
	return nil
}
