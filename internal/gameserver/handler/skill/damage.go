package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type shotCharger interface {
	SetChargedShot(modelitem.ShotKind, bool)
}

type chargedShotUser interface {
	shotCharger
	ChargedShot(modelitem.ShotKind) bool
}

// dischargeSoulshot spends the caster's soulshot at the end of a cast. The
// charge flag written is the skill's static-reuse flag, so a static-reuse
// skill leaves the shot marked charged.
func dischargeSoulshot(cast Cast) {
	if caster, ok := cast.Caster.(shotCharger); ok {
		caster.SetChargedShot(modelitem.ShotSoul, cast.Skill.StaticReuse)
	}
}

// dischargeSpiritshot spends the caster's blessed spiritshot when one is
// charged, otherwise its plain spiritshot, writing the static-reuse flag
// the way dischargeSoulshot does.
func dischargeSpiritshot(cast Cast) {
	caster, ok := cast.Caster.(chargedShotUser)
	if !ok {
		return
	}
	kind := modelitem.ShotSpirit
	if caster.ChargedShot(modelitem.ShotBlessedSpirit) {
		kind = modelitem.ShotBlessedSpirit
	}
	caster.SetChargedShot(kind, cast.Skill.StaticReuse)
}

// skillReflected rolls whether obj reflects cast's skill back at the
// caster. Only a creature can reflect.
func skillReflected(cast Cast, obj Actor) bool {
	src, ok := asCreature(obj)
	if !ok {
		return false
	}
	in := src.SkillReflectInput(cast.Skill)
	in.SkillType = skillTypeKey(cast.Skill.SkillType)
	return formulas.SkillReflects(in, rnd.Get(100))
}

// reflectEffectTarget returns the effect-list-owning destination for the
// cast's effects at obj when a reflect only redirects the destination and
// keeps the caster as effector: obj itself, or cast.Caster when obj
// reflects the skill back. Returns nil when obj has no effect list, or when
// reflect fires but the caster doesn't expose one to redirect onto. The
// second return reports whether reflect fired.
func reflectEffectTarget(cast Cast, obj Actor) (effect.Actor, bool) {
	target, ok := obj.(effect.Actor)
	if !ok {
		return nil, false
	}
	if !skillReflected(cast, obj) {
		return target, false
	}
	caster, ok := cast.Caster.(effect.Actor)
	if !ok {
		return nil, true
	}
	return caster, true
}

// landReflected lands a damage skill's effects back on its caster after
// reflector bounced the skill. The participants swap: reflector becomes the
// effects' effector, so it hosts any self-target kind, rolls each
// template's landing chance, and is the side the landing gates consult,
// while the caster is the effected. No shield or blessed-spiritshot outcome
// carries over from the original strike, and a resisted template is
// reported to reflector when it is a player, never to the caster.
func landReflected(cast Cast, reflector effect.Actor) {
	caster, ok := cast.Caster.(effect.Actor)
	if !ok {
		return
	}
	stopEffectsBySkillID(caster.EffectList(), cast.Skill.ID)
	resisted := applyEffectsWithLanding(reflector, caster, cast.Skill, cast.Skill.Effects, formulas.ShieldFailed, false)
	notify, ok := asPlayer(reflector)
	if !ok {
		return
	}
	for range resisted {
		notify.NotifyResistedSkill(actorName(cast.Caster), cast.Skill.ID, cast.Skill.Level)
	}
}

type pdamHandler struct{}

func (pdamHandler) Types() []string { return []string{"PDAM", "FATAL"} }

func (h pdamHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (pdamHandler) UseResult(cast Cast) Result {
	result := Result{messages: cast.messages}
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
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
			result.record(result.Dodges[len(result.Dodges)-1])
			continue
		}
		applyPdamEffects(cast, obj, in.Shield, &result)
		damage := formulas.PhysicalSkillDamage(in)
		if damage > 0 {
			if !applyPhysicalSkillCounter(cast, target, damage, target.CounterSkillPhysical(), false, &result) {
				target.ReduceHP(damage, cast.Caster, cast.Skill)
				recordDamage(&result, cast.Caster, target, int(damage), false, false)
			}
			applyLethalHit(cast, target, &result)
		} else {
			result.AttackFailed++
			result.record(AttackFailedMessage{})
		}
	}
	applySelfEffects(cast, cast.Skill)
	dischargeSoulshot(cast)
	return result
}

