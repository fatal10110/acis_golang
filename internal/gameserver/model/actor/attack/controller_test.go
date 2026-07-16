package attack

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

func TestCreatureAttackBroadcastsSimpleHitAndTracksState(t *testing.T) {
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponSword,
		attackSpeed: 500,
		soulshot:    true,
		weaponGrade: 2,
		hit: Hit{
			TargetID: 200,
			Damage:   37,
			Crit:     true,
			Shield:   formulas.ShieldSuccess,
		},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)

	ctrl.DoAttack(&target)
	defer ctrl.Stop()

	if !ctrl.AttackingNow() {
		t.Fatal("AttackingNow() = false after DoAttack")
	}
	if ctrl.BowCoolingDown() {
		t.Fatal("BowCoolingDown() = true for a sword attack")
	}
	if !ctrl.InHitAnimation() {
		t.Fatal("InHitAnimation() = false after DoAttack")
	}
	if actor.headingTarget != &target {
		t.Fatalf("heading target = %v, want attack target", actor.headingTarget)
	}
	wantFlags := uint8(HitSoulshot | 2 | HitCritical | HitShield)
	if len(actor.broadcasts) != 1 {
		t.Fatalf("broadcast count = %d, want 1", len(actor.broadcasts))
	}
	got := actor.broadcasts[0]
	if got.AttackerID != 100 || got.X != 10 || got.Y != 20 || got.Z != -30 {
		t.Fatalf("broadcast origin = %+v, want attacker id and position", got)
	}
	if len(got.Hits) != 1 || got.Hits[0] != (SnapshotHit{TargetID: 200, Damage: 37, Flags: wantFlags}) {
		t.Fatalf("broadcast hits = %+v, want one critical soulshot shield hit", got.Hits)
	}
}

func TestCreatureAttackLandsMeleeDamageMidSwing(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponSword,
		attackSpeed: 500,
		hit:         Hit{TargetID: 200, Damage: 37},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc

	ctrl.DoAttack(&target)

	if target.damageTaken != 0 {
		t.Fatalf("damage at swing start = %d, want 0", target.damageTaken)
	}
	clock.fire(500 * time.Millisecond)
	if target.damageTaken != 37 {
		t.Fatalf("damage after hit timer = %d, want 37", target.damageTaken)
	}
	if !ctrl.AttackingNow() {
		t.Fatal("AttackingNow() = false before full attack cycle")
	}
	clock.fire(time.Second)
	if ctrl.AttackingNow() {
		t.Fatal("AttackingNow() = true after full attack cycle")
	}
}

func TestCreatureAttackDropsInvalidPendingHit(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*attackActor, *attackTarget)
	}{
		{
			name: "target leaves known list",
			mutate: func(actor *attackActor, target *attackTarget) {
				actor.known[target.ObjectID()] = false
			},
		},
		{
			name: "target dies",
			mutate: func(actor *attackActor, target *attackTarget) {
				target.alikeDead = true
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeAttackClock{}
			actor := attackActor{
				id:          100,
				known:       map[int32]bool{200: true},
				canSee:      true,
				canReach:    true,
				attackType:  item.WeaponSword,
				attackSpeed: 500,
				hit:         Hit{TargetID: 200, Damage: 37},
			}
			target := attackTarget{id: 200, attackable: true}
			ctrl := NewCreature(&actor)
			ctrl.afterFunc = clock.AfterFunc

			ctrl.DoAttack(&target)
			test.mutate(&actor, &target)
			clock.fire(500 * time.Millisecond)

			if target.damageTaken != 0 {
				t.Fatalf("damage after invalidation = %d, want 0", target.damageTaken)
			}
		})
	}
}

func TestCreatureAttackStopCancelsPendingHit(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponSword,
		attackSpeed: 500,
		hit:         Hit{TargetID: 200, Damage: 37},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc

	ctrl.DoAttack(&target)
	ctrl.Stop()
	clock.fire(500 * time.Millisecond)

	if target.damageTaken != 0 {
		t.Fatalf("damage after Stop() = %d, want 0", target.damageTaken)
	}
}

