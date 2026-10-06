package npc

import (
	"sync"
	"sync/atomic"
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// FolkCastAI is the cast seam a civilian NPC's AI drives for one cast
// desire, from queueing it to starting the cast: the hostile AI's cast
// seam, plus what queueing a desire needs.
type FolkCastAI interface {
	ai.CastController
	// FinalTarget resolves the creature ref's target type aims at from the
	// requested target; nil drops the request.
	FinalTarget(target attackable.Combatant, ref modelskill.Ref) attackable.Combatant
	// CanDesire runs the gates a cast request passes before it is queued:
	// the skill's reuse, and the MP and HP its hit takes.
	CanDesire(target attackable.Combatant, ref modelskill.Ref) bool
	// SkillMP is the MP ref's data says its cast and its hit take together,
	// before any consume-rate stat; 0 for an unknown skill.
	SkillMP(ref modelskill.Ref) int
}

// FolkAI is the AI task a civilian NPC ticks on from its spawn: once a
// second, on its own queue, while its region is active. The NPC leaves it
// when it dies.
type FolkAI interface {
	Remove(task.AIActor)
}

// castDesireDecay is how much weight every cast desire loses each third AI
// tick.
const castDesireDecay = 66000

// routeDesireWeight is the weight of a route walker's wish to walk its
// route, which it holds for as long as it lives: only a heavier cast desire
// takes it off the route.
const routeDesireWeight = 1_000_000

// folkCast is a civilian NPC's AI and cast runtime: the controller its
// casts run on and the desires its AI picks them from. The controller is
// set once before the NPC is published; desires is safe for concurrent
// use; step, acting and offRoute belong to the NPC's queue, which alone
// writes lifeTime.
type folkCast struct {
	control CastControl
	castAI  FolkCastAI
	ai      FolkAI

	desires *ai.DesireQueue
	step    int
	// lifeTime counts the AI ticks the NPC has lived through. Its AI acts
	// on nothing, and does not idle, before the first one is over.
	lifeTime atomic.Int32
	// acting marks an NPC whose AI last acted on a cast desire rather than
	// going idle; the AI idles it at once when its desires run out.
	acting bool
	// offRoute marks a route walker its AI took off its route to act on a
	// cast desire, until it is back on it.
	offRoute bool
	// buffCheckAt is when a clan hall manager last checked its own support
	// buff, in Unix milliseconds: read and set on the NPC's queue, reset
	// from a dialog command.
	buffCheckAt atomic.Int64

	// currentMu guards current, the cast desire the AI last acted on (nil
	// when none): a cast break from another actor's queue closes it.
	currentMu sync.Mutex
	current   *ai.Desire

	// skillMu guards disabledSkills, read from the queues of the casts
	// that check the reuse.
	skillMu        sync.Mutex
	disabledSkills map[int32]time.Time
}

// SetCaster gives f its cast runtime: control is its cast controller, the
// surface its damage and crowd-control paths drive, and castAI the seam its
// AI casts through. Call it once, before f is published.
func (f *Folk) SetCaster(control CastControl, castAI FolkCastAI) {
	f.cast.control, f.cast.castAI = control, castAI
}

// CastControl returns f's cast controller, nil when it has none.
func (f *Folk) CastControl() CastControl { return f.cast.control }

// AddCastDesire asks f to cast ref at target with weight, the way a dialog
// command or script asks an NPC: a skill still in reuse, or one whose hit
// f cannot pay, is refused, and an equal desire already queued gains the
// weight instead. f's AI starts the cast on its next tick, closing in
// first when target is out of the skill's range or out of its sight.
func (f *Folk) AddCastDesire(target attackable.Combatant, ref modelskill.Ref, weight float64) {
	if target == nil || f.cast.castAI == nil || f.queue == nil {
		return
	}
	f.queue.Post(func() {
		castAI := f.cast.castAI
		if !castAI.CanDesire(target, ref) {
			return
		}
		final := castAI.FinalTarget(target, ref)
		if final == nil {
			return
		}
		f.cast.desires.AddOrUpdate(&ai.Desire{
			Kind: ai.IntentionCast, FinalTarget: final, Skill: ref,
			MoveToTarget: true, Weight: weight, QueuedAt: f.now(),
		})
	})
}

// AbortCast stops f's cast in flight, observers seeing it canceled, drops
// every desire it holds and takes it off the AI task, its lifetime over:
// what a death or a despawn does to the NPC's AI.
func (f *Folk) AbortCast() {
	f.StopCast()
	if f.queue == nil {
		return
	}
	f.queue.Post(func() {
		f.cast.desires.Clear()
		f.setCurrentDesire(nil)
		f.cast.offRoute, f.cast.acting = false, false
		f.cast.lifeTime.Store(0)
		if f.cast.ai != nil {
			f.cast.ai.Remove(f)
		}
	})
}

// LifeTime returns the number of AI ticks f has lived through since it
// spawned, zero again once it dies. Safe from any goroutine.
func (f *Folk) LifeTime() int32 { return f.cast.lifeTime.Load() }

// Tick does nothing: TickThink runs the whole AI step, in order.
func (f *Folk) Tick() {}

// TickThink runs one AI tick on f's queue: the see-creature point opens
// it, then invalid cast desires are dropped and the heaviest one left is
// acted on unless a cast is in flight; an NPC with no desire left and no
// cast in flight idles, unless it walks a route; and every third tick the
// cast desires lose weight, an NPC still acting on one switching to its
// run stance. A dead NPC's tick does nothing.
func (f *Folk) TickThink() error {
	if f.Dead() {
		return nil
	}
	f.AtHookPoint(ai.HookSeeCreature)
	f.runAI()
	if f.cast.desires.Len() == 0 && f.cast.lifeTime.Load() > 0 && !f.CastingNow() && !f.walksRoute() {
		f.thinkIdle()
	}
	f.cast.step++
	f.cast.lifeTime.Add(1)
	if f.cast.step%3 == 0 {
		f.cast.desires.DecreaseWeightByType(ai.IntentionCast, castDesireDecay)
		if f.currentDesire() != nil {
			f.forceRunStance()
		}
		f.cast.step = 0
	}
	return nil
}

// runAI prunes f's cast desires and, once f has lived through an AI tick
// and is not casting, acts on the heaviest one. A route walker acts on it
// only when it outweighs the walk, and otherwise goes back to its route;
// any other NPC that just ran out of desires idles.
func (f *Folk) runAI() {
	if castAI := f.cast.castAI; castAI != nil {
		f.cast.desires.RemoveIf(func(d *ai.Desire) bool {
			if d.Kind == ai.IntentionCast && (d.Weight <= 0 || !castAI.MeetsHPMPDisabled(d.FinalTarget, d.Skill)) {
				return true
			}
			return d.FinalTarget != nil && (!f.Knows(d.FinalTarget) || d.FinalTarget.AlikeDead())
		})
	}
	if f.denyAIAction() || f.CastingNow() || f.cast.lifeTime.Load() == 0 {
		return
	}
	f.cancelFollow()
	desire, ok := f.cast.desires.Peek()
	if !ok && !f.walksRoute() {
		f.setCurrentDesire(nil)
		if f.cast.acting {
			f.cast.acting = false
			f.thinkIdle()
		}
		return
	}
	if !ok || (f.walksRoute() && desire.Weight <= routeDesireWeight) {
		f.setCurrentDesire(nil)
		f.cast.acting = false
		f.backOnRoute()
		return
	}
	f.leaveRoute()
	f.setCurrentDesire(desire)
	f.cast.acting = true
	f.thinkCast(desire)
}

// thinkIdle is what an NPC with nothing left to do does: it stops any walk
// and cast, switches to its walk stance and opens the no-desire point. A
// clan hall manager instead checks its own support buff, and opens none.
func (f *Folk) thinkIdle() {
	if f.ClanHallManager() {
		f.hallManagerIdle()
		return
	}
	f.stopMoving()
	f.StopCast()
	f.forceWalkStance()
	f.AtHookPoint(ai.HookNoDesire)
}

// walksRoute reports whether f walks a route.
func (f *Folk) walksRoute() bool {
	return f.motion != nil && f.motion.cfg.Route != nil
}

// leaveRoute takes a route walker off its route while it acts on a cast
// desire.
func (f *Folk) leaveRoute() {
	if !f.walksRoute() {
		return
	}
	f.cast.offRoute = true
	f.motion.cfg.Route.LeaveRoute(f.motion.walker)
}

// backOnRoute sends a route walker its AI took off its route back to it,
// from the route node nearest to where it stands.
func (f *Folk) backOnRoute() {
	if !f.cast.offRoute {
		return
	}
	f.cast.offRoute = false
	if err := f.motion.cfg.Route.ResumeRoute(f.motion.walker); err != nil {
		f.motion.cfg.Log.Warn().Err(err).Msg("npc: folk route resume")
	}
}

// thinkCast acts on a cast desire; a clan hall manager's support magic on
// a player goes through supportCast.
func (f *Folk) thinkCast(d *ai.Desire) {
	if f.castsSupport(d) {
		f.supportCast(d)
		return
	}
	f.castOn(d)
}

// castOn acts on a cast desire: f closes in on a target out of the
// skill's range, or in range but out of its sight, in its run stance, and
// waits for it where it cannot walk; in reach it stops and faces its target
// for a cast long enough to show, and when the final gates refuse turns
// toward its target instead.
func (f *Folk) castOn(d *ai.Desire) {
	castAI := f.cast.castAI
	target, ref := d.FinalTarget, d.Skill
	if !f.Knows(target) && castAI.SkillType(ref) != "SUMMON_FRIEND" && target.ObjectID() != f.ObjectID() {
		return
	}
	if !castAI.CanAttempt(target, ref) {
		return
	}
	castRange := castAI.Range(ref)
	if f.waitsFor(target, castRange) {
		// The stance switches first, so the walk toward target runs.
		f.forceRunStance()
		f.closeIn(target, castRange)
		return
	}
	self := target.ObjectID() == f.ObjectID()
	if castAI.StopsMovement(ref) {
		f.stopMoving()
		if !self {
			f.setHeadingTo(target)
		}
	}
	if !castAI.CanCast(target, ref) || (castRange > 0 && !self && !f.canSee(target)) {
		if !self {
			f.broadcastMoveToPawn(target)
		}
		return
	}
	castAI.Cast(target, ref)
}

// waitsFor reports whether f waits for target instead of casting at
// castRange: target is out of reach, or in reach but out of sight of an f
// free to walk to it.
func (f *Folk) waitsFor(target attackable.Combatant, castRange int) bool {
	if castRange < 0 {
		return false
	}
	if f.outOfReach(target, castRange) {
		return true
	}
	m := f.motion
	return m != nil && !m.MovementDisabled() && m.IntentionMovesToTarget() && !f.canSee(target)
}

// closeIn has an f free to walk follow target, to castRange of it or, when
// already in reach, to its body.
func (f *Folk) closeIn(target attackable.Combatant, castRange int) {
	if f.motion != nil {
		_, _ = f.motion.ctl.MaybeStartOffensiveFollow(target, castRange)
	}
}

// cancelFollow drops f's offensive follow, leaving a walk under way going.
func (f *Folk) cancelFollow() {
	if f.motion != nil {
		f.motion.ctl.CancelFollow()
	}
}

// denyAIAction reports a state in which the NPC's AI does nothing: a
// teleport under way, or death.
func (f *Folk) denyAIAction() bool {
	return f.AlikeDead() || (f.motion != nil && f.motion.teleporting.Load())
}

// outOfReach reports whether target stands at castRange plus both bodies,
// and fifty more while it moves, or farther; a negative range is never out
// of reach.
func (f *Folk) outOfReach(target attackable.Combatant, castRange int) bool {
	if castRange < 0 {
		return false
	}
	reach := int(float64(castRange) + f.CollisionRadius() + target.CollisionRadius())
	if target.IsMoving() {
		reach += 50
	}
	sx, sy, _ := f.Position()
	tx, ty, _ := target.Position()
	dx, dy := float64(sx-tx), float64(sy-ty)
	return dx*dx+dy*dy >= float64(reach)*float64(reach)
}

func (f *Folk) stopMoving() {
	if f.motion != nil {
		f.motion.ctl.Stop()
	}
}

func (f *Folk) setHeadingTo(target attackable.Combatant) {
	sx, sy, _ := f.Position()
	tx, ty, _ := target.Position()
	f.SetHeading(location.Location{X: sx, Y: sy}.HeadingTo(location.Location{X: tx, Y: ty}))
}

func (f *Folk) broadcastMoveToPawn(target attackable.Combatant) {
	sx, sy, sz := f.Position()
	origin := location.Location{X: sx, Y: sy, Z: sz}
	tx, ty, tz := target.Position()
	dest := location.Location{X: tx, Y: ty, Z: tz}
	f.emit(event.MoveToPawn{TargetID: target.ObjectID(), Distance: int(origin.Distance3D(dest)), Origin: origin})
}

func (f *Folk) currentDesire() *ai.Desire {
	f.cast.currentMu.Lock()
	defer f.cast.currentMu.Unlock()
	return f.cast.current
}

func (f *Folk) setCurrentDesire(d *ai.Desire) {
	f.cast.currentMu.Lock()
	f.cast.current = d
	f.cast.currentMu.Unlock()
}

// castFinished closes the cast desire that drove f's last cast; a cast
// that ran to its end, on f's queue, has the AI pick the next one at once.
// An abort can come from another actor's queue: a hit's cast break or a
// crowd-control effect.
func (f *Folk) castFinished(interrupted bool) {
	f.cast.currentMu.Lock()
	cur := f.cast.current
	f.cast.current = nil
	f.cast.currentMu.Unlock()
	if cur != nil {
		f.cast.desires.RemoveIf(cur.Equal)
	}
	if !interrupted {
		f.runAI()
	}
}

// CastEvents returns the sink f's cast controller reports to: an abort
// shows observers the cancel, an offensive cast that reached a target puts
// f in its attack stance, and every cast end closes the desire behind it.
func (f *Folk) CastEvents() event.Sink { return folkCastSink{f} }

type folkCastSink struct{ f *Folk }

func (s folkCastSink) Emit(ev event.Event) {
	f := s.f
	switch e := ev.(type) {
	case event.CastAborted:
		f.BroadcastSkillCanceled()
	case event.AttackStanceRequested:
		f.emit(event.AttackStanceRequested{})
	case event.CastFinished:
		f.castFinished(e.Interrupted)
	}
}

// CastingNow reports whether f has a cast in flight.
func (f *Folk) CastingNow() bool {
	c := f.cast.control
	return c != nil && c.CastingNow()
}

// CurrentSkillIsMagic reports whether f's cast in flight is a magic skill.
func (f *Folk) CurrentSkillIsMagic() bool {
	c := f.cast.control
	return c != nil && c.CurrentSkillIsMagic()
}

// InterruptCast aborts f's cast while it is still inside its interrupt
// window; observers see MagicSkillCanceled.
func (f *Folk) InterruptCast() {
	if c := f.cast.control; c != nil {
		c.InterruptCast()
	}
}

// StopCast aborts f's cast unconditionally; observers see
// MagicSkillCanceled when a cast was in flight.
func (f *Folk) StopCast() {
	if c := f.cast.control; c != nil {
		c.StopCast()
	}
}

// BreakCastOnDamage rolls whether damage f takes breaks its cast, reading
// MEN, ATTACK_CANCEL and the roll from f. An invulnerable NPC is never
// broken, and one with no cast in flight draws no roll.
func (f *Folk) BreakCastOnDamage(damage float64) {
	c := f.cast.control
	if c == nil || !c.CastingNow() {
		return
	}
	c.InterruptCastOnDamage(damage, f.MEN(), func(base float64) float64 {
		return f.CalcStat(stat.AttackCancel, base)
	}, f.Roll(100), f.Invul())
}

// BroadcastSkillUse shows observers the start of f's cast of skillID at
// level on the target at the given position.
func (f *Folk) BroadcastSkillUse(targetID int32, targetX, targetY, targetZ int, skillID, level int32, hitTime, reuseDelay int) {
	sx, sy, sz := f.Position()
	f.emit(event.MagicSkillUse{
		CasterID: f.ObjectID(), CasterAt: location.Location{X: sx, Y: sy, Z: sz},
		TargetID: targetID, TargetAt: location.Location{X: targetX, Y: targetY, Z: targetZ},
		SkillID: skillID, Level: level, HitTime: hitTime, ReuseDelay: reuseDelay,
	})
}

// BroadcastSkillLaunched shows observers the launch of f's cast of skillID
// at level onto targetIDs.
func (f *Folk) BroadcastSkillLaunched(skillID, level int32, targetIDs []int32) {
	f.emit(event.SkillLaunched{SkillID: skillID, Level: level, TargetIDs: targetIDs})
}

// BroadcastSkillCanceled shows observers f's cast canceled.
func (f *Folk) BroadcastSkillCanceled() {
	f.emit(event.SkillCanceled{ObjectID: f.ObjectID()})
}

// TestCursesOnSkillSee reports false: only a playable caster draws the raid
// curse.
func (f *Folk) TestCursesOnSkillSee(modelskill.Definition, []skilltarget.Actor) bool { return false }

// NotePvPSkillTargets does nothing: NPCs take no part in PvP flagging.
func (f *Folk) NotePvPSkillTargets([]attackable.Combatant, bool, string) {}

// ConsumeHP takes a skill's HP cost from f, never below its floor.
func (f *Folk) ConsumeHP(amount float64) { f.reduceHP(amount, f) }

// SkillDisabled reports whether key is still waiting for its reuse delay.
func (f *Folk) SkillDisabled(key int32) bool {
	f.cast.skillMu.Lock()
	defer f.cast.skillMu.Unlock()
	expiresAt, ok := f.cast.disabledSkills[key]
	if !ok {
		return false
	}
	if f.now().Before(expiresAt) {
		return true
	}
	delete(f.cast.disabledSkills, key)
	return false
}

// DisableSkill marks key unusable until delay elapses.
func (f *Folk) DisableSkill(key int32, delay time.Duration) {
	if delay <= 0 {
		return
	}
	f.cast.skillMu.Lock()
	defer f.cast.skillMu.Unlock()
	if f.cast.disabledSkills == nil {
		f.cast.disabledSkills = make(map[int32]time.Time)
	}
	f.cast.disabledSkills[key] = f.now().Add(delay)
}

// AddSkillReuse installs an NPC-local reuse delay; nothing persists it.
func (f *Folk) AddSkillReuse(_ modelskill.Ref, key int32, delay time.Duration) {
	f.DisableSkill(key, delay)
}

// HeldItemTypeMask returns the item-type bits of f's right-hand weapon and
// left-hand shield.
func (f *Folk) HeldItemTypeMask() int32 { return f.heldMask }

// sighted is a body f can look at.
type sighted interface {
	Position() (x, y, z int)
	CollisionHeight() float64
}

// canSee reports whether f has line of sight to target.
func (f *Folk) canSee(target sighted) bool {
	if f.los == nil {
		return true
	}
	ox, oy, oz := f.Position()
	tx, ty, tz := target.Position()
	return f.los.CanSeeActor(ox, oy, oz, f.CollisionHeight(), tx, ty, tz, target.CollisionHeight())
}

// now reads the clock f's queue runs on.
func (f *Folk) now() time.Time {
	if f.queue == nil {
		return time.Now()
	}
	return f.queue.Now()
}

// templateHeldMask is the item-type bits of the weapon in t's right hand
// and the shield in its left, as items resolves them; an unknown or
// mismatched item id holds nothing.
func templateHeldMask(t *Template, items *item.Table) int32 {
	if items == nil {
		return 0
	}
	var mask int32
	if id := t.LeftHand; id != 0 {
		if tmpl, ok := items.Get(int32(id)); ok && tmpl.Kind == item.KindArmor && tmpl.Armor != nil {
			mask = tmpl.Armor.Type.Mask()
		}
	}
	if id := t.RightHand; id != 0 {
		if tmpl, ok := items.Get(int32(id)); ok && tmpl.Weapon != nil {
			mask |= tmpl.Weapon.Type.Mask()
		}
	}
	return mask
}
