package task

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// ---- from attackstance_test.go ----
type attackStanceFakeActor struct {
	id       int32
	queue    *sim.Queue
	owner    AttackStanceActor
	summon   AttackStanceActor
	cubics   []AttackStanceCubic
	inCombat bool
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

func (a *attackStanceFakeActor) SetInCombat(inCombat bool) bool {
	changed := a.inCombat != inCombat
	a.inCombat = inCombat
	return changed
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

type attackStanceNoopEffects struct{}

func (attackStanceNoopEffects) AutoAttackStop(AttackStanceActor) {}

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
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before refreshed deadline = %v, want none", got)
	}

	now = now.Add(14 * time.Second)
	stance.Tick()
	testLoop.Run()
	if got, want := effects.take(), []string{"stop 100"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at refreshed deadline = %v, want %v", got, want)
	}
	if stance.InAttackStance(actor) {
		t.Fatal("actor should be removed after timeout")
	}
}

func TestAttackStanceTickAllocationIsFlat(t *testing.T) {
	base := time.UnixMilli(0)
	now := base
	stance, err := NewAttackStance(attackStanceNoopEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	actors := make([]*attackStanceFakeActor, 128)
	for i := 0; i < 128; i++ {
		actors[i] = &attackStanceFakeActor{id: int32(i + 1)}
	}
	tick := func() {
		now = base
		for _, actor := range actors {
			stance.Add(actor)
		}
		now = base.Add(AttackStancePeriod)
		stance.Tick()
		testLoop.Run()
	}
	tick()
	for i, entry := range stance.scratch {
		if entry.actor != nil {
			t.Fatalf("scratch[%d] retains actor after Tick", i)
		}
	}

	// Each due actor costs the one task posted to its queue. The sweep
	// itself allocates nothing; the few extra allow for the test loop
	// growing its task slice. The budget describes the production build.
	if simdebugBuild {
		return
	}
	if allocs := testing.AllocsPerRun(100, tick); allocs > float64(len(actors)+4) {
		t.Fatalf("AllocsPerRun(128 actors) = %v, want <= %d", allocs, len(actors)+4)
	}
}

type attackStancePanicEffects struct{}

func (attackStancePanicEffects) AutoAttackStop(AttackStanceActor) { panic("boom") }

func TestAttackStanceTickClearsScratchOnPanic(t *testing.T) {
	base := time.UnixMilli(0)
	now := base
	stance, err := NewAttackStance(attackStancePanicEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	stance.Add(&attackStanceFakeActor{id: 1})
	now = base.Add(AttackStancePeriod)

	func() {
		defer func() { recover() }()
		stance.Tick()
		testLoop.Run()
	}()

	for i, entry := range stance.scratch {
		if entry.actor != nil {
			t.Fatalf("scratch[%d] retains actor after panicking Tick", i)
		}
	}
	if stance.ticking.Load() {
		t.Fatal("ticking guard left set after panicking Tick")
	}
}

// TestAttackStanceTickReturnsErrorOnReentrantCall covers a Tick that starts
// while another is still in flight: it does nothing and reports it.
func TestAttackStanceTickReturnsErrorOnReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &attackStanceFakeEffects{}
	stance, err := NewAttackStance(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	stance.Add(&attackStanceFakeActor{id: 1})
	now = now.Add(AttackStancePeriod)
	stance.ticking.Store(true) // another Tick is in flight

	if err := stance.Tick(); !errors.Is(err, ErrReentrantTick) {
		t.Fatalf("reentrant Tick() error = %v, want ErrReentrantTick", err)
	}
	testLoop.Run()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("reentrant Tick events = %v, want none", got)
	}
}

func TestAttackStanceTickLogsReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	stance, err := NewAttackStance(&attackStanceFakeEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	var buf bytes.Buffer
	stance.log = zerolog.New(&buf)
	stance.ticking.Store(true) // another Tick is in flight

	stance.Tick()
	testLoop.Run()

	if !strings.Contains(buf.String(), "AttackStance.Tick") || !strings.Contains(buf.String(), ErrReentrantTick.Error()) {
		t.Fatalf("reentrant Tick call was not logged, got %q", buf.String())
	}
}

func BenchmarkAttackStanceTickManyActors(b *testing.B) {
	now := time.UnixMilli(0)
	stance, err := NewAttackStance(attackStanceNoopEffects{}, func() time.Time { return now })
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 4096; i++ {
		stance.Add(&attackStanceFakeActor{id: int32(i + 1)})
	}
	stance.Tick()
	testLoop.Run()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stance.Tick()
		testLoop.Run()
	}
}

