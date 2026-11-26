package effect

import (
	"errors"
	"fmt"
	"strings"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// ErrUnsupportedCoreEffect marks New's rejection of an effect template whose
// name has no entry in coreKinds — a missing effect kind (or an unresolved
// #table name reference), tracked separately from stat-func construction.
var ErrUnsupportedCoreEffect = errors.New("effect: unsupported core effect")

// Flag is the bitmask exposed by an active effect to live actor state.
type Flag uint32

const (
	// FlagCharmOfCourage marks a player as carrying Charm of Courage, shown
	// in its status window.
	FlagCharmOfCourage Flag = 1 << iota
	// FlagCharmOfLuck marks a target as carrying Charm of Luck, consulted by
	// systems that exempt a lucky death from raising its own penalties.
	FlagCharmOfLuck
	// FlagPhoenixBlessing marks a target as carrying Phoenix Blessing,
	// consulted by systems that exempt a blessed death from its penalties.
	FlagPhoenixBlessing
	flagNoblesseBlessing
	// FlagSilentMove marks a target as moving without alerting nearby AI.
	FlagSilentMove
	// FlagProtectionBlessing marks a target as carrying Blessing of
	// Protection, consulted by the playable attack gates.
	FlagProtectionBlessing
	flagRelaxing
	// FlagFear marks a target as feared.
	FlagFear
	// FlagConfused marks a target as confused.
	FlagConfused
	// FlagBetrayed marks a summon as temporarily hostile to its owner.
	FlagBetrayed
	// FlagMuted marks a target as blocked from magic skills.
	FlagMuted
	// FlagPhysicalMuted marks a target as blocked from physical skills.
	FlagPhysicalMuted
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

// clanGateOpener is a ClanGate target that tells the other online members
// of its clan, if it has one, that the portal opened.
type clanGateOpener interface {
	NotifyClanGateOpened()
}

// TypeManaDamOverTime is a periodic MP-drain effect: a toggle skill's
// upkeep tick, or a plain continuous mana-drain buff.
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
	// List.IncreaseForce/DecreaseForce replace it one level up or down
	// while it's active, instead of it only ever starting or ending outright.
	TypeFusion Type = "FUSION"
	// TypeSignetGround marks the caster-side half of a ground signet: the
	// effect that owns the signet's world actor for as long as it runs.
	// Aborting the caster's cast drops it, which is how the signet it
	// carries stops existing.
	TypeSignetGround Type = "SIGNET_GROUND"
	// TypeClanGate is a court-magician portal buff: it marks its target with
	// the magic-circle abnormal effect for the buff's duration, and a
	// player target's clan learns the portal opened.
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
	// target grows the existing instance's power in place via List.IncreasePower
	// instead of replacing it.
	TypeSeed Type = "SEED"
	// TypeRecovery lowers a player target's death-penalty debuff level by
	// one on start; a non-player target rejects it.
	TypeRecovery Type = "RECOVERY"
)

// kind describes one effect implementation: the live state its effects
// carry and the hooks they run. New builds every effect from one.
type kind struct {
	typ        Type
	flag       Flag
	selfTarget bool
	// rejectsIfAffected marks a kind that refuses to be added at all (its
	// stop-task hook fires instead) when the owner is already affected by
	// its own Flag bit, from any currently held effect that carries it —
	// not just another instance of the same kind. Left false (the default
	// for every kind but these four) it never blocks; those four never
	// merge with or replace an existing same-flag effect, they simply
	// don't apply while one is live.
	rejectsIfAffected bool

	start  func(*Effect) bool
	action func(*Effect) bool
	exit   func(*Effect)
	// ownStart and ownExit mark a kind whose start or exit hook is entirely
	// its own: it ignores the template's abnormal visual there. Every other
	// kind shows the visual on start and clears it on exit, after its own
	// hook runs (see wireTemplateAbnormal).
	ownStart, ownExit bool
}

// signetGround is the kind behind Signet, SignetNoise, SignetAntiSummon,
// and SignetMDam, the effect-template names the datapack attaches to
// signet-family skills (454-460, 1419-1424). handler/skill/signet.go builds their real
// per-tick ground-actor behavior directly — spawning the EffectPoint
// actor, applying the linked sub-skill, stripping dances, unsummoning
// pets, or dealing magic damage — bypassing New/coreKinds entirely,
// because that needs the caster's npc-template registry, id allocator,
// and world, none of which New's signature carries. These entries exist
// only so New() (relog restore's generic replay, and shipped-template
// load validation) accepts the name instead of rejecting it; reached
// that way — never during a live cast — TypeSignetGround's OnStart
// declines outright, since there is no actor for it to drive.
// Like any failed start, it is held until its first tick ends it.
var signetGround = kind{typ: TypeSignetGround, start: declineStart, ownStart: true, ownExit: true}

var coreKinds = map[string]kind{
	"Buff":            {typ: TypeBuff},
	"Debuff":          {typ: TypeDebuff},
	"DamOverTime":     {typ: TypeDamOverTime, action: damageOverTimeAction},
	"ManaDamOverTime": {typ: TypeManaDamOverTime, action: manaDamageOverTimeAction},
	"Fear": {
		typ: TypeFear, flag: FlagFear, rejectsIfAffected: true,
		start: fearStart, action: fearAction, exit: fearExit, ownStart: true, ownExit: true,
	},
	"Root": {
		typ: TypeRoot, flag: FlagRooted, rejectsIfAffected: true,
		start: rootStart, exit: thinkAndRefreshExit, ownStart: true, ownExit: true,
	},
	"Sleep": {
		typ: TypeSleep, flag: FlagSleep, rejectsIfAffected: true,
		start: sleepStart, exit: thinkAndRefreshExit, ownStart: true, ownExit: true,
	},
	"Stun": {
		typ: TypeStun, flag: FlagStunned, rejectsIfAffected: true,
		start: stunStart, exit: refreshExit, ownStart: true, ownExit: true,
	},
	"AbortCast": {typ: TypeAbortCast, start: abortCastStart, ownStart: true},
	"ImmobileUntilAttacked": {
		typ: TypeImmobileUntilAttacked, flag: FlagMeditating,
		start: immobileUntilAttackedStart, action: immobileUntilAttackedAction, exit: immobileUntilAttackedExit,
		ownStart: true, ownExit: true,
	},
	"ImobileBuff": {
		typ:   TypeImmobilizeEffector,
		start: immobilizeEffectorStart, exit: immobilizeEffectorExit, ownStart: true, ownExit: true,
	},
	"Invincible":       {typ: TypeInvincible, start: invincibleStart, exit: invincibleExit},
	"ManaHealOverTime": {typ: TypeManaHealOverTime, action: manaHealOverTimeAction},
	"Mute": {
		typ: TypeMute, flag: FlagMuted,
		start: muteStart, exit: refreshExit, ownStart: true, ownExit: true,
	},
	"NoblesseBless": {typ: TypeNoblesseBless, flag: flagNoblesseBlessing, ownStart: true, ownExit: true},
	"Paralyze": {
		typ: TypeParalyze, flag: FlagParalyzed,
		start: paralyzeStart, exit: paralyzeExit, ownStart: true, ownExit: true,
	},
	"Petrification": {
		typ: TypePetrification, flag: FlagParalyzed,
		start: petrificationStart, exit: petrificationExit, ownStart: true, ownExit: true,
	},
	"PhysicalMute": {
		typ: TypePhysicalMute, flag: FlagPhysicalMuted,
		start: physicalMuteStart, exit: refreshExit, ownStart: true, ownExit: true,
	},
	"RemoveTarget": {typ: TypeRemoveTarget, start: removeTargetStart, ownStart: true, ownExit: true},
	"SilenceMagicPhysical": {
		typ: TypeSilenceAll, flag: FlagMuted | FlagPhysicalMuted,
		start: silenceAllStart, exit: refreshExit, ownStart: true, ownExit: true,
	},
	"SilentMove": {typ: TypeSilentMove, flag: FlagSilentMove, action: silentMoveAction},
	"StunSelf": {
		typ: TypeStunSelf, flag: FlagStunned, selfTarget: true,
		start: stunSelfStart, exit: stunSelfExit, ownStart: true, ownExit: true,
	},
	"Heal":            {typ: TypeHeal, start: healStart, ownStart: true},
	"HealOverTime":    {typ: TypeHealOverTime, start: healOverTimeStart, action: healOverTimeAction, ownStart: true},
	"ManaHeal":        {typ: TypeManaHeal, start: manaHealStart, ownStart: true},
	"IncreaseCharges": {typ: TypeIncreaseCharges, start: increaseChargesStart, ownStart: true},
	"TargetMe":        {typ: TypeTargetMe, start: targetMeStart, ownStart: true, ownExit: true},
	"Bluff":           {typ: TypeBluff, start: bluffStart, ownStart: true},
	"CharmOfCourage": {
		typ: TypeCharmOfCourage, flag: FlagCharmOfCourage,
		start: charmOfCourageStart, exit: charmOfCourageExit, ownStart: true, ownExit: true,
	},
	"CharmOfLuck": {
		typ: TypeCharmOfLuck, flag: FlagCharmOfLuck,
		exit: charmOfLuckExit, ownStart: true, ownExit: true,
	},
	"PhoenixBless": {
		typ: TypePhoenixBless, flag: FlagPhoenixBlessing,
		exit: phoenixBlessExit, ownStart: true, ownExit: true,
	},
	"BlockBuff":   {typ: TypeBlockBuff, ownStart: true},
	"BlockDebuff": {typ: TypeBlockDebuff},
	// Its start always fails: the blessing is held and flagged, but adds no
	// stats and runs no exit hook when it simply ends.
	"ProtectionBlessing": {
		typ: TypeProtectionBless, flag: FlagProtectionBlessing,
		start: declineStart, exit: protectionBlessExit, ownStart: true, ownExit: true,
	},
	"Recovery": {typ: TypeRecovery, start: recoveryStart, ownStart: true, ownExit: true},
	"Cancel":   {typ: TypeCancel, start: cancelStart, ownStart: true},
	"Negate":   {typ: TypeNegate, start: negateStart, ownStart: true},
	"Fusion":   {typ: TypeFusion, action: fusionAction},
	"ChanceSkillTrigger": {
		typ: TypeChanceSkillTrigger, start: chanceSkillTriggerStart, exit: chanceSkillTriggerExit,
	},
	"Spoil":               {typ: TypeSpoil, start: spoilStart, ownStart: true},
	"PolearmTargetSingle": {typ: TypePolearmTargetSingle},
	"BigHead": {
		typ: TypeBigHead, flag: flagBigHead,
		start: bigHeadStart, exit: bigHeadExit, ownStart: true, ownExit: true,
	},
	"CancelDebuff": {typ: TypeCancelDebuff, start: cancelDebuffStart, ownStart: true},
	"Relax":        {typ: TypeRelax, flag: flagRelaxing, start: relaxStart, action: relaxAction},
	"ChameleonRest": {
		typ: TypeChameleonRest, flag: FlagSilentMove | flagRelaxing,
		start: chameleonRestStart, action: chameleonRestAction,
	},
	"ImobilePetBuff": {
		typ:   TypeImmobilizePetBuff,
		start: immobilizePetBuffStart, exit: immobilizePetBuffExit, ownStart: true, ownExit: true,
	},
	"Distrust": {typ: TypeDistrust, start: distrustStart, ownStart: true, ownExit: true},
	"Confusion": {
		typ: TypeConfusion, flag: FlagConfused,
		start: confusionStart, exit: confusionExit, ownStart: true, ownExit: true,
	},
	"Betray": {
		typ: TypeBetray, flag: FlagBetrayed,
		start: betrayStart, exit: betrayExit, ownStart: true, ownExit: true,
	},
	"RandomizeHate": {typ: TypeRandomizeHate, start: randomizeHateStart, ownStart: true, ownExit: true},
	"ThrowUp": {
		typ: TypeThrowUp, flag: FlagStunned,
		start: throwUpStart, exit: throwUpExit, ownStart: true, ownExit: true,
	},
	"Grow": {typ: TypeGrow, start: growStart, exit: growExit, ownStart: true, ownExit: true},
	"FakeDeath": {
		typ: TypeFakeDeath, flag: FlagFakeDeath,
		start: fakeDeathStart, action: fakeDeathAction, exit: fakeDeathExit, ownStart: true, ownExit: true,
	},
	"Seed":             {typ: TypeSeed},
	"Signet":           signetGround,
	"SignetNoise":      signetGround,
	"SignetAntiSummon": signetGround,
	"SignetMDam":       signetGround,
	"ClanGate": {
		typ:   TypeClanGate,
		start: clanGateStart, exit: clanGateExit, ownStart: true, ownExit: true,
	},
}

// declineStart is the start hook of a kind whose start always fails.
func declineStart(*Effect) bool { return false }

func manaDamageOverTimeAction(e *Effect) bool { return manaDrain(e, e.Skill.Toggle) }

func bigHeadStart(e *Effect) bool {
	startAbnormalEffect(e.Effected, modelskill.AbnormalBigHead)
	return true
}

func bigHeadExit(e *Effect) { stopAbnormalEffect(e.Effected, modelskill.AbnormalBigHead) }

func clanGateStart(e *Effect) bool {
	startAbnormalEffect(e.Effected, modelskill.AbnormalMagicCircle)
	if opener, ok := e.Effected.(clanGateOpener); ok {
		opener.NotifyClanGateOpened()
	}
	return true
}

func clanGateExit(e *Effect) { stopAbnormalEffect(e.Effected, modelskill.AbnormalMagicCircle) }

// fearSparedPlayableSkillIDs are the skills whose fear never lands on a
// playable.
var fearSparedPlayableSkillIDs = map[modelskill.ID]bool{
	98:   true,
	1272: true,
	1381: true,
}

// FearSparesPlayables reports whether skill id's fear must not land on a
// player-controlled target even when its success roll passes. The fear
// effect refuses to start on one; the skill handlers apply the same set as a
// caster-side gate, and a PDAM among them skips playable targets outright.
func FearSparesPlayables(id modelskill.ID) bool { return fearSparedPlayableSkillIDs[id] }

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
	var stackType string
	if len(def.Effects) > 0 {
		stackType = def.Effects[0].StackType
	}
	return Skill{
		ID:                  def.ID,
		Level:               def.Level,
		Name:                def.Name,
		SkillType:           def.SkillType,
		StackType:           stackType,
		Debuff:              def.Debuff,
		Toggle:              def.Activation == modelskill.ActivationToggle,
		KillByDOT:           def.KillByDOT,
		Dance:               def.Dance,
		CanBeDispelled:      def.CanBeDispelled,
		StayAfterDeath:      def.StayAfterDeath,
		MagicLevel:          def.MagicLevel,
		LevelDepend:         def.LevelDepend,
		AbnormalLevel:       def.AbnormalLevel,
		EffectAbnormalLevel: def.Extra().EffectAbnormalLevel,
		EffectType:          def.LandingEffectType(),
		MaxNegatedEffects:   def.Extra().MaxNegatedEffects,
		NegateLevel:         def.Extra().NegateLevel,
		NegateIDs:           def.Extra().NegateIDs,
		NegateTypes:         def.Extra().NegateTypes,
		FlyRadius:           def.Extra().FlyRadius,
		Stat:                def.Extra().Stat,
	}
}

