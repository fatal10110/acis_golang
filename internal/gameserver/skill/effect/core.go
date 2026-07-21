package effect

import (
	"fmt"
	"math"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Flag is the bitmask exposed by an active effect to live actor state.
type Flag uint32

const (
	// FlagNone is the default effect flag mask.
	FlagNone Flag = 1 << iota
	flagCharmOfCourage
	flagCharmOfLuck
	flagPhoenixBlessing
	flagNoblesseBlessing
	// FlagSilentMove marks a target as moving without alerting nearby AI.
	FlagSilentMove
	flagProtectionBlessing
	flagRelaxing
	// FlagFear marks a target as feared.
	FlagFear
	// FlagConfused marks a target as confused.
	FlagConfused
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
	"TargetMe":              {typ: TypeTargetMe},
	"Bluff":                 {typ: TypeBluff},
	"CharmOfCourage":        {typ: TypeCharmOfCourage, flag: flagCharmOfCourage},
	"CharmOfLuck":           {typ: TypeCharmOfLuck, flag: flagCharmOfLuck},
	"PhoenixBless":          {typ: TypePhoenixBless, flag: flagPhoenixBlessing},
	"BlockBuff":             {typ: TypeBlockBuff},
	"BlockDebuff":           {typ: TypeBlockDebuff},
	"ProtectionBlessing":    {typ: TypeProtectionBless, flag: flagProtectionBlessing},
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
	}
}

type dotTarget interface {
	Dead() bool
	HP() float64
	ReduceHPByDOT(damage float64, effector any, skill Skill)
}

type lackHPNotifier interface {
	NotifyEffectRemovedDueLackHP(*Effect)
}

type mpDotTarget interface {
	Dead() bool
	MP() float64
	ReduceMP(amount float64)
}

type lackMPNotifier interface {
	NotifyEffectRemovedDueLackMP(*Effect)
}

type aborter interface {
	AbortAll(force bool)
}

type idleTarget interface {
	TryToIdle()
}

type moveStopper interface {
	StopMove()
}

type abnormalUpdater interface {
	UpdateAbnormalEffect()
}

type thinkTarget interface {
	Think()
}

type afraidTarget interface {
	Afraid() bool
}

type fearImmuneTarget interface {
	FearImmune() bool
}

type playableTarget interface {
	Playable() bool
}

type fleeTarget interface {
	FleeFrom(effector any, distance int)
}

type effectStopper interface {
	StopEffects(Type)
}

// raidTarget optionally reports whether a target is a raid boss or minion;
// a target without one is treated as not raid-related.
type raidTarget interface {
	RaidRelated() bool
}

// castInterrupter is implemented by an actor whose in-progress cast can be
// checked and forcibly interrupted.
type castInterrupter interface {
	CastingNow() bool
	InterruptCast()
}

// magicCastTarget is implemented by an actor whose in-progress cast can be
// checked for its magic/physical nature and stopped.
type magicCastTarget interface {
	CastingNow() bool
	CurrentSkillIsMagic() bool
	StopCast()
}

// castStopper is implemented by an actor whose in-progress cast can be
// unconditionally stopped.
type castStopper interface {
	StopCast()
}

// targetClearer is implemented by an actor that can drop its current
// target and abandon any attack in progress against it.
type targetClearer interface {
	ClearTarget()
	StopAttack()
}

// invulnerabilityTarget is implemented by an actor whose damage
// invulnerability can be toggled.
type invulnerabilityTarget interface {
	SetInvul(bool)
}

// immobilizeTarget is implemented by an actor whose movement-lock flag can
// be toggled. SetImmobilized reports whether the flag actually changed;
// this hook ignores that report.
type immobilizeTarget interface {
	SetImmobilized(bool) bool
}

// manaHealTarget is implemented by an actor whose mana pool can be checked
// for healing eligibility and restored. AddMP reports the amount actually
// applied (e.g. clamped to the actor's max MP).
type manaHealTarget interface {
	CanBeHealed() bool
	AddMP(amount float64) float64
}

// instantHealTarget is implemented by an actor whose HP can be checked for
// healing eligibility and restored. AddHP reports the amount actually
// applied (e.g. clamped to the actor's max HP).
type instantHealTarget interface {
	CanBeHealed() bool
	AddHP(amount float64) float64
}

// healProficiencyTarget optionally reports the additive bonus a heal
// effect's base value is boosted by before the percentage from
// healEffectivenessTarget is applied; absent, it defaults to 0.
type healProficiencyTarget interface {
	HealProficiency() float64
}

// healEffectivenessTarget optionally reports the percentage a heal amount
// is scaled by; absent, it defaults to 100.
type healEffectivenessTarget interface {
	HealEffectiveness() float64
}

// rechargeRateTarget optionally adjusts a base MP-restore amount by the
// actor's recharge rate before it is applied; absent, the base amount is
// used unadjusted.
type rechargeRateTarget interface {
	RechargeMP(base float64) float64
}

// targetRedirectTarget is implemented by an actor whose current target can
// be read or replaced, or turned into an attack.
type targetRedirectTarget interface {
	CurrentTarget() any
	SetTarget(any)
	TryToAttack(any)
}

