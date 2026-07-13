// Package cast models the shared skill-cast lifecycle for live creatures.
package cast

import (
	"errors"
	"sync"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
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

// Controller coordinates validation, resource consumption, cooldowns and
// interruption state for one actor's active cast.
//
// mu guards every mutable field below.
type Controller struct {
	actor Actor

	mu             sync.RWMutex
	casting        bool
	current        modelskill.Definition
	target         any
	plan           Plan
	startedAt      time.Time
	interruptUntil time.Time
}

// NewController returns a cast controller for actor.
func NewController(actor Actor) *Controller {
	return &Controller{actor: actor}
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

// Hit applies the final MP and HP costs for the active cast.
func (c *Controller) Hit() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.casting {
		return ErrNotCasting
	}

	if mp := c.actor.MPCost(c.current); mp > 0 {
		if mp > c.actor.MP() {
			c.clearLocked()
			return ErrNotEnoughMP
		}
		c.actor.ReduceMP(mp)
	}

	if hp := c.current.HPConsume; hp > 0 {
		if hp > c.actor.HP() {
			c.clearLocked()
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

// Stop aborts and clears the active cast.
func (c *Controller) Stop() {
	c.mu.Lock()
	c.clearLocked()
	c.mu.Unlock()
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
	c.casting = false
	c.current = modelskill.Definition{}
	c.target = nil
	c.plan = Plan{}
	c.startedAt = time.Time{}
	c.interruptUntil = time.Time{}
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
