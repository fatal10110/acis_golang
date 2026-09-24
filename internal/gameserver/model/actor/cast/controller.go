// Package cast models the shared skill-cast lifecycle for live creatures.
package cast

import (
	"errors"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/rs/zerolog"
)

var (
	// ErrInvalidTarget means a cast was requested without a target.
	ErrInvalidTarget = errors.New("cast: invalid target")
	// ErrAlreadyCasting means the actor already has an active cast.
	ErrAlreadyCasting = errors.New("cast: already casting")
	// ErrNotCasting means a cast phase was requested while no cast is active.
	ErrNotCasting = errors.New("cast: not casting")
	// ErrSkillDisabled means the skill's reuse key is still cooling down.
	ErrSkillDisabled = errors.New("cast: skill disabled")
	// ErrNotEnoughMP means the actor cannot pay the current MP cost.
	ErrNotEnoughMP = errors.New("cast: not enough mp")
	// ErrNotEnoughHP means the actor cannot pay the current HP cost.
	ErrNotEnoughHP = errors.New("cast: not enough hp")
	// ErrNotEnoughItems means the actor cannot pay the required item cost.
	ErrNotEnoughItems = errors.New("cast: not enough items")
	// ErrMagicMuted means the actor is blocked from magic casts.
	ErrMagicMuted = errors.New("cast: magic muted")
	// ErrPhysicalMuted means the actor is blocked from physical casts.
	ErrPhysicalMuted = errors.New("cast: physical muted")
	// ErrSkillUnavailable means a player cast request did not name a known
	// active skill.
	ErrSkillUnavailable = errors.New("cast: skill unavailable")
	// ErrCubicListFull means a self-targeted cubic-granting skill was cast
	// while the caster already holds as many cubics as Cubic Mastery
	// allows.
	ErrCubicListFull = errors.New("cast: cubic list full")
	// ErrAllSkillsDisabled means the actor is under a blanket skill lock
	// (crowd control, or Java's Duel-defeat lock once that lands).
	ErrAllSkillsDisabled = errors.New("cast: all skills disabled")
	// ErrGroundTargetUnset means a GROUND skill was requested before any
	// RequestExMagicSkillUseGround recorded a signet point, matching
	// PlayerCast.canAttemptCast's Location.DUMMY_LOC rejection
	// (PlayerCast.java:224).
	ErrGroundTargetUnset = errors.New("cast: ground target unset")
)

// Actor is the owner state a cast controller reads and updates while
// validating and advancing casts. Status implementations own stat
// calculation; the controller only consumes already-resolved costs, speeds,
// reuse rates and resource totals.
type Actor interface {
	AttackSpeed(magic bool) int
	ReuseRate(magic bool) float64

	MP() int
	HP() int
	MPInitialCost(modelskill.Definition) int
	MPCost(modelskill.Definition) int
	ReduceMP(int)
	ReduceHP(int)

	SkillDisabled(key int32) bool
	DisableSkill(key int32, delay time.Duration)
	AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration)

	MagicMuted() bool
	PhysicalMuted() bool
	SpiritshotCharged() bool
	BlessedSpiritshotCharged() bool
	SkillMastery(modelskill.Definition) bool

	ItemCount(itemID int) int
	ConsumeItem(itemID, count int) bool

	// CubicListFull reports whether a self-targeted cubic-granting skill must
	// be refused because the caster already holds its maximum cubics.
	CubicListFull() bool
	// ExitSignetGround drops the caster's live ground-signet effect on abort.
	ExitSignetGround()
	// AllSkillsDisabled reports the blanket skill lock CanCast gates on;
	// EnableAllSkills lifts it on abort.
	AllSkillsDisabled() bool
	EnableAllSkills()
	// GroundTargetUnset reports whether a ground-targeted cast has no signet
	// point set. Only players hold one; other casters never block.
	GroundTargetUnset() bool
	// IncreaseCharges and DecreaseCharges apply a skill's Force/Soul charges
	// at hit time; only players carry charges.
	IncreaseCharges(count, max int) bool
	DecreaseCharges(count int) bool
}

