package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type hpDamageTarget interface {
	Actor
	ReduceHP(amount float64, attacker creature.DeathActor, skill modelskill.Definition)
}

type physicalSkillTarget interface {
	hpDamageTarget
	PhysicalSkillInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.PhysicalSkillInput, bool)
}

type magicDamageTarget interface {
	hpDamageTarget
	MagicDamageInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.MagicDamageInput, bool)
}

type blowDamageTarget interface {
	hpDamageTarget
	BlowInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.BlowInput, bool)
}

type counterSkillPhysicalTarget interface {
	CounterSkillPhysical() float64
}

type chargeDamageCaster interface {
	Charges() int
}

type characterNameTarget interface {
	CharacterName() string
}

type manaDamageTarget interface {
	Actor
	MPValue() float64
	ReduceMP(float64) float64
	ManaDamageInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.ManaDamageInput, bool)
}

type shotCharger interface {
	SetChargedShot(modelitem.ShotKind, bool)
}

type chargedShotUser interface {
	shotCharger
	ChargedShot(modelitem.ShotKind) bool
}

type lethalTarget interface {
	LethalInput(caster creature.DeathActor, skill modelskill.Definition) (formulas.LethalInput, bool)
	ApplyLethalOutcome(formulas.LethalOutcome, creature.DeathActor, modelskill.Definition)
}

type lethalInvulnerableTarget interface {
	Invulnerable() bool
}

type lethalInvulTarget interface {
	Invul() bool
}

type lethalableTarget interface {
	Lethalable() bool
}

// reflectEffectTarget returns the effect-list-owning destination for def's
// effects cast at obj: obj itself, or cast.Caster when obj reflects the
// skill back — the rule disablers.go's reflectTarget applies, generalized
// to whatever satisfies effectListTarget rather than the fuller
// disablerTarget surface (Dead/Invul/Paralyzed) a damage handler's own
// Dead()/evasion checks already cover before this runs. Returns nil when
// reflect fires but the caster doesn't expose an effect list to redirect
// onto — a duck-typing gap safer to drop than to guess through, matching
// reflectTarget's own behavior. The second return reports whether reflect
// fired.
func reflectEffectTarget(cast Cast, obj Actor) (effectListTarget, bool) {
	target, ok := obj.(effectListTarget)
	if !ok {
		return nil, false
	}
	src, ok := obj.(skillReflectSource)
	if !ok {
		return target, false
	}
	in := src.SkillReflectInput(cast.Skill)
	in.SkillType = skillTypeKey(cast.Skill.SkillType)
	if !formulas.SkillReflects(in, rnd.Get(100)) {
		return target, false
	}
	caster, ok := cast.Caster.(effectListTarget)
	if !ok {
		return nil, true
	}
	return caster, true
}

type pdamHandler struct{}

func (pdamHandler) Types() []string { return []string{"PDAM", "FATAL"} }

func (h pdamHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (pdamHandler) UseResult(cast Cast) Result {
	var result Result
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(physicalSkillTarget)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.PhysicalSkillInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		if in.Evaded {
			result.Dodges = append(result.Dodges, Dodge{
				AttackerID: counterattackObjectID(cast.Caster), AttackerName: actorName(cast.Caster),
				DefenderID: counterattackObjectID(target), DefenderName: actorName(target),
			})
			continue
		}
		applyPdamEffects(cast, obj, in.Shield, &result)
		damage := formulas.PhysicalSkillDamage(in)
		if damage > 0 {
			if !applyPhysicalSkillCounter(cast, target, damage, &result) {
				target.ReduceHP(damage, cast.Caster, cast.Skill)
			}
			applyLethalHit(cast, target, &result)
		} else {
			result.AttackFailed++
		}
	}
	applySelfEffects(cast, cast.Skill)
	if caster, ok := cast.Caster.(shotCharger); ok {
		caster.SetChargedShot(modelitem.ShotSoul, cast.Skill.StaticReuse)
	}
	return result
}

type chargeDamHandler struct{}

func (chargeDamHandler) Types() []string { return []string{"CHARGEDAM"} }

func (chargeDamHandler) Use(cast Cast) {
	chargeDamHandler{}.UseResult(cast)
}

func (chargeDamHandler) UseResult(cast Cast) Result {
	var result Result
	if alikeDead(cast.Caster) {
		return result
	}
	modifier := 0.0
	if caster, ok := cast.Caster.(chargeDamageCaster); ok {
		modifier = .8 + .2*float64(caster.Charges()+cast.Skill.NumCharges)
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(physicalSkillTarget)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.PhysicalSkillInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		applyChargeDamEffects(cast, obj, in.Shield, &result)
		damage := formulas.PhysicalSkillDamage(in) * modifier
		if damage <= 0 {
			continue
		}
		if !applyPhysicalSkillCounter(cast, target, damage, &result) {
			target.ReduceHP(damage, cast.Caster, cast.Skill)
		}
	}
	applySelfEffects(cast, cast.Skill)
	return result
}