type chargeDamHandler struct{}

func (chargeDamHandler) Types() []string { return []string{"CHARGEDAM"} }

func (chargeDamHandler) Use(cast Cast) {
	chargeDamHandler{}.UseResult(cast)
}

func (chargeDamHandler) UseResult(cast Cast) Result {
	result := Result{messages: cast.messages}
	if alikeDead(cast.Caster) {
		return result
	}
	modifier := 0.0
	if caster, ok := asPlayer(cast.Caster); ok {
		modifier = .8 + .2*float64(caster.Charges()+cast.Skill.NumCharges)
	}
	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
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
			result.record(result.Dodges[len(result.Dodges)-1])
			continue
		}
		applyChargeDamEffects(cast, obj, in.Shield, &result)
		damage := formulas.PhysicalSkillDamage(in) * modifier
		if damage <= 0 {
			continue
		}
		if !applyPhysicalSkillCounter(cast, target, damage, target.CounterSkillPhysical(), false, &result) {
			target.ReduceHP(damage, cast.Caster, cast.Skill)
			recordDamage(&result, cast.Caster, target, int(damage), false, false)
		}
	}
	applySelfEffects(cast, cast.Skill)
	dischargeSoulshot(cast)
	return result
}

// applyPdamEffects applies a PDAM/FATAL skill's target effect list to obj:
// a target with an active BLOCK_DEBUFF effect is skipped, a reflecting
// target lands the effects back on the caster as their effector (see
// landReflected), and otherwise the target's prior instance of the same
// skill is dropped first so a repeat cast doesn't stack. Unlike Mdam/Blow,
// PDAM rolls no skill-level landing check; only a perfect shield block of
// the original strike stops the target's effects.
func applyPdamEffects(cast Cast, obj Actor, shield formulas.ShieldDefense, result *Result) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	elt, ok := obj.(effect.Actor)
	if !ok || hasEffectType(elt.EffectList(), "BLOCK_DEBUFF") {
		return
	}
	if skillReflected(cast, obj) {
		landReflected(cast, elt)
		return
	}
	stopEffectsBySkillID(elt.EffectList(), cast.Skill.ID)
	appendResistedCount(result, elt, cast.Skill, applyEffectsWithLanding(cast.Caster, elt, cast.Skill, cast.Skill.Effects, shield, false))
}

// mdamHandler resolves MDAM/DEATHLINK; magicFailures is the server's
// MagicFailures switch.
type mdamHandler struct{ magicFailures bool }

func (mdamHandler) Types() []string { return []string{"MDAM", "DEATHLINK"} }

func (h mdamHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h mdamHandler) UseResult(cast Cast) Result {
	result := Result{messages: cast.messages}
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.MagicDamageInput(cast.Caster, cast.Skill, h.magicFailures)
		if !ok {
			continue
		}
		if in.Shield != formulas.ShieldPerfect {
			reportMagicFailure(cast, target, in.Failure, &result)
		}
		damage := int(formulas.MagicDamage(in))
		if damage > 0 {
			// MDAM reports the damage before applying it, unlike the
			// physical handlers.
			recordDamage(&result, cast.Caster, target, damage, in.MagicCrit, false)
			target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
			applyMdamEffects(cast, obj, in.BlessedSoulShot, in.Shield, &result)
		}
	}
	applySelfEffects(cast, cast.Skill)
	dischargeSpiritshot(cast)
	return result
}

// applyMdamEffects applies an MDAM/DEATHLINK skill's target effect list to
// obj after a successful damage tick: BLOCK_DEBUFF skips it, a reflecting
// target lands it back on the caster as its effector with no skill-level
// landing check (see landReflected), and the non-reflect branch reuses the
// already-resolved shield outcome for the landing roll.
func applyMdamEffects(cast Cast, obj Actor, bss bool, shield formulas.ShieldDefense, result *Result) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	elt, ok := obj.(effect.Actor)
	if !ok || hasEffectType(elt.EffectList(), "BLOCK_DEBUFF") {
		return
	}
	if skillReflected(cast, obj) {
		landReflected(cast, elt)
		return
	}
	stopEffectsBySkillID(elt.EffectList(), cast.Skill.ID)
	succeeded, ok := checkSkillSuccessBSSWithShield(cast.Caster, elt, cast.Skill, bss, shield)
	if !ok {
		return
	}
	if !succeeded {
		appendResisted(result, elt, cast.Skill, 1, true) // id-only addSkillName overload: level 1
		return
	}
	appendResistedCount(result, elt, cast.Skill, applyEffectsWithLanding(cast.Caster, elt, cast.Skill, cast.Skill.Effects, shield, bss))
}

