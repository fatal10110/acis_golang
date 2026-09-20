package npc

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// AttackDisabled reports whether this NPC is unable to start an attack. No
// abnormal-effect system (petrify, fear, attack-block) is wired to a live
// NPC yet, so death is the only disabling condition modeled so far.
func (h *Hostile) AttackDisabled() bool {
	return h.Dead()
}

// MovementDisabled reports whether this NPC is unable to move: a
// canMove=false template, or a crowd-control / death / teleport lock.
// Fear is not included; it is an out-of-control state, not a movement lock.
func (h *Hostile) MovementDisabled() bool {
	return !h.Instance.Template.CanMove || h.AlikeDead() || h.Stunned() ||
		h.ImmobileUntilAttacked() || h.Rooted() || h.Sleeping() ||
		h.Paralyzed() || h.Immobilized() || h.Teleporting()
}

// InAttackRange reports whether target sits within this NPC's 2D physical
// attack reach, accounting for both actors' collision footprints and a
// moving-target grace margin. A target with no known position/footprint is
// out of range by definition.
func (h *Hostile) InAttackRange(target attackable.Combatant) bool {
	return attack.InPhysicalRange(h.location(), h.PhysicalAttackRange(), h.CollisionRadius(), target)
}

// LineOfSight is the geodata query CanSee needs to gate targeting on real
// terrain occlusion between two actors.
type LineOfSight interface {
	CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool
}

// CanSee reports whether target is visible to this NPC: a geodata
// line-of-sight query between the two actors' positions and eye heights, or
// permissive when no line-of-sight query is attached (e.g. in tests).
func (h *Hostile) CanSee(target attackable.Combatant) bool {
	if h.los == nil {
		return true
	}
	ox, oy, oz := h.Position()
	tx, ty, tz := target.Position()
	return h.los.CanSeeActor(ox, oy, oz, h.CollisionHeight(), tx, ty, tz, target.CollisionHeight())
}

// CanSeeTarget adapts NPC line-of-sight to the launch revalidation target
// surface.
func (h *Hostile) CanSeeTarget(target skilltarget.Actor) bool {
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
// Attach resolved. Unarmed (WeaponFist) when Attach found no
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
	return int(h.calcStat(stat.PowerAttackSpeed, h.Instance.Template.AtkSpd))
}

// MagicAttackSpeed returns this NPC's magic attack speed stat.
func (h *Hostile) MagicAttackSpeed() int {
	return int(h.calcStat(stat.MagicAttackSpeed, h.Instance.Template.AtkSpd))
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

// ConsumeBowMP spends the right-hand weapon's MP cost at bow fire time.
func (h *Hostile) ConsumeBowMP() {
	if h.weapon == nil || h.weapon.MPConsume <= 0 {
		return
	}
	h.ReduceMP(float64(h.weapon.MPConsume))
}

// WeaponGrade returns this NPC's resolved right-hand weapon's crystal
// grade, resolved by Attach. Zero (CrystalNone) when unarmed. Reference:
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
	x, y, z := h.Position()
	h.emit(event.ShotRecharged{SkillID: skillID, At: location.Location{X: x, Y: y, Z: z}})
}

// SetHeadingTo orients this NPC toward target.
func (h *Hostile) SetHeadingTo(target attackable.Combatant) {
	sx, sy, _ := h.Position()
	tx, ty, _ := target.Position()
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
// formula stats) always misses.
func (h *Hostile) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	hit := attack.Hit{Target: target, TargetID: target.ObjectID()}

	// attack.Actor, the world known-list and Hit all carry the
	// attackable.Combatant leaf surface, which cannot name a weapon type
	// without importing model/item. The narrowing stays here, at the one
	// point where a combatant becomes a formula operand; see #2362.
	other, ok := target.(creature.FormulaActor)
	if !ok {
		hit.Miss = true
		return hit
	}

	tpl := h.Instance.Template
	accuracy := int(h.calcStat(stat.AccuracyCombat, 0))
	evasion := other.Evasion()

	_, _, sz := h.Position()
	_, _, tz := other.Position()
	behind, inFront := creature.AttackFacing(other, h)
	rate := formulas.HitRate(accuracy, evasion, sz-tz, creature.Night(), behind, inFront)
	if formulas.Missed(rate, h.roll(1000)) {
		hit.Miss = true
		return hit
	}

	critRate := float64(min(int(h.calcStat(stat.CriticalRate, tpl.CritRate)), 500))
	crit := formulas.CritSucceeds(critRate, h.roll(1000))
	in, shield := creature.ResolvePhysicalAttackInput(h, other, crit)
	hit.Damage = creature.ApplyPhysicalAttackDamage(in, shield, split)
	hit.Crit = crit
	hit.Shield = shield
	return hit
}