func TestCreatureAttackRejectsNewAttackWhileBusy(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponSword,
		attackSpeed: 500,
		hit:         Hit{TargetID: 200, Damage: 37},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc

	ctrl.DoAttack(&target)
	if ctrl.CanAttack(&target) {
		t.Fatal("CanAttack() = true while the previous attack cycle is still active")
	}
	clock.fire(time.Second)
	if !ctrl.CanAttack(&target) {
		t.Fatal("CanAttack() = false after the attack cycle finished")
	}
}

func TestCreatureAttackRejectsBowReuseUntilCooldownEnds(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponBow,
		attackSpeed: 500,
		weaponReuse: time.Second,
		hit:         Hit{TargetID: 200, Damage: 37},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc

	ctrl.DoAttack(&target)
	clock.fire(time.Second)
	if ctrl.CanAttack(&target) {
		t.Fatal("CanAttack() = true while bow reuse is cooling down")
	}
	clock.fire(690 * time.Millisecond)
	if !ctrl.CanAttack(&target) {
		t.Fatal("CanAttack() = false after bow reuse cooldown ended")
	}
}

func TestCreatureAttackDualHitsAreSpacedOverFullCycle(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponDual,
		attackSpeed: 500,
		hit:         Hit{TargetID: 200, Damage: 40},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc

	ctrl.DoAttack(&target)

	clock.fire(250 * time.Millisecond)
	if target.damageTaken != 20 {
		t.Fatalf("damage after first dual hit = %d, want 20", target.damageTaken)
	}
	if !ctrl.AttackingNow() {
		t.Fatal("AttackingNow() = false after first dual hit")
	}

	clock.fire(750 * time.Millisecond)
	if target.damageTaken != 40 {
		t.Fatalf("damage after second dual hit = %d, want 40", target.damageTaken)
	}
	clock.fire(time.Second)
	if ctrl.AttackingNow() {
		t.Fatal("AttackingNow() = true after full dual cycle")
	}
}

func TestCreatureAttackBowLandsAtFullSwingAndScalesReuse(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponBow,
		attackSpeed: 500,
		weaponReuse: time.Second,
		hit:         Hit{TargetID: 200, Damage: 37},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc

	ctrl.DoAttack(&target)
	clock.fire(500 * time.Millisecond)
	if target.damageTaken != 0 {
		t.Fatalf("bow damage at half swing = %d, want 0", target.damageTaken)
	}

	clock.fire(time.Second)
	if target.damageTaken != 37 {
		t.Fatalf("bow damage at full swing = %d, want 37", target.damageTaken)
	}
	if !ctrl.BowCoolingDown() {
		t.Fatal("BowCoolingDown() = false before scaled reuse timer")
	}

	clock.fire(690 * time.Millisecond)
	if ctrl.BowCoolingDown() {
		t.Fatal("BowCoolingDown() = true after scaled reuse timer")
	}
}

func TestCreatureAttackBowHitStillLandsWhenFinishTimerRunsFirst(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponBow,
		attackSpeed: 500,
		hit:         Hit{TargetID: 200, Damage: 37},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc

	ctrl.DoAttack(&target)
	clock.fireReverse(time.Second)

	if target.damageTaken != 37 {
		t.Fatalf("bow damage when finish timer wins = %d, want 37", target.damageTaken)
	}
}

func TestCreatureAttackSetFinishedFiresOnceMeleeSwingCompletes(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponSword,
		attackSpeed: 500,
		hit:         Hit{TargetID: 200, Damage: 37},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc
	finishedCalls := 0
	ctrl.SetFinished(func() { finishedCalls++ })

	ctrl.DoAttack(&target)
	clock.fire(500 * time.Millisecond)
	if finishedCalls != 0 {
		t.Fatalf("finished calls at hit landing = %d, want 0", finishedCalls)
	}

	clock.fire(time.Second)
	if finishedCalls != 1 {
		t.Fatalf("finished calls after full swing = %d, want 1", finishedCalls)
	}
}