// Plan is the timing and reuse state for one accepted cast. Durations are
// measured from cast start unless the field name says otherwise.
type Plan struct {
	HitTime        time.Duration
	CoolTime       time.Duration
	ReuseDelay     time.Duration
	LaunchDelay    time.Duration
	HitDelay       time.Duration
	FinalDelay     time.Duration
	InterruptAfter time.Duration
	GaugeDuration  time.Duration
	ReuseKey       int32
	SkillMastery   bool
}

// DamageInterrupt is the state needed to decide whether incoming damage
// interrupts the current cast.
type DamageInterrupt struct {
	Damage       float64
	MEN          int
	AttackCancel func(float64) float64
	Roll         int
	Immune       bool
	Fusion       bool
}

// scheduledTimer is the subset of *time.Timer the delayed cast scheduler
// needs, narrow enough for tests to substitute a fake clock.
type scheduledTimer interface {
	Stop() bool
}

// afterFunc matches time.AfterFunc's signature, injectable for deterministic
// tests.
type afterFunc func(time.Duration, func()) scheduledTimer

// Controller coordinates validation, resource consumption, cooldowns and
// interruption state for one actor's active cast.
//
// mu guards every mutable field below, including the scheduled timers
// Schedule installs. It is never held while the actor pays a cast cost
// (item, reuse, MP, HP, charges): paying one can end the cast, which calls
// back into the controller.
type Controller struct {
	actor Actor

	mu             sync.RWMutex
	casting        bool
	current        modelskill.Definition
	target         Target
	plan           Plan
	startedAt      time.Time
	interruptUntil time.Time

	// castSeq increments every time the active cast is cleared (Stop,
	// Finish, or the start of a fresh cast), so a scheduled Launch/Hit/
	// Finish callback belonging to a superseded cast can recognize itself
	// as stale and no-op instead of acting on the wrong cast.
	castSeq   uint64
	timers    []scheduledTimer
	fusionEnd func()
	// finishHolds counts the outstanding HoldFinish grants for the active
	// cast, and finishArm is the Finish arming Hit deferred because of
	// them. clearLocked drops both with the rest of the cast state, so a
	// stopped or superseded cast never arms a Finish on a later one's
	// behalf.
	finishHolds int
	finishArm   func()
	afterFunc   afterFunc
	sink        event.Sink
	log         zerolog.Logger
}

// NewController returns a cast controller for actor. sink receives, each
// after the controller's lock is released so it may call back in:
//   - CastAborted, once whenever a cast that was actually in flight is
//     aborted (never for a natural finish, nor for a stop on an idle
//     controller); Interrupted reports the window-gated Interrupt path,
//     which additionally owes CASTING_INTERRUPTED, rather than an
//     unconditional Stop;
//   - CastStopAck, once on every Stop/Interrupt call whether or not a cast
//     was in flight, matching PlayerCast.stop()'s unconditional
//     _actor.getAI().clientActionFailed() (PlayerCast.java:381-387) that
//     runs after super.stop()'s isCastingNow()-gated cancel broadcast;
//   - CastFinished, once whenever an in-flight cast ends, aborted or
//     completed, letting the owner apply the nextActionAttack resume gate
//     (PlayableAI.onEvtFinishedCasting, PlayableAI.java:43-63).
//
// A nil sink drops all three.
func NewController(actor Actor, sink event.Sink) *Controller {
	return &Controller{actor: actor, sink: sink}
}

func (c *Controller) emit(e event.Event) {
	if c.sink != nil {
		c.sink.Emit(e)
	}
}

// SetQueue runs the controller's scheduled launch, hit and finish callbacks
// as tasks on q, the owning actor's queue.
func (c *Controller) SetQueue(q *sim.Queue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.afterFunc = func(d time.Duration, fn func()) scheduledTimer { return q.After(d, fn) }
}

// SetLogger records where a panic recovered from a scheduled cast callback
// (Launch/Hit/Finish) is logged. The zero value discards it.
func (c *Controller) SetLogger(log zerolog.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = log
}

