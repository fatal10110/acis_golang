package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type hpDamageTarget interface {
	Dead() bool
	ReduceHP(amount float64, attacker any, skill modelskill.Definition)
}

type physicalSkillTarget interface {
	hpDamageTarget
	PhysicalSkillInput(caster any, skill modelskill.Definition) (formulas.PhysicalSkillInput, bool)
}

type magicDamageTarget interface {
	hpDamageTarget
	MagicDamageInput(caster any, skill modelskill.Definition) (formulas.MagicDamageInput, bool)
}

type blowDamageTarget interface {
	hpDamageTarget
	BlowInput(caster any, skill modelskill.Definition) (formulas.BlowInput, bool)
}

type counterSkillPhysicalTarget interface {
	CounterSkillPhysical() float64
}

type objectIDTarget interface {
	ObjectID() int32
}

type manaDamageTarget interface {
	Dead() bool
	MPValue() float64
	ReduceMP(float64) float64
	ManaDamageInput(caster any, skill modelskill.Definition) (formulas.ManaDamageInput, bool)
}

type shotCharger interface {
	SetChargedShot(modelitem.ShotKind, bool)
}

type chargedShotUser interface {
	shotCharger
	ChargedShot(modelitem.ShotKind) bool
}

type lethalTarget interface {
	LethalInput(caster any, skill modelskill.Definition) (formulas.LethalInput, bool)
	ApplyLethalOutcome(formulas.LethalOutcome, any, modelskill.Definition)
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
// reflectTarget's own behavior.
func reflectEffectTarget(cast Cast, obj any) effectListTarget {
	target, ok := obj.(effectListTarget)
	if !ok {
		return nil
	}
	src, ok := obj.(skillReflectSource)
	if !ok {
		return target
	}
	in := src.SkillReflectInput(cast.Skill)
	in.SkillType = skillTypeKey(cast.Skill.SkillType)
	if !formulas.SkillReflects(in, rnd.Get(100)) {
		return target
	}
	caster, ok := cast.Caster.(effectListTarget)
	if !ok {
		return nil
	}
	return caster
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
		applyPdamEffects(cast, obj, in.Shield)
		damage := formulas.PhysicalSkillDamage(in)
		if damage > 0 {
			target.ReduceHP(damage, cast.Caster, cast.Skill)
			applyLethalHit(cast, target)
		} else {
			result.AttackFailed++
		}
	}
	applySelfEffects(cast.Caster, cast.Skill)
	return result
}

// applyPdamEffects applies a PDAM/FATAL skill's target effect list to obj,
// mirroring Pdam.java: a target with an active BLOCK_DEBUFF effect is
// skipped, a reflecting target sends the effects back onto the caster
// instead, and the destination's prior instance of the same skill is
// dropped first so a repeat cast doesn't stack. Unlike Mdam/Blow, PDAM
// applies these unconditionally unless the shared shield outcome is perfect.
func applyPdamEffects(cast Cast, obj any, shield formulas.ShieldDefense) {
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
	effected := reflectEffectTarget(cast, obj)
	if effected == nil {
		return
	}
	stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
	applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)
}

type mdamHandler struct{}

func (mdamHandler) Types() []string { return []string{"MDAM", "DEATHLINK"} }

func (mdamHandler) Use(cast Cast) {
	if alikeDead(cast.Caster) {
		return
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
		damage := int(formulas.MagicDamage(in))
		if damage > 0 {
			target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
			applyMdamEffects(cast, obj)
		}
	}
	applySelfEffects(cast.Caster, cast.Skill)
}