type blowHandler struct{}

func (blowHandler) Types() []string { return []string{"BLOW"} }

func (h blowHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (blowHandler) UseResult(cast Cast) Result {
	result := Result{messages: cast.messages}
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
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
			result.record(result.Dodges[len(result.Dodges)-1])
			continue
		}
		if in.Landed {
			counter := target.CounterSkillPhysical()
			applyBlowEffects(cast, obj, in.Shield, counterSkillReflects(cast.Skill, counter), &result)
			damage := 1
			if in.Shield != formulas.ShieldPerfect {
				damage = int(formulas.BlowDamage(in))
			}
			if in.Crit {
				damage *= 2
			}
			if damage > 0 {
				// A blow always reports itself as a physical critical.
				countered := applyPhysicalSkillCounter(cast, target, float64(damage), counter, true, &result)
				if !countered {
					target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
					recordDamage(&result, cast.Caster, target, damage, false, true)
				}
			}
			dischargeSoulshot(cast)
		}
		// Blow.java rolls the lethal chance unconditionally per target,
		// outside the landing gate — a missed blow can still proc it.
		applyLethalHit(cast, target, &result)
	}
	applySelfEffects(cast, cast.Skill)
	return result
}

// applyPhysicalSkillCounter turns a countered physical skill back onto its
// caster: the counter notices, the caster's HP loss, then the countering
// target's damage feedback, which carries the skill's physical-critical flag.
func applyPhysicalSkillCounter(cast Cast, target Creature, damage, counter float64, pcrit bool, result *Result) bool {
	if !counterSkillReflects(cast.Skill, counter) {
		return false
	}
	if result != nil {
		result.Counterattacks = append(result.Counterattacks, Counterattack{
			AttackerID:   counterattackObjectID(cast.Caster),
			AttackerName: actorName(cast.Caster),
			DefenderID:   counterattackObjectID(target),
			DefenderName: actorName(target),
		})
		result.record(result.Counterattacks[len(result.Counterattacks)-1])
	}
	if cast.Caster != nil {
		damage *= counter / 100
		cast.Caster.ReduceHP(damage, target, cast.Skill)
		recordDamage(result, target, cast.Caster, int(damage), false, pcrit)
	}
	return true
}

type damageSummon interface {
	OwnerID() int32
	IsPet() bool
}

// recordDamage records attacker's damage feedback against target at its
// position among the cast's other messages. Only a player or a summon
// reports damage, and a summon stays silent against its own owner.
func recordDamage(result *Result, attacker, target Actor, amount int, mcrit, pcrit bool) {
	if result == nil || attacker == nil || target == nil {
		return
	}
	m := Damage{Amount: int32(amount), MagicCrit: mcrit, PhysicalCrit: pcrit}
	switch attacker.Kind() {
	case actor.KindPlayer:
		m.RecipientID = attacker.ObjectID()
	case actor.KindSummon:
		s, ok := attacker.(damageSummon)
		if !ok || s.OwnerID() == 0 || s.OwnerID() == target.ObjectID() {
			return
		}
		m.RecipientID, m.Source = s.OwnerID(), DamageByServitor
		if s.IsPet() {
			m.Source = DamageByPet
		}
	default:
		return
	}
	if c, ok := asCreature(target); ok && c.Invul() {
		m.Blocked = true
		m.Petrified = c.Paralyzed()
	}
	result.record(m)
}

func counterattackObjectID(obj Actor) int32 {
	if obj == nil {
		return 0
	}
	return obj.ObjectID()
}

func actorName(obj Actor) string {
	if target, ok := asEffected(obj); ok {
		return target.CharacterName()
	}
	return ""
}

