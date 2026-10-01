package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// Hooks are the phase callbacks a scheduled cast invokes as it advances
// through Launch, Hit and Finish, layered on top of the resource and
// cooldown state Start/Hit/Finish already own.
type Hooks struct {
	// Launch runs when the cast reaches its launch point, after the actor
	// has committed to it. Returning false means the target was lost or is
	// no longer valid, which stops the cast instead of continuing to Hit —
	// the mid-cast target/range/line-of-sight recheck.
	// A nil Launch always continues.
	Launch func() bool
	// Hit runs once Controller.Hit has consumed the final MP/HP cost, and
	// is where a caller applies the skill's effects to its targets.
	Hit func()
	// Finish runs once the cast's cool-down phase elapses and the cast has
	// fully completed.
	Finish func()
	// Failed runs if Controller.Hit reports an error (not enough MP/HP once
	// the final cost is checked). The cast is stopped either way.
	Failed func(error)
}

// Schedule advances an already-started cast (see Start) through its Launch,
// Hit and Finish phases on plan's timers, invoking hooks along the way.
//
// A skill cast triggered directly by a network packet handler replays this
// timeline itself, packet by packet, driven by the client's own round trip.
// An AI/NPC cast has no packet round trip to pace it, so it drives the same
// timeline through Schedule instead. Interrupting the cast (Stop, Interrupt,
// InterruptOnDamage) cancels any timer Schedule has pending.
func (c *Controller) Schedule(plan Plan, hooks Hooks) {
	c.mu.Lock()
	if !c.casting {
		c.mu.Unlock()
		return
	}
	seq := c.castSeq
	c.scheduleLocked(plan.LaunchDelay, func() { c.runLaunch(seq, plan, hooks) })
	c.mu.Unlock()
}

// ScheduleFusion keeps a FUSION cast active until its channel window ends.
// Its effect lands at channel start, so it bypasses ordinary launch and hit
// phases. The controller owns its timers and runs end on either outcome.
func (c *Controller) ScheduleFusion(plan Plan, interval time.Duration, check func() bool, end func()) bool {
	c.mu.Lock()
	if !c.casting {
		c.mu.Unlock()
		return false
	}
	seq := c.castSeq
	c.fusionEnd = end
	c.scheduleLocked(plan.LaunchDelay, func() {
		if c.stillCasting(seq) {
			c.Finish()
		}
	})
	if interval > 0 && check != nil {
		c.scheduleLocked(interval, func() { c.runFusionCheck(seq, interval, check) })
	}
	c.mu.Unlock()
	return true
}

// signetFinishDelay is how long a SIGNET_CASTTIME cast lasts past its launch,
// whatever its hit time: it has no cool phase.
const signetFinishDelay = 400 * time.Millisecond

// ScheduleSignetCast advances a SIGNET_CASTTIME cast, whose effect already
// landed when it started, on the fusion-cast timeline. At plan.LaunchDelay
// launch broadcasts the launch over the targets it resolves and returns how
// many there were; the caster then charges its shots again and pays the
// final MP, and the cast finishes signetFinishDelay later. A caster short of
// that MP is reported to failed and the cast stops. There is no range, sight
// or peace-zone recheck at launch, no HP cost and no charge step.
func (c *Controller) ScheduleSignetCast(plan Plan, launch func() int, failed func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.casting {
		return
	}
	seq := c.castSeq
	c.scheduleLocked(plan.LaunchDelay, func() { c.runSignetLaunch(seq, launch, failed) })
}

func (c *Controller) runSignetLaunch(seq uint64, launch func() int, failed func(error)) {
	if !c.stillCasting(seq) {
		return
	}
	def, _ := c.CurrentSkill()
	n := 0
	if launch != nil {
		n = launch()
	}
	c.SetLaunchTargets(n)
	if physical, magic := def.UsesSoulShot(), def.UsesSpiritShot(); physical || magic {
		c.emit(event.ShotsRechargeRequested{Physical: physical, Magic: magic})
	}
	if !c.stillCasting(seq) {
		return
	}
	if mp := c.actor.MPCost(def); mp > 0 {
		if mp > c.actor.MP() {
			if failed != nil {
				failed(ErrNotEnoughMP)
			}
			c.Stop()
			return
		}
		c.actor.ReduceMP(mp)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.castingLocked(seq) {
		c.scheduleLocked(signetFinishDelay, func() {
			if c.stillCasting(seq) {
				c.Finish()
			}
		})
	}
}

func (c *Controller) runFusionCheck(seq uint64, interval time.Duration, check func() bool) {
	if !c.stillCasting(seq) {
		return
	}
	if !check() {
		c.Stop()
		return
	}
	c.mu.Lock()
	if c.castingLocked(seq) {
		c.scheduleLocked(interval, func() { c.runFusionCheck(seq, interval, check) })
	}
	c.mu.Unlock()
}

func (c *Controller) runLaunch(seq uint64, plan Plan, hooks Hooks) {
	if !c.stillCasting(seq) {
		return
	}
	if hooks.Launch != nil && !hooks.Launch() {
		c.Stop()
		return
	}

	c.mu.Lock()
	if !c.castingLocked(seq) {
		c.mu.Unlock()
		return
	}
	c.scheduleLocked(plan.HitDelay, func() { c.runHit(seq, plan, hooks) })
	c.mu.Unlock()
}

func (c *Controller) runHit(seq uint64, plan Plan, hooks Hooks) {
	if !c.stillCasting(seq) {
		return
	}
	if err := c.Hit(); err != nil {
		if hooks.Failed != nil {
			hooks.Failed(err)
		}
		c.Stop()
		return
	}
	if hooks.Hit != nil {
		hooks.Hit()
	}

	c.mu.Lock()
	if !c.castingLocked(seq) {
		c.mu.Unlock()
		return
	}
	arm := func() { c.scheduleLocked(plan.FinalDelay, func() { c.runFinish(seq, hooks) }) }
	// A Hit effect that took a HoldFinish grant is still completing off this
	// actor's queue; arming Finish now would let the cast end ahead of it.
	// releaseFinish runs arm once the last hold is gone.
	if c.finishHolds > 0 {
		c.finishArm = arm
	} else {
		arm()
	}
	c.mu.Unlock()
}

func (c *Controller) runFinish(seq uint64, hooks Hooks) {
	if !c.stillCasting(seq) {
		return
	}
	c.Finish()
	if hooks.Finish != nil {
		hooks.Finish()
	}
}

func (c *Controller) stillCasting(seq uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.castingLocked(seq)
}

func (c *Controller) castingLocked(seq uint64) bool {
	return c.casting && c.castSeq == seq
}