// headingTarget is implemented by an actor whose facing can be read or set.
type headingTarget interface {
	Heading() int
	SetHeading(int)
}

// bluffExemptTarget optionally reports whether an actor ignores a
// facing-redirect effect (some non-combatant and event-specific actors do);
// absent, the actor is not exempt.
type bluffExemptTarget interface {
	BluffExempt() bool
}

// playerTarget optionally reports whether an actor is specifically a
// player, as opposed to any other playable (pet, summon, ...); absent, the
// actor is treated as not a player.
type playerTarget interface {
	IsPlayer() bool
}

// charmOfLuckStopper is implemented by an actor that reacts to its Charm of
// Luck buff ending.
type charmOfLuckStopper interface {
	StopCharmOfLuck(*Effect)
}

// phoenixBlessStopper is implemented by an actor that reacts to its Phoenix
// Blessing buff ending.
type phoenixBlessStopper interface {
	StopPhoenixBlessing(*Effect)
}

// skillIDEffectStopper is implemented by an actor whose active effects can
// be stopped by owning skill id.
type skillIDEffectStopper interface {
	StopSkillEffectsByID(id modelskill.ID)
}

// deadChecker reports whether an actor is dead, consulted by a cancel-family
// effect before it strips anything.
type deadChecker interface {
	Dead() bool
}

// effectListOwner is implemented by an actor whose active effect list can be
// inspected and stripped directly, for cancel- and negate-family effects
// that act on other effects rather than the actor's stats.
type effectListOwner interface {
	EffectList() *List
}

// cancelVulnerabilitySource optionally supplies an actor's already-resolved
// vulnerability multiplier for a classification tag; an actor without one is
// treated as unmodified (1.0).
type cancelVulnerabilitySource interface {
	CancelVulnerability(classification string) float64
}

// objectIDTarget optionally reports an actor's world object id.
type objectIDTarget interface {
	ObjectID() int32
}

// levelTarget optionally reports an actor's level.
type levelTarget interface {
	Level() int
}

// spoilCaster is implemented by an actor whose identity and level a spoil
// effect's magic-resist roll needs.
type spoilCaster interface {
	objectIDTarget
	levelTarget
}

// spoilTarget is implemented by a monster-kind actor whose spoil pool can
// be checked and marked, mirroring the SPOIL skill-type handler's own
// target surface for a caster-applied spoil effect.
type spoilTarget interface {
	deadChecker
	levelTarget
	SpoilPool() *item.SpoilPool
}

// weaponGradePenalized optionally reports whether the caster's equipped
// weapon grade is insufficient for the skill being cast (a flat magic-
// resist penalty); a caster without one is treated as unpenalized.
type weaponGradePenalized interface {
	WeaponGradePenalty() bool
}

// standingTarget optionally reports whether an actor is currently standing
// rather than sitting; an actor without one is treated as not standing, so
// a rest-family effect's seated-check never rejects it.
type standingTarget interface {
	Standing() bool
}

// sitTarget is implemented by an actor whose sit/stand state can be set.
type sitTarget interface {
	SetStanding(bool) bool
}

// hpFullTarget optionally reports whether an actor's HP is already at (or
// within the reference effect's own +1 rounding tolerance of) its maximum,
// consulted by Relax before it drains MP on a tick; an actor without one is
// treated as never full.
type hpFullTarget interface {
	HPFull() bool
}

// summonOwnerTarget is implemented by a summon actor whose owning player's
// object id can be read, to confirm an effector is that summon's own
// owner.
type summonOwnerTarget interface {
	OwnerID() int32
}

// hateRaiser is implemented by an actor whose physical-attack threat can be
// raised against another combatant.
type hateRaiser interface {
	AddDamageHate(attacker attackable.Combatant, damage, hate float64)
}

// monsterKindTarget optionally reports whether an actor is specifically a
// Monster-family hostile NPC (as opposed to a guard, siege guard, or
// friendly monster); an actor without one is treated as not Monster-family.
type monsterKindTarget interface {
	MonsterKind() bool
}

// nearbyMonsterFinder is implemented by an actor that can pick a random
// other Monster-family actor within radius units of itself, for an effect
// that redirects one monster's aggression onto another; an actor without
// one finds no candidate.
type nearbyMonsterFinder interface {
	RandomNearbyMonster(radius int) (attackable.Combatant, bool)
}

// nearbyCombatTarget is implemented by an actor that can pick a random
// other attackable actor within radius units of itself, for an effect that
// redirects its own aggression onto a random bystander; an actor without
// one finds no candidate.
type nearbyCombatTarget interface {
	RandomNearbyCombatant(radius int) (attackable.Combatant, bool)
}

// mostHatedResetter is implemented by an actor whose physical threat table
// can have its current top target's hate cleared, used to drop a confusion
// effect's forced redirect once the effect ends.
type mostHatedResetter interface {
	StopMostHatedTarget()
}

