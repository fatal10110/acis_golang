package attack

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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
	clock.fire(750 * time.Millisecond)
	if ctrl.AttackingNow() || finished != 1 {
		t.Fatalf("completion at 3*attackTime/4: attacking = %v, finished = %d; want false, 1", ctrl.AttackingNow(), finished)
	}
}

type timingClock struct{ timers []*timingTimer }

func (c *timingClock) AfterFunc(delay time.Duration, f func()) scheduledTimer {
	timer := &timingTimer{delay: delay, f: f}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *timingClock) fire(delay time.Duration) {
	for _, timer := range c.timers {
		if timer.delay == delay && !timer.stopped {
			timer.stopped = true
			timer.f()
		}
	}
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
}

func (a *timingActor) ObjectID() int32                         { return 1 }
func (a *timingActor) SiegeGuard() bool                        { return false }
func (a *timingActor) AlikeDead() bool                         { return false }
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
func (a *timingActor) Dead() bool                              { return false }
func (a *timingActor) Category() target.Category               { return target.CategoryAttackable }
func (a *timingActor) SetHeadingTo(attackable.Combatant)       {}
func (a *timingActor) MakeAttackHit(t attackable.Combatant, _ bool) Hit {
	return Hit{Target: t, Damage: 1}
}
func (a *timingActor) BroadcastAttack(Snapshot) error { return nil }

type timingTarget struct {
	id   int32
	hits int
}

func (t *timingTarget) ObjectID() int32  { return t.id }
func (t *timingTarget) SiegeGuard() bool { return false }
func (t *timingTarget) AlikeDead() bool  { return false }
func (t *timingTarget) TakeDamage(_ int, _ creature.DeathActor) bool {
	t.hits++
	return false
}
