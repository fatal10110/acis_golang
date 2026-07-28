package effect

import (
	"fmt"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Flag is the bitmask exposed by an active effect to live actor state.
type Flag uint32

const (
	// FlagNone is the default effect flag mask.
	FlagNone Flag = 1 << iota
	flagCharmOfCourage
	// FlagCharmOfLuck marks a target as carrying Charm of Luck, consulted by
	// systems that exempt a lucky death from raising its own penalties.
	FlagCharmOfLuck
	// FlagPhoenixBlessing marks a target as carrying Phoenix Blessing,
	// consulted by systems that exempt a blessed death from its penalties.
	FlagPhoenixBlessing
	flagNoblesseBlessing
	// FlagSilentMove marks a target as moving without alerting nearby AI.
	FlagSilentMove
	flagProtectionBlessing
	flagRelaxing
	// FlagFear marks a target as feared.
	FlagFear
	// FlagConfused marks a target as confused.
	FlagConfused
	// FlagBetrayed marks a summon as temporarily hostile to its owner.
	FlagBetrayed
	flagMuted
	flagPhysicalMuted
	// FlagRooted marks a target as rooted.
	FlagRooted
	// FlagSleep marks a target as asleep.
	FlagSleep
	// FlagStunned marks a target as stunned.
	FlagStunned
	// FlagParalyzed marks a target as paralyzed.
	FlagParalyzed
	// FlagMeditating marks a target as immobile until it is next attacked.
	FlagMeditating
	// flagBigHead marks a target as carrying the big-head cosmetic buff.
	flagBigHead
	// FlagFakeDeath marks a target as playing dead.
	FlagFakeDeath
)

// TypeManaDamOverTime is a periodic MP-drain effect: a toggle skill's
// upkeep tick, or a plain continuous mana-drain buff. Declared here rather
// than alongside the other Type constants so this file's additions stay
// out of the effect list's stacking logic.
const TypeManaDamOverTime Type = "MANA_DMG_OVER_TIME"

// Type values for the additional effect kinds wired below. Each names the
// runtime behavior it drives, not the datapack classification a skill
// carries (several of these share a datapack classification with a plain
// buff but need distinct hook wiring here).
const (
	// TypeAbortCast interrupts the target's current cast, if any.
	TypeAbortCast Type = "ABORT_CAST"
	// TypeImmobileUntilAttacked locks the target in place until the effect
	// ends or is stopped early.
	TypeImmobileUntilAttacked Type = "IMMOBILE_UNTIL_ATTACKED"
	// TypeImmobilizeEffector locks the caster in place for the duration.
	TypeImmobilizeEffector Type = "IMMOBILIZE_EFFECTOR"
	// TypeInvincible grants the target damage invulnerability.
	TypeInvincible Type = "INVINCIBLE"
	// TypeManaHealOverTime is a periodic MP restore effect.
	TypeManaHealOverTime Type = "MANA_HEAL_OVER_TIME"
	// TypeMute blocks the target from casting magic skills.
	TypeMute Type = "MUTE"
	// TypeNoblesseBless is a marker buff consulted by revive handling.
	TypeNoblesseBless Type = "NOBLESSE_BLESSING"
	// TypeParalyze locks the target in place and aborts its current action.
	TypeParalyze Type = "PARALYZE"
	// TypePetrification locks and invulns the target for the duration.
	TypePetrification Type = "PETRIFICATION"
	// TypePhysicalMute blocks the target from using physical skills.
	TypePhysicalMute Type = "PHYSICAL_MUTE"
	// TypeRemoveTarget clears the target's current target and stops any
	// attack or cast against it.
	TypeRemoveTarget Type = "REMOVE_TARGET"
	// TypeSilenceAll blocks the target from casting any skill, magic or
	// physical.
	TypeSilenceAll Type = "SILENCE_MAGIC_PHYSICAL"
	// TypeSilentMove is a periodic MP-consuming stealth movement buff.
	TypeSilentMove Type = "SILENT_MOVE"
	// TypeStunSelf idles the target and refreshes the caster's own status.
	TypeStunSelf Type = "STUN_SELF"
	// TypeHeal restores HP once when the effect starts.
	TypeHeal Type = "HEAL"
	// TypeHealOverTime restores HP on each periodic tick.
	TypeHealOverTime Type = "HEAL_OVER_TIME"
	// TypeManaHeal restores MP once when the effect starts.
	TypeManaHeal Type = "MANA_HEAL"
	// TypeIncreaseCharges adds Force/Soul charges once when the effect
	// starts, up to the effect template's count cap.
	TypeIncreaseCharges Type = "INCREASE_CHARGES"
	// TypeTargetMe redirects the target's current target onto the
	// effector, or turns an existing lock onto the effector into an attack.
	TypeTargetMe Type = "TARGET_ME"
	// TypeBluff redirects the target's facing onto the effector's, unless
	// the target is exempt from facing-redirect effects.
	TypeBluff Type = "BLUFF"
	// TypeCharmOfCourage is a marker buff limited to players; other actors
	// reject it outright.
	TypeCharmOfCourage Type = "CHARM_OF_COURAGE"
	// TypeCharmOfLuck is a marker buff consulted by whatever system reacts
	// to it ending.
	TypeCharmOfLuck Type = "CHARM_OF_LUCK"
	// TypePhoenixBless is a marker buff consulted by whatever system reacts
	// to it ending.
	TypePhoenixBless Type = "PHOENIX_BLESSING"
	// TypeBlockBuff is a marker buff that makes its owner reject incoming
	// buff effects for its duration.
	TypeBlockBuff Type = "BLOCK_BUFF"
	// TypeBlockDebuff is a marker buff that makes its owner reject incoming
	// debuff effects for its duration.
	TypeBlockDebuff Type = "BLOCK_DEBUFF"
	// TypeProtectionBless is a marker buff (player-kill protection) a cancel
	// skill can never strip.
	TypeProtectionBless Type = "PROTECTION_BLESSING"
	// TypeCancel strips a random subset of the effected actor's active
	// non-toggle, non-debuff effects.
	TypeCancel Type = "CANCEL"
	// TypeNegate strips every effect owned by a configured skill id, plus
	// every effect whose classification and abnormal level fall under a
	// configured skill-type/level threshold.
	TypeNegate Type = "NEGATE"
	// TypeFusion links a skill's applied level to a scalable force effect:
	// IncreaseEffect/DecreaseForce can grow or shrink it while it's active,
	// instead of it only ever starting or ending outright.
	TypeFusion Type = "FUSION"
	// TypeChanceSkillTrigger installs a live chance-to-trigger-another-skill
	// condition on its target for as long as the effect is active.
	TypeChanceSkillTrigger Type = "CHANCE_SKILL_TRIGGER"
	// TypeSpoil rolls a magic-resist check against a monster target and
	// marks it spoiled on success.
	TypeSpoil Type = "SPOIL"
	// TypePolearmTargetSingle is a classification marker consulted by
	// weapon-target-count logic elsewhere; it carries no behavior of its
	// own.
	TypePolearmTargetSingle Type = "POLEARM_TARGET_SINGLE"
	// TypeBigHead is a cosmetic marker buff.
	TypeBigHead Type = "BIG_HEAD"
	// TypeCancelDebuff strips a capped, independently-rolled selection of a
	// player target's active dispellable debuffs.
	TypeCancelDebuff Type = "CANCEL_DEBUFF"
	// TypeRelax sits its target down and periodically drains MP while it
	// stays seated.
	TypeRelax Type = "RELAX"
	// TypeChameleonRest sits its target down and periodically drains MP
	// while a continuous cast keeps it seated.
	TypeChameleonRest Type = "CHAMELEON_REST"
	// TypeImmobilizePetBuff locks the effected summon in place for the
	// duration, gated on the caster being that summon's own owner.
	TypeImmobilizePetBuff Type = "IMMOBILIZE_PET_BUFF"
	// TypeDistrust turns a Monster-family target's aggression toward
	// another nearby monster.
	TypeDistrust Type = "DISTRUST"
	// TypeConfusion aborts its non-player target's current action and
	// redirects its aggression toward a random nearby combatant.
	TypeConfusion Type = "CONFUSION"
	// TypeBetray turns an effected summon against its owner for the effect
	// duration.
	TypeBetray Type = "BETRAY"
	// TypeRandomizeHate swaps a random valid attacker into its target's
	// most-hated slot, ahead of the current top-hate attacker.
	TypeRandomizeHate Type = "RANDOMIZE_HATE"
	// TypeThrowUp is a knockback: it stuns the target for the effect's
	// duration, computes a geo-corrected landing point once at start, and
	// teleports the target there at effect end.
	TypeThrowUp Type = "THROW_UP"
	// TypeGrow scales an Npc-shaped target's runtime collision radius for
	// the effect's duration, restoring it on exit.
	TypeGrow Type = "GROW"
	// TypeFakeDeath is the Fake Death toggle: it drains MP each tick while
	// active and marks its target as playing dead.
	TypeFakeDeath Type = "FAKE_DEATH"
	// TypeSeed is an elemental-seed charge: a pure counter (Level tracks its
	// power, starting at the skill's own level) with no start/exit/tick
	// behavior of its own. Recasting the same seed skill on an already-seeded
	// target grows the existing instance's power in place via IncreasePower
	// instead of replacing it.
	TypeSeed Type = "SEED"
	// TypeRecovery lowers a player target's death-penalty debuff level by
	// one on start; a non-player target rejects it.
	TypeRecovery Type = "RECOVERY"
)

type kind struct {
	typ    Type
	flag   Flag
	debuff bool
	// rejectsIfAffected marks a kind that refuses to be added at all (its
	// stop-task hook fires instead) when the owner is already affected by
	// its own Flag bit, from any currently held effect that carries it —
	// not just another instance of the same kind. Left false (the default
	// for every kind but these four) it never blocks; those four never
	// merge with or replace an existing same-flag effect, they simply
	// don't apply while one is live.
	rejectsIfAffected bool
}

var coreKinds = map[string]kind{
	"Buff":                  {typ: TypeBuff},
	"Debuff":                {typ: TypeDebuff, debuff: true},
	"DamOverTime":           {typ: TypeDamOverTime, debuff: true},
	"ManaDamOverTime":       {typ: TypeManaDamOverTime},
	"Fear":                  {typ: TypeFear, flag: FlagFear, debuff: true, rejectsIfAffected: true},
	"Root":                  {typ: TypeRoot, flag: FlagRooted, debuff: true, rejectsIfAffected: true},
	"Sleep":                 {typ: TypeSleep, flag: FlagSleep, debuff: true, rejectsIfAffected: true},
	"Stun":                  {typ: TypeStun, flag: FlagStunned, debuff: true, rejectsIfAffected: true},
	"AbortCast":             {typ: TypeAbortCast},
	"ImmobileUntilAttacked": {typ: TypeImmobileUntilAttacked, flag: FlagMeditating},
	"ImobileBuff":           {typ: TypeImmobilizeEffector},
	"Invincible":            {typ: TypeInvincible},
	"ManaHealOverTime":      {typ: TypeManaHealOverTime},
	"Mute":                  {typ: TypeMute, flag: flagMuted, debuff: true},
	"NoblesseBless":         {typ: TypeNoblesseBless, flag: flagNoblesseBlessing},
	"Paralyze":              {typ: TypeParalyze, flag: FlagParalyzed, debuff: true},
	"Petrification":         {typ: TypePetrification, flag: FlagParalyzed, debuff: true},
	"PhysicalMute":          {typ: TypePhysicalMute, flag: flagPhysicalMuted, debuff: true},
	"RemoveTarget":          {typ: TypeRemoveTarget},
	"SilenceMagicPhysical":  {typ: TypeSilenceAll, flag: flagMuted | flagPhysicalMuted, debuff: true},
	"SilentMove":            {typ: TypeSilentMove, flag: FlagSilentMove},
	"StunSelf":              {typ: TypeStunSelf, flag: FlagStunned},
	"Heal":                  {typ: TypeHeal},
	"HealOverTime":          {typ: TypeHealOverTime},
	"ManaHeal":              {typ: TypeManaHeal},
	"IncreaseCharges":       {typ: TypeIncreaseCharges},
	"TargetMe":              {typ: TypeTargetMe},
	"Bluff":                 {typ: TypeBluff},
	"CharmOfCourage":        {typ: TypeCharmOfCourage, flag: flagCharmOfCourage},
	"CharmOfLuck":           {typ: TypeCharmOfLuck, flag: FlagCharmOfLuck},
	"PhoenixBless":          {typ: TypePhoenixBless, flag: FlagPhoenixBlessing},
	"BlockBuff":             {typ: TypeBlockBuff},
	"BlockDebuff":           {typ: TypeBlockDebuff},
	"ProtectionBlessing":    {typ: TypeProtectionBless, flag: flagProtectionBlessing},
	"Recovery":              {typ: TypeRecovery},
	"Cancel":                {typ: TypeCancel},
	"Negate":                {typ: TypeNegate},
	"Fusion":                {typ: TypeFusion},
	"ChanceSkillTrigger":    {typ: TypeChanceSkillTrigger},
	"Spoil":                 {typ: TypeSpoil},
	"PolearmTargetSingle":   {typ: TypePolearmTargetSingle},
	"BigHead":               {typ: TypeBigHead, flag: flagBigHead},
	"CancelDebuff":          {typ: TypeCancelDebuff},
	"Relax":                 {typ: TypeRelax, flag: flagRelaxing},
	"ChameleonRest":         {typ: TypeChameleonRest, flag: FlagSilentMove | flagRelaxing},
	"ImobilePetBuff":        {typ: TypeImmobilizePetBuff},
	"Distrust":              {typ: TypeDistrust},
	"Confusion":             {typ: TypeConfusion, flag: FlagConfused},
	"Betray":                {typ: TypeBetray, flag: FlagBetrayed, debuff: true},
	"RandomizeHate":         {typ: TypeRandomizeHate},
	"ThrowUp":               {typ: TypeThrowUp, flag: FlagStunned, debuff: true},
	"Grow":                  {typ: TypeGrow},
	"FakeDeath":             {typ: TypeFakeDeath, flag: FlagFakeDeath},
	"Seed":                  {typ: TypeSeed},
}

var fearSkippedPlayableSkillIDs = map[modelskill.ID]bool{
	98:   true,
	1272: true,
	1381: true,
}

// fearHalvedDurationPlayableSkillIDs are skill ids whose fear effect runs at
// half its configured tick count against a playable target.
var fearHalvedDurationPlayableSkillIDs = map[modelskill.ID]bool{
	65:   true,
	1092: true,
	1169: true,
}

// New builds a runtime effect from a parsed core effect template.
func New(skill Skill, tmpl modelskill.EffectTemplate) (*Effect, error) {
	k, ok := coreKinds[tmpl.Name]
	if !ok {
		return nil, fmt.Errorf("effect: unsupported core effect %q", tmpl.Name)
	}
	if tmpl.AttachCondition != nil {
		return nil, fmt.Errorf("effect %s: attach conditions are not wired yet", tmpl.Name)
	}
	if k.typ == TypeChanceSkillTrigger {
		if _, _, err := modelskill.ParseChanceCondition(tmpl.ChanceType, tmpl.ActivationChance); err != nil {
			return nil, fmt.Errorf("effect %s: %w", tmpl.Name, err)
		}
	}
	if k.flag == 0 {
		k.flag = FlagNone
	}

	skill.Debuff = skill.Debuff || k.debuff
	e := &Effect{
		Skill:             skill,
		Template:          tmpl,
		Type:              k.typ,
		Flag:              k.flag,
		Level:             skill.Level,
		RejectsIfAffected: k.rejectsIfAffected,
	}

	funcs, err := statFuncs(e, tmpl.Funcs)
	if err != nil {
		return nil, fmt.Errorf("effect %s: %w", tmpl.Name, err)
	}
	e.Funcs = funcs
	wireHooks(e)
	return e, nil
}

// ClassTag returns the effect's classification tag: the explicit datapack
// effectType attribute when present, otherwise the runtime effect kind.
// Marker effects (buff/debuff immunity, the cancel-exempt blessings) carry
// no datapack attribute, so the handlers that branch on classification
// match them through the kind the same way the effect's own type is matched.
func (e *Effect) ClassTag() string {
	if e.Template.EffectType != "" {
		return e.Template.EffectType
	}
	return string(e.Type)
}

func wireHooks(e *Effect) {
	switch e.Type {
	case TypeDamOverTime:
		e.OnAction = damageOverTimeAction
	case TypeManaDamOverTime:
		e.OnAction = manaDamageOverTimeAction
	case TypeFear:
		e.OnStart = fearStart
		e.OnAction = fearAction
		e.OnExit = fearExit
	case TypeRoot:
		e.OnStart = rootStart
		e.OnExit = thinkAndRefreshExit
	case TypeSleep:
		e.OnStart = sleepStart
		e.OnExit = thinkAndRefreshExit
	case TypeStun:
		e.OnStart = stunStart
		e.OnExit = refreshExit
	case TypeAbortCast:
		e.OnStart = abortCastStart
	case TypeImmobileUntilAttacked:
		e.OnStart = immobileUntilAttackedStart
		e.OnExit = immobileUntilAttackedExit
		e.OnAction = immobileUntilAttackedAction
	case TypeImmobilizeEffector:
		e.OnStart = immobilizeEffectorStart
		e.OnExit = immobilizeEffectorExit
	case TypeInvincible:
		e.OnStart = invincibleStart
		e.OnExit = invincibleExit
	case TypeManaHealOverTime:
		e.OnAction = manaHealOverTimeAction
	case TypeMute:
		e.OnStart = muteStart
		e.OnExit = refreshExit
	case TypeParalyze:
		e.OnStart = paralyzeStart
		e.OnExit = paralyzeExit
	case TypePetrification:
		e.OnStart = petrificationStart
		e.OnExit = petrificationExit
	case TypePhysicalMute:
		e.OnStart = physicalMuteStart
		e.OnExit = refreshExit
	case TypeRemoveTarget:
		e.OnStart = removeTargetStart
	case TypeSilenceAll:
		e.OnStart = silenceAllStart
		e.OnExit = refreshExit
	case TypeSilentMove:
		e.OnAction = silentMoveAction
	case TypeStunSelf:
		e.OnStart = stunSelfStart
		e.OnExit = stunSelfExit
	case TypeHeal:
		e.OnStart = healStart
	case TypeHealOverTime:
		e.OnAction = healOverTimeAction
	case TypeManaHeal:
		e.OnStart = manaHealStart
	case TypeIncreaseCharges:
		e.OnStart = increaseChargesStart
	case TypeTargetMe:
		e.OnStart = targetMeStart
	case TypeBluff:
		e.OnStart = bluffStart
	case TypeCharmOfCourage:
		e.OnStart = charmOfCourageStart
	case TypeCharmOfLuck:
		e.OnExit = charmOfLuckExit
	case TypePhoenixBless:
		e.OnExit = phoenixBlessExit
	case TypeCancel:
		e.OnStart = cancelStart
	case TypeNegate:
		e.OnStart = negateStart
	case TypeFusion:
		e.OnAction = fusionAction
	case TypeChanceSkillTrigger:
		e.OnStart = chanceSkillTriggerStart
		e.OnExit = chanceSkillTriggerExit
	case TypeSpoil:
		e.OnStart = spoilStart
	case TypeCancelDebuff:
		e.OnStart = cancelDebuffStart
	case TypeRelax:
		e.OnStart = relaxStart
		e.OnAction = relaxAction
	case TypeChameleonRest:
		e.OnStart = chameleonRestStart
		e.OnAction = chameleonRestAction
	case TypeImmobilizePetBuff:
		e.OnStart = immobilizePetBuffStart
		e.OnExit = immobilizePetBuffExit
	case TypeDistrust:
		e.OnStart = distrustStart
	case TypeConfusion:
		e.OnStart = confusionStart
		e.OnExit = confusionExit
	case TypeBetray:
		e.OnStart = betrayStart
		e.OnExit = betrayExit
	case TypeRandomizeHate:
		e.OnStart = randomizeHateStart
	case TypeThrowUp:
		e.OnStart = throwUpStart
		e.OnExit = throwUpExit
	case TypeGrow:
		e.OnStart = growStart
		e.OnExit = growExit
	case TypeFakeDeath:
		e.OnStart = fakeDeathStart
		e.OnAction = fakeDeathAction
		e.OnExit = fakeDeathExit
	case TypeRecovery:
		e.OnStart = recoveryStart
	}
}

func (e *Effect) IncreaseEffect(list *List, maxLevel int, reapply func(level int)) {
	if e == nil || list == nil || e.Level >= maxLevel {
		return
	}
	e.Level++
	list.Remove(e)
	if reapply != nil {
		reapply(e.Level)
	}
}

// DecreaseForce shrinks a live fusion effect by one level. Once its level
// drops below 1 it is removed outright instead of reapplied.
func (e *Effect) DecreaseForce(list *List, reapply func(level int)) {
	if e == nil || list == nil {
		return
	}
	e.Level--
	list.Remove(e)
	if e.Level >= 1 && reapply != nil {
		reapply(e.Level)
	}
}

// IncreasePower grows a live seed effect's charge level by one in place,
// matching EffectSeed.increasePower(): unlike a fusion effect's
// IncreaseEffect, the same instance keeps running — only its Level (power)
// changes, nothing is removed or reapplied.
func (e *Effect) IncreasePower() {
	if e == nil {
		return
	}
	e.Level++
}

// chanceTriggerTarget is implemented by an actor that tracks its own set of
// active chance-triggered skill effects, for whatever system later reacts
// to combat/cast events against it. No actor in this port implements it
// yet — installing and removing the effect degrades to a no-op until one
// does, the same graceful-degradation pattern every optional capability in
// this file follows.
