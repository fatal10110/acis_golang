package cast

import "time"

// Hooks are the phase callbacks a scheduled cast invokes as it advances
// through Launch, Hit and Finish, layered on top of the resource and
// cooldown state Start/Hit/Finish already own.
type Hooks struct {
	// Launch runs when the cast reaches its launch point, after the actor
	// has committed to it. Returning false means the target was lost or is
	// no longer valid, which stops the cast instead of continuing to Hit —
	// mirroring the oracle's mid-cast target/range/line-of-sight recheck.
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
func (c *Controller) ScheduleFusion(plan Plan, interval time.Duration, check func() bool, end func()) {
	c.mu.Lock()
	if !c.casting {
		c.mu.Unlock()
		return
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
	c.scheduleLocked(plan.FinalDelay, func() { c.runFinish(seq, hooks) })
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