// HoldFinish keeps the active cast's Finish phase from being armed until
// the returned release is called, and returns a no-op release when no cast
// is in flight. Hit still runs on time; only the transition out of the cast
// waits.
//
// It exists for a Hit-phase effect whose own work cannot finish inside the
// Hit task — one that has to leave the actor's queue and come back, such as
// a summon that must read its saved state. The reference does that work
// synchronously inside the skill handler and schedules its finalizer only
// afterwards, so the actor stays in its cast for the duration; a held Finish
// is how that shape survives the work moving off the queue.
//
// Release is idempotent and safe from any goroutine. Releasing after the
// cast was stopped, interrupted or superseded does nothing: the grant is
// bound to the cast that was active when it was taken. The caller owns the
// release and must run it on every path, including its own failures — an
// unreleased hold leaves the actor casting until something else stops it.
func (c *Controller) HoldFinish() func() {
	c.mu.Lock()
	if !c.casting {
		c.mu.Unlock()
		return func() {}
	}
	seq := c.castSeq
	c.finishHolds++
	c.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { c.releaseFinish(seq) }) }
}

// releaseFinish drops one hold taken for the cast identified by seq and arms
// the Finish that Hit deferred once the last one is gone.
func (c *Controller) releaseFinish(seq uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.castingLocked(seq) || c.finishHolds == 0 {
		return
	}
	c.finishHolds--
	if c.finishHolds > 0 || c.finishArm == nil {
		return
	}
	arm := c.finishArm
	c.finishArm = nil
	arm()
}

// CastingNow reports whether the actor currently has an active cast.
func (c *Controller) CastingNow() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.casting
}

// CurrentSkill returns the active skill definition and whether a cast is
// active.
func (c *Controller) CurrentSkill() (modelskill.Definition, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.casting {
		return modelskill.Definition{}, false
	}
	return c.current, true
}

// CanAttemptCast is the pre-movement gate: already casting, every skill
// disabled, and per-skill reuse. Callers that stop a walk for a long hit
// time must run this first so those rejections leave movement alone.
func (c *Controller) CanAttemptCast(target Target, def modelskill.Definition) error {
	if c.actor == nil || target == nil {
		return ErrInvalidTarget
	}
	if c.CastingNow() {
		return ErrAlreadyCasting
	}
	if c.actor.AllSkillsDisabled() {
		return ErrAllSkillsDisabled
	}
	if c.actor.SkillDisabled(ReuseKey(def)) {
		return ErrSkillDisabled
	}
	if def.Target == modelskill.TargetGround {
		if c.actor.GroundTargetUnset() {
			return ErrGroundTargetUnset
		}
	}
	return nil
}

// CanCast validates the reusable pre-cast checks for target, reuse, current
// MP/HP, mute state, and required skill items.
func (c *Controller) CanCast(target Target, def modelskill.Definition) error {
	if c.actor == nil || target == nil {
		return ErrInvalidTarget
	}
	if c.actor.AllSkillsDisabled() {
		return ErrAllSkillsDisabled
	}
	key := ReuseKey(def)
	if c.actor.SkillDisabled(key) {
		return ErrSkillDisabled
	}
	initialMP := c.actor.MPInitialCost(def)
	mp := c.actor.MPCost(def)
	if (initialMP > 0 || mp > 0) && c.actor.MP() < initialMP+mp {
		return ErrNotEnoughMP
	}
	if def.HPConsume > 0 && c.actor.HP() <= def.HPConsume {
		return ErrNotEnoughHP
	}
	if def.Magic {
		if c.actor.MagicMuted() {
			return ErrMagicMuted
		}
	} else if c.actor.PhysicalMuted() {
		return ErrPhysicalMuted
	}
	if def.SkillType == "SUMMON" && def.IsCubic && def.Target == modelskill.TargetSelf {
		if c.actor.CubicListFull() {
			return ErrCubicListFull
		}
	}
	if def.ItemConsumeID > 0 && def.ItemConsumeCount > 0 && c.actor.ItemCount(def.ItemConsumeID) < def.ItemConsumeCount {
		return ErrNotEnoughItems
	}
	return nil
}