// applyMdamEffects applies an MDAM/DEATHLINK skill's target effect list to
// obj after a successful damage tick, mirroring Mdam.java: BLOCK_DEBUFF
// skips it, reflect redirects it onto the caster, and — matching
// disablers.go's own reflect+landing-roll shape (checkSkillSuccess run
// once against whichever side ends up effected) — a landing-rate roll
// gates activation regardless of which side that is.
func applyMdamEffects(cast Cast, obj any) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	elt, ok := obj.(effectListTarget)
	if !ok || hasEffectType(elt.EffectList(), "BLOCK_DEBUFF") {
		return
	}
	effected := reflectEffectTarget(cast, obj)
	if effected == nil {
		return
	}
	stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
	succeeded, ok := checkSkillSuccess(cast.Caster, effected, cast.Skill)
	if !ok || !succeeded {
		return
	}
	applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)
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
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.BlowInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		if in.Landed {
			damage := 1
			if in.Shield != formulas.ShieldPerfect {
				damage = int(formulas.BlowDamage(in))
			}
			if damage > 0 {
				countered := false
				if source, ok := target.(counterSkillPhysicalTarget); ok {
					counter := source.CounterSkillPhysical()
					countered = counterSkillReflects(cast.Skill, counter)
					if countered {
						if caster, ok := cast.Caster.(hpDamageTarget); ok {
							caster.ReduceHP(float64(damage)*counter/100, target, cast.Skill)
						}
						result.Counterattacks = append(result.Counterattacks, Counterattack{
							AttackerID: counterattackObjectID(cast.Caster),
							DefenderID: counterattackObjectID(target),
						})
					}
				}
				if !countered {
					target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
				}
				applyBlowEffects(cast, obj, in.Shield)
			}
			if caster, ok := cast.Caster.(shotCharger); ok {
				caster.SetChargedShot(modelitem.ShotSoul, cast.Skill.StaticReuse)
			}
		}
		// Blow.java rolls the lethal chance unconditionally per target,
		// outside the landing gate — a missed blow can still proc it.
		applyLethalHit(cast, target)
	}
	applySelfEffects(cast.Caster, cast.Skill)
	return result
}

func counterattackObjectID(obj any) int32 {
	if target, ok := obj.(objectIDTarget); ok {
		return target.ObjectID()
	}
	return 0
}

func counterSkillReflects(def modelskill.Definition, counter float64) bool {
	if def.IgnoreResists || !def.CanBeReflected || counter <= 0 {
		return false
	}
	return def.Magic || (def.CastRange != -1 && def.CastRange <= 40)
}

// applyBlowEffects applies a BLOW skill's target effect list to obj after a
// successful hit, mirroring Blow.java: reflect redirects it onto the
// caster (unlike Pdam/Mdam, Blow never checks BLOCK_DEBUFF), and a
// landing-rate roll gates activation with the blessed-spiritshot input
// forced true — Blow.java hardcodes that argument regardless of the
// caster's real charge state.
func applyBlowEffects(cast Cast, obj any, shield formulas.ShieldDefense) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	effected := reflectEffectTarget(cast, obj)
	if effected == nil {
		return
	}
	stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
	succeeded, ok := checkSkillSuccessBSSWithShield(cast.Caster, effected, cast.Skill, true, shield)
	if !ok || !succeeded {
		return
	}
	applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)
}

type manaDamageHandler struct{}

func (manaDamageHandler) Types() []string { return []string{"MANADAM"} }

func (manaDamageHandler) Use(cast Cast) {
	if alikeDead(cast.Caster) {
		return
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(manaDamageTarget)
		if !ok || target.Dead() {
			continue
		}
		var effected effectListTarget
		effective := any(target)
		if _, ok := obj.(effectListTarget); ok {
			effected = reflectEffectTarget(cast, obj)
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
		if invul, ok := target.(interface{ Invul() bool }); ok && invul.Invul() || !in.Affected {
			continue
		}
		if effected != nil && len(cast.Skill.Effects) > 0 {
			stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
			succeeded, ok := checkSkillSuccess(cast.Caster, effected, cast.Skill)
			if ok && succeeded {
				applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)
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
	applySelfEffects(cast.Caster, cast.Skill)
	if caster, ok := cast.Caster.(chargedShotUser); ok {
		kind := modelitem.ShotSpirit
		if caster.ChargedShot(modelitem.ShotBlessedSpirit) {
			kind = modelitem.ShotBlessedSpirit
		}
		caster.SetChargedShot(kind, cast.Skill.StaticReuse)
	}
}

func applyLethalHit(cast Cast, obj any) {
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
	}
}
