package npc

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

var _ attack.Target = (*Folk)(nil)

var _ attack.CreatureActor = (*Folk)(nil)

const (
	// folkAttackHateDecay is how much hate every aggro list entry, and weight
	// every attack desire, loses each third AI tick.
	folkAttackHateDecay = 6.6
	// folkAttackDesireRange is the 3D distance past which an attack desire is
	// dropped while the NPC is in control.
	folkAttackDesireRange = 1500
	// The follow step: a follower more than folkFollowLooseRadius away from
	// its target walks to a random point within folkFollowScatter of it; one
	// whose path finding failed folkFollowGeoFailLimit times in a row jumps
	// to within folkFollowTeleportOffset of it instead.
	folkFollowLooseRadius    = 150
	folkFollowScatter        = 300
	folkFollowGeoFailLimit   = 10
	folkFollowTeleportOffset = 10
)

// FolkAttack is the attack controller a civilian NPC swings with: the
// hostile AI's attack seam, plus the hit animation window during which its
// AI takes up no new desire.
type FolkAttack interface {
	ai.AttackController
	InHitAnimation() bool
}

// folkFight is a civilian NPC's fighting state: what it swings with, its
// aggro list, and the parts of its AI's desire selection that only the
// fighting desires use. attack, weapon and crystal are set before the NPC is
// published; aggro is safe for concurrent use; folkCast.brain guards next,
// latched and lastKind.
type folkFight struct {
	// attack is the NPC's attack controller; nil leaves it unable to swing.
	attack FolkAttack
	// weapon is the template's right-hand weapon, nil for none, and crystal
	// its grade.
	weapon  *item.WeaponDetail
	crystal item.CrystalType
	// aggro is the NPC's aggro list: per creature, the damage it dealt the
	// NPC and the hate an attack desire on it raised.
	aggro *attackable.ThreatTable
	// next is the attack the AI goes on with once the swing in flight ends:
	// the attack step meeting a swing still in flight sets it, taking up a
	// desire clears it.
	next *ai.Desire
	// latched is the attack desire selection took up just after a wander or
	// before any other desire: the next selection takes it up again, even
	// once its desire has left the queue, and then clears it.
	latched *ai.Desire
	// lastKind is the kind of the desire selection last took up,
	// IntentionIdle before the first one.
	lastKind ai.Intention
	// roll draws the follow step's dice, a uniform integer in [0, n); nil
	// uses the server's random source. Tests set it before the NPC runs.
	roll func(n int) int
}

// followRoll draws a uniform integer in [0, n) for the follow step.
func (f *Folk) followRoll(n int) int {
	if r := f.fight.roll; r != nil {
		return r(n)
	}
	return rnd.Get(n)
}

// SetAttacker gives f the attack controller it swings with. Call it once,
// before f is published.
func (f *Folk) SetAttacker(a FolkAttack) { f.fight.attack = a }

// resolveWeapon reads the template's right-hand weapon from items: an
// unknown id or an item that is no weapon leaves f unarmed.
func (f *Folk) resolveWeapon(items *item.Table) {
	if items == nil || f.Instance.Template.RightHand == 0 {
		return
	}
	tmpl, ok := items.Get(int32(f.Instance.Template.RightHand))
	if !ok || tmpl.Weapon == nil {
		return
	}
	f.fight.weapon, f.fight.crystal = tmpl.Weapon, tmpl.Crystal
}

// AttackEvents returns the sink f's attack controller reports to: a
// damaging hit puts f in its attack stance, the end of the hit animation
// and of a bow shot re-run desire selection, and a finished swing re-runs
// it, then goes on with the attack or the current intention. A bow's reuse
// ending takes one more step of the current intention only while no attack
// was put aside; an attack put aside waits for the next desire selection.
func (f *Folk) AttackEvents() event.Sink { return folkAttackSink{f} }

type folkAttackSink struct{ f *Folk }

func (s folkAttackSink) Emit(ev event.Event) {
	f := s.f
	switch e := ev.(type) {
	case event.AttackStanceRequested:
		f.emit(event.AttackStanceRequested{})
	case event.AttackRethink:
		f.runAI(false)
	case event.AttackFinished:
		f.cast.brain.Lock()
		defer f.cast.brain.Unlock()
		if e.BowReuse {
			if f.fight.next == nil {
				f.thinkCurrent()
			}
			return
		}
		f.runAILocked(false)
		f.attackFinished()
	}
}

// attackFinished is f's AI on a swing ending: the attack put aside while
// the swing was in flight is taken up, or else the current intention takes
// one more step.
func (f *Folk) attackFinished() {
	if next := f.fight.next; next != nil {
		f.doIntention(next)
		return
	}
	f.thinkCurrent()
}