// MeetsHPMPDisabled checks HP, MP, and mute gates for target and def. It
// does not apply reuse, item, or line-of-sight checks; those belong to
// CanCast at commit time.
func (c *Controller) MeetsHPMPDisabled(target Target, def modelskill.Definition) error {
	if c.actor == nil || target == nil {
		return ErrInvalidTarget
	}
	initialMP := c.actor.MPInitialCost(def)
	mp := c.actor.MPCost(def)
	if (initialMP > 0 || mp > 0) && c.actor.MP() < initialMP+mp {
		return ErrNotEnoughMP
	}
	if def.HPConsume > 0 && c.actor.HP() <= def.HPConsume {
		return ErrNotEnoughHP
	}
	if def.Magic {
		if c.actor.MagicMuted() {
			return ErrMagicMuted
		}
	} else if c.actor.PhysicalMuted() {
		return ErrPhysicalMuted
	}
	return nil
}

// Start accepts a cast, applies the start-of-cast costs and cooldowns, and
// stores the active cast state. The caller owns scheduling Launch, Hit and
// Finish according to the returned Plan. A cost that ends the cast by calling
// back into Stop leaves the costs charged — the reference charges reuse and
// the initial MP before it claims the cast, and charges the skill item after
// it — and Start reports ErrNotCasting so the caller does not announce a cast
// that is already cancelled.
func (c *Controller) Start(now time.Time, target Target, def modelskill.Definition) (Plan, error) {
	if err := c.CanCast(target, def); err != nil {
		return Plan{}, err
	}

	c.mu.Lock()
	if c.casting {
		c.mu.Unlock()
		return Plan{}, ErrAlreadyCasting
	}
	plan := c.buildPlan(def)
	c.casting = true
	c.current = def
	c.target = target
	c.plan = plan
	c.startedAt = now
	c.interruptUntil = now.Add(plan.InterruptAfter)
	seq := c.castSeq
	c.mu.Unlock()

	// The cast is claimed above so a concurrent Start is rejected; a failed
	// item consume releases the claim unless the cast was already ended.
	//
	// Deliberate divergence from the reference (issue #2336): the reference
	// destroys the consume item after claiming the cast and ignores a failed
	// destroy, so a race that empties the item mid-cast still casts uncharged.
	// CanCast already verified the count here, so ConsumeItem can only fail on
	// that same narrow race; releasing the claim and rejecting the cast in
	// that case is preferred over silently casting an unpaid skill.
	//
	// The ItemConsumeCount > 0 half of this guard (and CanCast's matching
	// check) is a second, separate divergence: the reference gates solely on
	// ItemConsumeID > 0, so a skill shipping ItemConsumeCount == 0 with a real
	// ItemConsumeID (e.g. skills 2234, 2276) still runs the reference's
	// destroy/reject path there. Go skips the gate entirely for such a skill
	// instead of destroying zero units and reporting success/failure for an
	// item it never touched — again preferred over reproducing that
	// zero-count side effect.
	if def.ItemConsumeID > 0 && def.ItemConsumeCount > 0 && !c.actor.ConsumeItem(def.ItemConsumeID, def.ItemConsumeCount) {
		c.mu.Lock()
		if c.castingLocked(seq) {
			c.clearLocked()
		}
		c.mu.Unlock()
		return Plan{}, ErrNotEnoughItems
	}

	if !plan.SkillMastery {
		if plan.ReuseDelay > 30*time.Second {
			c.actor.AddSkillReuse(modelskill.Ref{ID: def.ID, Level: def.Level}, plan.ReuseKey, plan.ReuseDelay)
		}
		if plan.ReuseDelay > 10*time.Millisecond {
			c.actor.DisableSkill(plan.ReuseKey, plan.ReuseDelay)
		}
	}

	if initialMP := c.actor.MPInitialCost(def); initialMP > 0 {
		c.actor.ReduceMP(initialMP)
	}

	// A Stop that ended the cast has already acknowledged the client's
	// pending action, so the caller's rejection acknowledges it a second
	// time. That is deliberate: the acknowledgement only releases the
	// client's action lock, and skipping it here would leave that lock held
	// whenever the cast ended through Finish rather than Stop.
	c.mu.RLock()
	claimed := c.castingLocked(seq)
	c.mu.RUnlock()
	if !claimed {
		return Plan{}, ErrNotCasting
	}
	return plan, nil
}