// Apply instantiates each of templates as a fresh live effect of effector on
// effected and hosts it where Attach places it. It rolls no landing chance
// and checks no landing gate; callers decide that first (see Lands). A
// template whose attached condition rejects the pair (see Admits) is
// skipped rather than failing the whole batch; a template naming no effect
// core never gets this far, since its skill or item fails to load (see
// Known).
func Apply(effector, effected Actor, meta Skill, templates []modelskill.EffectTemplate) {
	for _, tmpl := range templates {
		if !Admits(effector, effected, meta, tmpl) {
			continue
		}
		e, err := New(meta, tmpl)
		if err != nil {
			continue
		}
		Attach(e, effector, effected)
	}
}

// ApplyRestored instantiates each of templates and adds it to list, seeded to
// resume from count and elapsedSeconds — the tick count and time-since-last-
// tick a persisted effect had at logout — instead of starting fresh from the
// template: each is built, given its saved count and time, then scheduled.
// effector and effected are both the relogging character: the original
// caster identity is not persisted, so every reinstated effect is treated as
// self-applied, and a template whose attached condition rejects that pair
// (see Admits) is not reinstated. It activates and refreshes icons; inside a
// List.RestoreAnnounced it sends the stack system messages an add sends,
// otherwise it sends no stack or expiry system messages (see
// List.AddRestored).
//
// restoredAt, when set, is the instant the saved state was read back: each
// effect's schedule runs from there, not from the replay. One whose ticks
// run no action and whose first tick came due in between has ended and is
// not reinstated (see resumeRestored). One whose ticks run an action runs
// the actions of the ticks due in between, in time order with those of the
// other effects the same List.Restore reinstates (see List.Restore). A
// damage-over-time tick that may kill does too: a death it causes there
// runs on a character not yet in the world, whose owner keeps it silent.
func ApplyRestored(list *List, effector, effected Actor, meta Skill, templates []modelskill.EffectTemplate, count, elapsedSeconds int32, restoredAt time.Time) {
	if list == nil {
		return
	}
	list.Restore(func() {
		applyRestored(list, effector, effected, meta, templates, count, elapsedSeconds, restoredAt)
	})
}

