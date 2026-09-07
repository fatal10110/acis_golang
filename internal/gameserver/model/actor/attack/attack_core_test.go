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

// ---- from controller_test.go ----
func TestControllerRaidCurseGateBeforeDamage(t *testing.T) {
	tests := []struct {
		name       string
		blocks     bool
		wantDamage int
	}{
		{name: "level gap petrification blocks", blocks: true},
		{name: "mounted anti-strider curse continues", wantDamage: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &curseTimingPlayer{timingPlayer: timingPlayer{timingActor: timingActor{attackSpeed: 500}}, blocks: tt.blocks}
			target := &timingTarget{id: 2, raidRelated: true}
			ctrl := NewPlayable(actor)

			ctrl.deliverHits(0, []Hit{{Target: target, Damage: 1}})

			if actor.curseCalls != 1 {
				t.Fatalf("curse checks = %d, want 1", actor.curseCalls)
			}
			if target.hits != tt.wantDamage {
				t.Fatalf("damage hits = %d, want %d", target.hits, tt.wantDamage)
			}
		})
	}
}

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
	if got := clock.activeCount(500 * time.Millisecond); got != 1 {
		t.Fatalf("first-hit timers at attackTime/2 = %d, want 1", got)
	}
	if got := clock.activeCount(time.Second); got != 0 {
		t.Fatalf("completion timers before second landing = %d, want 0", got)
	}

	clock.fire(250 * time.Millisecond)
	if target.hits != 0 {
		t.Fatalf("hits at attackTime/4 = %d, want 0", target.hits)
	}
	clock.fire(500 * time.Millisecond)
	if target.hits != 1 {
		t.Fatalf("hits at attackTime/2 = %d, want 1", target.hits)
	}
	clock.fire(800 * time.Millisecond)
	if ctrl.InHitAnimation() {
		t.Fatal("InHitAnimation() after first dual landing + 300ms = true, want false")
	}
	clock.fire(time.Second)
	if target.hits != 2 {
		t.Fatalf("hits at attackTime = %d, want 2", target.hits)
	}
	if !ctrl.AttackingNow() || finished != 0 {
		t.Fatalf("completion before 3*attackTime/2: attacking = %v, finished = %d; want true, 0", ctrl.AttackingNow(), finished)
	}
	clock.fire(1500 * time.Millisecond)
	if ctrl.AttackingNow() || finished != 1 {
		t.Fatalf("completion at 3*attackTime/2: attacking = %v, finished = %d; want false, 1", ctrl.AttackingNow(), finished)
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
			clock.fire(time.Second)
		}
	}

	if err := ctrl.DoAttack(target); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}
	clock.fire(500 * time.Millisecond)
	if target.hits != 1 || finished != 0 {
		t.Fatalf("after slow first hit: hits = %d, finished = %d; want 1, 0", target.hits, finished)
	}
	clock.fire(1500 * time.Millisecond)
	if target.hits != 2 || finished != 0 {
		t.Fatalf("after delayed second hit: hits = %d, finished = %d; want 2, 0", target.hits, finished)
	}
	clock.fire(2 * time.Second)
	if finished != 1 {
		t.Fatalf("finished after delayed second hit = %d, want 1", finished)
	}
}

func TestControllerStopsWhenMainTargetDiesBeforeHit(t *testing.T) {
	actor := &timingActor{attackSpeed: 500}
	target := &timingTarget{id: 2}
	clock := &timingClock{}
	ctrl := NewCreature(actor)
	ctrl.afterFunc = clock.AfterFunc

	if err := ctrl.DoAttack(target); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}
	target.dead = true
	clock.fire(500 * time.Millisecond)

	if ctrl.AttackingNow() {
		t.Fatal("AttackingNow() after main target dies before hit = true, want false")
	}
}

func TestControllerStopIsSilent(t *testing.T) {
	actor := &timingPlayer{}
	NewPlayer(actor).Stop()

	if actor.idles != 0 || actor.actionFailed != 0 {
		t.Fatalf("Stop() notifications = idle %d, ActionFailed %d; want 0, 0", actor.idles, actor.actionFailed)
	}
}

func TestControllerBowFireConsumesThenDrawsThenBroadcasts(t *testing.T) {
	// Independent oracle: sAtk = max(100, 500000/500) = 1000; scaled reuse
	// = 1500*345/500 = 1035; gauge = 2035.
	const wantGauge = 2035

	actor := &timingPlayer{timingActor: timingActor{
		attackType:  item.WeaponBow,
		attackSpeed: 500,
		reuse:       1500 * time.Millisecond,
	}}
	target := &timingTarget{id: 2}
	ctrl := NewPlayer(actor)
	ctrl.afterFunc = (&timingClock{}).AfterFunc

	if err := ctrl.DoAttack(target); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}
	if got, want := actor.events, []string{"consume", "mp", "hit", "draw", "broadcast"}; !slices.Equal(got, want) {
		t.Fatalf("bow fire events = %v, want %v", got, want)
	}
	if actor.drawMs != wantGauge {
		t.Fatalf("NotifyBowDraw ms = %d, want %d", actor.drawMs, wantGauge)
	}
}