// Hit applies the final MP and HP costs for the active cast. It leaves an
// unaffordable cast in flight for the caller to abort through Stop, so the
// caller can report why the cast failed before the abort funnel cancels it
// — the packet order the reference produces. A lethal HP cost may stop the
// cast from inside ReduceHP; Hit still reports success for the cost paid.
func (c *Controller) Hit() error {
	def, casting := c.CurrentSkill()
	if !casting {
		return ErrNotCasting
	}

	if mp := c.actor.MPCost(def); mp > 0 {
		if mp > c.actor.MP() {
			return ErrNotEnoughMP
		}
		c.actor.ReduceMP(mp)
	}

	if hp := def.HPConsume; hp > 0 {
		if hp > c.actor.HP() {
			return ErrNotEnoughHP
		}
		c.actor.ReduceHP(hp)
	}

	// Force/Soul charge apply, matching CreatureCast.onMagicHitTimer
	// (CreatureCast.java:276-282): runs after the MP/HP consume above and
	// before the caller's Hooks.Hit applies the skill's effects.
	if def.NumCharges > 0 {
		if def.MaxCharges > 0 {
			c.actor.IncreaseCharges(def.NumCharges, def.MaxCharges)
		} else {
			c.actor.DecreaseCharges(def.NumCharges)
		}
	}
	return nil
}

// Finish clears the active cast after its hit and cool phases complete.
func (c *Controller) Finish() {
	c.mu.Lock()
	finish := c.finishLocked()
	c.mu.Unlock()
	if finish != nil {
		finish(false)
	}
}

// Stop aborts and clears the active cast. It is the single funnel every
// abort reason passes through, which is what lets "abort for any reason"
// behave uniformly without each call site enumerating its own cleanup.
//
// The two owner-state steps and the stop-ack observer run unconditionally,
// as the reference does them ahead of (owner state) or regardless of
// (clientActionFailed) its own casting check; only CastAborted is
// reserved for a cast that was really in flight.
func (c *Controller) Stop() {
	c.stopInternal(false)
}

func (c *Controller) stopInternal(interrupted bool) {
	c.exitSignetGround()
	c.enableAllSkills()

	c.mu.Lock()
	abort, finish := c.abortLocked()
	c.mu.Unlock()
	if abort != nil {
		abort(interrupted)
	}
	c.emit(event.CastStopAck{})
	if finish != nil {
		finish(true)
	}
}

// abortLocked clears the cast and returns the observer the caller must run
// once it has released mu, or nil when no cast was in flight.
func (c *Controller) abortLocked() (func(bool), func(bool)) {
	aborted := c.casting
	current := c.current
	fusionEnd := c.fusionEnd
	c.clearLocked()
	if !aborted {
		return nil, nil
	}
	return func(interrupted bool) {
			if fusionEnd != nil {
				fusionEnd()
			}
			c.emit(event.CastAborted{Interrupted: interrupted})
		}, func(interrupted bool) {
			c.emit(event.CastFinished{Interrupted: interrupted, Skill: current})
		}
}

func (c *Controller) finishLocked() func(bool) {
	if !c.casting {
		return nil
	}
	current := c.current
	fusionEnd := c.fusionEnd
	c.clearLocked()
	return func(aborted bool) {
		if fusionEnd != nil {
			fusionEnd()
		}
		c.emit(event.CastFinished{Interrupted: aborted, Skill: current})
	}
}

func (c *Controller) exitSignetGround() {
	c.actor.ExitSignetGround()
}

func (c *Controller) enableAllSkills() {
	if c.actor.AllSkillsDisabled() {
		c.actor.EnableAllSkills()
	}
}

// CanAbort reports whether an active cast is still inside its interrupt
// window at now.
func (c *Controller) CanAbort(now time.Time) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.casting && now.Before(c.interruptUntil)
}