// thinkCurrent takes one more step of the current intention: an NPC that
// idles idles again, opening the no-desire point.
func (f *Folk) thinkCurrent() {
	cur := f.currentDesire()
	if cur == nil {
		if !f.cast.acting {
			f.thinkIdle()
		}
		return
	}
	switch cur.Kind {
	case ai.IntentionAttack:
		f.thinkAttack(cur)
	case ai.IntentionCast:
		f.thinkCast(cur)
	case ai.IntentionFollow:
		f.thinkFollow(cur)
	case ai.IntentionMoveTo:
		f.thinkMoveTo(cur)
	case ai.IntentionPickUp:
		f.thinkPickUp(cur)
	}
}

// AddAttackDesire asks f to attack target with weight, closing in on it,
// and adds weight to f's hate of target; an equal desire (same target)
// already queued gains the weight instead. While f hates no one, its AI
// runs desire selection at once, in place on the caller's goroutine, before
// the hate is added: a hit's attacked hook has f react before the hit's HP
// change. Safe from any goroutine, a hook's included.
func (f *Folk) AddAttackDesire(target attackable.Combatant, weight float64) {
	f.addAttackDesire(target, 0, weight, true)
}

// AddAttackDesireHold is AddAttackDesire for an f that stays where it is:
// it never walks toward target.
func (f *Folk) AddAttackDesireHold(target attackable.Combatant, weight float64) {
	f.addAttackDesire(target, 0, weight, false)
}

// AddAttackDesireDamage is AddAttackDesire that also counts damage as dealt
// to f by target.
func (f *Folk) AddAttackDesireDamage(target attackable.Combatant, damage int, weight float64) {
	f.addAttackDesire(target, damage, weight, true)
}

func (f *Folk) addAttackDesire(target attackable.Combatant, damage int, weight float64, moveToTarget bool) {
	if target == nil {
		return
	}
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: target, MoveToTarget: moveToTarget, Weight: weight, QueuedAt: f.now()})
	if _, ok := f.fight.aggro.MostHated(); !ok {
		f.runAI(false)
	}
	f.fight.aggro.AddDamage(target, float64(damage), weight)
}

// AddFollowDesire asks f to follow target with weight; an equal desire
// already queued gains the weight instead. Safe from any goroutine.
func (f *Folk) AddFollowDesire(target attackable.Combatant, weight float64) {
	if target == nil {
		return
	}
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionFollow, FinalTarget: target, Weight: weight, QueuedAt: f.now()})
}

// AddFleeDesire asks f to run distance away from target with weight,
// counted from where f stands now; an equal desire (same target) already
// queued gains the weight instead and keeps its start and distance. While
// the flee is current and its desire queued, desire selection holds; its
// arrival drops the desire. Refused for an f that cannot move. Safe from any
// goroutine.
func (f *Folk) AddFleeDesire(target attackable.Combatant, distance int, weight float64) {
	if target == nil || f.MovementDisabled() {
		return
	}
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionFlee, FinalTarget: target, Location: f.location(), Distance: distance, Weight: weight, QueuedAt: f.now()})
}

// ClearDesires drops every desire f holds, whatever its kind. Safe from
// any goroutine.
func (f *Folk) ClearDesires() { f.cast.desires.Clear() }

// Desires returns f's desire queue, for reads. It is safe for concurrent
// use.
func (f *Folk) Desires() *ai.DesireQueue { return f.cast.desires }

// Aggro returns f's aggro list, for reads. It is safe for concurrent use.
func (f *Folk) Aggro() *attackable.ThreatTable { return f.fight.aggro }

// MostHated returns the creature f hates most, false when it hates none or
// is dead.
func (f *Folk) MostHated() (attackable.Combatant, bool) {
	t, ok := f.fight.aggro.MostHated()
	return t.Attacker, ok
}

// StopHate zeroes f's hate of target, sends f back to peace when it then
// hates no one, and drops its attack desire on target.
func (f *Folk) StopHate(target attackable.Combatant) {
	if target == nil || f.fight.aggro.IsEmpty() {
		return
	}
	f.fight.aggro.StopHate(target)
	f.backToPeaceIfCalm()
	f.cast.desires.Remove(ai.IntentionAttack, target)
}

// backToPeaceIfCalm empties f's aggro list once f hates no one: what a
// civilian NPC's AI does going back to peace. It keeps no skill hate.
func (f *Folk) backToPeaceIfCalm() {
	if _, ok := f.fight.aggro.MostHated(); !ok {
		f.fight.aggro.Clear()
	}
}