func reportMagicFailure(cast Cast, target Actor, failure formulas.MagicFailure, result *Result) {
	if result == nil || failure == formulas.MagicFailureNone {
		return
	}
	switch failure {
	case formulas.MagicFailureHalf:
		result.AttackFailed++
		result.record(AttackFailedMessage{})
	case formulas.MagicFailureFull:
		// Formulas.java:614 gates this send `attacker instanceof Player` —
		// unlike Mdam/Blow/Manadam's own unconditional skill-level resist —
		// so it never reaches a Summon's owner in the reference.
		appendResisted(result, target, cast.Skill, cast.Skill.Level, false)
	}
	if target.Kind() == actor.KindPlayer {
		result.MagicResists = append(result.MagicResists, MagicResist{
			TargetID:     target.ObjectID(),
			AttackerName: actorName(cast.Caster),
		})
		result.record(result.MagicResists[len(result.MagicResists)-1])
	}
}

// deliverMagicFailure sends the caster/target resist system messages a
// magic-damage failure produces, for paths that do not return a skill
// handler Result (signet ticks).
func deliverMagicFailure(caster Creature, target Actor, def modelskill.Definition, failure formulas.MagicFailure) {
	var result Result
	reportMagicFailure(Cast{Caster: caster, Skill: def}, target, failure, &result)
	if n, ok := asPlayer(caster); ok {
		for i := 0; i < result.AttackFailed; i++ {
			n.NotifyAttackFailed()
		}
	}
	if n, ok := asPlayer(caster); ok {
		for _, r := range result.Resisted {
			n.NotifyResistedSkill(r.TargetName, r.SkillID, r.SkillLevel)
		}
	}
	if n, ok := asPlayer(target); ok {
		for _, r := range result.MagicResists {
			n.NotifyResistedMagic(r.AttackerName)
		}
	}
}

// appendResisted records the caster-facing resisted-your-skill report for
// target. The name is taken from whatever name the target exposes and is
// never gated on being non-empty: every reference call site builds the
// message from the target creature's name unconditionally, and the datapack
// ships nameless targetable monsters (npc ids 27201-27213), so an empty name
// still owes the caster the report.
//
// level is the skill level written into the message: the reference builds it
// from the skill object (the cast level) at most producers, but Mdam adds only
// the skill id, so its message always carries level 1.
func appendResisted(result *Result, target Actor, def modelskill.Definition, level int, unconditional bool) {
	if result == nil {
		return
	}
	result.Resisted = append(result.Resisted, Resisted{TargetName: actorName(target), SkillID: def.ID, SkillLevel: level, Unconditional: unconditional})
	result.record(result.Resisted[len(result.Resisted)-1])
}

// appendResistedCount records count per-effect-template resists produced by
// applyEffectsWithLanding's landing roll — the L2Skill.getEffects generic
// resist, gated to a Player caster in the reference — never the caster's own
// unconditional skill-level resist.
func appendResistedCount(result *Result, target Actor, def modelskill.Definition, count int) {
	for range count {
		appendResisted(result, target, def, def.Level, false)
	}
}

func counterSkillReflects(def modelskill.Definition, counter float64) bool {
	if def.IgnoreResists || !def.CanBeReflected || counter <= 0 {
		return false
	}
	return def.Magic || (def.CastRange != -1 && def.CastRange <= 40)
}

// applyBlowEffects applies a BLOW skill's target effect list to obj after a
// successful hit: only a normal reflection lands it back on the caster as
// its effector (see landReflected); a combined normal-reflect and counter
// outcome keeps effects on the target. Unlike PDAM/MDAM, BLOW never checks
// BLOCK_DEBUFF, and on the target a landing-rate roll gates activation with
// the blessed-spiritshot input forced true — Blow.java hardcodes that
// argument regardless of the caster's real charge state.
func applyBlowEffects(cast Cast, obj Actor, shield formulas.ShieldDefense, countered bool, result *Result) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	elt, ok := obj.(effect.Actor)
	if !ok {
		return
	}
	if skillReflected(cast, obj) && !countered {
		landReflected(cast, elt)
		return
	}
	landRolledDamageEffects(cast, elt, shield, result)
}

// applyChargeDamEffects applies a CHARGEDAM skill's target effect list to
// obj: a reflecting target lands it back on the caster as its effector (see
// landReflected); otherwise the Blow-shaped landing roll gates it.
func applyChargeDamEffects(cast Cast, obj Actor, shield formulas.ShieldDefense, result *Result) {
	if len(cast.Skill.Effects) == 0 {
		return
	}
	elt, ok := obj.(effect.Actor)
	if !ok {
		return
	}
	if skillReflected(cast, obj) {
		landReflected(cast, elt)
		return
	}
	landRolledDamageEffects(cast, elt, shield, result)
}

