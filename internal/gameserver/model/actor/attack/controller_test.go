package attack

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestControllerDualHitAndCompletionTiming(t *testing.T) {
	actor := &timingActor{attackType: item.WeaponDual, attackSpeed: 500}
	target := &timingTarget{id: 2}
	clock := &timingClock{}
	ctrl := NewCreature(actor)
	ctrl.afterFunc = clock.AfterFunc
	finished := 0
	ctrl.SetFinished(func() { finished++ })

	if err := ctrl.DoAttack(target); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}
	if got := clock.activeCount(500 * time.Millisecond); got != 0 {
		t.Fatalf("second-hit timers before first landing = %d, want 0", got)
	}
	if got := clock.activeCount(750 * time.Millisecond); got != 0 {
		t.Fatalf("completion timers before second landing = %d, want 0", got)
	}

	clock.fire(250 * time.Millisecond)
	if target.hits != 1 {
		t.Fatalf("hits at attackTime/4 = %d, want 1", target.hits)
	}
	clock.fire(500 * time.Millisecond)
	if target.hits != 2 {
		t.Fatalf("hits at attackTime/2 = %d, want 2", target.hits)
	}
	if !ctrl.AttackingNow() || finished != 0 {
		t.Fatalf("completion before 3*attackTime/4: attacking = %v, finished = %d; want true, 0", ctrl.AttackingNow(), finished)
	}
	clock.fire(550 * time.Millisecond)
	if ctrl.InHitAnimation() {
		t.Fatal("InHitAnimation() after first dual landing + 300ms = true, want false")
	}
	clock.fire(750 * time.Millisecond)
	if ctrl.AttackingNow() || finished != 1 {
		t.Fatalf("completion at 3*attackTime/4: attacking = %v, finished = %d; want false, 1", ctrl.AttackingNow(), finished)
	}
}

func TestControllerDualSlowFirstHitDelaysSecondHitAndCompletion(t *testing.T) {
	actor := &timingActor{attackType: item.WeaponDual, attackSpeed: 500}
	target := &timingTarget{id: 2}
	clock := &timingClock{}
	ctrl := NewCreature(actor)
	ctrl.afterFunc = clock.AfterFunc
	finished := 0
	ctrl.SetFinished(func() { finished++ })
	target.onDamage = func() {
		if target.hits == 1 {
			clock.fire(750 * time.Millisecond)
		}
	}

	if err := ctrl.DoAttack(target); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}
	clock.fire(250 * time.Millisecond)
	if target.hits != 1 || finished != 0 {
		t.Fatalf("after slow first hit: hits = %d, finished = %d; want 1, 0", target.hits, finished)
	}
	clock.fire(time.Second)
	if target.hits != 2 || finished != 0 {
		t.Fatalf("after delayed second hit: hits = %d, finished = %d; want 2, 0", target.hits, finished)
	}
	clock.fire(1250 * time.Millisecond)
	if finished != 1 {
		t.Fatalf("finished after delayed second hit = %d, want 1", finished)
	}
}