func applyRestored(list *List, effector, effected Actor, meta Skill, templates []modelskill.EffectTemplate, count, elapsedSeconds int32, restoredAt time.Time) {
	var now time.Time
	if !restoredAt.IsZero() {
		now = list.now()
	}
	for _, tmpl := range templates {
		if !Admits(effector, effected, meta, tmpl) {
			continue
		}
		e, err := New(meta, tmpl)
		if err != nil {
			continue
		}
		if !restoredAt.IsZero() && e.OnAction == nil {
			if _, _, ok := resumeRestored(tmpl, count, elapsedSeconds, restoredAt, now); !ok {
				continue
			}
		}
		e.Effector = effector
		e.Effected = effected
		e.seedRestore(count, elapsedSeconds, restoredAt)
		list.AddRestored(e)
	}
}

// actsOnTick reports whether an effect built from tmpl has a tick action.
func actsOnTick(tmpl modelskill.EffectTemplate) bool {
	return coreKinds[tmpl.Name].action != nil
}

// Known reports whether New can build an effect named name. The match is
// exact. An <effect name> resolves to its implementation when the effect
// loads, and a name with none rejects the skill or item that carries it.
func Known(name string) bool {
	_, ok := coreKinds[name]
	return ok
}

// New builds a runtime effect from a parsed core effect template.
func New(skill Skill, tmpl modelskill.EffectTemplate) (*Effect, error) {
	k, ok := coreKinds[tmpl.Name]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnsupportedCoreEffect, tmpl.Name)
	}
	if k.typ == TypeChanceSkillTrigger {
		if tmpl.TriggeredID == 0 {
			return nil, fmt.Errorf("effect %s: requires triggeredId", tmpl.Name)
		}
		if tmpl.ChanceType == "" {
			return nil, fmt.Errorf("effect %s: requires chanceType", tmpl.Name)
		}
		if _, _, err := modelskill.ParseChanceCondition(tmpl.ChanceType, tmpl.ActivationChance); err != nil {
			return nil, fmt.Errorf("effect %s: %w", tmpl.Name, err)
		}
	}

	// A seed effect's Level is a charge counter (List.IncreasePower), not the
	// applied skill level: it always starts at 1, regardless of the skill
	// level.
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
		SelfTarget:        k.selfTarget,
		// A skill whose name contains "Herb" makes a herb effect:
		// a herb buff is identified by its owning skill's name, not by how it
		// was cast, so this applies uniformly to every application path.
		Herb: strings.Contains(skill.Name, "Herb"),
	}

	// tmpl.AttachCondition gates creating the effect itself (see Admits),
	// not its funcs.
	funcs, err := statFuncs(ModOwnerEffect(e), tmpl.Funcs, conditions.SkillOf(skill.Stat))
	if err != nil {
		return nil, fmt.Errorf("effect %s: %w", tmpl.Name, err)
	}
	e.Funcs = funcs
	e.OnStart, e.OnAction, e.OnExit = k.start, k.action, k.exit
	wireTemplateAbnormal(e, k)
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

// wireTemplateAbnormal chains the template's abnormal visual onto e's start
// and exit hooks, unless its kind k owns that hook outright.
func wireTemplateAbnormal(e *Effect, k kind) {
	mask := e.Template.AbnormalEffect
	if mask == 0 {
		return
	}
	if !k.ownStart {
		start := e.OnStart
		e.OnStart = func(e *Effect) bool {
			if start != nil && !start(e) {
				return false
			}
			startAbnormalEffect(e.Effected, mask)
			return true
		}
	}
	if !k.ownExit {
		exit := e.OnExit
		e.OnExit = func(e *Effect) {
			if exit != nil {
				exit(e)
			}
			stopAbnormalEffect(e.Effected, mask)
		}
	}
}

// iconLevel reports the level to send on this effect's abnormal-status
// icon. The icon always carries the skill's level, never a seed's grown
// power: a seed's Level field doubles as its charge counter, so its icon
// must read the fixed skill level instead, unlike every other kind whose
// Level is the applied skill level already.
func (e *Effect) iconLevel() int {
	if e.Type == TypeSeed {
		return e.Skill.Level
	}
	return e.Level
}