// refreshAggro drops every attack desire of an f that hates no one, and
// otherwise drops the attack desire on each creature of its aggro list that
// died, stopping its hate, or that f no longer knows, forgetting it.
func (f *Folk) refreshAggro() {
	if f.fight.aggro.IsEmpty() {
		f.cast.desires.RemoveKind(ai.IntentionAttack)
		return
	}
	for _, c := range f.fight.aggro.Refresh(f.Knows) {
		f.cast.desires.Remove(ai.IntentionAttack, c)
	}
}

// reduceAggro lowers every hate f holds by amount, f going back to peace
// once it hates no one.
func (f *Folk) reduceAggro(amount float64) {
	if f.fight.aggro.IsEmpty() {
		return
	}
	f.fight.aggro.ReduceAllHate(amount)
	f.backToPeaceIfCalm()
}

// thinkAttack is the attack step: f, when it can act and its target is
// still known, closes in on it; in reach, a swing or a bow's reuse in
// flight puts the attack aside for when it ends, and otherwise f stops and
// swings when the attack gate allows it.
func (f *Folk) thinkAttack(d *ai.Desire) {
	a := f.fight.attack
	if a == nil || f.denyAIAction() {
		return
	}
	target := d.FinalTarget
	if target == nil || !f.Knows(target) {
		return
	}
	if f.offensiveFollow(target, f.PhysicalAttackRange()) {
		return
	}
	if (f.AttackType() == item.WeaponBow && a.BowCoolingDown()) || a.AttackingNow() {
		f.fight.next = d
		return
	}
	if !a.CanAttack(target) {
		return
	}
	f.stopMoving()
	a.DoAttack(target)
}

// offensiveFollow has f close in on target to attackRange and reports
// whether f is to wait rather than act: target is out of reach, or in reach
// but out of sight of an f free to walk to it. An f that cannot move waits
// while target is out of reach.
func (f *Folk) offensiveFollow(target attackable.Combatant, attackRange int) bool {
	if f.motion != nil {
		following, _ := f.motion.ctl.MaybeStartOffensiveFollow(target, attackRange)
		return following
	}
	return f.outOfReach(target, attackRange)
}

// thinkFlee is the flee step: f runs, in its run stance, its distance
// straight away from where the target stands now. It does not move when it
// cannot, flees from itself, has a distance under 10, or already stands its
// distance or more from where the flee was queued.
func (f *Folk) thinkFlee(d *ai.Desire) {
	if f.motion == nil || f.MovementDisabled() {
		return
	}
	target := d.FinalTarget
	if target == nil || target.ObjectID() == f.ObjectID() || d.Distance < 10 {
		return
	}
	from := f.location()
	if from.Distance2D(d.Location) >= float64(d.Distance) {
		return
	}
	f.forceRunStance()
	tx, ty, _ := target.Position()
	_, _ = f.motion.ctl.MoveToLocation(from.FleeFrom(tx, ty, d.Distance))
}

// thinkFollow is the follow step, taken every second AI tick: f switches
// to its run stance and, when it can act and move, follows its target
// loosely. A target gone, dead or f itself drops the desire; a follower
// whose path finding keeps failing jumps next to its target.
func (f *Folk) thinkFollow(d *ai.Desire) {
	if f.cast.step%2 == 0 {
		return
	}
	f.forceRunStance()
	if f.denyAIAction() || f.motion == nil || f.MovementDisabled() {
		return
	}
	target := d.FinalTarget
	if target == nil || target.ObjectID() == f.ObjectID() || target.Dead() {
		f.cast.desires.RemoveIf(d.Equal)
		return
	}
	if f.motion.GeoPathFailCount() >= folkFollowGeoFailLimit {
		f.abortAll()
		f.teleportNear(target, folkFollowTeleportOffset)
		f.motion.ResetGeoPathFailCount()
		return
	}
	pos := combatantLocation(target)
	if f.location().Distance2D(pos) <= folkFollowLooseRadius || f.followRoll(100) <= 50 || f.IsMoving() {
		return
	}
	distance := math.Sqrt(float64(f.followRoll(1000000))/1000000) * folkFollowScatter
	angle := float64(f.followRoll(1000000)) / 1000000 * math.Pi * 2
	_, _ = f.motion.ctl.MoveToLocation(location.Location{
		X: int(distance*math.Cos(angle)) + pos.X,
		Y: int(distance*math.Sin(angle)) + pos.Y,
		Z: pos.Z,
	})
}