// applyPdamEffects applies a PDAM/FATAL skill's target effect list to obj,
// mirroring Pdam.java: a target with an active BLOCK_DEBUFF effect is
// skipped, a reflecting target sends the effects back onto the caster
// instead, and the destination's prior instance of the same skill is
// dropped first so a repeat cast doesn't stack. Unlike Mdam/Blow, PDAM
// applies these unconditionally unless the shared shield outcome is perfect.
func applyPdamEffects(cast Cast, obj Actor, shield formulas.ShieldDefense, result *Result) {
	if shield == formulas.ShieldPerfect {
		return
	}
	if len(cast.Skill.Effects) == 0 {
		return
	}
	elt, ok := obj.(effectListTarget)
	if !ok || hasEffectType(elt.EffectList(), "BLOCK_DEBUFF") {
		return
	}
	effected, _ := reflectEffectTarget(cast, obj)
	if effected == nil {
		return
	}
	stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
	appendResistedCount(result, effected, cast.Skill, applyEffectsWithLanding(cast.Caster, effected, cast.Skill, cast.Skill.Effects, shield, false))
}

type mdamHandler struct{}

func (mdamHandler) Types() []string { return []string{"MDAM", "DEATHLINK"} }

func (h mdamHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (mdamHandler) UseResult(cast Cast) Result {
	var result Result
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(magicDamageTarget)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.MagicDamageInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		if in.Shield != formulas.ShieldPerfect {
			reportMagicFailure(cast, target, in.Failure, &result)
		}
		damage := int(formulas.MagicDamage(in))
		if damage > 0 {
			target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
			applyMdamEffects(cast, obj, in.BlessedSoulShot, in.Shield, &result)
		}
	}
	applySelfEffects(cast, cast.Skill)
	if caster, ok := cast.Caster.(chargedShotUser); ok {
		kind := modelitem.ShotSpirit
		if caster.ChargedShot(modelitem.ShotBlessedSpirit) {
			kind = modelitem.ShotBlessedSpirit
		}
		caster.SetChargedShot(kind, cast.Skill.StaticReuse)
	}
	return result
}

// applyMdamEffects applies an MDAM/DEATHLINK skill's target effect list to
// obj after a successful damage tick: BLOCK_DEBUFF skips it, reflect
// redirects it onto the caster, and the non-reflect branch reuses the
// already-resolved shield outcome for the landing roll. The reflect
// branch applies effects unconditionally with no landing check.
func applyMdamEffects(cast Cast, obj Actor, bss bool, shield formulas.ShieldDefense, result *Result) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	elt, ok := obj.(effectListTarget)
	if !ok || hasEffectType(elt.EffectList(), "BLOCK_DEBUFF") {
		return
	}
	effected, reflected := reflectEffectTarget(cast, obj)
	if effected == nil {
		return
	}
	stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
	effectShield := formulas.ShieldFailed
	if reflected {
		bss = false
	} else {
		effectShield = shield
		succeeded, ok := checkSkillSuccessBSSWithShield(cast.Caster, effected, cast.Skill, bss, shield)
		if !ok {
			return
		}
		if !succeeded {
			appendResisted(result, effected, cast.Skill)
			return
		}
	}
	appendResistedCount(result, effected, cast.Skill, applyEffectsWithLanding(cast.Caster, effected, cast.Skill, cast.Skill.Effects, effectShield, bss))
}

type blowHandler struct{}

func (blowHandler) Types() []string { return []string{"BLOW"} }

func (h blowHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (blowHandler) UseResult(cast Cast) Result {
	var result Result
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(blowDamageTarget)
		if !ok || alikeDead(target) {
			continue
		}
		in, ok := target.BlowInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		if in.Evaded {
			result.Dodges = append(result.Dodges, Dodge{
				AttackerID:   counterattackObjectID(cast.Caster),
				AttackerName: actorName(cast.Caster),
				DefenderID:   counterattackObjectID(target),
				DefenderName: actorName(target),
			})
			continue
		}
		if in.Landed {
			damage := 1
			if in.Shield != formulas.ShieldPerfect {
				damage = int(formulas.BlowDamage(in))
			}
			if in.Crit {
				damage *= 2
			}
			if damage > 0 {
				countered := applyPhysicalSkillCounter(cast, target, float64(damage), &result)
				if !countered {
					target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
				}
				applyBlowEffects(cast, obj, in.Shield, countered, &result)
			}
			if caster, ok := cast.Caster.(shotCharger); ok {
				caster.SetChargedShot(modelitem.ShotSoul, cast.Skill.StaticReuse)
			}
		}
		// Blow.java rolls the lethal chance unconditionally per target,
		// outside the landing gate — a missed blow can still proc it.
		applyLethalHit(cast, target, &result)
	}
	applySelfEffects(cast, cast.Skill)
	return result
}

