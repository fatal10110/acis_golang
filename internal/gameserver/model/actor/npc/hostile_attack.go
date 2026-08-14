package npc

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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

// CanSeeTarget adapts NPC line-of-sight to the launch revalidation target
// surface.
func (h *Hostile) CanSeeTarget(target skilltarget.Creature) bool {
	combatant, ok := target.(attackable.Combatant)
	return ok && h.CanSee(combatant)
}

// CollisionRadius returns this NPC's body radius, used to resolve attack
// and follow ranges: a live runtime override (e.g. from the Grow effect) if
// one is set, otherwise the template value.
func (h *Hostile) CollisionRadius() float64 {
	if r := h.collisionRadiusOverride.Load(); r != nil {
		return *r
	}
	return h.Instance.Template.CollisionRadius
}

// SetCollisionRadius installs a runtime body-radius override, e.g. the Grow
// effect's radius*1.19 scaling.
func (h *Hostile) SetCollisionRadius(radius float64) {
	h.collisionRadiusOverride.Store(&radius)
}

// ResetCollisionRadius clears any runtime body-radius override, restoring
// the template value, e.g. on the Grow effect's exit.
func (h *Hostile) ResetCollisionRadius() {
	h.collisionRadiusOverride.Store(nil)
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

// WeaponGrade returns this NPC's resolved right-hand weapon's crystal
// grade, recorded by SetWeapon. Zero (CrystalNone) when unarmed. Reference:
// Npc.getActiveWeaponItem, Npc.java:371-375.
func (h *Hostile) WeaponGrade() int {
	return int(h.weaponCrystal)
}

func (h *Hostile) SoulshotCharged() bool {
	h.shotsMu.RLock()
	defer h.shotsMu.RUnlock()
	return h.shotsMask&item.ShotSoul.Mask() != 0
}

// SetChargedShot charges or discharges kind on this NPC's shot mask.
func (h *Hostile) SetChargedShot(kind item.ShotKind, charged bool) {
	h.shotsMu.Lock()
	defer h.shotsMu.Unlock()
	if charged {
		h.shotsMask |= kind.Mask()
	} else {
		h.shotsMask &^= kind.Mask()
	}
}

// CurrentSoulshotCount reports the remaining per-spawn soulshot charges.
func (h *Hostile) CurrentSoulshotCount() int {
	h.shotsMu.RLock()
	defer h.shotsMu.RUnlock()
	return h.currentSoulshots
}

// CurrentSpiritshotCount reports the remaining per-spawn spiritshot charges.
func (h *Hostile) CurrentSpiritshotCount() int {
	h.shotsMu.RLock()
	defer h.shotsMu.RUnlock()
	return h.currentSpiritshots
}

// RechargeShots charges the requested NPC shot types once, consuming their
// per-spawn counters and showing the matching animation to observers within
// 600 units.
func (h *Hostile) RechargeShots(physical, magic bool) {
	var skills []int32
	h.shotsMu.Lock()
	if physical && h.currentSoulshots > 0 && h.shotsMask&item.ShotSoul.Mask() == 0 {
		h.currentSoulshots--
		h.shotsMask |= item.ShotSoul.Mask()
		skills = append(skills, 2154)
	}
	if magic && h.currentSpiritshots > 0 && h.shotsMask&item.ShotSpirit.Mask() == 0 {
		h.currentSpiritshots--
		h.shotsMask |= item.ShotSpirit.Mask()
		skills = append(skills, 2061)
	}
	h.shotsMu.Unlock()

	for _, skillID := range skills {
		h.broadcastShotRecharge(skillID)
	}
}

// RollAttackedShotRecharge ports the generic monster AI's onAttacked shot
// roll (MonsterBehavior/WarriorBase/WizardBase.onAttacked in the aCis Java
// reference): on every landed hit, an NPC configured with a nonzero
// SoulShot/SpiritShot AI parameter rolls its matching *Rate parameter
// (percent, [0,100)) and recharges that shot type on success. Callers are
// the same three HP-reduction paths that record attacker hate — TakeDamage,
// ReduceHP, and ReduceHPByDOT — matching Npc.reduceCurrentHp's unconditional
// (isDOT included) addDamageHate-then-onAttacked sequence.
func (h *Hostile) RollAttackedShotRecharge() {
	physical := h.CurrentSoulshotCount() > 0 && h.soulshotRate > 0 && h.Roll(100) < h.soulshotRate
	magic := h.CurrentSpiritshotCount() > 0 && h.spiritshotRate > 0 && h.Roll(100) < h.spiritshotRate
	if physical || magic {
		h.RechargeShots(physical, magic)
	}
}

func (h *Hostile) broadcastShotRecharge(skillID int32) {
	if h.world == nil || h.frames == nil {
		return
	}
	x, y, z := h.Position()
	self := location.Location{X: x, Y: y, Z: z}
	var frame wire.Frame
	built := false
	defer func() { frame.Release() }()
	h.world.ForEachKnownInRadius(h, 600, func(o world.Tracked) {
		receiver, ok := o.(interface{ BroadcastFrame(wire.Frame) bool })
		if ok {
			if !built {
				frame = h.frames.SkillUse(h.ObjectID(), self, h.ObjectID(), self, skillID, 1, 0, 0, false)
				built = true
			}
			owned, copied := wire.CopyFrame(frame)
			if copied {
				receiver.BroadcastFrame(owned)
			}
		}
	})
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

	randomMul := creature.RandomDamageMultiplier(h, modelskill.Definition{})

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
func (h *Hostile) BroadcastAttack(snapshot attack.Snapshot) error {
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	return h.broadcastFrame(func() wire.Frame {
		return h.frames.Attack(snapshot)
	})
}

// BroadcastSkillUse sends a cast-start animation packet from this actor to
// the target at (targetX, targetY, targetZ), to every currently known
// observer capable of receiving one. It is a no-op until SetWorld has been
// called.
func (h *Hostile) BroadcastSkillUse(targetID int32, targetX, targetY, targetZ int, skillID, level int32, hitTime, reuseDelay int) error {
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	sx, sy, sz := h.Position()
	origin := location.Location{X: sx, Y: sy, Z: sz}
	targetAt := location.Location{X: targetX, Y: targetY, Z: targetZ}
	return h.broadcastFrame(func() wire.Frame {
		return h.frames.SkillUse(h.ObjectID(), origin, targetID, targetAt, skillID, level, hitTime, reuseDelay, false)
	})
}

// BroadcastSkillLaunched sends the cast-launch target packet for skillID at
// level, listing targetIDs, to every currently known observer capable of
// receiving one. It is a no-op until SetWorld has been called.
func (h *Hostile) BroadcastSkillLaunched(skillID, level int32, targetIDs []int32) error {
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	return h.broadcastFrame(func() wire.Frame {
		return h.frames.SkillLaunched(h.ObjectID(), skillID, level, targetIDs)
	})
}

// BroadcastSkillCanceled sends the cast-cancel animation packet for
// objectID to every currently known observer capable of receiving one. It
// is a no-op until SetWorld has been called.
func (h *Hostile) BroadcastSkillCanceled(objectID int32) error {
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	return h.broadcastFrame(func() wire.Frame {
		return h.frames.SkillCanceled(objectID)
	})
}

// BroadcastDie sends the death packet to every currently known observer
// capable of receiving one, so clients play the corpse-fall animation
// instead of leaving this NPC standing until its corpse decays. It is a
// no-op until SetWorld has been called.
func (h *Hostile) BroadcastDie() error {
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	return h.broadcastFrame(func() wire.Frame {
		return h.frames.Die(h.ObjectID())
	})
}

// BroadcastMove sends a MoveToLocation packet for event to every currently
// known observer capable of receiving one. It is a no-op until SetWorld has
// been called.
func (h *Hostile) BroadcastMove(event move.Event) error {
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	return h.broadcastFrame(func() wire.Frame { return h.frames.Move(h.ObjectID(), event) })
}

// BroadcastMoveToPawn sends a rotation-only MoveToPawn notice toward target
// to every currently known observer capable of receiving one, matching the
// reference's fallback when an AI-initiated cast is rejected after movement
// has already turned the actor toward target. It is a no-op until SetWorld
// has been called or if target exposes no position.
func (h *Hostile) BroadcastMoveToPawn(target attackable.Combatant) error {
	if h.world == nil {
		return ErrNoWorld
	}
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	located, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return nil
	}
	sx, sy, sz := h.Position()
	origin := location.Location{X: sx, Y: sy, Z: sz}
	tx, ty, tz := located.Position()
	dest := location.Location{X: tx, Y: ty, Z: tz}
	distance := int(origin.Distance3D(dest))

	return h.broadcastFrame(func() wire.Frame {
		return h.frames.MoveToPawn(h.ObjectID(), target.ObjectID(), distance, origin)
	})
}

// BroadcastStop sends a stop-in-place notice to every currently known
// observer capable of receiving one. It is a no-op until SetWorld has been
// called.
func (h *Hostile) BroadcastStop() error {
	if h.world == nil {
		return ErrNoWorld
	}
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	x, y, z := h.Position()
	at := location.Location{X: x, Y: y, Z: z}
	return h.broadcastFrame(func() wire.Frame { return h.frames.Stop(h.ObjectID(), at, h.Heading()) })
}

// BroadcastStatus sends this NPC's current/max HP to every currently known
// observer capable of receiving one, so a target's health bar reflects
// damage as it lands rather than only the moment it dies. It is a no-op
// until SetWorld has been called.
func (h *Hostile) BroadcastStatus() error {
	if h.world == nil {
		return ErrNoWorld
	}
	if h.frames == nil {
		return ErrNoFrameBuilder
	}
	maxHP, curHP := h.MaxHP(), h.CurrentHP()
	return h.broadcastFrame(func() wire.Frame { return h.frames.Status(h.ObjectID(), maxHP, curHP) })
}

func (h *Hostile) broadcastFrame(build func() wire.Frame) error {
	if h.world == nil {
		return ErrNoWorld
	}
	known := append([]world.Tracked(nil), h.appendKnown()...)
	h.releaseKnown()
	var frame wire.Frame
	built := false
	defer func() { frame.Release() }()
	for _, o := range known {
		if receiver, ok := o.(interface{ BroadcastFrame(wire.Frame) bool }); ok {
			if !built {
				frame = build()
				built = true
			}
			owned, copied := wire.CopyFrame(frame)
			if copied {
				receiver.BroadcastFrame(owned)
			}
		}
	}
	return nil
}

func (h *Hostile) appendKnown() []world.Tracked {
	return h.known.Snapshot(h.world, h)
}

func (h *Hostile) releaseKnown() {
	h.known.Release()
}

// AttackableBy reports whether attacker may physically attack this NPC.
func (h *Hostile) AttackableBy(attacker skilltarget.Creature) bool {
	return attacker != nil && attacker.ObjectID() != h.ObjectID() && !h.AlikeDead()
}

// AttackableWithoutForceBy uses the ordinary NPC attackability rule.
func (h *Hostile) AttackableWithoutForceBy(caster skilltarget.Creature) bool {
	return h.AttackableBy(caster)
}
