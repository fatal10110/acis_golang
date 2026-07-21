package npc

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
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
	at := h.location()
	return location.In3DRange(at.X, at.Y, at.Z, tx, ty, tz, totalRadius)
}

// LineOfSight is the geodata query CanSee needs to gate targeting on real
// terrain occlusion between two actors.
type LineOfSight interface {
	CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool
}

// SetLineOfSight records the geodata line-of-sight query used by CanSee. A
// nil los (e.g. in tests that don't exercise geodata) leaves CanSee
// permissive.
func (h *Hostile) SetLineOfSight(los LineOfSight) {
	h.los = los
}

// CanSee reports whether target is visible to this NPC: a geodata
// line-of-sight query between the two actors' positions and eye heights, or
// permissive when no line-of-sight query is attached (e.g. in tests).
func (h *Hostile) CanSee(target attackable.Combatant) bool {
	if h.los == nil {
		return true
	}
	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return false
	}
	var theight float64
	if th, ok := target.(interface{ CollisionHeight() float64 }); ok {
		theight = th.CollisionHeight()
	}

	ox, oy, oz := h.Position()
	tx, ty, tz := other.Position()
	return h.los.CanSeeActor(ox, oy, oz, h.CollisionHeight(), tx, ty, tz, theight)
}

// CollisionRadius returns this NPC's body radius, used to resolve attack
// and follow ranges.
func (h *Hostile) CollisionRadius() float64 {
	return h.Instance.Template.CollisionRadius
}

// CollisionHeight returns this NPC's body height, used for line-of-sight
// eye-height resolution.
func (h *Hostile) CollisionHeight() float64 {
	return h.Instance.Template.CollisionHeight
}

// AttackType returns this NPC's attack style, resolved from the weapon
// SetWeapon recorded. Unarmed (WeaponFist) when SetWeapon found no
// right-hand weapon — the common case, since the overwhelming majority of
// monster templates carry no weapon item id in the shipped data.
func (h *Hostile) AttackType() item.WeaponType {
	if h.weapon == nil {
		return item.WeaponFist
	}
	return h.weapon.Type
}

// AttackSpeed returns this NPC's physical attack speed stat.
func (h *Hostile) AttackSpeed() int {
	return int(h.Instance.Template.AtkSpd)
}

// WeaponReuseDelay returns this NPC's weapon reuse delay; only read for a
// bow attacker. Zero when unarmed or not wielding a template-defined
// weapon.
func (h *Hostile) WeaponReuseDelay() time.Duration {
	if h.weapon == nil {
		return 0
	}
	return time.Duration(h.weapon.ReuseDelay) * time.Millisecond
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

// PDef returns this NPC's physical defense stat, finalized through its stat
// calculator (level scaling plus any active buff/debuff).
func (h *Hostile) PDef() float64 {
	return h.calcStat(stat.PowerDefence, h.Instance.Template.PDef)
}

// Evasion returns this NPC's physical evasion rating (per-mille), finalized
// through its stat calculator (base DEX/level plus any active buff/debuff).
func (h *Hostile) Evasion() int {
	return int(h.calcStat(stat.EvasionRate, 0))
}

// MakeAttackHit resolves one physical attack against target: a hit/miss
// roll, a critical roll, and a damage roll through the shared
// physical-damage formula. A target that can't exchange physical damage (no
// physicalTarget surface) always misses.
func (h *Hostile) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	hit := attack.Hit{Target: target, TargetID: target.ObjectID()}

	other, ok := target.(physicalTarget)
	if !ok {
		hit.Miss = true
		return hit
	}

	tpl := h.Instance.Template

	accuracy := int(h.calcStat(stat.AccuracyCombat, 0))
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

	critRate := math.Min(h.calcStat(stat.CriticalRate, tpl.CritRate), 500)
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
		AttackPower:       h.calcStat(stat.PowerAttack, tpl.PAtk),
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

// BroadcastDie sends the death packet to every currently known observer
// capable of receiving one, so clients play the corpse-fall animation
// instead of leaving this NPC standing until its corpse decays. It is a
// no-op until SetWorld has been called.
func (h *Hostile) BroadcastDie() {
	if h.world == nil {
		return
	}
	h.world.ForEachKnown(h, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameDie(h.ObjectID(), serverpackets.DieOptions{}))
	})
}

// BroadcastMove sends a MoveToLocation packet for event to every currently
// known observer capable of receiving one. It is a no-op until SetWorld has
// been called.
func (h *Hostile) BroadcastMove(event move.Event) {
	if h.world == nil {
		return
	}
	known := h.appendKnown()
	defer h.releaseKnown()
	for _, o := range known {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			continue
		}
		receiver.SendFrame(serverpackets.FrameMove(h.ObjectID(), event))
	}
}

// BroadcastStop sends a stop-in-place notice to every currently known
// observer capable of receiving one. It is a no-op until SetWorld has been
// called.
func (h *Hostile) BroadcastStop() {
	if h.world == nil {
		return
	}
	x, y, z := h.Position()
	at := location.Location{X: x, Y: y, Z: z}
	known := h.appendKnown()
	defer h.releaseKnown()
	for _, o := range known {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			continue
		}
		receiver.SendFrame(serverpackets.FrameStopMove(h.ObjectID(), at, h.Heading()))
	}
}

// BroadcastStatus sends this NPC's current/max HP to every currently known
// observer capable of receiving one, so a target's health bar reflects
// damage as it lands rather than only the moment it dies. It is a no-op
// until SetWorld has been called.
func (h *Hostile) BroadcastStatus() {
	if h.world == nil {
		return
	}
	attrs := []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusMaxHP, Value: h.MaxHP()},
		{Type: serverpackets.StatusCurrentHP, Value: h.CurrentHP()},
	}
	known := h.appendKnown()
	defer h.releaseKnown()
	for _, o := range known {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			continue
		}
		receiver.SendFrame(serverpackets.FrameStatusUpdate(h.ObjectID(), attrs))
	}
}

func (h *Hostile) appendKnown() []world.Tracked {
	return h.known.Snapshot(h.world, h)
}

func (h *Hostile) releaseKnown() {
	h.known.Release()
}

// AttackableBy reports whether attacker may physically attack this NPC.
func (h *Hostile) AttackableBy(attacker attack.CreatureActor) bool {
	return attacker != nil && attacker != h && !h.AlikeDead()
}
