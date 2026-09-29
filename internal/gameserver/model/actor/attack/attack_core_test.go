package attack

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target/targettest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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
			ctrl := NewPlayable(actor, nil)

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
	clock := newTimingClock()
	ctrl := NewCreature(actor, nil)
	ctrl.SetQueue(clock.q)
	rec := &event.Recorder{}
	ctrl.sink = rec
	finished := func() int { return event.Count[event.AttackFinished](rec) }
	// Each hit arms the next step only once it has landed, so a hit whose
	// task runs late on a busy queue delays the rest of the attack with it.
	target.onDamage = func() {
		if got, want := armedTimers(ctrl), target.hits; got != want {
			t.Fatalf("timers armed while hit %d lands = %d, want %d", target.hits, got, want)
		}
	}

	ctrl.DoAttack(target)
	if next := clock.next(); next != 500*time.Millisecond {
		t.Fatalf("first timer due at %v, want attackTime/2", next)
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
	if !ctrl.AttackingNow() || finished() != 0 {
		t.Fatalf("completion before 3*attackTime/2: attacking = %v, finished = %d; want true, 0", ctrl.AttackingNow(), finished())
	}
	clock.fire(1500 * time.Millisecond)
	if ctrl.AttackingNow() || finished() != 1 {
		t.Fatalf("completion at 3*attackTime/2: attacking = %v, finished = %d; want false, 1", ctrl.AttackingNow(), finished())
	}
}

func TestControllerStopsWhenMainTargetDiesBeforeHit(t *testing.T) {
	actor := &timingActor{attackSpeed: 500}
	target := &timingTarget{id: 2}
	clock := newTimingClock()
	ctrl := NewCreature(actor, nil)
	ctrl.SetQueue(clock.q)

	ctrl.DoAttack(target)
	target.dead = true
	clock.fire(500 * time.Millisecond)

	if ctrl.AttackingNow() {
		t.Fatal("AttackingNow() after main target dies before hit = true, want false")
	}
}