// BroadcastAttack reports one resolved attack swing.
func (h *Hostile) BroadcastAttack(snapshot event.Attack) error {
	h.emit(snapshot)
	return nil
}

// BroadcastSkillUse reports a cast-start animation from this actor to the
// target at (targetX, targetY, targetZ).
func (h *Hostile) BroadcastSkillUse(targetID int32, targetX, targetY, targetZ int, skillID, level int32, hitTime, reuseDelay int) error {
	sx, sy, sz := h.Position()
	h.emit(event.MagicSkillUse{
		CasterID: h.ObjectID(), CasterAt: location.Location{X: sx, Y: sy, Z: sz},
		TargetID: targetID, TargetAt: location.Location{X: targetX, Y: targetY, Z: targetZ},
		SkillID: skillID, Level: level, HitTime: hitTime, ReuseDelay: reuseDelay,
	})
	return nil
}

// BroadcastSkillLaunched reports the cast launch of skillID at level onto
// targetIDs.
func (h *Hostile) BroadcastSkillLaunched(skillID, level int32, targetIDs []int32) error {
	h.emit(event.SkillLaunched{SkillID: skillID, Level: level, TargetIDs: targetIDs})
	return nil
}

// BroadcastSkillCanceled reports the cast-cancel animation for objectID.
func (h *Hostile) BroadcastSkillCanceled(objectID int32) error {
	h.emit(event.SkillCanceled{ObjectID: objectID})
	return nil
}

// BroadcastDie reports this NPC's death, so clients play the corpse-fall
// animation instead of leaving it standing until its corpse decays.
func (h *Hostile) BroadcastDie() error {
	h.emit(event.Died{})
	return nil
}

// BroadcastMove reports a server-driven movement start.
func (h *Hostile) BroadcastMove(ev event.Move) error {
	h.emit(ev)
	return nil
}

// BroadcastMoveToPawn reports a rotation-only MoveToPawn notice toward
// target, matching the reference's fallback when an AI-initiated cast is
// rejected after movement has already turned the actor toward target.
func (h *Hostile) BroadcastMoveToPawn(target attackable.Combatant) error {
	sx, sy, sz := h.Position()
	origin := location.Location{X: sx, Y: sy, Z: sz}
	tx, ty, tz := target.Position()
	dest := location.Location{X: tx, Y: ty, Z: tz}
	h.emit(event.MoveToPawn{TargetID: target.ObjectID(), Distance: int(origin.Distance3D(dest)), Origin: origin})
	return nil
}

// BroadcastStop reports a stop in place.
func (h *Hostile) BroadcastStop() error {
	h.emit(event.Stopped{})
	return nil
}

// BroadcastStatus reports this NPC's current/max HP, so a target's health
// bar reflects damage as it lands rather than only the moment it dies.
func (h *Hostile) BroadcastStatus() error {
	h.emit(event.Status{Attrs: []event.StatusAttr{{Kind: event.StatusMaxHP, Value: h.MaxHP()}, {Kind: event.StatusCurrentHP, Value: h.CurrentHP()}}})
	return nil
}

// AttackableBy reports whether attacker may physically attack this NPC.
func (h *Hostile) AttackableBy(attacker skilltarget.Actor) bool {
	return attacker != nil && attacker.ObjectID() != h.ObjectID() && !h.AlikeDead()
}

// AttackableWithoutForceBy uses the ordinary NPC attackability rule.
func (h *Hostile) AttackableWithoutForceBy(caster skilltarget.Actor) bool {
	return h.AttackableBy(caster)
}