func damageOverTimeAction(e *Effect) bool {
	target, ok := e.Effected.(dotTarget)
	if !ok {
		return false
	}

	result := DamageOverTimeTick(DamageOverTimeInput{
		Dead:      target.Dead(),
		HP:        target.HP(),
		Damage:    e.Template.Value,
		KillByDOT: e.Skill.KillByDOT,
		Toggle:    e.Skill.Toggle,
	})
	if result.RemovedForLackHP {
		if notifier, ok := e.Effected.(lackHPNotifier); ok {
			notifier.NotifyEffectRemovedDueLackHP(e)
		}
	}
	if result.Damage > 0 {
		target.ReduceHPByDOT(result.Damage, e.Effector, e.Skill)
	}
	return result.Continue
}

func manaDamageOverTimeAction(e *Effect) bool {
	target, ok := e.Effected.(mpDotTarget)
	if !ok {
		return false
	}

	result := ManaDamageOverTimeTick(ManaDamageOverTimeInput{
		Dead:   target.Dead(),
		MP:     target.MP(),
		Damage: e.Template.Value,
		Toggle: e.Skill.Toggle,
	})
	if result.RemovedForLackMP {
		if notifier, ok := e.Effected.(lackMPNotifier); ok {
			notifier.NotifyEffectRemovedDueLackMP(e)
		}
	}
	if result.Damage > 0 {
		target.ReduceMP(result.Damage)
	}
	return result.Continue
}

func manaHealOverTimeAction(e *Effect) bool {
	target, ok := e.Effected.(manaHealTarget)
	if !ok {
		return false
	}
	if !target.CanBeHealed() {
		return false
	}
	target.AddMP(e.Template.Value)
	return true
}

func stunStart(e *Effect) bool {
	abortAll(e.Effected)
	if target, ok := e.Effected.(idleTarget); ok {
		target.TryToIdle()
	}
	refresh(e.Effected)
	return true
}

func rootStart(e *Effect) bool {
	if target, ok := e.Effected.(moveStopper); ok {
		target.StopMove()
	}
	refresh(e.Effected)
	return true
}

func sleepStart(e *Effect) bool {
	abortAll(e.Effected)
	refresh(e.Effected)
	return true
}

func fearStart(e *Effect) bool {
	if isPlayable(e.Effected) && fearHalvedDurationPlayableSkillIDs[e.Skill.ID] {
		e.Template.Count /= 2
	}
	if fearImmune(e.Effected) || isAfraid(e.Effected) {
		return false
	}
	if isPlayable(e.Effected) && fearSkippedPlayableSkillIDs[e.Skill.ID] {
		return false
	}

	abortAll(e.Effected)
	refresh(e.Effected)
	return fearAction(e)
}

func fearAction(e *Effect) bool {
	target, ok := e.Effected.(fleeTarget)
	if !ok {
		return false
	}
	target.FleeFrom(e.Effector, 500)
	return true
}

func fearExit(e *Effect) {
	if target, ok := e.Effected.(effectStopper); ok {
		target.StopEffects(TypeFear)
	}
	refresh(e.Effected)
}

func thinkAndRefreshExit(e *Effect) {
	if target, ok := e.Effected.(thinkTarget); ok {
		target.Think()
	}
	refresh(e.Effected)
}

func refreshExit(e *Effect) {
	refresh(e.Effected)
}

func abortCastStart(e *Effect) bool {
	if e.Effected == nil || e.Effected == e.Effector {
		return false
	}
	if rt, ok := e.Effected.(raidTarget); ok && rt.RaidRelated() {
		return false
	}
	if target, ok := e.Effected.(castInterrupter); ok && target.CastingNow() {
		target.InterruptCast()
	}
	return true
}

func immobileUntilAttackedStart(e *Effect) bool {
	abortAll(e.Effected)
	refresh(e.Effected)
	return true
}

func immobileUntilAttackedExit(e *Effect) {
	if target, ok := e.Effected.(skillIDEffectStopper); ok {
		target.StopSkillEffectsByID(e.Skill.ID)
	}
	if target, ok := e.Effected.(thinkTarget); ok {
		target.Think()
	}
	refresh(e.Effected)
}

// immobileUntilAttackedAction always ends the effect on its first tick; an
// early trigger (e.g. the target taking damage) is expected to reschedule
// this tick sooner, not something this hook decides on its own.
func immobileUntilAttackedAction(e *Effect) bool {
	immobileUntilAttackedExit(e)
	return false
}

func immobilizeEffectorStart(e *Effect) bool {
	if target, ok := e.Effector.(immobilizeTarget); ok {
		target.SetImmobilized(true)
	}
	return true
}

func immobilizeEffectorExit(e *Effect) {
	if target, ok := e.Effector.(immobilizeTarget); ok {
		target.SetImmobilized(false)
	}
}

func invincibleStart(e *Effect) bool {
	if target, ok := e.Effected.(invulnerabilityTarget); ok {
		target.SetInvul(true)
	}
	return true
}

func invincibleExit(e *Effect) {
	if target, ok := e.Effected.(invulnerabilityTarget); ok {
		target.SetInvul(false)
	}
}