func TestCreatureAttackSetFinishedFiresOnceBowReuseClears(t *testing.T) {
	clock := &fakeAttackClock{}
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponBow,
		attackSpeed: 500,
		weaponReuse: time.Second,
		hit:         Hit{TargetID: 200, Damage: 37},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)
	ctrl.afterFunc = clock.AfterFunc
	finishedCalls := 0
	ctrl.SetFinished(func() { finishedCalls++ })

	ctrl.DoAttack(&target)
	clock.fire(time.Second)
	if finishedCalls != 0 {
		t.Fatalf("finished calls at full swing (still cooling) = %d, want 0", finishedCalls)
	}

	clock.fire(690 * time.Millisecond)
	if finishedCalls != 1 {
		t.Fatalf("finished calls after scaled reuse timer = %d, want 1", finishedCalls)
	}
}

func TestPlayableAttackRejectsPeaceZoneForPlayableTargets(t *testing.T) {
	actor := attackActor{known: map[int32]bool{200: true}, canSee: true, canReach: true, playable: true, peace: true}
	target := attackTarget{id: 200, attackable: true, playable: true}

	if NewPlayable(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for playable attacker in peace zone")
	}

	actor.peace = false
	target.peace = true
	if NewPlayable(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for playable target in peace zone")
	}
}

func TestPlayerAttackRejectsFishingRodAndBowRequirements(t *testing.T) {
	target := attackTarget{id: 200, attackable: true}

	actor := playerAttackActor{attackActor: attackActor{
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		playable:    true,
		attackType:  item.WeaponFishingRod,
		attackSpeed: 500,
	}}
	if NewPlayer(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true with a fishing rod")
	}

	actor.attackType = item.WeaponBow
	actor.arrows = false
	if NewPlayer(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for bow without arrows")
	}

	actor.arrows = true
	actor.mpConsume = 5
	actor.mp = 4
	if NewPlayer(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for bow without enough MP")
	}
}

func TestPlayerAttackClearsFakeDeathAndStopSendsActionFailure(t *testing.T) {
	actor := playerAttackActor{attackActor: attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		playable:    true,
		attackType:  item.WeaponSword,
		attackSpeed: 500,
		hit:         Hit{TargetID: 200, Damage: 1},
	}}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewPlayer(&actor)

	ctrl.DoAttack(&target)
	ctrl.Stop()

	if actor.clearFakeDeathCalls != 1 {
		t.Fatalf("ClearRecentFakeDeath calls = %d, want 1", actor.clearFakeDeathCalls)
	}
	if actor.idleCalls != 1 {
		t.Fatalf("TryToIdle calls = %d, want 1", actor.idleCalls)
	}
	if actor.actionFailedCalls != 1 {
		t.Fatalf("ClientActionFailed calls = %d, want 1", actor.actionFailedCalls)
	}
}

func TestAttackableAttackRejectsFakeDeathTarget(t *testing.T) {
	actor := attackActor{known: map[int32]bool{200: true}, canSee: true, canReach: true}
	target := attackTarget{id: 200, attackable: true, fakeDeath: true}

	if NewAttackable(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for fake-death target")
	}
}

type attackActor struct {
	id int32

	known    map[int32]bool
	canSee   bool
	canReach bool

	attackDisabled   bool
	movementDisabled bool
	attackType       item.WeaponType
	attackSpeed      int
	weaponReuse      time.Duration
	weaponGrade      int
	soulshot         bool
	hit              Hit
	playable         bool
	peace            bool

	headingTarget attackable.Combatant
	broadcasts    []Snapshot
	idleCalls     int
}

func (a *attackActor) ObjectID() int32 {
	if a.id == 0 {
		return 100
	}
	return a.id
}