func TestControllerBowFireSkipsPlayerPacketsForCreatures(t *testing.T) {
	actor := &timingActor{attackType: item.WeaponBow, attackSpeed: 500}
	target := &timingTarget{id: 2}
	ctrl := NewCreature(actor)
	ctrl.afterFunc = (&timingClock{}).AfterFunc

	if err := ctrl.DoAttack(target); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}
	if got, want := actor.events, []string{"mp", "hit", "broadcast"}; !slices.Equal(got, want) {
		t.Fatalf("creature bow events = %v, want %v", got, want)
	}
}

func TestControllerBowReuseIsFrozenAtFireTime(t *testing.T) {
	// Independent oracle at fire time: sAtk = 1000; scaled reuse
	// = 1500*345/500 = 1035. After the draw window, AttackSpeed 350 would
	// recompute reuse as 1478; the cooldown must stay 1035.
	actor := &timingPlayer{timingActor: timingActor{
		attackType:  item.WeaponBow,
		attackSpeed: 500,
		reuse:       1500 * time.Millisecond,
	}}
	target := &timingTarget{id: 2}
	clock := &timingClock{}
	ctrl := NewPlayer(actor)
	ctrl.afterFunc = clock.AfterFunc

	if err := ctrl.DoAttack(target); err != nil {
		t.Fatalf("DoAttack() error: %v", err)
	}
	if actor.drawMs != 2035 {
		t.Fatalf("NotifyBowDraw ms = %d, want 2035", actor.drawMs)
	}

	actor.attackSpeed = 350
	clock.fire(time.Second)
	clock.fire(time.Second)

	if got := clock.activeCount(2035 * time.Millisecond); got != 1 {
		t.Fatalf("frozen reuse timers at 2035ms = %d, want 1", got)
	}
	if got := clock.activeCount(2478 * time.Millisecond); got != 0 {
		t.Fatalf("live-recomputed reuse timers at 2478ms = %d, want 0", got)
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
	attackType       item.WeaponType
	attackSpeed      int
	reuse            time.Duration
	poleMax          int
	known            []attackable.Combatant
	queryRadius      int
	snapshot         Snapshot
	broadcasts       int
	events           []string
	dead             bool
	movementDisabled bool
	outOfRange       bool
}

type timingPlayer struct {
	timingActor
	idles        int
	actionFailed int
	drawMs       int
}

type curseTimingPlayer struct {
	timingPlayer
	blocks     bool
	curseCalls int
}

func (a *curseTimingPlayer) TestCursesOnAttack(attackable.Combatant) bool {
	a.curseCalls++
	return a.blocks
}

func (a *timingPlayer) InPeaceZone() bool         { return false }
func (a *timingPlayer) TryToIdle()                { a.idles++ }
func (a *timingPlayer) CheckAndEquipArrows() bool { return true }
func (a *timingPlayer) WeaponMPConsume() int      { return 0 }
func (a *timingPlayer) MP() int                   { return 1 }
func (a *timingPlayer) ConsumeBowShot() {
	a.events = append(a.events, "consume")
}
func (a *timingPlayer) NotifyBowDraw(gaugeMs int) {
	a.drawMs = gaugeMs
	a.events = append(a.events, "draw")
}
func (a *timingPlayer) ClearRecentFakeDeath() {}
func (a *timingPlayer) ClientActionFailed()   { a.actionFailed++ }

func (a *timingActor) ObjectID() int32                         { return 1 }
func (a *timingActor) SiegeGuard() bool                        { return false }
func (a *timingActor) AlikeDead() bool                         { return a.dead }
func (a *timingActor) AttackDisabled() bool                    { return false }
func (a *timingActor) MovementDisabled() bool                  { return a.movementDisabled }
func (a *timingActor) InAttackRange(attackable.Combatant) bool { return !a.outOfRange }
func (a *timingActor) Knows(attackable.Combatant) bool         { return true }
func (a *timingActor) CanSee(attackable.Combatant) bool        { return true }
func (a *timingActor) AttackType() item.WeaponType             { return a.attackType }
func (a *timingActor) AttackSpeed() int                        { return a.attackSpeed }
func (a *timingActor) WeaponReuseDelay() time.Duration         { return a.reuse }
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
	a.events = append(a.events, "hit")
	return Hit{Target: t, Damage: 1}
}
func (a *timingActor) ConsumeBowMP() { a.events = append(a.events, "mp") }
func (a *timingActor) BroadcastAttack(snapshot Snapshot) error {
	a.snapshot = snapshot
	a.broadcasts++
	a.events = append(a.events, "broadcast")
	return nil
}

