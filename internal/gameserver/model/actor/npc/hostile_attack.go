package npc

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// physicalTarget is the surface MakeAttackHit needs from an opponent to
// resolve a physical hit and deliver its result: position for the attack's
// altitude term, defense and evasion for the hit/damage rolls, and a way to
// apply the computed damage. Any live combatant capable of exchanging
// physical damage should satisfy this.
type physicalTarget interface {
	attackable.Combatant
	Position() (int, int, int)
	PDef() float64
	Evasion() int
}

// AttackDisabled reports whether this NPC is unable to start an attack. No
// abnormal-effect system (petrify, fear, attack-block) is wired to a live
// NPC yet, so death is the only disabling condition modeled so far.
func (h *Hostile) AttackDisabled() bool {
	return h.Dead()
}

// MovementDisabled reports whether this NPC is unable to move. No
// abnormal-effect system (root, sleep, paralysis) is wired to a live NPC
// yet, so the template's own movement flag is the only condition modeled
// so far.
func (h *Hostile) MovementDisabled() bool {
	return !h.Instance.Template.CanMove
}

// InAttackRange reports whether target sits within this NPC's physical
// attack range, accounting for both actors' collision footprints. A target
// with no known position/footprint is out of range by definition.
func (h *Hostile) InAttackRange(target attackable.Combatant) bool {
	other, ok := target.(interface {
		Position() (int, int, int)
		CollisionRadius() float64
	})
	if !ok {
		return false
	}

	tx, ty, tz := other.Position()
	totalRadius := h.PhysicalAttackRange() + int(h.CollisionRadius()) + int(other.CollisionRadius())
	return in3DRange(h.location(), location.Location{X: tx, Y: ty, Z: tz}, totalRadius)
}

// CanSee reports whether target is visible to this NPC. No geodata
// line-of-sight query is wired into a live NPC yet, so every known target
// counts as visible until that's plumbed in.
func (h *Hostile) CanSee(attackable.Combatant) bool {
	return true
}

// CollisionRadius returns this NPC's body radius, used to resolve attack
// and follow ranges.
func (h *Hostile) CollisionRadius() float64 {
	return h.Instance.Template.CollisionRadius
}

// AttackType always resolves to an unarmed strike: hostile NPCs in this
// port fight with their body, not equipped gear. The overwhelming majority
// of monster templates carry no weapon item id in the shipped data;
// resolving the rare weapon-wielding template's right-hand item id to its
// weapon kind data is deferred until an NPC-side equipment/item-table
// lookup is wired to a live actor.
func (h *Hostile) AttackType() item.WeaponType {
	return item.WeaponFist
}

// AttackSpeed returns this NPC's physical attack speed stat.
func (h *Hostile) AttackSpeed() int {
	return int(h.Instance.Template.AtkSpd)
}

// WeaponReuseDelay is only read for a bow attacker; hostile NPCs always
// fight unarmed (see AttackType), so this never gates a real cooldown.
func (h *Hostile) WeaponReuseDelay() time.Duration {
	return 0
}

// WeaponGrade only matters when SoulshotCharged reports true, which this
// type never does yet — see SoulshotCharged.
func (h *Hostile) WeaponGrade() int {
	return 0
}

// SoulshotCharged always reports false. A template can define an
// AI-driven soulshot recharge counter (its <ai> SoulShot value), but that
// stateful recharge loop isn't wired to a live NPC yet.
func (h *Hostile) SoulshotCharged() bool {
	return false
}

// SetHeadingTo orients this NPC toward target. A target with no known
// position is ignored.
func (h *Hostile) SetHeadingTo(target attackable.Combatant) {
	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return
	}
	sx, sy, _ := h.Position()
	tx, ty, _ := other.Position()
	h.Presence.SetHeading(location.Location{X: sx, Y: sy}.HeadingTo(location.Location{X: tx, Y: ty}))
}

// PDef returns this NPC's physical defense stat.
func (h *Hostile) PDef() float64 {
	return h.Instance.Template.PDef
}

// Evasion returns this NPC's physical evasion rating (per-mille), derived
// from its base DEX and level. Equipment- and effect-driven evasion bonuses
// aren't modeled yet — no gear or buff/debuff system exists for a live NPC
// actor.
func (h *Hostile) Evasion() int {
	tpl := h.Instance.Template
	return int(statbonus.BaseEvasionAccuracy[statbonus.ClampIndex(tpl.DEX)]) + tpl.Level
}

// MakeAttackHit resolves one physical attack against target: a hit/miss
// roll, a critical roll, and a damage roll through the shared
// physical-damage formula. A target that can't exchange physical damage (no
// physicalTarget surface) always misses.
//
// Accuracy, evasion, critical rate and attack power are derived directly
// from base template stats (DEX/level/STR-independent PAtk); the full
// buff/debuff and STR/level finalization stat chain isn't wired to a live
// NPC yet, so these are the pre-modifier values, not a live creature's
// fully finalized combat stats.
func (h *Hostile) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	hit := attack.Hit{Target: target, TargetID: target.ObjectID()}

	other, ok := target.(physicalTarget)
	if !ok {
		hit.Miss = true
		return hit
	}

	tpl := h.Instance.Template
	dexIdx := statbonus.ClampIndex(tpl.DEX)

	accuracy := int(statbonus.BaseEvasionAccuracy[dexIdx]) + tpl.Level
	evasion := other.Evasion()

	sx, sy, sz := h.Position()
	_, _, tz := other.Position()
	_ = sx
	_ = sy

	rate := formulas.HitRate(accuracy, evasion, sz-tz, false, false, true)
	if formulas.Missed(rate, h.roll(1000)) {
		hit.Miss = true
		return hit
	}

	critRate := tpl.CritRate * statbonus.DEXBonus[dexIdx] * 10
	crit := formulas.CritSucceeds(critRate, h.roll(1000))

	randomMul := 1.0
	if spread := tpl.BaseRandomDamage; spread > 0 {
		randomMul = 1 + float64(h.roll(2*spread+1)-spread)/100
	}

	defence := other.PDef()
	if defence <= 0 {
		defence = 1
	}

	damage := formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower:       tpl.PAtk,
		Defence:           defence,
		Crit:              crit,
		PosMul:            formulas.PosMul(false, true, crit),
		ElementalMul:      1,
		RandomMul:         randomMul,
		RaceMul:           1,
		WeaponVulnMul:     1,
		PvPMul:            1,
		CritDamageMul:     1,
		CritDamagePosMul:  1,
		CritVulnMul:       1,
		CritDamageAddBase: 0,
	})

	if split {
		damage /= 2
	}

	hit.Damage = int(damage)
	hit.Crit = crit
	return hit
}

// BroadcastAttack sends the attack packet to every currently known
// observer capable of receiving one (i.e. a connected player session). It
// is a no-op until SetWorld has been called.
func (h *Hostile) BroadcastAttack(snapshot attack.Snapshot) {
	if h.world == nil {
		return
	}
	h.world.ForEachKnown(h, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameAttack(snapshot))
	})
}