func TestControllerStopIsSilent(t *testing.T) {
	actor := &timingPlayer{}
	NewPlayer(actor, nil).Stop()

	if actor.actionFailed != 0 {
		t.Fatalf("Stop() ActionFailed notifications = %d, want 0", actor.actionFailed)
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
	ctrl := NewPlayer(actor, nil)
	ctrl.SetQueue(newTimingClock().q)

	ctrl.DoAttack(target)
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
	ctrl := NewCreature(actor, nil)
	ctrl.SetQueue(newTimingClock().q)

	ctrl.DoAttack(target)
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
	clock := newTimingClock()
	ctrl := NewPlayer(actor, nil)
	ctrl.SetQueue(clock.q)

	ctrl.DoAttack(target)
	if actor.drawMs != 2035 {
		t.Fatalf("NotifyBowDraw ms = %d, want 2035", actor.drawMs)
	}

	actor.attackSpeed = 350
	clock.fire(time.Second)
	if !ctrl.BowCoolingDown() {
		t.Fatal("BowCoolingDown() once the attack finished = false, want true")
	}
	clock.fire(time.Second + 1034*time.Millisecond)
	if !ctrl.BowCoolingDown() {
		t.Fatal("BowCoolingDown() 1ms before the frozen 1035ms reuse = false, want true")
	}
	clock.fire(time.Second + 1035*time.Millisecond)
	if ctrl.BowCoolingDown() {
		t.Fatal("BowCoolingDown() at the frozen 1035ms reuse = true, want false (a live recompute would hold it to 1478ms)")
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
	clock := newTimingClock()
	ctrl := NewCreature(actor, nil)
	ctrl.SetQueue(clock.q)
	rec := &event.Recorder{}
	ctrl.sink = rec
	finished := func() int { return event.Count[event.AttackFinished](rec) }
	primary.onDamage = func() {
		// One timer for the whole pole group; completion is armed only
		// after the group has landed.
		if got := armedTimers(ctrl); got != 1 {
			t.Fatalf("timers armed while the pole group lands = %d, want 1 (one pole group)", got)
		}
		actor.dead = true
		ctrl.Stop()
	}

	ctrl.DoAttack(primary)

	if actor.queryRadius != 100 {
		t.Fatalf("known-combatant radius = %d, want 100", actor.queryRadius)
	}
	if got, want := snapshotTargetIDs(actor.snapshot), []int32{2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("snapshot target IDs = %v, want %v", got, want)
	}
	if actor.broadcasts != 1 {
		t.Fatalf("attack broadcasts = %d, want 1", actor.broadcasts)
	}
	if next := clock.next(); next != 500*time.Millisecond {
		t.Fatalf("first timer due at %v, want the pole group at attackTime/2", next)
	}

	clock.fire(500 * time.Millisecond)
	if finished() != 0 {
		t.Fatalf("pole completed with its hit group: finished = %d, want 0", finished())
	}
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
	clock := newTimingClock()
	ctrl := NewCreature(actor, nil)
	ctrl.SetQueue(clock.q)

	ctrl.DoAttack(primary)

	if got, want := snapshotTargetIDs(actor.snapshot), []int32{2}; !slices.Equal(got, want) {
		t.Fatalf("snapshot target IDs = %v, want %v", got, want)
	}
	clock.fire(500 * time.Millisecond)
	if primary.hits != 1 || secondary.hits != 0 {
		t.Fatalf("hits at attackTime/2 = primary %d, secondary %d; want 1, 0", primary.hits, secondary.hits)
	}
}

func snapshotTargetIDs(snapshot event.Attack) []int32 {
	ids := make([]int32, len(snapshot.Hits))
	for i, hit := range snapshot.Hits {
		ids[i] = hit.TargetID
	}
	return ids
}

// timingClock drives a controller's queue on a virtual clock, addressed in
// offsets from the attack's start.
type timingClock struct {
	in    *sim.Inline
	q     *sim.Queue
	start time.Time
}

func newTimingClock() *timingClock {
	start := time.Unix(1000, 0)
	in := sim.NewInline(start)
	return &timingClock{in: in, q: in.NewQueue("attacker"), start: start}
}

// fire runs everything due up to offset at.
func (c *timingClock) fire(at time.Duration) {
	c.in.Advance(c.start.Add(at).Sub(c.in.Now()))
}

// next is the offset of the earliest armed timer.
func (c *timingClock) next() time.Duration {
	at, ok := c.in.NextTimer()
	if !ok {
		return -1
	}
	return at.Sub(c.start)
}

// armedTimers counts the timers ctrl armed for its current attack.
func armedTimers(ctrl *Controller) int {
	ctrl.mu.RLock()
	defer ctrl.mu.RUnlock()
	return len(ctrl.timers)
}

type timingActor struct {
	world.Presence
	targettest.Actor
	attackType       item.WeaponType
	attackSpeed      int
	reuse            time.Duration
	poleMax          int
	known            []attackable.Combatant
	queryRadius      int
	snapshot         event.Attack
	broadcasts       int
	events           []string
	dead             bool
	movementDisabled bool
	outOfRange       bool
	stats            map[stat.Stat]float64
	level            int
}

type timingPlayer struct {
	timingActor
	actionFailed int
	drawMs       int
}

type curseTimingPlayer struct {
	world.Presence
	timingPlayer
	blocks     bool
	curseCalls int
}

func (a *curseTimingPlayer) TestCursesOnAttack(attackable.Combatant) bool {
	a.curseCalls++
	return a.blocks
}

func (a *timingPlayer) InPeaceZone() bool         { return false }
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
func (a *timingPlayer) BroadcastStatus()      { a.events = append(a.events, "status") }
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
func (a *timingActor) CalcStat(s stat.Stat, base float64) float64 {
	if v, ok := a.stats[s]; ok {
		return v
	}
	return base
}

func (a *timingActor) AddHP(amount float64) float64 {
	a.events = append(a.events, fmt.Sprintf("absorb %g", amount))
	return amount
}

func (a *timingActor) TakeDamage(damage int, attacker attackable.Combatant) bool {
	a.events = append(a.events, fmt.Sprintf("reflected %d from %d", damage, attacker.ObjectID()))
	return false
}
func (a *timingActor) Level() int { return a.level }
func (a *timingActor) BroadcastAttack(snapshot event.Attack) {
	a.snapshot = snapshot
	a.broadcasts++
	a.events = append(a.events, "broadcast")
}

type timingTarget struct {
	world.Presence
	targettest.Actor
	id          int32
	x, y, z     int
	attackable  bool
	dead        bool
	hits        int
	landed      *[]int32
	onDamage    func()
	raidRelated bool
	invul       bool
	paralyzed   bool
}

func (t *timingTarget) ObjectID() int32   { return t.id }
func (t *timingTarget) SiegeGuard() bool  { return false }
func (t *timingTarget) AlikeDead() bool   { return t.dead }
func (t *timingTarget) Heading() int      { return 0 }
func (t *timingTarget) Dead() bool        { return t.dead }
func (t *timingTarget) RaidRelated() bool { return t.raidRelated }
func (t *timingTarget) Position() (int, int, int) {
	return t.x, t.y, t.z
}
func (t *timingTarget) AttackableBy(target.Actor) bool             { return t.attackable }
func (t *timingTarget) AttackableWithoutForceBy(target.Actor) bool { return t.attackable }
func (t *timingTarget) TakeDamage(_ int, _ attackable.Combatant) bool {
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
	ctrl := NewCreature(actor, nil)

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
	attackabletest.Combatant
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

func (*curseTimingPlayer) Kind() actor.Kind { return actor.KindNPC }

func (*timingTarget) Kind() actor.Kind { return actor.KindNPC }

func (*timingActor) Kind() actor.Kind { return actor.KindNPC }

func (rangeTarget) Kind() actor.Kind { return actor.KindNPC }

func (rangeTarget) Heading() int { return 0 }

func (*timingPlayer) NotePvPAttack(attackable.Combatant) {}

func (*timingPlayer) TestCursesOnAttack(attackable.Combatant) bool { return false }

// TestControllerFinishClosesHitAnimation pins CreatureAttack.clearAttackTask
// run as a swing or bow shot finishes: the hit animation window closes then,
// even when its 300ms timer has not fired yet, so the AI re-run that follows
// sees it closed.
func TestControllerFinishClosesHitAnimation(t *testing.T) {
	tests := []struct {
		name     string
		actor    *timingActor
		finishAt time.Duration
	}{
		// attackTime 500ms: the hit lands at 250ms, the swing finishes at
		// 500ms, before the hit animation's 550ms end.
		{name: "fast melee swing", actor: &timingActor{attackSpeed: 1000}, finishAt: 500 * time.Millisecond},
		// attackTime 1000ms: the arrow lands and the shot finishes at 1000ms.
		{name: "bow shot", actor: &timingActor{attackType: item.WeaponBow, attackSpeed: 500, reuse: 1500 * time.Millisecond}, finishAt: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newTimingClock()
			ctrl := NewAttackable(tt.actor, nil)
			ctrl.SetQueue(clock.q)

			ctrl.DoAttack(&timingTarget{id: 2})
			clock.fire(tt.finishAt - time.Millisecond)
			if !ctrl.InHitAnimation() {
				t.Fatal("InHitAnimation() just before the finish = false, want true")
			}
			clock.fire(tt.finishAt)
			if ctrl.InHitAnimation() {
				t.Fatal("InHitAnimation() once the attack finished = true, want false")
			}
		})
	}
}

// TestControllerAttackRethinkOnlyForNPCs pins which attack phases re-run an
// NPC's AI: the hit animation ending and a bow shot landing emit
// AttackRethink from an NPC controller only; a swing finishing reports
// AttackFinished, and a bow's reuse ending AttackFinished{BowReuse}.
func TestControllerAttackRethinkOnlyForNPCs(t *testing.T) {
	newCtrl := map[string]func(*timingPlayer) *Controller{
		"npc":      func(a *timingPlayer) *Controller { return NewAttackable(&a.timingActor, nil) },
		"creature": func(a *timingPlayer) *Controller { return NewCreature(&a.timingActor, nil) },
		"player":   func(a *timingPlayer) *Controller { return NewPlayer(a, nil) },
	}
	for name, build := range newCtrl {
		npc := name == "npc"
		t.Run(name+"/melee", func(t *testing.T) {
			// attackTime 1000ms: hit at 500ms, hit animation ends at 800ms,
			// swing finishes at 1000ms.
			actor := &timingPlayer{timingActor: timingActor{attackSpeed: 500}}
			clock := newTimingClock()
			ctrl := build(actor)
			ctrl.SetQueue(clock.q)
			rec := &event.Recorder{}
			ctrl.sink = rec

			ctrl.DoAttack(&timingTarget{id: 2})
			clock.fire(799 * time.Millisecond)
			if got := event.Count[event.AttackRethink](rec); got != 0 {
				t.Fatalf("AttackRethink inside the hit animation = %d, want 0", got)
			}
			clock.fire(800 * time.Millisecond)
			if got, want := event.Count[event.AttackRethink](rec), rethinks(npc, 1); got != want {
				t.Fatalf("AttackRethink at the hit animation end = %d, want %d", got, want)
			}
			clock.fire(time.Second)
			if got, want := event.Count[event.AttackRethink](rec), rethinks(npc, 1); got != want {
				t.Fatalf("AttackRethink once the swing finished = %d, want %d", got, want)
			}
			if got := event.Of[event.AttackFinished](rec); len(got) != 1 || got[0].BowReuse {
				t.Fatalf("AttackFinished once the swing finished = %+v, want one swing finish", got)
			}
		})
		t.Run(name+"/bow", func(t *testing.T) {
			// attackTime 1000ms: the arrow lands at 1000ms, the hit animation
			// ends at 1300ms, the 1035ms reuse ends at 2035ms.
			actor := &timingPlayer{timingActor: timingActor{attackType: item.WeaponBow, attackSpeed: 500, reuse: 1500 * time.Millisecond}}
			clock := newTimingClock()
			ctrl := build(actor)
			ctrl.SetQueue(clock.q)
			rec := &event.Recorder{}
			ctrl.sink = rec

			ctrl.DoAttack(&timingTarget{id: 2})
			clock.fire(999 * time.Millisecond)
			if got := event.Count[event.AttackRethink](rec); got != 0 {
				t.Fatalf("AttackRethink before the arrow lands = %d, want 0", got)
			}
			clock.fire(time.Second)
			if got, want := event.Count[event.AttackRethink](rec), rethinks(npc, 1); got != want {
				t.Fatalf("AttackRethink once the arrow landed = %d, want %d", got, want)
			}
			if got := event.Count[event.AttackFinished](rec); got != 0 {
				t.Fatalf("AttackFinished before the reuse ends = %d, want 0", got)
			}
			clock.fire(1300 * time.Millisecond)
			if got, want := event.Count[event.AttackRethink](rec), rethinks(npc, 2); got != want {
				t.Fatalf("AttackRethink at the hit animation end = %d, want %d", got, want)
			}
			clock.fire(2034 * time.Millisecond)
			if got := event.Count[event.AttackFinished](rec); got != 0 {
				t.Fatalf("AttackFinished before the reuse ends = %d, want 0", got)
			}
			clock.fire(2035 * time.Millisecond)
			if got := event.Of[event.AttackFinished](rec); len(got) != 1 || !got[0].BowReuse {
				t.Fatalf("AttackFinished once the reuse ended = %+v, want one bow reuse finish", got)
			}
		})
	}
}

// rethinks is the AttackRethink count an NPC controller has emitted by a
// phase; any other controller emits none.
func rethinks(npc bool, n int) int {
	if npc {
		return n
	}
	return 0
}

// TestControllerReportsHitFeedbackPerActorKind pins the attacker's damage
// feedback on every resolved hit: a player reports misses, criticals and
// the target's invulnerability; a summon reports only landed hits on
// anyone but its owner; an NPC reports nothing. The feedback goes out
// before the target takes the damage.
func TestControllerReportsHitFeedbackPerActorKind(t *testing.T) {
	owner := &timingTarget{id: 9}
	tests := []struct {
		name   string
		build  func() *Controller
		hit    Hit
		target *timingTarget
		want   []event.HitDealt
	}{
		{
			name:  "player miss",
			build: func() *Controller { return NewPlayer(&timingPlayer{}, nil) },
			hit:   Hit{Miss: true},
			want:  []event.HitDealt{{Miss: true}},
		},
		{
			name:  "player critical",
			build: func() *Controller { return NewPlayer(&timingPlayer{}, nil) },
			hit:   Hit{Damage: 120, Crit: true},
			want:  []event.HitDealt{{Damage: 120, Crit: true}},
		},
		{
			name:  "player normal hit",
			build: func() *Controller { return NewPlayer(&timingPlayer{}, nil) },
			hit:   Hit{Damage: 45},
			want:  []event.HitDealt{{Damage: 45}},
		},
		{
			name:   "player against invulnerable target",
			build:  func() *Controller { return NewPlayer(&timingPlayer{}, nil) },
			hit:    Hit{Damage: 45},
			target: &timingTarget{id: 2, invul: true},
			want:   []event.HitDealt{{Damage: 45, Blocked: true}},
		},
		{
			name:   "player against petrified target",
			build:  func() *Controller { return NewPlayer(&timingPlayer{}, nil) },
			hit:    Hit{Damage: 45, Crit: true},
			target: &timingTarget{id: 2, invul: true, paralyzed: true},
			want:   []event.HitDealt{{Damage: 45, Crit: true, Blocked: true, Petrified: true}},
		},
		{
			name:   "paralyzed but vulnerable target takes the plain message",
			build:  func() *Controller { return NewPlayer(&timingPlayer{}, nil) },
			hit:    Hit{Damage: 45},
			target: &timingTarget{id: 2, paralyzed: true},
			want:   []event.HitDealt{{Damage: 45}},
		},
		{
			name:  "summon critical on another target",
			build: func() *Controller { return NewPlayable(&timingSummon{owner: owner}, nil) },
			hit:   Hit{Damage: 80, Crit: true},
			want:  []event.HitDealt{{Damage: 80, Crit: true}},
		},
		{
			name:  "summon miss is silent",
			build: func() *Controller { return NewPlayable(&timingSummon{owner: owner}, nil) },
			hit:   Hit{Miss: true},
		},
		{
			name:   "summon hitting its owner is silent",
			build:  func() *Controller { return NewPlayable(&timingSummon{owner: owner}, nil) },
			hit:    Hit{Damage: 80},
			target: &timingTarget{id: owner.id},
		},
		{
			name:  "ownerless summon is silent",
			build: func() *Controller { return NewPlayable(&timingSummon{}, nil) },
			hit:   Hit{Damage: 80},
		},
		{
			name:  "npc is silent",
			build: func() *Controller { return NewAttackable(&timingActor{}, nil) },
			hit:   Hit{Damage: 80, Crit: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := tt.build()
			rec := &event.Recorder{}
			ctrl.sink = rec
			target := tt.target
			if target == nil {
				target = &timingTarget{id: 2}
			}
			reportedBeforeDamage := -1
			target.onDamage = func() { reportedBeforeDamage = event.Count[event.HitDealt](rec) }
			hit := tt.hit
			hit.Target = target

			ctrl.deliverHits(0, []Hit{hit})

			if got := event.Of[event.HitDealt](rec); !slices.Equal(got, tt.want) {
				t.Fatalf("HitDealt = %+v, want %+v", got, tt.want)
			}
			if hit.Miss {
				if target.hits != 0 {
					t.Fatalf("missed hit damaged the target %d times", target.hits)
				}
				return
			}
			if target.hits != 1 {
				t.Fatalf("landed hit damaged the target %d times, want 1", target.hits)
			}
			if reportedBeforeDamage != len(tt.want) {
				t.Fatalf("HitDealt reported when the damage landed = %d, want %d (feedback before damage)", reportedBeforeDamage, len(tt.want))
			}
		})
	}
}

// TestControllerReportsEachPoleHit covers a pole sweep: every hit of the
// group reports its own feedback, in landing order.
func TestControllerReportsEachPoleHit(t *testing.T) {
	primary := &timingTarget{id: 2, x: 40, attackable: true}
	secondary := &timingTarget{id: 3, x: 50, attackable: true, invul: true}
	actor := &timingPlayer{timingActor: timingActor{
		attackType:  item.WeaponPole,
		attackSpeed: 500,
		poleMax:     2,
		known:       []attackable.Combatant{secondary},
	}}
	clock := newTimingClock()
	ctrl := NewPlayer(actor, nil)
	ctrl.SetQueue(clock.q)
	rec := &event.Recorder{}
	ctrl.sink = rec

	ctrl.DoAttack(primary)
	clock.fire(500 * time.Millisecond)

	want := []event.HitDealt{{Damage: 1}, {Damage: 1, Blocked: true}}
	if got := event.Of[event.HitDealt](rec); !slices.Equal(got, want) {
		t.Fatalf("pole HitDealt = %+v, want %+v", got, want)
	}
}

type timingSummon struct {
	timingActor
	owner attackable.Combatant
}

func (a *timingSummon) InPeaceZone() bool                            { return false }
func (a *timingSummon) TestCursesOnAttack(attackable.Combatant) bool { return false }
func (a *timingSummon) Owner() (attackable.Combatant, bool) {
	return a.owner, a.owner != nil
}

func (t *timingTarget) Invul() bool     { return t.invul }
func (t *timingTarget) Paralyzed() bool { return t.paralyzed }

// reactingTarget is a hit target whose AI reacts to hits, as players and
// summons do. log is shared with the attacker's sink so the test can read
// the order in which the attacker and the target heard of each hit.
type reactingTarget struct {
	timingTarget
	log *[]string
}

func (t *reactingTarget) NotifyAttacked(attacker attackable.Combatant) {
	*t.log = append(*t.log, fmt.Sprintf("attacked %d by %d", t.id, attacker.ObjectID()))
}

func (t *reactingTarget) NotifyEvaded(attacker attackable.Combatant) {
	*t.log = append(*t.log, fmt.Sprintf("evaded %d by %d", t.id, attacker.ObjectID()))
}

// orderSink logs the attacker-side events a hit reports into the same log
// the targets write to.
type orderSink struct{ log *[]string }

func (s orderSink) Emit(e event.Event) {
	switch e := e.(type) {
	case event.HitDealt:
		*s.log = append(*s.log, fmt.Sprintf("feedback miss=%v", e.Miss))
	case event.AttackStanceRequested:
		*s.log = append(*s.log, "stance")
	case event.HitLanded:
		*s.log = append(*s.log, fmt.Sprintf("landed %d", e.Target.ObjectID()))
	}
}

// TestControllerNotifiesHitTargets pins CreatureAttack.doHit's reactions
// (CreatureAttack.java:223-240) for every hit of a group: a miss tells the
// target it evaded before the attacker's feedback; a damaging hit puts the
// attacker in stance and tells the target it was attacked, both before the
// damage lands; a zero-damage hit does neither; a target without an AI
// reaction is only damaged.
func TestControllerNotifiesHitTargets(t *testing.T) {
	var log []string
	actor := &timingPlayer{}
	ctrl := NewPlayer(actor, orderSink{log: &log})
	primary := &reactingTarget{timingTarget: timingTarget{id: 2}, log: &log}
	secondary := &reactingTarget{timingTarget: timingTarget{id: 3}, log: &log}
	zero := &reactingTarget{timingTarget: timingTarget{id: 4}, log: &log}
	plain := &timingTarget{id: 5}
	for _, target := range []*reactingTarget{primary, secondary, zero} {
		target.onDamage = func() { log = append(log, fmt.Sprintf("damage %d", target.id)) }
	}
	plain.onDamage = func() { log = append(log, "damage 5") }

	ctrl.deliverHits(0, []Hit{
		{Target: primary, Damage: 30},
		{Target: secondary, Miss: true},
		{Target: zero},
		{Target: plain, Damage: 10},
	})
	// A dual weapon's second hit lands as its own group.
	ctrl.deliverHits(0, []Hit{{Target: primary, Miss: true}})

	want := []string{
		"feedback miss=false", "stance", "attacked 2 by 1", "damage 2", "landed 2",
		"evaded 3 by 1", "feedback miss=true",
		"feedback miss=false",
		"feedback miss=false", "stance", "damage 5", "landed 5",
		"evaded 2 by 1", "feedback miss=true",
	}
	if !slices.Equal(log, want) {
		t.Fatalf("hit order =\n%q\nwant\n%q", log, want)
	}
}

// reflectTarget is a hit target carrying stats and a cast a hit may break.
// It logs into the attacker's event log, so a test reads the order in which
// both sides of a hit applied.
type reflectTarget struct {
	timingTarget
	level  int
	stats  map[stat.Stat]float64
	maxHP  float64
	events *[]string
}

func (t *reflectTarget) Level() int { return t.level }
func (t *reflectTarget) CalcStat(s stat.Stat, base float64) float64 {
	if v, ok := t.stats[s]; ok {
		return v
	}
	return base
}
func (t *reflectTarget) MaxHPValue() float64 { return t.maxHP }
func (t *reflectTarget) BreakCastOnDamage(damage float64) {
	*t.events = append(*t.events, fmt.Sprintf("break %g", damage))
}

// hitLandedLog records the HitLanded events a hit reports into the
// attacker's event log.
type hitLandedLog struct{ events *[]string }

func (s hitLandedLog) Emit(e event.Event) {
	if e, ok := e.(event.HitLanded); ok {
		*s.events = append(*s.events, fmt.Sprintf("landed reflected=%v", e.Reflected))
	}
}

// TestControllerReflectAbsorbOrder pins CreatureAttack.doHit's damage steps
// (CreatureAttack.java:243-289): the target takes the hit, the attacker
// takes the reflected share (REFLECT_DAMAGE_PERCENT of the damage, capped at
// the target's max HP), heals ABSORB_DAMAGE_PERCENT of the damage, the
// target's cast-break roll runs, and only then do the procs run. A bow
// neither reflects nor absorbs; an invulnerable target reflects nothing and
// rolls no break; a raid-related target reflects nothing onto a player more
// than 8 levels above it.
func TestControllerReflectAbsorbOrder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		weapon      item.WeaponType
		damage      int
		reflect     float64
		maxHP       float64
		invul       bool
		raid        bool
		playerLevel int
		want        []string
	}{
		{
			name: "melee", weapon: item.WeaponSword, damage: 99, reflect: 10, maxHP: 1000,
			want: []string{"damage", "reflected 9 from 2", "absorb 29.7", "status", "break 99", "landed reflected=true"},
		},
		{
			name: "reflect capped at max HP", weapon: item.WeaponSword, damage: 100, reflect: 50, maxHP: 40,
			want: []string{"damage", "reflected 40 from 2", "absorb 30", "status", "break 100", "landed reflected=true"},
		},
		{
			name: "bow", weapon: item.WeaponBow, damage: 99, reflect: 10, maxHP: 1000,
			want: []string{"damage", "break 99", "landed reflected=false"},
		},
		{
			name: "invulnerable target", weapon: item.WeaponSword, damage: 99, reflect: 10, maxHP: 1000, invul: true,
			want: []string{"damage", "absorb 29.7", "status", "landed reflected=false"},
		},
		{
			name: "raid 8 levels below", weapon: item.WeaponSword, damage: 99, reflect: 10, maxHP: 1000, raid: true, playerLevel: 28,
			want: []string{"damage", "reflected 9 from 2", "absorb 29.7", "status", "landed reflected=true"},
		},
		{
			name: "raid 9 levels below", weapon: item.WeaponSword, damage: 99, reflect: 10, maxHP: 1000, raid: true, playerLevel: 29,
			want: []string{"damage", "absorb 29.7", "status", "landed reflected=false"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := &timingPlayer{timingActor: timingActor{
				attackType: tc.weapon,
				level:      tc.playerLevel,
				stats:      map[stat.Stat]float64{stat.AbsorbDamagePercent: 30},
			}}
			ctrl := NewPlayer(actor, hitLandedLog{events: &actor.events})
			target := &reflectTarget{
				timingTarget: timingTarget{id: 2, invul: tc.invul, raidRelated: tc.raid},
				level:        20,
				stats:        map[stat.Stat]float64{stat.ReflectDamagePercent: tc.reflect},
				maxHP:        tc.maxHP,
				events:       &actor.events,
			}
			target.onDamage = func() { actor.events = append(actor.events, "damage") }

			ctrl.deliverHit(Hit{Target: target, Damage: tc.damage})

			if !slices.Equal(actor.events, tc.want) {
				t.Fatalf("hit steps =\n%q\nwant\n%q", actor.events, tc.want)
			}
		})
	}
}
