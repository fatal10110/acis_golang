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

// folkCast is a civilian NPC's AI and cast runtime: the controller its
// casts run on and the desires its AI picks its actions from. The controller is
// set once before the NPC is published; desires is safe for concurrent
// use; brain guards step, acting, onRoute, oot and the fight state the
// selection keeps (folkFight.next, latched and lastKind).
type folkCast struct {
	control CastControl
	castAI  FolkCastAI
	ai      FolkAI

	// brain serializes the AI's entry points: the tick, desire selection
	// run in place from the goroutine that queued a first attack, the
	// attack and cast ends, and the arrivals. A pass releases it at each
	// hook point (atHookPoint), so a hook may raise a whole nested pass,
	// and the outer one reads what brain guards afresh after the point.
	// Code holding it stops the cast controller only while no cast is in
	// flight: the abort would run the skill-finished hooks with brain
	// held, and a hook that queues a first attack takes brain. A cast only
	// starts from a selection, which marks the NPC acting, and the idles
	// that stop a cast run only once the NPC is no longer acting.
	brain   sync.Mutex
	desires *ai.DesireQueue
	step    int
	// lifeTime counts the AI ticks the NPC has lived through. Its AI does
	// not idle before the first one is over, and acts on nothing then
	// unless it holds an attack desire.
	lifeTime atomic.Int32
	// acting marks an NPC whose AI last acted on a desire rather than going
	// idle; the AI idles it at once when its desires run out.
	acting bool
	// onRoute marks an NPC its AI put on its route, until the AI takes up
	// another desire.
	onRoute bool
	// oot is this life's stale-aggro sweep firing; the rest of the NPC's
	// out-of-territory state lives in its Scratch.
	oot folkOutOfTerritory
	// buffCheckAt is when a clan hall manager last checked its own support
	// buff, in Unix milliseconds: read and set by the AI under brain, reset
	// from a dialog command.
	buffCheckAt atomic.Int64

	// currentMu guards current, the desire the AI last acted on (nil when
	// idle): a cast break from another actor's queue closes it.
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
// command or script asks an NPC; an equal desire already queued gains the
// weight instead. The gates run and the desire is queued at once, on the
// caller's goroutine. f's AI starts the cast on its next tick, closing in first
// when target is out of the skill's range or out of its sight. Every
// refusal is silent:
//
//   - checkConditions refuses a skill still in reuse, or one whose hit f
//     cannot pay; without it the request skips those gates.
//   - A hold (moveToTarget false) is refused when target is not strictly
//     within the skill's cast range plus both collision radii, measured
//     flat, since only walking would bring it in range.
//   - A skill whose target type resolves no creature from target is
//     refused.
func (f *Folk) AddCastDesire(target attackable.Combatant, ref modelskill.Ref, weight float64, checkConditions, moveToTarget bool) {
	castAI := f.cast.castAI
	if target == nil || castAI == nil {
		return
	}
	if checkConditions && !castAI.CanDesire(target, ref) {
		return
	}
	if !moveToTarget && !ai.InCastReach(f, target, castAI.Range(ref)) {
		return
	}
	final := castAI.FinalTarget(target, ref)
	if final == nil {
		return
	}
	f.cast.desires.AddOrUpdate(&ai.Desire{
		Kind: ai.IntentionCast, FinalTarget: final, Skill: ref,
		MoveToTarget: moveToTarget, Weight: weight, QueuedAt: f.now(),
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
		f.cast.brain.Lock()
		defer f.cast.brain.Unlock()
		f.cast.desires.Clear()
		f.setCurrentDesire(nil)
		f.cast.onRoute, f.cast.acting = false, false
		f.fight.next, f.fight.latched = nil, nil
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

// TickThink runs one AI tick on f's queue: desire selection with its
// see-creature point (runAI), then the tick's own idle, clock and decay. A
// dead NPC's tick does nothing.
func (f *Folk) TickThink() error {
	if f.Dead() {
		return nil
	}
	f.runAI(true)
	return nil
}

// runAI is f's desire selection, under brain, on whichever goroutine asks
// for it: f's queue for the tick and the attack and cast ends, the caller's
// for a first attack desire. updateTick makes it the AI tick's: the
// see-creature point opens it, and the tick's idle, clock and decay close
// it. In order:
//
//   - Invalid desires are dropped: a cast that has lost its weight or its
//     HP, MP or mute gate, and any desire aimed at a creature f no longer
//     knows or that is dead; then, unless f is out of control, an attack
//     on a creature more than 1500 away.
//   - Unless f is out of control, casting, held by a social animation or
//     fleeing on a desire still queued, and only once f has lived through
//     an AI tick or holds an attack desire: an f with no desire and no
//     latched attack idles, and otherwise, outside its hit animation, the
//     latched attack or the heaviest desire is taken up. A wander already
//     current is not taken up again over a wander. An attack taken up right
//     after a wander, or before any other desire, is latched for the next
//     selection. A route walk puts f on its route, any other desire takes
//     it off.
//   - The tick then idles an f with no desire left and no cast in flight,
//     and every third tick refreshes the aggro list, lowers every hate and
//     attack desire by 6.6, every cast desire by 66000 and every do-nothing
//     desire by 0.5, and switches an f attacking or casting to its run
//     stance. A route desire never leaves the queue, so a route walker never
//     idles.
func (f *Folk) runAI(updateTick bool) {
	f.cast.brain.Lock()
	defer f.cast.brain.Unlock()
	f.runAILocked(updateTick)
}

// runAILocked is runAI for a caller holding brain.
func (f *Folk) runAILocked(updateTick bool) {
	instantRun := f.cast.lifeTime.Load() == 0 && f.cast.desires.HasKind(ai.IntentionAttack)
	if updateTick {
		f.atHookPoint(ai.HookSeeCreature)
	}
	f.pruneDesires()
	if !f.denyAIAction() {
		f.pruneFarAttacks()
		if !f.CastingNow() && (f.cast.lifeTime.Load() > 0 || instantRun) && !f.socialHeld() && !f.fleeHeld() {
			f.selectDesire()
		}
	}
	if !updateTick {
		return
	}
	if f.cast.desires.Len() == 0 && f.cast.lifeTime.Load() > 0 && !f.CastingNow() {
		f.thinkIdle()
	}
	f.cast.step++
	f.cast.lifeTime.Add(1)
	if f.cast.step%3 == 0 {
		f.refreshAggro()
		f.reduceAggro(folkAttackHateDecay)
		f.cast.desires.DecreaseWeightByType(ai.IntentionAttack, folkAttackHateDecay)
		f.cast.desires.DecreaseWeightByType(ai.IntentionCast, castDesireDecay)
		f.cast.desires.DecreaseWeightByType(ai.IntentionNothing, nothingDesireDecay)
		if cur := f.currentDesire(); cur != nil && (cur.Kind == ai.IntentionAttack || cur.Kind == ai.IntentionCast) {
			f.forceRunStance()
		}
		f.cast.step = 0
	}
}

// pruneDesires drops the cast desires that lost their weight or their HP,
// MP or mute gate, and every desire aimed at a creature f no longer knows
// or that is dead.
func (f *Folk) pruneDesires() {
	castAI := f.cast.castAI
	f.cast.desires.RemoveIf(func(d *ai.Desire) bool {
		if castAI != nil && d.Kind == ai.IntentionCast && (d.Weight <= 0 || !castAI.MeetsHPMPDisabled(d.FinalTarget, d.Skill)) {
			return true
		}
		return d.FinalTarget != nil && (!f.Knows(d.FinalTarget) || d.FinalTarget.AlikeDead())
	})
}

// pruneFarAttacks drops the attack desires on a creature more than
// folkAttackDesireRange from f, measured in 3D.
func (f *Folk) pruneFarAttacks() {
	from := f.location()
	f.cast.desires.RemoveIf(func(d *ai.Desire) bool {
		return d.Kind == ai.IntentionAttack && d.FinalTarget != nil && from.Distance3D(combatantLocation(d.FinalTarget)) > folkAttackDesireRange
	})
}

// fleeHeld reports whether f flees on a desire still queued: the one it
// took the flee up from, same target, start and distance.
func (f *Folk) fleeHeld() bool {
	cur := f.currentDesire()
	if cur == nil || cur.Kind != ai.IntentionFlee {
		return false
	}
	return f.cast.desires.AnyMatch(func(d *ai.Desire) bool {
		return d.Kind == ai.IntentionFlee && d.Equal(cur) && d.Location == cur.Location && d.Distance == cur.Distance
	})
}

// selectDesire takes up the latched attack or the heaviest desire, or idles
// an f that has neither (see runAI).
func (f *Folk) selectDesire() {
	desire := f.fight.latched
	if desire == nil {
		if d, ok := f.cast.desires.Peek(); ok {
			desire = d
		}
	}
	if desire == nil {
		f.setCurrentDesire(nil)
		if f.cast.acting {
			f.cast.acting = false
			f.cancelFollow()
			f.fight.next = nil
			f.thinkIdle()
		}
		return
	}
	if f.inHitAnimation() {
		return
	}
	if cur := f.currentDesire(); cur != nil && cur.Kind == ai.IntentionWander && desire.Kind == ai.IntentionWander {
		return
	}
	if desire.Kind == ai.IntentionAttack && (f.fight.lastKind == ai.IntentionIdle || f.fight.lastKind == ai.IntentionWander) {
		f.fight.latched = desire
	} else {
		f.fight.latched = nil
	}
	f.fight.lastKind = desire.Kind
	f.cast.acting = true
	if desire.Kind == ai.IntentionMoveRoute {
		f.cancelFollow()
		f.fight.next = nil
		f.setCurrentDesire(nil)
		f.thinkMoveRoute(desire.RouteName)
		return
	}
	f.leaveRoute()
	f.doIntention(desire)
}

// doIntention makes d f's current intention and takes its step: any follow
// under way is dropped, and so is an attack put aside for the end of a
// swing.
func (f *Folk) doIntention(d *ai.Desire) {
	f.cancelFollow()
	f.fight.next = nil
	f.setCurrentDesire(d)
	f.act(d)
}

// thinkIdle is what an NPC with nothing left to do does: it stops any walk,
// swing and cast, switches to its walk stance and opens the no-desire point. A
// clan hall manager instead checks its own support buff, and opens none.
func (f *Folk) thinkIdle() {
	if f.ClanHallManager() {
		f.hallManagerIdle()
		return
	}
	f.stopMoving()
	f.stopAttack()
	f.StopCast()
	f.forceWalkStance()
	f.atHookPoint(ai.HookNoDesire)
}

// AddMoveRouteDesire asks f to walk the route named name with weight; an
// equal desire already queued gains the weight instead. The desire never
// loses weight and stays queued while f lives, so f walks the route
// whenever nothing outweighs it.
func (f *Folk) AddMoveRouteDesire(name string, weight float64) {
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionMoveRoute, RouteName: name, Weight: weight, QueuedAt: f.now()})
}

// StartRoute puts f on the route named name at once, as its AI does when it
// acts on a route desire. Call it before f is on the AI task: from then on
// brain guards the AI's route state.
func (f *Folk) StartRoute(name string) { f.thinkMoveRoute(name) }

// thinkMoveRoute acts on a route desire: f, when not on the route named
// name, heads for its node nearest to f, and walks on when already on it.
// An NPC with no route walk task, or a route with no node keyed under its
// template alias, moves nowhere.
func (f *Folk) thinkMoveRoute(name string) {
	if f.motion == nil || f.motion.cfg.Route == nil {
		return
	}
	f.cast.onRoute = true
	if err := f.motion.cfg.Route.Walk(f.motion.walker, name, f.Instance.Template.Alias); err != nil {
		f.motion.cfg.Log.Debug().Err(err).Str("route", name).Msg("npc: folk route walk")
	}
}

// leaveRoute takes a route walker off its route while it acts on another
// desire.
func (f *Folk) leaveRoute() {
	if !f.cast.onRoute {
		return
	}
	f.cast.onRoute = false
	f.motion.cfg.Route.LeaveRoute(f.motion.walker)
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
// toward its target instead. A target no longer in f's world is given up,
// and so is one f does not know, unless the skill is SUMMON_FRIEND.
func (f *Folk) castOn(d *ai.Desire) {
	castAI := f.cast.castAI
	target, ref := d.FinalTarget, d.Skill
	if !f.inWorld(target) {
		return
	}
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
			f.SetHeadingTo(target)
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
// teleport under way, or death. The stun, sleep, paralyze, fear and
// hold-until-attacked states never hold here: folkAdmits keeps every
// effect but a plain buff or debuff off the NPC, and those carry no such
// flag.
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

// SetHeadingTo turns f to face target.
func (f *Folk) SetHeadingTo(target attackable.Combatant) {
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

// castFinished closes the cast desire that drove f's last cast, then runs
// the skill-finished hooks; a cast that ran to its end, on f's queue, has
// the AI pick the next one at once.
// An abort can come from another actor's queue: a hit's cast break or a
// crowd-control effect.
func (f *Folk) castFinished(e event.CastFinished) {
	f.cast.currentMu.Lock()
	cur := f.cast.current
	f.cast.current = nil
	f.cast.currentMu.Unlock()
	if cur != nil {
		f.cast.desires.RemoveIf(cur.Equal)
	}
	raiseSkillFinished(f.scripts, f, e)
	if !e.Interrupted {
		f.runAI(false)
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
		f.castFinished(e)
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

// ConsumeHP takes a skill's HP cost from f, never below its floor. The
// cost is a consumption, not a hit: it raises no attacked hook and makes no
// clan call.
func (f *Folk) ConsumeHP(amount float64) { f.loseHP(amount, f, modelskill.Ref{}, false) }

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