// Interrupt aborts the current cast if it is still inside its interrupt
// window. It reports whether the cast was aborted.
func (c *Controller) Interrupt(now time.Time) bool {
	if !c.CanAbort(now) {
		return false
	}
	c.stopInternal(true)
	return true
}

// InterruptCast aborts the current cast if it is still inside its interrupt
// window, for callers that don't already hold `now` — the effect-driven
// abort-cast surface (castInterrupter) uses this.
func (c *Controller) InterruptCast() {
	c.Interrupt(time.Now())
}

// StopCast aborts the current cast unconditionally, matching the
// effect-driven abort surfaces (mute, silence, remove-target) that don't
// gate on the interrupt window or send CASTING_INTERRUPTED.
func (c *Controller) StopCast() {
	c.Stop()
}

// CanAbortCast reports whether the active cast is still inside its
// interrupt window, for callers that don't already hold `now` — the Esc
// cast-cancel path (RequestTargetCancel.java:26, only fires
// AiEventType.CANCEL when canAbortCast() is true) uses this to decide
// whether onEvtCancel's unconditional stop() applies at all.
func (c *Controller) CanAbortCast() bool {
	return c.CanAbort(time.Now())
}

// CurrentSkillIsMagic reports whether the active cast's skill is a magic
// skill, letting a caller distinguish Mute (magic-only) from PhysicalMute
// (physical-only) without holding the skill definition itself.
func (c *Controller) CurrentSkillIsMagic() bool {
	def, casting := c.CurrentSkill()
	return casting && def.Magic
}

// InterruptCastOnDamage applies the damage-based cast-break rule
// (Formulas.calcCastBreak) to the active cast using time.Now(), for callers
// outside this package that don't hold a DamageInterrupt value already.
// Fusion reflects whether the active cast is a FUSION skill
// (target.getFusionSkill() != null in Formulas.calcCastBreak, Formulas.java:732),
// which unconditionally interrupts on any damage — no rate roll, no MEN. It
// is read from c.current (set atomically with c.casting at Start) rather
// than c.fusionEnd, which ScheduleFusion only sets a few statements later —
// using fusionEnd would leave a window between cast start and channel
// registration where damage on a live FUSION cast reads as non-fusion.
func (c *Controller) InterruptCastOnDamage(damage float64, men int, attackCancel func(float64) float64, roll int, immune bool) bool {
	c.mu.RLock()
	fusion := c.casting && c.current.SkillType == "FUSION"
	c.mu.RUnlock()
	return c.InterruptOnDamage(time.Now(), DamageInterrupt{
		Damage:       damage,
		MEN:          men,
		AttackCancel: attackCancel,
		Roll:         roll,
		Immune:       immune,
		Fusion:       fusion,
	})
}

// InterruptOnDamage applies the damage-based magic cast break rule to the
// active cast. It reports whether the cast was aborted.
func (c *Controller) InterruptOnDamage(now time.Time, d DamageInterrupt) bool {
	if d.Immune {
		return false
	}
	if d.Fusion {
		return c.Interrupt(now)
	}

	c.mu.RLock()
	casting := c.casting
	magic := c.current.Magic
	c.mu.RUnlock()
	if !casting || !magic {
		return false
	}

	rate := formulas.CastBreakRate(d.Damage, d.MEN, d.AttackCancel)
	if !formulas.CastBreaks(rate, d.Roll) {
		return false
	}
	return c.Interrupt(now)
}

// SkillOnCooldown reports whether def's reuse key is still cooling down. It
// is the lightweight pre-movement cast gate an AI loop checks before
// committing to close distance on a target, ahead of the fuller CanCast
// check run immediately before the cast itself starts.
func (c *Controller) SkillOnCooldown(def modelskill.Definition) bool {
	if c.actor == nil {
		return false
	}
	return c.actor.SkillDisabled(ReuseKey(def))
}

// ReuseKey returns the cooldown key for def, using a shared-reuse reference
// when one is configured.
func ReuseKey(def modelskill.Definition) int32 {
	ref := modelskill.Ref{ID: def.ID, Level: def.Level}
	if def.SharedReuse != nil {
		ref = *def.SharedReuse
	}
	return int32(ref.ID)*256 + int32(ref.Level)
}