type timingTarget struct {
	id          int32
	x, y, z     int
	attackable  bool
	dead        bool
	hits        int
	landed      *[]int32
	onDamage    func()
	raidRelated bool
}

func (t *timingTarget) ObjectID() int32   { return t.id }
func (t *timingTarget) SiegeGuard() bool  { return false }
func (t *timingTarget) AlikeDead() bool   { return t.dead }
func (t *timingTarget) Heading() int      { return 0 }
func (t *timingTarget) Dead() bool        { return t.dead }
func (t *timingTarget) RaidRelated() bool { return t.raidRelated }
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

func TestControllerRejectsOutOfRangeWhenMovementDisabled(t *testing.T) {
	actor := &timingActor{attackSpeed: 300, movementDisabled: true, outOfRange: true}
	target := &timingTarget{id: 2, attackable: true}
	ctrl := NewCreature(actor)

	if ctrl.CanAttack(target) {
		t.Fatal("CanAttack() = true when movement-disabled and out of range")
	}
	actor.outOfRange = false
	if !ctrl.CanAttack(target) {
		t.Fatal("CanAttack() = false when movement-disabled but in range")
	}
	actor.movementDisabled = false
	actor.outOfRange = true
	if !ctrl.CanAttack(target) {
		t.Fatal("CanAttack() = false when mobile and out of range; range is only gated while movement-disabled")
	}
}

func TestPhysicalReachTruncatesSumOnce(t *testing.T) {
	// Independent oracle: int(range + r1 + r2 + grace) with grace 10/60.
	const attackRange = 40
	const attackerR = 9.6
	const targetR = 11.6

	if got, want := PhysicalReach(attackRange, attackerR, targetR, false), 71; got != want {
		t.Fatalf("standing PhysicalReach() = %d, want %d (sum-then-trunc, not per-radius)", got, want)
	}
	if got, want := PhysicalReach(attackRange, attackerR, targetR, true), 121; got != want {
		t.Fatalf("moving PhysicalReach() = %d, want %d", got, want)
	}
	if got, want := PhysicalReach(40, 0, 0, false), 50; got != want {
		t.Fatalf("zero-radius standing PhysicalReach() = %d, want %d", got, want)
	}
	if got, want := PhysicalReach(40, 0, 0, true), 100; got != want {
		t.Fatalf("zero-radius moving PhysicalReach() = %d, want %d", got, want)
	}
}

func TestInPhysicalRange2DGraceAndBoundary(t *testing.T) {
	from := location.Location{X: 0, Y: 0, Z: 0}

	tests := []struct {
		name     string
		atkRange int
		selfR    float64
		target   rangeTarget
		want     bool
	}{
		{
			name:     "altitude ignored inside 2D reach",
			atkRange: 40,
			target:   rangeTarget{x: 49, z: 1000},
			want:     true,
		},
		{
			name:     "strict 2D boundary excluded",
			atkRange: 40,
			target:   rangeTarget{x: 50},
		},
		{
			name:     "one unit inside standing reach",
			atkRange: 40,
			target:   rangeTarget{x: 49},
			want:     true,
		},
		{
			name:     "moving grace extends reach",
			atkRange: 40,
			target:   rangeTarget{x: 99, moving: true},
			want:     true,
		},
		{
			name:     "moving grace still strict at boundary",
			atkRange: 40,
			target:   rangeTarget{x: 100, moving: true},
		},
		{
			name:     "fractional radii use summed truncation",
			atkRange: 40,
			selfR:    9.6,
			target:   rangeTarget{x: 70, radius: 11.6},
			want:     true,
		},
		{
			name:     "fractional radii exclude exact truncated reach",
			atkRange: 40,
			selfR:    9.6,
			target:   rangeTarget{x: 71, radius: 11.6},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InPhysicalRange(from, tt.atkRange, tt.selfR, tt.target)
			if got != tt.want {
				t.Fatalf("InPhysicalRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

type rangeTarget struct {
	x, y, z int
	radius  float64
	moving  bool
}

func (t rangeTarget) ObjectID() int32           { return 2 }
func (t rangeTarget) SiegeGuard() bool          { return false }
func (t rangeTarget) AlikeDead() bool           { return false }
func (t rangeTarget) Position() (int, int, int) { return t.x, t.y, t.z }
func (t rangeTarget) CollisionRadius() float64  { return t.radius }
func (t rangeTarget) IsMoving() bool            { return t.moving }
