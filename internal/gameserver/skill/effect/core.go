package effect

import (
	"errors"
	"fmt"
	"strings"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ErrUnsupportedCoreEffect marks New's rejection of an effect template whose
// name has no entry in coreKinds — a missing effect kind (or an unresolved
// #table name reference), tracked separately from stat-func construction.
var ErrUnsupportedCoreEffect = errors.New("effect: unsupported core effect")

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

// magicCircleAbnormalMask is AbnormalEffect.MAGIC_CIRCLE's client bitmask,
// ClanGate's abnormal-effect icon.
const magicCircleAbnormalMask = 0x800000

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
	// TypeSignetGround marks the caster-side half of a ground signet: the
	// effect that owns the signet's world actor for as long as it runs.
	// Aborting the caster's cast drops it, which is how the signet it
	// carries stops existing.
	TypeSignetGround Type = "SIGNET_GROUND"
	// TypeClanGate is a court-magician portal buff: it marks its target with
	// the magic-circle abnormal effect for the buff's duration.
	TypeClanGate Type = "CLAN_GATE"
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
	typ  Type
	flag Flag
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
	"Debuff":                {typ: TypeDebuff},
	"DamOverTime":           {typ: TypeDamOverTime},
	"ManaDamOverTime":       {typ: TypeManaDamOverTime},
	"Fear":                  {typ: TypeFear, flag: FlagFear, rejectsIfAffected: true},
	"Root":                  {typ: TypeRoot, flag: FlagRooted, rejectsIfAffected: true},
	"Sleep":                 {typ: TypeSleep, flag: FlagSleep, rejectsIfAffected: true},
	"Stun":                  {typ: TypeStun, flag: FlagStunned, rejectsIfAffected: true},
	"AbortCast":             {typ: TypeAbortCast},
	"ImmobileUntilAttacked": {typ: TypeImmobileUntilAttacked, flag: FlagMeditating},
	"ImobileBuff":           {typ: TypeImmobilizeEffector},
	"Invincible":            {typ: TypeInvincible},
	"ManaHealOverTime":      {typ: TypeManaHealOverTime},
	"Mute":                  {typ: TypeMute, flag: flagMuted},
	"NoblesseBless":         {typ: TypeNoblesseBless, flag: flagNoblesseBlessing},
	"Paralyze":              {typ: TypeParalyze, flag: FlagParalyzed},
	"Petrification":         {typ: TypePetrification, flag: FlagParalyzed},
	"PhysicalMute":          {typ: TypePhysicalMute, flag: flagPhysicalMuted},
	"RemoveTarget":          {typ: TypeRemoveTarget},
	"SilenceMagicPhysical":  {typ: TypeSilenceAll, flag: flagMuted | flagPhysicalMuted},
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
	"Betray":                {typ: TypeBetray, flag: FlagBetrayed},
	"RandomizeHate":         {typ: TypeRandomizeHate},
	"ThrowUp":               {typ: TypeThrowUp, flag: FlagStunned},
	"Grow":                  {typ: TypeGrow},
	"FakeDeath":             {typ: TypeFakeDeath, flag: FlagFakeDeath},
	"Seed":                  {typ: TypeSeed},
	// Signet, SignetNoise, SignetAntiSummon, and SignetMDam are the effect-
	// template names the datapack attaches to signet-family skills
	// (454-460, 1419-1424). handler/skill/signet.go builds their real
	// per-tick ground-actor behavior directly — spawning the EffectPoint
	// actor, applying the linked sub-skill, stripping dances, unsummoning
	// pets, or dealing magic damage — bypassing New/coreKinds entirely,
	// because that needs the caster's npc-template registry, id allocator,
	// and world, none of which New's signature carries. These entries exist
	// only so New() (relog restore's generic replay, and shipped-template
	// load validation) accepts the name instead of rejecting it; reached
	// that way — never during a live cast — TypeSignetGround's OnStart
	// declines outright, since there is no actor for it to drive.
	"Signet":           {typ: TypeSignetGround},
	"SignetNoise":      {typ: TypeSignetGround},
	"SignetAntiSummon": {typ: TypeSignetGround},
	"SignetMDam":       {typ: TypeSignetGround},
	"ClanGate":         {typ: TypeClanGate},
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

// SkillFromDefinition builds the effect-list metadata a skill definition
// contributes to every effect instance it applies, shared by a live cast's
// applyEffects and a relog restore's effect replay.
func SkillFromDefinition(def modelskill.Definition) Skill {
	return Skill{
		ID:                  def.ID,
		Level:               def.Level,
		Name:                def.Name,
		SkillType:           def.SkillType,
		Debuff:              def.Debuff,
		Toggle:              def.Activation == modelskill.ActivationToggle,
		KillByDOT:           def.KillByDOT,
		Dance:               def.Dance,
		CanBeDispelled:      def.CanBeDispelled,
		MagicLevel:          def.MagicLevel,
		LevelDepend:         def.LevelDepend,
		AbnormalLevel:       def.AbnormalLevel,
		EffectAbnormalLevel: def.EffectAbnormalLevel,
		EffectType:          def.EffectType,
		MaxNegatedEffects:   def.MaxNegatedEffects,
		NegateLevel:         def.NegateLevel,
		NegateIDs:           def.NegateIDs,
		NegateTypes:         def.NegateTypes,
		FlyRadius:           def.FlyRadius,
	}
}

// ApplyRestored instantiates each of templates and adds it to list, seeded to
// resume from count and elapsedSeconds — the tick count and time-since-last-
// tick a persisted effect had at logout — instead of starting fresh from the
// template, mirroring Player.restoreEffects()'s
// template.getEffect(this, this, skill) -> setCount/setTime ->
// scheduleEffect() chain. effector and effected are both the relogging
// character: the original caster identity is not persisted, so every
// reinstated effect is treated as self-applied, matching the reference.
func ApplyRestored(list *List, effector, effected Participant, meta Skill, templates []modelskill.EffectTemplate, count, elapsedSeconds int32) {
	if list == nil {
		return
	}
	for _, tmpl := range templates {
		e, err := New(meta, tmpl)
		if err != nil {
			continue
		}
		e.Effector = effector
		e.Effected = effected
		e.seedRestore(count, elapsedSeconds)
		list.Add(e)
	}
}

// New builds a runtime effect from a parsed core effect template.
func New(skill Skill, tmpl modelskill.EffectTemplate) (*Effect, error) {
	k, ok := coreKinds[tmpl.Name]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnsupportedCoreEffect, tmpl.Name)
	}
	if k.typ == TypeChanceSkillTrigger {
		if _, _, err := modelskill.ParseChanceCondition(tmpl.ChanceType, tmpl.ActivationChance); err != nil {
			return nil, fmt.Errorf("effect %s: %w", tmpl.Name, err)
		}
	}
	if k.flag == 0 {
		k.flag = FlagNone
	}

	// A seed effect's Level is a charge counter (IncreasePower), not the
	// applied skill level: EffectSeed.java:10 hardcodes _power = 1 at
	// construction regardless of skill.getLevel().
	level := skill.Level
	if k.typ == TypeSeed {
		level = 1
	}
	e := &Effect{
		Skill:             skill,
		Template:          tmpl,
		Type:              k.typ,
		Flag:              k.flag,
		Level:             level,
		RejectsIfAffected: k.rejectsIfAffected,
		// Mirrors AbstractEffect._isHerbEffect = _skill.getName().contains("Herb"):
		// a herb buff is identified by its owning skill's name, not by how it
		// was cast, so this applies uniformly to every application path.
		Herb: strings.Contains(skill.Name, "Herb"),
	}

	// tmpl.AttachCondition is the <cond> sibling gating the whole <effect>
	// block (not any one func's own predicate); AND it onto every func this
	// effect contributes, matching the Java reference's per-func attach-
	// condition propagation (DocumentBase's cond stays in scope for every
	// stat element until the next cond resets it).
	attachGate, err := funcCondition(nil, tmpl.AttachCondition)
	if err != nil {
		return nil, fmt.Errorf("effect %s: %w", tmpl.Name, err)
	}
	funcs, err := statFuncs(ModOwnerEffect(e), tmpl.Funcs, attachGate)
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
		e.OnStart = healOverTimeStart
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
	case TypeBigHead:
		e.OnStart = func(e *Effect) bool {
			startAbnormalEffect(e.Effected, 0x002000)
			return true
		}
		e.OnExit = func(e *Effect) { stopAbnormalEffect(e.Effected, 0x002000) }
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
	case TypeSignetGround:
		// Only reached via New() (relog restore, load validation), never a
		// live cast — see the coreKinds comment above. No actor exists to
		// drive it here, so it declines rather than fake-apply.
		e.OnStart = func(*Effect) bool { return false }
	case TypeClanGate:
		e.OnStart = func(e *Effect) bool {
			startAbnormalEffect(e.Effected, magicCircleAbnormalMask)
			return true
		}
		e.OnExit = func(e *Effect) { stopAbnormalEffect(e.Effected, magicCircleAbnormalMask) }
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

// iconLevel reports the level to send on this effect's abnormal-status
// icon. AbnormalStatusUpdate.addEffect() -> EffectHolder(skill, period)
// (EffectHolder.java) always uses skill.getLevel(), never a seed's grown
// _power: a seed's Level field doubles as its charge counter, so its icon
// must read the fixed skill level instead, unlike every other kind whose
// Level is the applied skill level already.
func (e *Effect) iconLevel() int {
	if e.Type == TypeSeed {
		return e.Skill.Level
	}
	return e.Level
}

// chanceTriggerTarget is implemented by an actor that tracks its own set of
// active chance-triggered skill effects, for whatever system later reacts
// to combat/cast events against it. No actor in this port implements it
// yet — installing and removing the effect degrades to a no-op until one
// does, the same graceful-degradation pattern every optional capability in
// this file follows.