func TestControllerPoleSelectsForwardTargetsUpToCap(t *testing.T) {
	var landed []int32
	primary := &timingTarget{id: 2, x: 40, attackable: true, landed: &landed}
	first := &timingTarget{id: 3, x: 50, y: 20, attackable: true, landed: &landed}
	second := &timingTarget{id: 4, x: 60, y: -20, attackable: true, landed: &landed}
	beyondCap := &timingTarget{id: 5, x: 70, attackable: true}
	behind := &timingTarget{id: 6, x: -30, attackable: true}
	outOfRange := &timingTarget{id: 7, x: 101, attackable: true}
	notAttackable := &timingTarget{id: 8, x: 30}
	outsideCone := &timingTarget{id: 9, x: 30, y: 83, attackable: true}
	actor := &timingActor{
		attackType:  item.WeaponPole,
		attackSpeed: 500,
		poleMax:     3,
	}
	actor.known = []attackable.Combatant{actor, primary, outsideCone, first, second, beyondCap, behind, outOfRange, notAttackable}
	clock := &timingClock{}
	ctrl := NewCreature(actor)
	ctrl.afterFunc = clock.AfterFunc
	finished := 0
	ctrl.SetFinished(func() { finished++ })
	primary.onDamage = func() {
		clock.fire(time.Second)
		if finished != 0 {
			t.Fatalf("pole completed during hit group: finished = %d, want 0", finished)
		}
		actor.dead = true
		ctrl.Stop()
	}

	if err := ctrl.DoAttack(primary); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}

	if actor.queryRadius != 100 {
		t.Fatalf("known-combatant radius = %d, want 100", actor.queryRadius)
	}
	if got, want := snapshotTargetIDs(actor.snapshot), []int32{2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("snapshot target IDs = %v, want %v", got, want)
	}
	if actor.broadcasts != 1 {
		t.Fatalf("attack broadcasts = %d, want 1", actor.broadcasts)
	}
	if got := clock.count(500 * time.Millisecond); got != 1 {
		t.Fatalf("timers at attackTime/2 = %d, want one pole group", got)
	}
	if got := clock.activeCount(time.Second); got != 0 {
		t.Fatalf("completion timers before pole group landing = %d, want 0", got)
	}

	clock.fire(500 * time.Millisecond)
	if want := []int32{2, 3, 4}; !slices.Equal(landed, want) {
		t.Fatalf("landing order = %v, want %v", landed, want)
	}
	for _, target := range []*timingTarget{primary, first, second} {
		if target.hits != 1 {
			t.Errorf("target %d hits at attackTime/2 = %d, want 1", target.id, target.hits)
		}
	}
	for _, target := range []*timingTarget{beyondCap, behind, outOfRange, notAttackable, outsideCone} {
		if target.hits != 0 {
			t.Errorf("excluded target %d hits = %d, want 0", target.id, target.hits)
		}
	}
}

func TestControllerPoleSingleTargetEffectKeepsOnlyPrimary(t *testing.T) {
	primary := &timingTarget{id: 2, x: 40, attackable: true}
	secondary := &timingTarget{id: 3, x: 50, attackable: true}
	actor := &timingActor{
		attackType:  item.WeaponPole,
		attackSpeed: 500,
		poleMax:     1,
		known:       []attackable.Combatant{secondary},
	}
	clock := &timingClock{}
	ctrl := NewCreature(actor)
	ctrl.afterFunc = clock.AfterFunc

	if err := ctrl.DoAttack(primary); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}

	if got, want := snapshotTargetIDs(actor.snapshot), []int32{2}; !slices.Equal(got, want) {
		t.Fatalf("snapshot target IDs = %v, want %v", got, want)
	}
	clock.fire(500 * time.Millisecond)
	if primary.hits != 1 || secondary.hits != 0 {
		t.Fatalf("hits at attackTime/2 = primary %d, secondary %d; want 1, 0", primary.hits, secondary.hits)
	}
}

func snapshotTargetIDs(snapshot Snapshot) []int32 {
	ids := make([]int32, len(snapshot.Hits))
	for i, hit := range snapshot.Hits {
		ids[i] = hit.TargetID
	}
	return ids
}

type timingClock struct {
	now    time.Duration
	timers []*timingTimer
}

func (c *timingClock) AfterFunc(delay time.Duration, f func()) scheduledTimer {
	timer := &timingTimer{delay: c.now + delay, f: f}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *timingClock) fire(delay time.Duration) {
	c.now = delay
	for _, timer := range c.timers {
		if timer.delay == delay && !timer.stopped {
			timer.stopped = true
			timer.f()
		}
	}
}

func (c *timingClock) activeCount(delay time.Duration) int {
	count := 0
	for _, timer := range c.timers {
		if timer.delay == delay && !timer.stopped {
			count++
		}
	}
	return count
}