func (a *attackActor) SiegeGuard() bool { return false }
func (a *attackActor) AlikeDead() bool  { return false }
func (a *attackActor) AttackDisabled() bool {
	return a.attackDisabled
}
func (a *attackActor) MovementDisabled() bool {
	return a.movementDisabled
}
func (a *attackActor) InAttackRange(attackable.Combatant) bool {
	return a.canReach
}
func (a *attackActor) Knows(target attackable.Combatant) bool {
	return a.known[target.ObjectID()]
}
func (a *attackActor) CanSee(attackable.Combatant) bool { return a.canSee }
func (a *attackActor) AttackType() item.WeaponType      { return a.attackType }
func (a *attackActor) AttackSpeed() int {
	if a.attackSpeed == 0 {
		return 500
	}
	return a.attackSpeed
}
func (a *attackActor) WeaponReuseDelay() time.Duration { return a.weaponReuse }
func (a *attackActor) WeaponGrade() int                { return a.weaponGrade }
func (a *attackActor) SoulshotCharged() bool           { return a.soulshot }
func (a *attackActor) Position() (int, int, int)       { return 10, 20, -30 }
func (a *attackActor) SetHeadingTo(target attackable.Combatant) {
	a.headingTarget = target
}
func (a *attackActor) MakeAttackHit(target attackable.Combatant, split bool) Hit {
	hit := a.hit
	hit.Target = target
	if hit.TargetID == 0 {
		hit.TargetID = target.ObjectID()
	}
	if split {
		hit.Damage /= 2
	}
	return hit
}
func (a *attackActor) BroadcastAttack(snapshot Snapshot) {
	a.broadcasts = append(a.broadcasts, snapshot)
}
func (a *attackActor) InPeaceZone() bool { return a.peace }
func (a *attackActor) TryToIdle()        { a.idleCalls++ }

type playerAttackActor struct {
	attackActor

	arrows              bool
	mpConsume           int
	mp                  int
	clearFakeDeathCalls int
	actionFailedCalls   int
}

func (a *playerAttackActor) CheckAndEquipArrows() bool { return a.arrows }
func (a *playerAttackActor) WeaponMPConsume() int      { return a.mpConsume }
func (a *playerAttackActor) MP() int                   { return a.mp }
func (a *playerAttackActor) ClearRecentFakeDeath()     { a.clearFakeDeathCalls++ }
func (a *playerAttackActor) ClientActionFailed()       { a.actionFailedCalls++ }

type attackTarget struct {
	id          int32
	attackable  bool
	siegeGuard  bool
	alikeDead   bool
	playable    bool
	peace       bool
	fakeDeath   bool
	damageTaken int
}

func (t *attackTarget) ObjectID() int32  { return t.id }
func (t *attackTarget) SiegeGuard() bool { return t.siegeGuard }
func (t *attackTarget) AlikeDead() bool  { return t.alikeDead }
func (t *attackTarget) AttackableBy(CreatureActor) bool {
	return t.attackable
}
func (t *attackTarget) Playable() bool    { return t.playable }
func (t *attackTarget) InPeaceZone() bool { return t.peace }
func (t *attackTarget) FakeDeath() bool   { return t.fakeDeath }
func (t *attackTarget) TakeDamage(dmg int, _ creature.DeathActor) bool {
	t.damageTaken += dmg
	return false
}

type fakeAttackClock struct {
	timers []*fakeAttackTimer
}

func (c *fakeAttackClock) AfterFunc(delay time.Duration, f func()) scheduledTimer {
	timer := &fakeAttackTimer{delay: delay, f: f}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *fakeAttackClock) fire(delay time.Duration) {
	for _, timer := range c.timers {
		if timer.delay == delay && !timer.stopped {
			timer.stopped = true
			timer.f()
		}
	}
}

func (c *fakeAttackClock) fireReverse(delay time.Duration) {
	for i := len(c.timers) - 1; i >= 0; i-- {
		timer := c.timers[i]
		if timer.delay == delay && !timer.stopped {
			timer.stopped = true
			timer.f()
		}
	}
}

type fakeAttackTimer struct {
	delay   time.Duration
	f       func()
	stopped bool
}

func (t *fakeAttackTimer) Stop() bool {
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}
