package effect

import (
	"fmt"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
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
	flagSilentMove
	flagProtectionBlessing
	flagRelaxing
	// FlagFear marks a target as feared.
	FlagFear
	flagConfused
	flagMuted
	flagPhysicalMuted
	// FlagRooted marks a target as rooted.
	FlagRooted
	// FlagSleep marks a target as asleep.
	FlagSleep
	// FlagStunned marks a target as stunned.
	FlagStunned
	flagParalyzed
	flagMeditating
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
)

type kind struct {
	typ    Type
	flag   Flag
	debuff bool
}

var coreKinds = map[string]kind{
	"Buff":                  {typ: TypeBuff},
	"Debuff":                {typ: TypeDebuff, debuff: true},
	"DamOverTime":           {typ: TypeDamOverTime, debuff: true},
	"ManaDamOverTime":       {typ: TypeManaDamOverTime},
	"Fear":                  {typ: TypeFear, flag: FlagFear, debuff: true},
	"Root":                  {typ: TypeRoot, flag: FlagRooted, debuff: true},
	"Sleep":                 {typ: TypeSleep, flag: FlagSleep, debuff: true},
	"Stun":                  {typ: TypeStun, flag: FlagStunned, debuff: true},
	"AbortCast":             {typ: TypeAbortCast},
	"ImmobileUntilAttacked": {typ: TypeImmobileUntilAttacked, flag: flagMeditating},
	"ImobileBuff":           {typ: TypeImmobilizeEffector},
	"Invincible":            {typ: TypeInvincible},
	"ManaHealOverTime":      {typ: TypeManaHealOverTime},
	"Mute":                  {typ: TypeMute, flag: flagMuted, debuff: true},
	"NoblesseBless":         {typ: TypeNoblesseBless, flag: flagNoblesseBlessing},
	"Paralyze":              {typ: TypeParalyze, flag: flagParalyzed, debuff: true},
	"Petrification":         {typ: TypePetrification, flag: flagParalyzed, debuff: true},
	"PhysicalMute":          {typ: TypePhysicalMute, flag: flagPhysicalMuted, debuff: true},
	"RemoveTarget":          {typ: TypeRemoveTarget},
	"SilenceMagicPhysical":  {typ: TypeSilenceAll, flag: flagMuted | flagPhysicalMuted, debuff: true},
	"SilentMove":            {typ: TypeSilentMove, flag: flagSilentMove},
	"StunSelf":              {typ: TypeStunSelf, flag: FlagStunned},
}

var fearSkippedPlayableSkillIDs = map[modelskill.ID]bool{
	98:   true,
	1272: true,
	1381: true,
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
	if k.flag == 0 {
		k.flag = FlagNone
	}

	skill.Debuff = skill.Debuff || k.debuff
	e := &Effect{
		Skill:    skill,
		Template: tmpl,
		Type:     k.typ,
		Flag:     k.flag,
	}

	funcs, err := statFuncs(e, tmpl.Funcs)
	if err != nil {
		return nil, fmt.Errorf("effect %s: %w", tmpl.Name, err)
	}
	e.Funcs = funcs
	wireHooks(e)
	return e, nil
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
// be toggled.
type immobilizeTarget interface {
	SetImmobilized(bool)
}

// manaHealTarget is implemented by an actor whose mana pool can be checked
// for healing eligibility and restored.
type manaHealTarget interface {
	CanBeHealed() bool
	AddMP(amount float64)
}

// skillIDEffectStopper is implemented by an actor whose active effects can
// be stopped by owning skill id.
type skillIDEffectStopper interface {
	StopSkillEffectsByID(id modelskill.ID)
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