func applyPhysicalSkillCounter(cast Cast, target hpDamageTarget, damage float64, result *Result) bool {
	source, ok := target.(counterSkillPhysicalTarget)
	if !ok {
		return false
	}
	counter := source.CounterSkillPhysical()
	if !counterSkillReflects(cast.Skill, counter) {
		return false
	}
	if caster, ok := cast.Caster.(hpDamageTarget); ok {
		caster.ReduceHP(damage*counter/100, target, cast.Skill)
	}
	if result != nil {
		result.Counterattacks = append(result.Counterattacks, Counterattack{
			AttackerID:   counterattackObjectID(cast.Caster),
			AttackerName: actorName(cast.Caster),
			DefenderID:   counterattackObjectID(target),
			DefenderName: actorName(target),
		})
	}
	return true
}

func counterattackObjectID(obj Actor) int32 {
	if obj == nil {
		return 0
	}
	return obj.ObjectID()
}

func actorName(obj Actor) string {
	if target, ok := obj.(characterNameTarget); ok {
		return target.CharacterName()
	}
	return ""
}

type worldPlayerTarget interface {
	WorldPlayer()
}

func reportMagicFailure(cast Cast, target Actor, failure formulas.MagicFailure, result *Result) {
	if result == nil || failure == formulas.MagicFailureNone {
		return
	}
	switch failure {
	case formulas.MagicFailureHalf:
		result.AttackFailed++
	case formulas.MagicFailureFull:
		appendResisted(result, target, cast.Skill)
	}
	if _, ok := target.(worldPlayerTarget); ok {
		result.MagicResists = append(result.MagicResists, MagicResist{
			TargetID:     target.ObjectID(),
			AttackerName: actorName(cast.Caster),
		})
	}
}

type attackFailedNotifier interface {
	NotifyAttackFailed()
}

type resistedSkillNotifier interface {
	NotifyResistedSkill(targetName string, skillID modelskill.ID, level int)
}

type resistedMagicNotifier interface {
	NotifyResistedMagic(attackerName string)
}

// deliverMagicFailure sends the caster/target resist system messages a
// magic-damage failure produces, for paths that do not return a skill
// handler Result (signet ticks).
func deliverMagicFailure(caster, target Actor, def modelskill.Definition, failure formulas.MagicFailure) {
	var result Result
	reportMagicFailure(Cast{Caster: caster, Skill: def}, target, failure, &result)
	if n, ok := caster.(attackFailedNotifier); ok {
		for i := 0; i < result.AttackFailed; i++ {
			n.NotifyAttackFailed()
		}
	}
	if n, ok := caster.(resistedSkillNotifier); ok {
		for _, r := range result.Resisted {
			n.NotifyResistedSkill(r.TargetName, r.SkillID, r.SkillLevel)
		}
	}
	if n, ok := target.(resistedMagicNotifier); ok {
		for _, r := range result.MagicResists {
			n.NotifyResistedMagic(r.AttackerName)
		}
	}
}

func appendResisted(result *Result, target Actor, def modelskill.Definition) {
	if result == nil {
		return
	}
	if name := actorName(target); name != "" {
		result.Resisted = append(result.Resisted, Resisted{TargetName: name, SkillID: def.ID, SkillLevel: def.Level})
	}
}

func appendResistedCount(result *Result, target Actor, def modelskill.Definition, count int) {
	for range count {
		appendResisted(result, target, def)
	}
}

func counterSkillReflects(def modelskill.Definition, counter float64) bool {
	if def.IgnoreResists || !def.CanBeReflected || counter <= 0 {
		return false
	}
	return def.Magic || (def.CastRange != -1 && def.CastRange <= 40)
}

// applyBlowEffects applies a BLOW skill's target effect list to obj after a
// successful hit: only a normal reflection redirects it onto the caster;
// a combined normal-reflect and counter outcome keeps effects on the target.
// Unlike PDAM/MDAM, BLOW never checks BLOCK_DEBUFF, and a
// landing-rate roll gates activation with the blessed-spiritshot input
// forced true — Blow.java hardcodes that argument regardless of the
// caster's real charge state.
func applyBlowEffects(cast Cast, obj Actor, shield formulas.ShieldDefense, countered bool, result *Result) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	effected, reflected := reflectEffectTarget(cast, obj)
	if countered && reflected {
		effected, _ = obj.(effectListTarget)
	}
	if effected == nil {
		return
	}
	stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
	succeeded, ok := checkSkillSuccessBSSWithShield(cast.Caster, effected, cast.Skill, true, shield)
	if !ok {
		return
	}
	if !succeeded {
		appendResisted(result, effected, cast.Skill)
		return
	}
	appendResistedCount(result, effected, cast.Skill, applyEffectsWithLanding(cast.Caster, effected, cast.Skill, cast.Skill.Effects, shield, false))
}

