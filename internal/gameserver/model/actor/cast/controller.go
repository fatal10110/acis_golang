// Package cast models the shared skill-cast lifecycle for live creatures.
package cast

import (
	"errors"
	"sync"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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
)

// cubicLister is the narrow surface a self-targeted cubic-granting skill
// checks before casting is allowed to start at all — matching the
// reference's CubicList.isFull() gate in L2SkillSummon.checkCondition. A
// mass-cubic skill (target type other than SELF) skips this gate entirely;
// each recipient's own list silently evicts its oldest cubic instead, once
// the skill actually applies its effect.
type cubicLister interface {
	CubicListFull() bool
}

// signetGroundExiter is the optional owner surface an abort uses to drop a
// live ground-signet effect, matching the reference exiting the actor's
// first SIGNET_GROUND effect on every stop. An owner that cannot hold one
// simply doesn't implement it.
type signetGroundExiter interface {
	ExitSignetGround()
}

// allSkillsDisabler is the optional owner surface for the blanket
// skill-lock an abort lifts. No owner implements it yet: nothing in the
// port installs the lock in the first place, so the funnel below is wired
// but inert until that state exists.
type allSkillsDisabler interface {
	AllSkillsDisabled() bool
	EnableAllSkills()
}

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
// Schedule installs.
type Controller struct {
	actor Actor

	mu             sync.RWMutex
	casting        bool
	current        modelskill.Definition
	target         any
	plan           Plan
	startedAt      time.Time
	interruptUntil time.Time

	// castSeq increments every time the active cast is cleared (Stop,
	// Finish, or the start of a fresh cast), so a scheduled Launch/Hit/
	// Finish callback belonging to a superseded cast can recognize itself
	// as stale and no-op instead of acting on the wrong cast.
	castSeq   uint64
	timers    []scheduledTimer
	afterFunc afterFunc
	onAbort   func()
	log       zerolog.Logger
}

// NewController returns a cast controller for actor.
func NewController(actor Actor) *Controller {
	return &Controller{actor: actor}
}

// SetLogger records where a panic recovered from a scheduled cast callback
// (Launch/Hit/Finish) is logged. The zero value discards it.
func (c *Controller) SetLogger(log zerolog.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = log
}

// SetOnAbort registers the observer fired once whenever a cast that was
// actually in flight is aborted, so the owner can tell the client the cast
// was cancelled and react to the interruption. It never fires for a natural
// Finish, nor for a Stop on an idle controller. The observer runs after the
// controller's lock is released, so it may call back into the controller.
func (c *Controller) SetOnAbort(f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onAbort = f
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

// CanCast validates the reusable pre-cast checks for target, reuse, current
// MP/HP, mute state, and required skill items.
func (c *Controller) CanCast(target any, def modelskill.Definition) error {
	if c.actor == nil || target == nil {
		return ErrInvalidTarget
	}
	key := ReuseKey(def)
	if c.actor.SkillDisabled(key) {
		return ErrSkillDisabled
	}
	if def.SkillType == "SUMMON" && def.IsCubic && def.Target == modelskill.TargetSelf {
		if lister, ok := c.actor.(cubicLister); ok && lister.CubicListFull() {
			return ErrCubicListFull
		}
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
	if def.ItemConsumeID > 0 && def.ItemConsumeCount > 0 && c.actor.ItemCount(def.ItemConsumeID) < def.ItemConsumeCount {
		return ErrNotEnoughItems
	}
	return nil
}

// Start accepts a cast, applies the start-of-cast costs and cooldowns, and
// stores the active cast state. The caller owns scheduling Launch, Hit and
// Finish according to the returned Plan.
func (c *Controller) Start(now time.Time, target any, def modelskill.Definition) (Plan, error) {
	if err := c.CanCast(target, def); err != nil {
		return Plan{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.casting {
		return Plan{}, ErrAlreadyCasting
	}

	plan := c.buildPlan(def)
	if def.ItemConsumeID > 0 && def.ItemConsumeCount > 0 && !c.actor.ConsumeItem(def.ItemConsumeID, def.ItemConsumeCount) {
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

	c.casting = true
	c.current = def
	c.target = target
	c.plan = plan
	c.startedAt = now
	c.interruptUntil = now.Add(plan.InterruptAfter)
	return plan, nil
}

// Hit applies the final MP and HP costs for the active cast. It leaves an
// unaffordable cast in flight for the caller to abort through Stop, so the
// caller can report why the cast failed before the abort funnel cancels it
// — the packet order the reference produces.
func (c *Controller) Hit() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hitLocked()
}

func (c *Controller) hitLocked() error {
	if !c.casting {
		return ErrNotCasting
	}

	if mp := c.actor.MPCost(c.current); mp > 0 {
		if mp > c.actor.MP() {
			return ErrNotEnoughMP
		}
		c.actor.ReduceMP(mp)
	}

	if hp := c.current.HPConsume; hp > 0 {
		if hp > c.actor.HP() {
			return ErrNotEnoughHP
		}
		c.actor.ReduceHP(hp)
	}
	return nil
}

// Finish clears the active cast after its hit and cool phases complete.
func (c *Controller) Finish() {
	c.mu.Lock()
	c.clearLocked()
	c.mu.Unlock()
}

// Stop aborts and clears the active cast. It is the single funnel every
// abort reason passes through, which is what lets "abort for any reason"
// behave uniformly without each call site enumerating its own cleanup.
//
// The two owner-state steps run unconditionally, as the reference does them
// ahead of its own casting check; only the abort observer is reserved for a
// cast that was really in flight.
func (c *Controller) Stop() {
	c.exitSignetGround()
	c.enableAllSkills()

	c.mu.Lock()
	abort := c.abortLocked()
	c.mu.Unlock()
	if abort != nil {
		abort()
	}
}

// abortLocked clears the cast and returns the observer the caller must run
// once it has released mu, or nil when no cast was in flight.
func (c *Controller) abortLocked() func() {
	aborted := c.casting
	c.clearLocked()
	if !aborted {
		return nil
	}
	return c.onAbort
}

func (c *Controller) exitSignetGround() {
	if s, ok := c.actor.(signetGroundExiter); ok {
		s.ExitSignetGround()
	}
}

func (c *Controller) enableAllSkills() {
	if d, ok := c.actor.(allSkillsDisabler); ok && d.AllSkillsDisabled() {
		d.EnableAllSkills()
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
	c.Stop()
	return true
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
	hitTime := def.HitTime
	coolTime := def.CoolTime
	if !def.StaticHitTime {
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
	if !def.StaticReuse {
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

func (c *Controller) clearLocked() {
	c.stopTimersLocked()
	c.castSeq++
	c.casting = false
	c.current = modelskill.Definition{}
	c.target = nil
	c.plan = Plan{}
	c.startedAt = time.Time{}
	c.interruptUntil = time.Time{}
}

func (c *Controller) stopTimersLocked() {
	for _, t := range c.timers {
		t.Stop()
	}
	c.timers = nil
}

func (c *Controller) scheduleLocked(delay time.Duration, f func()) {
	source := c.afterFunc
	if source == nil {
		log := c.log
		source = func(d time.Duration, fn func()) scheduledTimer {
			return time.AfterFunc(d, func() {
				defer func() {
					if r := recover(); r != nil {
						log.Error().Interface("panic", r).Msg("cast: recovered panic in scheduled callback")
					}
				}()
				fn()
			})
		}
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