func (c *timingClock) count(delay time.Duration) int {
	count := 0
	for _, timer := range c.timers {
		if timer.delay == delay {
			count++
		}
	}
	return count
}

type timingTimer struct {
	delay   time.Duration
	f       func()
	stopped bool
}

func (t *timingTimer) Stop() bool {
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}

type timingActor struct {
	attackType  item.WeaponType
	attackSpeed int
	poleMax     int
	known       []attackable.Combatant
	queryRadius int
	snapshot    Snapshot
	broadcasts  int
	dead        bool
}

func (a *timingActor) ObjectID() int32                         { return 1 }
func (a *timingActor) SiegeGuard() bool                        { return false }
func (a *timingActor) AlikeDead() bool                         { return a.dead }
func (a *timingActor) AttackDisabled() bool                    { return false }
func (a *timingActor) MovementDisabled() bool                  { return false }
func (a *timingActor) InAttackRange(attackable.Combatant) bool { return true }
func (a *timingActor) Knows(attackable.Combatant) bool         { return true }
func (a *timingActor) CanSee(attackable.Combatant) bool        { return true }
func (a *timingActor) AttackType() item.WeaponType             { return a.attackType }
func (a *timingActor) AttackSpeed() int                        { return a.attackSpeed }
func (a *timingActor) WeaponReuseDelay() time.Duration         { return 0 }
func (a *timingActor) WeaponGrade() int                        { return 0 }
func (a *timingActor) SoulshotCharged() bool                   { return false }
func (a *timingActor) SetChargedShot(item.ShotKind, bool)      {}
func (a *timingActor) Position() (int, int, int)               { return 0, 0, 0 }
func (a *timingActor) Heading() int                            { return 0 }
func (a *timingActor) Dead() bool                              { return a.dead }
func (a *timingActor) Category() target.Category               { return target.CategoryAttackable }
func (a *timingActor) SetHeadingTo(attackable.Combatant)       {}

func (a *timingActor) PhysicalAttackRange() int { return 100 }
func (a *timingActor) PoleAttackAngle() int     { return 120 }
func (a *timingActor) PoleAttackCountMax() int  { return a.poleMax }
func (a *timingActor) ForEachKnownCombatantInRadius(radius int, fn func(attackable.Combatant)) {
	a.queryRadius = radius
	for _, candidate := range a.known {
		positioned, ok := candidate.(interface{ Position() (int, int, int) })
		if !ok {
			continue
		}
		x, y, z := positioned.Position()
		if location.In3DRange(0, 0, 0, x, y, z, radius) {
			fn(candidate)
		}
	}
}
func (a *timingActor) MakeAttackHit(t attackable.Combatant, _ bool) Hit {
	return Hit{Target: t, Damage: 1}
}
func (a *timingActor) BroadcastAttack(snapshot Snapshot) error {
	a.snapshot = snapshot
	a.broadcasts++
	return nil
}

type timingTarget struct {
	id         int32
	x, y, z    int
	attackable bool
	hits       int
	landed     *[]int32
	onDamage   func()
}

func (t *timingTarget) ObjectID() int32  { return t.id }
func (t *timingTarget) SiegeGuard() bool { return false }
func (t *timingTarget) AlikeDead() bool  { return false }
func (t *timingTarget) Heading() int     { return 0 }
func (t *timingTarget) Dead() bool       { return false }
func (t *timingTarget) Category() target.Category {
	return target.CategoryAttackable
}
func (t *timingTarget) Position() (int, int, int) {
	return t.x, t.y, t.z
}
func (t *timingTarget) AttackableBy(target.Creature) bool             { return t.attackable }
func (t *timingTarget) AttackableWithoutForceBy(target.Creature) bool { return t.attackable }
func (t *timingTarget) TakeDamage(_ int, _ creature.DeathActor) bool {
	t.hits++
	if t.landed != nil {
		*t.landed = append(*t.landed, t.id)
	}
	if t.onDamage != nil {
		t.onDamage()
	}
	return false
}