func applyChargeDamEffects(cast Cast, obj Actor, shield formulas.ShieldDefense, result *Result) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	effected, reflected := reflectEffectTarget(cast, obj)
	if effected == nil {
		return
	}
	stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
	if !reflected {
		succeeded, ok := checkSkillSuccessBSSWithShield(cast.Caster, effected, cast.Skill, true, shield)
		if !ok {
			return
		}
		if !succeeded {
			appendResisted(result, effected, cast.Skill)
			return
		}
	}
	appendResistedCount(result, effected, cast.Skill, applyEffectsWithLanding(cast.Caster, effected, cast.Skill, cast.Skill.Effects, shield, false))
}

type manaDamageHandler struct{}

func (manaDamageHandler) Types() []string { return []string{"MANADAM"} }

func (h manaDamageHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (manaDamageHandler) UseResult(cast Cast) Result {
	var result Result
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(manaDamageTarget)
		if !ok || target.Dead() {
			continue
		}
		var effected effectListTarget
		var effective Actor = target
		if _, ok := obj.(effectListTarget); ok {
			effected, _ = reflectEffectTarget(cast, obj)
			if effected == nil {
				continue
			}
			effective = effected
		}
		target, ok = effective.(manaDamageTarget)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.ManaDamageInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		if invul, ok := target.(lethalInvulTarget); ok && invul.Invul() || !in.Affected {
			continue
		}
		if effected != nil && len(cast.Skill.Effects) > 0 {
			stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
			succeeded, ok := checkSkillSuccess(cast.Caster, effected, cast.Skill)
			if ok && succeeded {
				appendResistedCount(&result, effected, cast.Skill, applyEffectsWithLanding(cast.Caster, effected, cast.Skill, cast.Skill.Effects, formulas.ShieldFailed, false))
			} else if ok {
				appendResisted(&result, effected, cast.Skill)
			}
		}
		rawDamage := formulas.ManaDamage(in)
		mp := rawDamage
		if mp > target.MPValue() {
			mp = target.MPValue()
		}
		if mp > 0 {
			target.ReduceMP(mp)
		}
		// Manadam.java stops SLEEP/IMMOBILE_UNTIL_ATTACKED once the raw
		// (pre-clamp) damage is positive, after the drain. No production
		// actor implements a StopEffects(Type) method, so this goes
		// through the same effect-list removal path stopEffectsBySkillID
		// uses rather than a type assertion that only test fakes satisfy.
		if rawDamage > 0 {
			if elt, ok := effective.(effectListTarget); ok {
				removeMatching(elt.EffectList(), 0, func(e *effect.Effect) bool {
					return e.Type == effect.TypeSleep || e.Type == effect.TypeImmobileUntilAttacked
				})
			}
		}
	}
	applySelfEffects(cast, cast.Skill)
	if caster, ok := cast.Caster.(chargedShotUser); ok {
		kind := modelitem.ShotSpirit
		if caster.ChargedShot(modelitem.ShotBlessedSpirit) {
			kind = modelitem.ShotBlessedSpirit
		}
		caster.SetChargedShot(kind, cast.Skill.StaticReuse)
	}
	return result
}

func applyLethalHit(cast Cast, obj Actor, result *Result) {
	target, ok := obj.(lethalTarget)
	if !ok {
		return
	}
	if v, ok := obj.(lethalInvulnerableTarget); ok && v.Invulnerable() {
		return
	}
	if v, ok := obj.(lethalInvulTarget); ok && v.Invul() {
		return
	}
	if v, ok := obj.(raidRelatedTarget); ok && v.RaidRelated() {
		return
	}
	if v, ok := obj.(lethalableTarget); ok && !v.Lethalable() {
		return
	}
	in, ok := target.LethalInput(cast.Caster, cast.Skill)
	if !ok {
		return
	}
	outcome := formulas.LethalHit(in, rnd.Get)
	if outcome != formulas.LethalNone {
		target.ApplyLethalOutcome(outcome, cast.Caster, cast.Skill)
		if result != nil {
			result.Lethals = append(result.Lethals, Lethal{
				AttackerID: counterattackObjectID(cast.Caster),
				TargetID:   counterattackObjectID(obj),
			})
		}
	}
}