func TestAttackStanceConcurrentAccess(t *testing.T) {
	stance, err := NewAttackStance(attackStanceNoopEffects{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	actors := make([]*attackStanceFakeActor, 20)
	for i := range actors {
		actors[i] = &attackStanceFakeActor{id: int32(i + 1)}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, actor := range actors {
				stance.Add(actor)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, actor := range actors {
				stance.Remove(actor)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			stance.Tick()
			testLoop.Run()
		}
	}()
	wg.Wait()
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
	testLoop.Run()

	if got, want := effects.take(), []string{"stop 100", "stop 200"}; !slices.Equal(got, want) {
		t.Fatalf("timeout events = %v, want %v", got, want)
	}
}

func TestAttackStanceTimeoutClearsCombatFlag(t *testing.T) {
	now := time.UnixMilli(0)
	stance, err := NewAttackStance(attackStanceNoopEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	actor := &attackStanceFakeActor{id: 100, inCombat: true}
	stance.Add(actor)

	now = now.Add(AttackStancePeriod)
	if err := stance.Tick(); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	testLoop.Run()
	if actor.inCombat {
		t.Fatal("combat flag should clear after attack stance timeout")
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

func (a *attackStanceFakeActor) Queue() *sim.Queue { return cmp.Or(a.queue, testQueue) }

// TestAttackStanceRefreshBeforeQueuedExpiryKeepsStance covers the ordering a
// sweep on the ticker goroutine opens up: the expiry runs as a task on the
// actor's queue, and an attack that refreshes the deadline can reach that
// queue first. The refreshed stance must survive, and registry membership
// must agree with the combat flag.
func TestAttackStanceRefreshBeforeQueuedExpiryKeepsStance(t *testing.T) {
	inline := sim.NewInline(time.UnixMilli(0))
	base := time.UnixMilli(0)
	now := base
	effects := &attackStanceFakeEffects{}
	stance, err := NewAttackStance(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	actor := &attackStanceFakeActor{id: 1, queue: inline.NewQueue("actor-1"), inCombat: true}
	stance.Add(actor)

	now = base.Add(AttackStancePeriod + time.Second)
	// The fresh attack lands on the queue ahead of the sweep's expiry.
	actor.queue.Post(func() { stance.Add(actor) })
	if err := stance.Tick(); err != nil {
		t.Fatalf("Tick() = %v", err)
	}
	inline.Run()

	if !actor.inCombat {
		t.Error("refreshed stance: inCombat = false, want true")
	}
	if stops := effects.take(); len(stops) != 0 {
		t.Errorf("refreshed stance: AutoAttackStop calls = %v, want none", stops)
	}
	if !stance.InAttackStance(actor) {
		t.Error("refreshed stance: tracked = false, want true")
	}

	// With no refresh, the next sweep's queued expiry still stops it.
	now = now.Add(AttackStancePeriod + time.Second)
	if err := stance.Tick(); err != nil {
		t.Fatalf("Tick() = %v", err)
	}
	inline.Run()
	if actor.inCombat {
		t.Error("expired stance: inCombat = true, want false")
	}
	if stance.InAttackStance(actor) {
		t.Error("expired stance: still tracked")
	}
}