// landRolledDamageEffects is the unreflected BLOW/CHARGEDAM landing: the
// target's prior instance of the skill is dropped, then a skill-level
// landing roll with the blessed-spiritshot input forced true gates the
// effects, reporting a failure at the cast's skill level.
func landRolledDamageEffects(cast Cast, target effect.Actor, shield formulas.ShieldDefense, result *Result) {
	stopEffectsBySkillID(target.EffectList(), cast.Skill.ID)
	succeeded, ok := checkSkillSuccessBSSWithShield(cast.Caster, target, cast.Skill, true, shield)
	if !ok {
		return
	}
	if !succeeded {
		appendResisted(result, target, cast.Skill, cast.Skill.Level, true)
		return
	}
	appendResistedCount(result, target, cast.Skill, applyEffectsWithLanding(cast.Caster, target, cast.Skill, cast.Skill.Effects, shield, false))
}

type manaDamageHandler struct{}

func (manaDamageHandler) Types() []string { return []string{"MANADAM"} }

func (h manaDamageHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (manaDamageHandler) UseResult(cast Cast) Result {
	result := Result{messages: cast.messages}
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
		if !ok || target.Dead() {
			continue
		}
		var effected effect.Actor
		var effective Actor = target
		if _, ok := obj.(effect.Actor); ok {
			effected, _ = reflectEffectTarget(cast, obj)
			if effected == nil {
				continue
			}
			effective = effected
		}
		target, ok = asCreature(effective)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.ManaDamageInput(cast.Caster, cast.Skill)
		if !ok {
			if target.Invulnerable() || target.Invul() {
				result.ManaDamageMissed++
				result.record(ManaDamageMissedMessage{})
			}
			continue
		}
		if !in.Affected {
			result.ManaDamageMissed++
			result.record(ManaDamageMissedMessage{})
			continue
		}
		if effected != nil && len(cast.Skill.Effects) > 0 {
			stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
			succeeded, ok := checkSkillSuccess(cast.Caster, effected, cast.Skill)
			if ok && succeeded {
				appendResistedCount(&result, effected, cast.Skill, applyEffectsWithLanding(cast.Caster, effected, cast.Skill, cast.Skill.Effects, formulas.ShieldFailed, false))
			} else if ok {
				appendResisted(&result, effected, cast.Skill, cast.Skill.Level, true)
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
		if obj.Kind() == actor.KindPlayer {
			result.ManaDrains = append(result.ManaDrains, ManaDrain{
				TargetID:   obj.ObjectID(),
				CasterName: actorName(cast.Caster),
				MP:         int32(mp),
			})
			result.record(result.ManaDrains[len(result.ManaDrains)-1])
		}
		if cast.Caster != nil && cast.Caster.Kind() == actor.KindPlayer {
			result.OpponentMPReduced = append(result.OpponentMPReduced, int32(mp))
			result.record(OpponentMPReducedMessage{MP: int32(mp)})
		}
		// Manadam.java stops SLEEP/IMMOBILE_UNTIL_ATTACKED once the raw
		// (pre-clamp) damage is positive, after the drain, through the
		// same effect-list removal path stopEffectsBySkillID uses.
		if rawDamage > 0 {
			if elt, ok := effective.(effect.Actor); ok {
				removeMatching(elt.EffectList(), 0, func(e *effect.Effect) bool {
					return e.Type == effect.TypeSleep || e.Type == effect.TypeImmobileUntilAttacked
				})
			}
		}
	}
	applySelfEffects(cast, cast.Skill)
	dischargeSpiritshot(cast)
	return result
}

func applyLethalHit(cast Cast, obj Actor, result *Result) {
	target, ok := asCreature(obj)
	if !ok {
		return
	}
	if target.Invulnerable() || target.Invul() || target.RaidRelated() {
		return
	}
	// Only an NPC limits which lethal strikes may apply to it.
	if n, ok := asNPC(obj); ok && !n.Lethalable() {
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
			result.record(result.Lethals[len(result.Lethals)-1])
		}
	}
}