func muteStart(e *Effect) bool {
	if target, ok := e.Effected.(magicCastTarget); ok && target.CastingNow() && target.CurrentSkillIsMagic() {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func physicalMuteStart(e *Effect) bool {
	if target, ok := e.Effected.(magicCastTarget); ok && target.CastingNow() && !target.CurrentSkillIsMagic() {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func paralyzeStart(e *Effect) bool {
	abortAll(e.Effected)
	return true
}

func paralyzeExit(e *Effect) {
	if target, ok := e.Effected.(thinkTarget); ok {
		target.Think()
	}
}

func petrificationStart(e *Effect) bool {
	abortAll(e.Effected)
	if target, ok := e.Effected.(invulnerabilityTarget); ok {
		target.SetInvul(true)
	}
	return true
}

func petrificationExit(e *Effect) {
	if target, ok := e.Effected.(thinkTarget); ok {
		target.Think()
	}
	if target, ok := e.Effected.(invulnerabilityTarget); ok {
		target.SetInvul(false)
	}
}

func removeTargetStart(e *Effect) bool {
	if target, ok := e.Effected.(targetClearer); ok {
		target.ClearTarget()
		target.StopAttack()
	}
	if target, ok := e.Effected.(castStopper); ok {
		target.StopCast()
	}
	return true
}

func silenceAllStart(e *Effect) bool {
	if target, ok := e.Effected.(castStopper); ok {
		target.StopCast()
	}
	refresh(e.Effected)
	return true
}

func silentMoveAction(e *Effect) bool {
	if e.Skill.SkillType != "CONT" {
		return false
	}
	target, ok := e.Effected.(mpDotTarget)
	if !ok {
		return false
	}
	result := ManaDamageOverTimeTick(ManaDamageOverTimeInput{
		Dead:   target.Dead(),
		MP:     target.MP(),
		Damage: e.Template.Value,
		Toggle: true,
	})
	if result.RemovedForLackMP {
		if notifier, ok := e.Effected.(lackMPNotifier); ok {
			notifier.NotifyEffectRemovedDueLackMP(e)
		}
	}
	if result.Damage > 0 {
		target.ReduceMP(result.Damage)
	}
	return result.Continue
}

func stunSelfStart(e *Effect) bool {
	if p, ok := e.Effected.(playableTarget); ok && p.Playable() {
		if target, ok := e.Effected.(idleTarget); ok {
			target.TryToIdle()
		}
	}
	refresh(e.Effector)
	return true
}

func stunSelfExit(e *Effect) {
	refresh(e.Effector)
}

func healStart(e *Effect) bool {
	target, ok := e.Effected.(instantHealTarget)
	if !ok || !target.CanBeHealed() {
		return false
	}

	power := e.Template.Value
	if p, ok := e.Effected.(healProficiencyTarget); ok {
		power += p.HealProficiency()
	}
	effectiveness := 100.0
	if eff, ok := e.Effected.(healEffectivenessTarget); ok {
		effectiveness = eff.HealEffectiveness()
	}

	amount := target.AddHP(power * effectiveness / 100)
	// The applied amount is added a second time; this reproduces the
	// reference heal effect's own behavior exactly, not a Go-side bug.
	target.AddHP(amount)
	return true
}

func healOverTimeAction(e *Effect) bool {
	target, ok := e.Effected.(instantHealTarget)
	if !ok || !target.CanBeHealed() {
		return false
	}
	target.AddHP(e.Template.Value)
	return true
}

func manaHealStart(e *Effect) bool {
	target, ok := e.Effected.(manaHealTarget)
	if !ok || !target.CanBeHealed() {
		return false
	}

	power := e.Template.Value
	if r, ok := e.Effected.(rechargeRateTarget); ok {
		power = r.RechargeMP(power)
	}

	amount := target.AddMP(power)
	// The applied amount is added a second time; this reproduces the
	// reference heal effect's own behavior exactly, not a Go-side bug.
	target.AddMP(amount)
	return true
}

func targetMeStart(e *Effect) bool {
	target, ok := e.Effected.(targetRedirectTarget)
	if !ok {
		return false
	}
	if target.CurrentTarget() == e.Effector {
		target.TryToAttack(e.Effector)
	} else {
		target.SetTarget(e.Effector)
	}
	return true
}

func bluffStart(e *Effect) bool {
	if rt, ok := e.Effected.(raidTarget); ok && rt.RaidRelated() {
		return false
	}
	if ex, ok := e.Effected.(bluffExemptTarget); ok && ex.BluffExempt() {
		return false
	}
	target, ok := e.Effected.(headingTarget)
	if !ok {
		return false
	}
	effector, ok := e.Effector.(headingTarget)
	if !ok {
		return false
	}
	target.SetHeading(effector.Heading())
	return true
}

func charmOfCourageStart(e *Effect) bool {
	target, ok := e.Effected.(playerTarget)
	return ok && target.IsPlayer()
}

func charmOfLuckExit(e *Effect) {
	if target, ok := e.Effected.(charmOfLuckStopper); ok {
		target.StopCharmOfLuck(e)
	}
}

func phoenixBlessExit(e *Effect) {
	if target, ok := e.Effected.(phoenixBlessStopper); ok {
		target.StopPhoenixBlessing(e)
	}
}

// cancelStart strips a random subset of the effected actor's active
// non-toggle, non-debuff effects, up to e.Skill.MaxNegatedEffects (0 means
// unlimited). Each candidate rolls independently against
// formulas.EffectCancelSuccessRate.
//
// The classification checked against the protected-marker exemption list is
// this effect's own tag (e.ClassTag()), which is always the cancel
// classification and never matches any of the four protected markers
// (courage/luck charms, noblesse and protection blessings) — so that
// exemption can never actually trigger here, and those markers remain
// cancellable through this path even though the check reads as if it
// guards them. This is the required behavior; do not "fix" it by checking
// the candidate's tag instead.
func cancelStart(e *Effect) bool {
	if target, ok := e.Effected.(deadChecker); ok && target.Dead() {
		return false
	}
	owner, ok := e.Effected.(effectListOwner)
	if !ok {
		return true
	}
	list := owner.EffectList()
	if list == nil {
		return true
	}
	if effectNotCancellable[strings.ToUpper(e.ClassTag())] {
		return true
	}

	vuln := 1.0
	if v, ok := e.Effected.(cancelVulnerabilitySource); ok {
		vuln = v.CancelVulnerability(e.ClassTag())
	}

	count := e.Skill.MaxNegatedEffects
	candidates := list.All()
	shuffleEffects(candidates)

	for _, cand := range candidates {
		if cand.Skill.Toggle || cand.Skill.Debuff {
			continue
		}

		rate := formulas.EffectCancelSuccessRate(e.Skill.MagicLevel, cand.Skill.MagicLevel, cand.Template.Time, e.Template.EffectPower, vuln)
		if formulas.CancelSucceeds(float64(rate), rnd.Get(100)) {
			list.Remove(cand)
		}

		if count > 0 {
			count--
			if count == 0 {
				break
			}
		}
	}
	return true
}

// effectNotCancellable are effect classification tags that appear to be
// exempt from cancelStart's strip loop; see cancelStart's doc comment for
// why the exemption never actually applies there.
var effectNotCancellable = map[string]bool{
	"CHARM_OF_COURAGE":    true,
	"CHARM_OF_LUCK":       true,
	"NOBLESSE_BLESSING":   true,
	"PROTECTION_BLESSING": true,
}

// shuffleEffects randomizes candidates in place (Fisher-Yates) so a capped
// cancel/dispel loop doesn't always prefer the same array position.
func shuffleEffects(candidates []*Effect) {
	for i := len(candidates) - 1; i > 0; i-- {
		j := rnd.Get(i + 1)
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}
}

// negateStart strips every active effect on the effected actor that's owned
// by one of e.Skill.NegateIDs, plus every active effect whose classification
// matches one of e.Skill.NegateTypes and whose abnormal level (per-effect
// when its owning skill sets EffectType, per-skill otherwise) is within
// e.Skill.NegateLevel — or any level when NegateLevel is -1.
func negateStart(e *Effect) bool {
	owner, ok := e.Effected.(effectListOwner)
	if !ok {
		return true
	}
	list := owner.EffectList()
	if list == nil {
		return true
	}

	for _, id := range e.Skill.NegateIDs {
		if id == 0 {
			continue
		}
		for _, cand := range list.All() {
			if int(cand.Skill.ID) == id {
				list.Remove(cand)
			}
		}
	}

	for _, negType := range e.Skill.NegateTypes {
		negType = strings.ToUpper(strings.TrimSpace(negType))
		for _, cand := range list.All() {
			if !negateTypeMatches(cand.Skill, negType) {
				continue
			}
			if !negateLevelAllows(cand.Skill, e.Skill.NegateLevel) {
				continue
			}
			list.Remove(cand)
		}
	}
	return true
}

// negateTypeMatches reports whether candidate's classification (its own
// skill type, or its own effect-type tag when set) matches negType.
func negateTypeMatches(candidate Skill, negType string) bool {
	if strings.EqualFold(candidate.SkillType, negType) {
		return true
	}
	return candidate.EffectType != "" && strings.EqualFold(candidate.EffectType, negType)
}

// negateLevelAllows reports whether candidate's applicable abnormal level
// (EffectAbnormalLevel when its own EffectType is set, AbnormalLevel
// otherwise) is within negateLvl, or negateLvl is -1 (unrestricted).
func negateLevelAllows(candidate Skill, negateLvl int) bool {
	if negateLvl == -1 {
		return true
	}
	if candidate.EffectType != "" && candidate.EffectAbnormalLevel >= 0 && candidate.EffectAbnormalLevel <= negateLvl {
		return true
	}
	return candidate.AbnormalLevel >= 0 && candidate.AbnormalLevel <= negateLvl
}

// fusionAction is a fusion effect's periodic tick: it never ends on its own
// action timer, only when its Time runs out or IncreaseEffect/
// DecreaseForce removes it.
func fusionAction(*Effect) bool {
	return true
}

// IncreaseEffect grows a live fusion effect by one level, up to maxLevel.
// It removes this instance from list and, unless it was already at
// maxLevel, asks reapply to install a fresh instance at the grown level —
// exactly what constructing a new effect at that level in this one's place
// would produce. Doing nothing at maxLevel (rather than reapplying at the
// same level) matches the reference: the growth attempt is a plain no-op
// once the effect is already maxed out.
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

// chanceTriggerTarget is implemented by an actor that tracks its own set of
// active chance-triggered skill effects, for whatever system later reacts
// to combat/cast events against it. No actor in this port implements it
// yet — installing and removing the effect degrades to a no-op until one
// does, the same graceful-degradation pattern every optional capability in
// this file follows.
type chanceTriggerTarget interface {
	AddChanceTrigger(e *Effect)
	RemoveChanceTrigger(e *Effect)
}

func chanceSkillTriggerStart(e *Effect) bool {
	if target, ok := e.Effected.(chanceTriggerTarget); ok {
		target.AddChanceTrigger(e)
	}
	return true
}

func chanceSkillTriggerExit(e *Effect) {
	if target, ok := e.Effected.(chanceTriggerTarget); ok {
		target.RemoveChanceTrigger(e)
	}
}

// spoilRoll is the upper bound of the uniform random draw a spoil effect's
// magic-resist roll compares against, matching the SPOIL skill-type
// handler's own roll.
const spoilRoll = 10000

// spoilStart rolls a magic-resist check against a live, unspoiled monster
// target and marks it spoiled by the caster on success. It always reports
// success once the roll is attempted, regardless of the roll's outcome.
func spoilStart(e *Effect) bool {
	caster, ok := e.Effector.(spoilCaster)
	if !ok {
		return false
	}
	target, ok := e.Effected.(spoilTarget)
	if !ok || target.Dead() {
		return false
	}
	pool := target.SpoilPool()
	if pool == nil || pool.IsSpoiled() {
		return false
	}

	penalty := false
	if p, ok := e.Effector.(weaponGradePenalized); ok {
		penalty = p.WeaponGradePenalty()
	}

	rate := formulas.MagicSuccessRate(target.Level(), caster.Level(), e.Skill.MagicLevel, e.Skill.LevelDepend, penalty)
	if formulas.MagicSucceeds(rate, rnd.Get(spoilRoll)) {
		pool.Mark(caster.ObjectID())
	}
	return true
}

// distrustRadius is the search radius, in game units, a distrust effect
// scans for a redirect candidate.
const distrustRadius = 600

// distrustStart turns a Monster-family target's aggression toward a random
// other Monster-family actor found nearby (excluding lootable chests),
// with an aggro amount scaled by the caster's level. A target with no
// nearby candidate, or no radius-search capability at all, still reports
// success.
func distrustStart(e *Effect) bool {
	target, ok := e.Effected.(hateRaiser)
	if !ok {
		return false
	}
	if m, ok := e.Effected.(monsterKindTarget); !ok || !m.MonsterKind() {
		return false
	}
	finder, ok := e.Effected.(nearbyMonsterFinder)
	if !ok {
		return true
	}
	candidate, ok := finder.RandomNearbyMonster(distrustRadius)
	if !ok {
		return true
	}

	level := 0
	if lv, ok := e.Effector.(levelTarget); ok {
		level = lv.Level()
	}
	aggro := float64((5 + rnd.Get(5)) * level)
	target.AddDamageHate(candidate, 0, aggro)
	return true
}

// confusionRadius is the search radius, in game units, a confusion
// effect scans for a redirect candidate.
const confusionRadius = 1000

// confusionStart aborts a non-player target's current move, then redirects
// its aggression toward a random nearby attackable actor found within
// confusionRadius. A player target is left untouched entirely. Unlike the
// reference effect's own 2D-only distance check, the radius search this
// port reuses (world.State.ForEachKnownInRadius) filters in 3D — a
// documented simplification, not a byte-exact match, since no 2D-only
// radius query exists in this port yet.
func confusionStart(e *Effect) bool {
	if isPlayer(e.Effected) {
		return true
	}
	if target, ok := e.Effected.(moveStopper); ok {
		target.StopMove()
	}
	refresh(e.Effected)

	finder, ok := e.Effected.(nearbyCombatTarget)
	if !ok {
		return true
	}
	candidate, ok := finder.RandomNearbyCombatant(confusionRadius)
	if !ok {
		return true
	}
	if target, ok := e.Effected.(hateRaiser); ok {
		target.AddDamageHate(candidate, 0, math.MaxInt32)
	}
	return true
}

func confusionExit(e *Effect) {
	refresh(e.Effected)
	if isPlayer(e.Effected) {
		return
	}
	if target, ok := e.Effected.(mostHatedResetter); ok {
		target.StopMostHatedTarget()
	}
}

// relaxStart sits the target down; it always reports success.
func relaxStart(e *Effect) bool {
	if target, ok := e.Effected.(sitTarget); ok {
		target.SetStanding(false)
	}
	return true
}

// relaxAction drains MP each tick while the target stays seated and its HP
// isn't already full, reusing the same lack-MP handling as the other
// mana-drain ticks in this file. Unlike TypeChameleonRest, this tick has no
// continuous-skill gate: the reference effect never checks one.
func relaxAction(e *Effect) bool {
	target, ok := e.Effected.(mpDotTarget)
	if !ok {
		return false
	}
	if st, ok := e.Effected.(standingTarget); ok && st.Standing() {
		return false
	}
	if full, ok := e.Effected.(hpFullTarget); ok && full.HPFull() {
		return false
	}
	return manaDrainTick(e, target)
}

// chameleonRestStart sits the target down; it always reports success.
func chameleonRestStart(e *Effect) bool {
	if target, ok := e.Effected.(sitTarget); ok {
		target.SetStanding(false)
	}
	return true
}

// chameleonRestAction drains MP each tick while a continuous cast keeps the
// tick alive and the target stays seated, reusing the same lack-MP
// handling as the other mana-drain ticks in this file.
func chameleonRestAction(e *Effect) bool {
	if e.Skill.SkillType != "CONT" {
		return false
	}
	target, ok := e.Effected.(mpDotTarget)
	if !ok {
		return false
	}
	if st, ok := e.Effected.(standingTarget); ok && st.Standing() {
		return false
	}
	return manaDrainTick(e, target)
}

// manaDrainTick runs one ManaDamageOverTimeTick against target and applies
// its result, shared by relaxAction and chameleonRestAction.
//
// Toggle is forced true regardless of e.Skill.Toggle: the reference Relax
// and ChameleonRest effects check "cost exceeds current MP" unconditionally,
// not only for toggle skills (unlike EffectManaDamOverTime, whose lack-MP
// check really is toggle-gated). Every skill carrying either effect in the
// current datapack happens to be TOGGLE-typed, so reading e.Skill.Toggle
// would produce the same result today — but that's a data coincidence, not
// a contract; force true here so a future non-toggle skill using these
// effects still gets the unconditional check Java requires.
func manaDrainTick(e *Effect, target mpDotTarget) bool {
	result := ManaDamageOverTimeTick(ManaDamageOverTimeInput{
		Dead:   target.Dead(),
		MP:     target.MP(),
		Damage: e.Template.Value,
		Toggle: true,
	})
	if result.RemovedForLackMP {
		if notifier, ok := e.Effected.(lackMPNotifier); ok {
			notifier.NotifyEffectRemovedDueLackMP(e)
		}
	}
	if result.Damage > 0 {
		target.ReduceMP(result.Damage)
	}
	return result.Continue
}

// immobilizePetBuffStart locks the effected summon in place, gated on the
// caster being specifically a player and that summon's own owner.
func immobilizePetBuffStart(e *Effect) bool {
	if !isPlayer(e.Effector) {
		return false
	}
	player, ok := e.Effector.(objectIDTarget)
	if !ok {
		return false
	}
	summon, ok := e.Effected.(summonOwnerTarget)
	if !ok || summon.OwnerID() != player.ObjectID() {
		return false
	}
	target, ok := e.Effected.(immobilizeTarget)
	if !ok {
		return false
	}
	target.SetImmobilized(true)
	return true
}

func immobilizePetBuffExit(e *Effect) {
	if target, ok := e.Effected.(immobilizeTarget); ok {
		target.SetImmobilized(false)
	}
}

// cancelDebuffStart strips a capped selection of a player target's active,
// dispellable debuffs against an independently-rolled per-candidate chance
// (formulas.EffectCancelDebuffSuccessRate), scanning the effect list's
// current snapshot from its most-recently-added entry back to its oldest.
//
// The scan runs up to two full passes over the same snapshot whenever the
// cap isn't reached (or is unlimited, cap 0) in the first pass — including
// re-examining candidates the first pass already removed, since removing
// an already-removed candidate is a safe no-op but still counts against
// the cap exactly as it does in the reference effect. A candidate whose
// owning skill id matches the immediately preceding removal is stripped
// without its own roll. Both quirks reproduce the reference effect's own
// two-pass loop exactly; do not "simplify" this into cancelStart's single
// shuffled pass.
func cancelDebuffStart(e *Effect) bool {
	target, ok := e.Effected.(playerTarget)
	if !ok || !target.IsPlayer() {
		return false
	}
	if dc, ok := e.Effected.(deadChecker); ok && dc.Dead() {
		return false
	}
	owner, ok := e.Effected.(effectListOwner)
	if !ok {
		return true
	}
	list := owner.EffectList()
	if list == nil {
		return true
	}

	vuln := 1.0
	if v, ok := e.Effected.(cancelVulnerabilitySource); ok {
		vuln = v.CancelVulnerability(e.ClassTag())
	}

	candidates := list.All()
	count := cancelDebuffPass(list, candidates, e.Skill.MagicLevel, vuln, e.Skill.MaxNegatedEffects)
	if count != 0 {
		cancelDebuffPass(list, candidates, e.Skill.MagicLevel, vuln, count)
	}
	return true
}

// cancelDebuffPass runs one reverse-order sweep of candidates, stripping
// dispellable debuffs against an independent roll each, up to count
// removals (0 meaning unlimited); it returns the remaining count.
func cancelDebuffPass(list *List, candidates []*Effect, cancelLvl int, vuln float64, count int) int {
	lastCanceledSkillID := modelskill.ID(0)
	for i := len(candidates) - 1; i >= 0; i-- {
		cand := candidates[i]
		if !cand.Skill.Debuff || !cand.Skill.CanBeDispelled {
			continue
		}
		if cand.Skill.ID == lastCanceledSkillID {
			list.Remove(cand)
			continue
		}

		// Template.Time (the candidate's full configured duration) stands in for the
		// reference effect's remaining duration (period-elapsed): this port has no
		// live elapsed-time tracking per effect yet, so "remaining" always reads as
		// "full". Revisit once effects track elapsed time.
		rate := formulas.EffectCancelDebuffSuccessRate(cancelLvl, cand.Skill.MagicLevel, cand.Template.Time, vuln)
		if !formulas.CancelSucceeds(float64(rate), rnd.Get(100)) {
			continue
		}

		lastCanceledSkillID = cand.Skill.ID
		list.Remove(cand)
		count--
		if count == 0 {
			break
		}
	}
	return count
}

func abortAll(target any) {
	if target, ok := target.(aborter); ok {
		target.AbortAll(false)
	}
}

func refresh(target any) {
	if target, ok := target.(abnormalUpdater); ok {
		target.UpdateAbnormalEffect()
	}
}

func fearImmune(target any) bool {
	t, ok := target.(fearImmuneTarget)
	return ok && t.FearImmune()
}

func isAfraid(target any) bool {
	t, ok := target.(afraidTarget)
	return ok && t.Afraid()
}

func isPlayable(target any) bool {
	t, ok := target.(playableTarget)
	return ok && t.Playable()
}

func isPlayer(target any) bool {
	t, ok := target.(playerTarget)
	return ok && t.IsPlayer()
}

// statFuncs builds the stat functions templates describes, attributed to
// owner. owner is opaque here (see basefunc.Func.Owner): a running buff
// passes itself, a passive skill passes an identity stable for as long as
// it stays learned.
func statFuncs(owner any, templates []modelskill.FuncTemplate) ([]basefunc.Func, error) {
	funcs := make([]basefunc.Func, 0, len(templates))
	for _, tmpl := range templates {
		if tmpl.AttachCondition != nil || tmpl.Condition != nil {
			return nil, fmt.Errorf("conditional stat funcs are not wired yet")
		}
		s, err := stat.ByName(tmpl.Stat)
		if err != nil {
			return nil, err
		}
		fn, err := statFunc(owner, s, tmpl)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, fn)
	}
	return funcs, nil
}

func statFunc(owner any, s stat.Stat, tmpl modelskill.FuncTemplate) (basefunc.Func, error) {
	switch tmpl.Op {
	case modelskill.FuncAdd:
		return basefunc.NewAdd(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncAddMul:
		return basefunc.NewAddMul(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncSub:
		return basefunc.NewSub(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncSubDiv:
		return basefunc.NewSubDiv(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncMul:
		return basefunc.NewMul(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncBaseMul:
		return basefunc.NewBaseMul(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncDiv:
		return basefunc.NewDiv(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncSet:
		return basefunc.NewSet(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncBaseAdd:
		return basefunc.NewBaseAdd(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncEnchant:
		return nil, fmt.Errorf("enchant stat funcs need an item owner")
	default:
		return nil, fmt.Errorf("unknown stat func op %s", tmpl.Op)
	}
}

// DamageOverTimeInput is the state a periodic HP damage tick needs.
type DamageOverTimeInput struct {
	Dead      bool
	HP        float64
	Damage    float64
	KillByDOT bool
	Toggle    bool
}

// DamageOverTimeResult reports the effect of one periodic HP damage tick.
type DamageOverTimeResult struct {
	Damage           float64
	Continue         bool
	RemovedForLackHP bool
}

// DamageOverTimeTick computes one periodic HP damage tick without mutating
// actor state.
func DamageOverTimeTick(in DamageOverTimeInput) DamageOverTimeResult {
	if in.Dead {
		return DamageOverTimeResult{}
	}

	damage := in.Damage
	if damage >= in.HP {
		if in.Toggle {
			return DamageOverTimeResult{RemovedForLackHP: true}
		}
		if !in.KillByDOT {
			if in.HP <= 1 {
				return DamageOverTimeResult{Continue: true}
			}
			damage = in.HP - 1
		}
	}
	return DamageOverTimeResult{Damage: damage, Continue: true}
}

// ManaDamageOverTimeInput is the state a periodic MP upkeep tick needs.
type ManaDamageOverTimeInput struct {
	Dead   bool
	MP     float64
	Damage float64
	Toggle bool
}

// ManaDamageOverTimeResult reports the effect of one periodic MP upkeep
// tick.
type ManaDamageOverTimeResult struct {
	Damage           float64
	Continue         bool
	RemovedForLackMP bool
}

// ManaDamageOverTimeTick computes one periodic MP upkeep tick without
// mutating actor state. A toggle skill whose upkeep strictly exceeds the
// available MP drops instead of draining below zero; every other
// mana-drain effect always pays its cost, however low that leaves MP.
// The strict inequality (not "at least") matters: a toggle whose upkeep
// exactly equals the remaining MP still pays it and keeps running.
func ManaDamageOverTimeTick(in ManaDamageOverTimeInput) ManaDamageOverTimeResult {
	if in.Dead {
		return ManaDamageOverTimeResult{}
	}
	if in.Toggle && in.Damage > in.MP {
		return ManaDamageOverTimeResult{RemovedForLackMP: true}
	}
	return ManaDamageOverTimeResult{Damage: in.Damage, Continue: true}
}