func (c *Controller) buildPlan(def modelskill.Definition) Plan {
	// PlayerCast.doFusionCast (PlayerCast.java:75-76,52) reads
	// skill.getHitTime()/getCoolTime()/getReuseDelay() raw, with no atkSpd,
	// spiritshot, or reuse-rate scaling applied — channel length, gauge,
	// interrupt window, and reuse are fixed for every caster regardless of
	// attack speed.
	fusion := def.SkillType == "FUSION"

	hitTime := def.HitTime
	coolTime := def.CoolTime
	if !def.StaticHitTime && !fusion {
		hitTime = formulas.AtkSpd(def.Magic, positive(c.actor.AttackSpeed(true)), positive(c.actor.AttackSpeed(false)), float64(hitTime))
		if coolTime > 0 {
			coolTime = formulas.AtkSpd(def.Magic, positive(c.actor.AttackSpeed(true)), positive(c.actor.AttackSpeed(false)), float64(coolTime))
		}
		if def.Magic && (c.actor.SpiritshotCharged() || c.actor.BlessedSpiritshotCharged()) {
			hitTime = int(0.70 * float64(hitTime))
			coolTime = int(0.70 * float64(coolTime))
		}
		if def.HitTime >= 500 && hitTime < 500 {
			hitTime = 500
		}
	}

	reuseDelay := def.ReuseDelay
	if !def.StaticReuse && !fusion {
		reuseDelay = int(float64(reuseDelay) * c.actor.ReuseRate(def.Magic))
		reuseDelay = int(float64(reuseDelay) * 333.0 / float64(positive(c.actor.AttackSpeed(def.Magic))))
	}

	plan := Plan{
		HitTime:        ms(hitTime),
		CoolTime:       ms(coolTime),
		ReuseDelay:     ms(reuseDelay),
		InterruptAfter: ms(hitTime - 200),
		ReuseKey:       ReuseKey(def),
		SkillMastery:   c.actor.SkillMastery(def),
	}
	if hitTime > 410 {
		plan.LaunchDelay = ms(hitTime - 400)
		plan.HitDelay = 400 * time.Millisecond
		plan.GaugeDuration = plan.HitTime
		if coolTime > 0 {
			plan.FinalDelay = plan.CoolTime
		}
	}
	return plan
}

// claimToggle marks an instantaneous toggle cast as in flight so the abort
// funnel has a cast to report, and returns the sequence the release must
// match. It reports false when a cast is already in flight, which keeps its
// own claim and its own abort.
func (c *Controller) claimToggle(def modelskill.Definition) (uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.casting {
		return c.castSeq, false
	}
	c.casting = true
	c.current = def
	return c.castSeq, true
}

// releaseToggle drops a claim taken by claimToggle without emitting the
// abort or finish observers, leaving a toggle whose costs were paid
// uneventfully exactly as silent as it was before the claim existed. A
// claim already ended — by a lethal cost aborting through Stop — has moved
// the sequence on and is left alone.
func (c *Controller) releaseToggle(seq uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.castingLocked(seq) {
		c.clearLocked()
	}
}

func (c *Controller) clearLocked() {
	c.stopTimersLocked()
	c.finishHolds = 0
	c.finishArm = nil
	c.castSeq++
	c.casting = false
	c.current = modelskill.Definition{}
	c.target = nil
	c.plan = Plan{}
	c.startedAt = time.Time{}
	c.interruptUntil = time.Time{}
	c.fusionEnd = nil
}

func (c *Controller) stopTimersLocked() {
	for _, t := range c.timers {
		t.Stop()
	}
	c.timers = nil
}

func (c *Controller) scheduleLocked(delay time.Duration, f func()) {
	source := c.afterFunc
	if source == nil { // no queue: only unit tests build one this way
		source = func(d time.Duration, fn func()) scheduledTimer { return sim.AfterOr(nil, d, fn, c.log) }
	}
	c.timers = append(c.timers, source(delay, f))
}

func positive(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func ms(n int) time.Duration {
	return time.Duration(n) * time.Millisecond
}
