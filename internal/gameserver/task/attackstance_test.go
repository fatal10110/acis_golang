package task

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

type attackStanceFakeActor struct {
	id     int32
	owner  AttackStanceActor
	summon AttackStanceActor
	cubics []AttackStanceCubic
}

func (a *attackStanceFakeActor) ObjectID() int32 { return a.id }
func (a *attackStanceFakeActor) Owner() AttackStanceActor {
	return a.owner
}
func (a *attackStanceFakeActor) Summon() AttackStanceActor {
	return a.summon
}
func (a *attackStanceFakeActor) Cubics() []AttackStanceCubic {
	return a.cubics
}

type attackStanceFakeCubic struct {
	id      int
	actions int
}

func (c *attackStanceFakeCubic) ID() int { return c.id }
func (c *attackStanceFakeCubic) Action() { c.actions++ }

type attackStanceFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *attackStanceFakeEffects) AutoAttackStop(actor AttackStanceActor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, fmt.Sprintf("stop %d", actor.ObjectID()))
}

func (e *attackStanceFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestAttackStanceAddRefreshesTimeoutAndFiresCubics(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &attackStanceFakeEffects{}
	stance, err := NewAttackStance(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}

	life := &attackStanceFakeCubic{id: LifeCubicID}
	damage := &attackStanceFakeCubic{id: 7}
	actor := &attackStanceFakeActor{id: 100, cubics: []AttackStanceCubic{life, damage}}

	stance.Add(actor)
	if !stance.InAttackStance(actor) {
		t.Fatal("actor should be in attack stance after Add")
	}
	if life.actions != 0 || damage.actions != 1 {
		t.Fatalf("cubic actions = life:%d damage:%d, want 0/1", life.actions, damage.actions)
	}

	now = now.Add(14 * time.Second)
	stance.Add(actor)
	now = now.Add(time.Second)
	stance.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before refreshed deadline = %v, want none", got)
	}

	now = now.Add(14 * time.Second)
	stance.Tick()
	if got, want := effects.take(), []string{"stop 100"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at refreshed deadline = %v, want %v", got, want)
	}
	if stance.InAttackStance(actor) {
		t.Fatal("actor should be removed after timeout")
	}
}

func TestAttackStanceTimeoutAlsoStopsPlayerSummon(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &attackStanceFakeEffects{}
	stance, _ := NewAttackStance(effects, func() time.Time { return now })
	summon := &attackStanceFakeActor{id: 200}
	player := &attackStanceFakeActor{id: 100, summon: summon}

	stance.Add(player)
	now = now.Add(AttackStancePeriod)
	stance.Tick()

	if got, want := effects.take(), []string{"stop 100", "stop 200"}; !slices.Equal(got, want) {
		t.Fatalf("timeout events = %v, want %v", got, want)
	}
}

func TestAttackStanceSummonUsesOwnerRegistration(t *testing.T) {
	effects := &attackStanceFakeEffects{}
	stance, _ := NewAttackStance(effects, nil)
	owner := &attackStanceFakeActor{id: 100}
	summon := &attackStanceFakeActor{id: 200, owner: owner}

	stance.Add(owner)
	if !stance.InAttackStance(summon) {
		t.Fatal("summon should report owner's attack stance")
	}
	if !stance.Remove(summon) {
		t.Fatal("Remove(summon) should remove the owner entry")
	}
	if stance.InAttackStance(owner) {
		t.Fatal("owner should no longer be in attack stance after removing summon")
	}
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Remove should not emit stop packet itself, got %v", got)
	}
}