// teleportNear jumps f to within offset of target, on ground its geodata
// lets it stand on.
func (f *Folk) teleportNear(target attackable.Combatant, offset int) {
	pos := combatantLocation(target)
	if geo := f.motion.cfg.Geo; geo != nil && offset > 0 {
		valid := geo.ValidLocation(pos.X, pos.Y, pos.Z, pos.X-offset+f.followRoll(2*offset+1), pos.Y-offset+f.followRoll(2*offset+1), pos.Z)
		pos.X, pos.Y = valid.X, valid.Y
	}
	f.motion.TeleportTo(pos)
}

// abortAll stops f's walk, its swing and its cast.
func (f *Folk) abortAll() {
	f.stopMoving()
	f.stopAttack()
	f.StopCast()
}

// stopAttack aborts f's swing in flight, if any.
func (f *Folk) stopAttack() {
	if a := f.fight.attack; a != nil {
		a.Stop()
	}
}

// inHitAnimation reports whether f's hit animation window is open.
func (f *Folk) inHitAnimation() bool {
	a := f.fight.attack
	return a != nil && a.InHitAnimation()
}

func (f *Folk) location() location.Location {
	x, y, z := f.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// AttackDisabled reports whether f cannot start a swing: it is dead or
// paralyzed. No other effect that holds a creature lands on a civilian NPC.
func (f *Folk) AttackDisabled() bool { return f.AlikeDead() || f.Paralyzed() }

// Think takes one more step of f's current intention, as a lock on its
// actions lifting does. Call it on f's queue; a dead NPC does nothing.
func (f *Folk) Think() {
	if f.Dead() {
		return
	}
	f.cast.brain.Lock()
	defer f.cast.brain.Unlock()
	f.thinkCurrent()
}

// InAttackRange reports whether target is within f's physical reach.
func (f *Folk) InAttackRange(target attackable.Combatant) bool {
	return attack.InPhysicalRange(f.location(), f.PhysicalAttackRange(), f.CollisionRadius(), target)
}

// CanSee reports whether f has line of sight to target.
func (f *Folk) CanSee(target attackable.Combatant) bool { return f.canSee(target) }

// PhysicalAttackRange returns the template's melee reach.
func (f *Folk) PhysicalAttackRange() int { return f.Instance.Template.BaseAttackRange }

// PoleAttackAngle returns the forward cone a pole swing sweeps.
func (f *Folk) PoleAttackAngle() int { return int(f.CalcStat(stat.PowerAttackAngle, 120)) }

// PoleAttackCountMax returns how many targets a pole swing reaches, its
// main target included.
func (f *Folk) PoleAttackCountMax() int { return int(f.CalcStat(stat.AttackCountMax, 0)) }

// WeaponReuseDelay returns the weapon's reuse delay; zero when unarmed.
func (f *Folk) WeaponReuseDelay() time.Duration {
	if f.fight.weapon == nil {
		return 0
	}
	return time.Duration(f.fight.weapon.ReuseDelay) * time.Millisecond
}

// WeaponGrade returns the weapon's crystal grade; zero when unarmed.
func (f *Folk) WeaponGrade() int { return int(f.fight.crystal) }

// ConsumeBowMP spends the weapon's MP cost as a bow fires.
func (f *Folk) ConsumeBowMP() {
	if w := f.fight.weapon; w != nil && w.MPConsume > 0 {
		f.ReduceMP(float64(w.MPConsume))
	}
}

// BroadcastAttack shows observers one swing.
func (f *Folk) BroadcastAttack(e event.Attack) { f.emit(e) }

// MakeAttackHit resolves one swing at target, as a hostile NPC's: a hit
// roll, a critical roll and the physical damage. A target with no combat
// stats is always missed.
func (f *Folk) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	hit := attack.Hit{TargetID: target.ObjectID()}
	other, ok := target.(creature.FormulaActor)
	if !ok {
		hit.Miss = true
		return hit
	}
	accuracy := int(f.CalcStat(stat.AccuracyCombat, 0))
	_, _, sz := f.Position()
	_, _, tz := other.Position()
	behind, inFront := creature.AttackFacing(other, f)
	rate := formulas.HitRate(accuracy, other.Against(f.ConditionActor(), nil).Evasion(), sz-tz, f.effects.IsNight(), behind, inFront)
	if formulas.Missed(rate, f.Roll(1000)) {
		hit.Miss = true
		return hit
	}
	crit := formulas.CritSucceeds(f.Against(conditions.ActorOf(other), nil).CriticalRate(), f.Roll(1000))
	in, shield := creature.ResolvePhysicalAttackInput(f, other, crit)
	hit.Damage = creature.ApplyPhysicalAttackDamage(in, shield, split)
	hit.Crit = crit
	hit.Shield = shield
	return hit
}
