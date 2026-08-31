package task

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/rs/zerolog"
)

// ---- from attackstance_test.go ----
type attackStanceFakeActor struct {
	id       int32
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
	}
	tick()
	for i, entry := range stance.scratch {
		if entry.actor != nil {
			t.Fatalf("scratch[%d] retains actor after Tick", i)
		}
	}

	if allocs := testing.AllocsPerRun(100, tick); allocs != 0 {
		t.Fatalf("AllocsPerRun(128 actors) = %v, want 0", allocs)
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

type attackStanceReentrantEffects struct {
	stance   *AttackStance
	innerErr error
}

func (e *attackStanceReentrantEffects) AutoAttackStop(AttackStanceActor) {
	e.innerErr = e.stance.Tick()
}

func TestAttackStanceTickReturnsErrorOnReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &attackStanceReentrantEffects{}
	stance, err := NewAttackStance(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	effects.stance = stance
	stance.Add(&attackStanceFakeActor{id: 1})
	now = now.Add(AttackStancePeriod)

	if err := stance.Tick(); err != nil {
		t.Fatalf("outer Tick() error = %v, want nil", err)
	}

	if !errors.Is(effects.innerErr, ErrReentrantTick) {
		t.Fatalf("reentrant Tick() error = %v, want ErrReentrantTick", effects.innerErr)
	}
	if stance.ticking.Load() {
		t.Fatal("ticking guard left set after outer Tick returned")
	}
}

func TestAttackStanceTickLogsReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &attackStanceReentrantEffects{}
	stance, err := NewAttackStance(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	effects.stance = stance
	var buf bytes.Buffer
	stance.log = zerolog.New(&buf)
	stance.Add(&attackStanceFakeActor{id: 1})
	now = now.Add(AttackStancePeriod)

	stance.Tick()

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

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stance.Tick()
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

// ---- from autosave_test.go ----
type autosaveFakeActor struct {
	id int32
}

func (a *autosaveFakeActor) ObjectID() int32 { return a.id }

type autosaveFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *autosaveFakeEffects) Save(actor AutosaveActor) {
	e.record(fmt.Sprintf("%d save", actor.ObjectID()))
}

func (e *autosaveFakeEffects) record(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, s)
}

func (e *autosaveFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestAutosaveFiresAfterInitialDelayThenRepeatsAtInterval(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &autosaveFakeEffects{}
	a, err := NewAutosave(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAutosave() error = %v", err)
	}

	actor := &autosaveFakeActor{id: 1}
	a.Add(actor)

	now = now.Add(AutosaveInitialDelay - time.Second)
	a.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before initial delay elapsed = %v, want none", got)
	}

	now = now.Add(time.Second)
	a.Tick()
	if got, want := effects.take(), []string{"1 save"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at initial delay = %v, want %v", got, want)
	}

	now = now.Add(AutosaveInterval - time.Second)
	a.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before interval elapsed = %v, want none", got)
	}

	now = now.Add(time.Second)
	a.Tick()
	if got, want := effects.take(), []string{"1 save"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at next interval = %v, want %v", got, want)
	}
}

func TestAutosaveRemoveStopsSaving(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &autosaveFakeEffects{}
	a, _ := NewAutosave(effects, func() time.Time { return now })

	actor := &autosaveFakeActor{id: 1}
	a.Add(actor)
	a.Remove(actor.ObjectID())

	now = now.Add(AutosaveInitialDelay)
	a.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after Remove = %v, want none", got)
	}
}

func TestAutosaveRemoveUntrackedActorIsNoop(t *testing.T) {
	effects := &autosaveFakeEffects{}
	a, _ := NewAutosave(effects, nil)

	a.Remove(1)
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Remove on untracked actor = %v, want none", got)
	}
}

func TestAutosaveAddAlreadyTrackedActorIsNoop(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &autosaveFakeEffects{}
	a, _ := NewAutosave(effects, func() time.Time { return now })

	actor := &autosaveFakeActor{id: 1}
	a.Add(actor)

	now = now.Add(time.Minute)
	a.Add(actor)

	// The original AutosaveInitialDelay deadline still applies, not one
	// reset by the second Add.
	now = time.UnixMilli(0).Add(AutosaveInitialDelay)
	a.Tick()
	if got, want := effects.take(), []string{"1 save"}; !slices.Equal(got, want) {
		t.Fatalf("Tick after re-Add = %v, want %v", got, want)
	}
}

func TestAutosaveConcurrentAccess(t *testing.T) {
	effects := &autosaveFakeEffects{}
	a, _ := NewAutosave(effects, nil)
	actors := make([]*autosaveFakeActor, 20)
	for i := range actors {
		actors[i] = &autosaveFakeActor{id: int32(i)}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for range 200 {
			for _, act := range actors {
				a.Add(act)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			for _, act := range actors {
				a.Remove(act.ObjectID())
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			a.Tick()
		}
	}()
	wg.Wait()
}

// ---- from decay_test.go ----
type decayFakeActor struct {
	id int32
}

func (a *decayFakeActor) ObjectID() int32 { return a.id }

type decayFakeEffects struct {
	mu     sync.Mutex
	events []string
}

type decayNoopEffects struct{}

func (decayNoopEffects) Decay(DecayActor) {}

func (e *decayFakeEffects) Decay(actor DecayActor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, fmt.Sprintf("decay %d", actor.ObjectID()))
}

func (e *decayFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestNewDecayRejectsNilEffects(t *testing.T) {
	if _, err := NewDecay(nil, nil); err == nil {
		t.Fatal("NewDecay() error = nil, want error for nil effects")
	}
}

func TestDecayAddThenTickFiresAfterDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, 7*time.Second)
	if !decay.Tracked(actor) {
		t.Fatal("actor should be tracked after Add")
	}

	now = now.Add(6 * time.Second)
	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before deadline = %v, want none", got)
	}

	now = now.Add(time.Second)
	decay.Tick()
	if got, want := effects.take(), []string{"decay 100"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if decay.Tracked(actor) {
		t.Fatal("actor should be removed after decay fires")
	}
}

func TestDecayTickAllocationIsFlat(t *testing.T) {
	now := time.UnixMilli(0)
	decay, err := NewDecay(decayNoopEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	actors := make([]*decayFakeActor, 128)
	for i := 0; i < 128; i++ {
		actors[i] = &decayFakeActor{id: int32(i + 1)}
	}
	tick := func() {
		for _, actor := range actors {
			decay.Add(actor, -time.Second)
		}
		decay.Tick()
	}
	tick()
	for i, entry := range decay.scratch {
		if entry.actor != nil {
			t.Fatalf("scratch[%d] retains actor after Tick", i)
		}
	}

	if allocs := testing.AllocsPerRun(100, tick); allocs != 0 {
		t.Fatalf("AllocsPerRun(128 actors) = %v, want 0", allocs)
	}
}

type decayPanicEffects struct{}

func (decayPanicEffects) Decay(DecayActor) { panic("boom") }

func TestDecayTickClearsScratchOnPanic(t *testing.T) {
	now := time.UnixMilli(0)
	decay, err := NewDecay(decayPanicEffects{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	decay.Add(&decayFakeActor{id: 1}, -time.Second)

	func() {
		defer func() { recover() }()
		decay.Tick()
	}()

	for i, entry := range decay.scratch {
		if entry.actor != nil {
			t.Fatalf("scratch[%d] retains actor after panicking Tick", i)
		}
	}
	if decay.ticking.Load() {
		t.Fatal("ticking guard left set after panicking Tick")
	}
}

type decayReentrantEffects struct {
	decay    *Decay
	innerErr error
}

func (e *decayReentrantEffects) Decay(DecayActor) {
	e.innerErr = e.decay.Tick()
}

func TestDecayTickReturnsErrorOnReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayReentrantEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	effects.decay = decay
	decay.Add(&decayFakeActor{id: 1}, -time.Second)

	if err := decay.Tick(); err != nil {
		t.Fatalf("outer Tick() error = %v, want nil", err)
	}

	if !errors.Is(effects.innerErr, ErrReentrantTick) {
		t.Fatalf("reentrant Tick() error = %v, want ErrReentrantTick", effects.innerErr)
	}
	if decay.ticking.Load() {
		t.Fatal("ticking guard left set after outer Tick returned")
	}
}

func TestDecayTickLogsReentrantCall(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayReentrantEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}
	effects.decay = decay
	var buf bytes.Buffer
	decay.log = zerolog.New(&buf)
	decay.Add(&decayFakeActor{id: 1}, -time.Second)

	decay.Tick()

	if !strings.Contains(buf.String(), "Decay.Tick") || !strings.Contains(buf.String(), ErrReentrantTick.Error()) {
		t.Fatalf("reentrant Tick call was not logged, got %q", buf.String())
	}
}

func BenchmarkDecayTickManyActors(b *testing.B) {
	now := time.UnixMilli(0)
	decay, err := NewDecay(decayNoopEffects{}, func() time.Time { return now })
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 4096; i++ {
		decay.Add(&decayFakeActor{id: int32(i + 1)}, time.Hour)
	}
	decay.Tick()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decay.Tick()
	}
}

func TestDecayDeadlineReportsTrackedDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	if _, ok := decay.Deadline(actor); ok {
		t.Fatal("Deadline() ok = true before Add, want false")
	}

	decay.Add(actor, 7*time.Second)
	if got, ok := decay.Deadline(actor); !ok || !got.Equal(now.Add(7*time.Second)) {
		t.Fatalf("Deadline() = %v, %v; want %v, true", got, ok, now.Add(7*time.Second))
	}

	decay.Cancel(actor)
	if _, ok := decay.Deadline(actor); ok {
		t.Fatal("Deadline() ok = true after Cancel, want false")
	}
}

func TestDecayCancelStopsPendingDecay(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, time.Second)

	if !decay.Cancel(actor) {
		t.Fatal("Cancel() = false, want true for tracked actor")
	}
	if decay.Cancel(actor) {
		t.Fatal("Cancel() = true, want false for already-removed actor")
	}

	now = now.Add(time.Hour)
	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after cancel = %v, want none", got)
	}
}

func TestDecayAddReplacesExistingDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, func() time.Time { return now })

	actor := &decayFakeActor{id: 100}
	decay.Add(actor, time.Second)
	decay.Add(actor, 10*time.Second)

	now = now.Add(time.Second)
	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before replaced deadline = %v, want none", got)
	}

	now = now.Add(9 * time.Second)
	decay.Tick()
	if got, want := effects.take(), []string{"decay 100"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at replaced deadline = %v, want %v", got, want)
	}
}

func TestDecayConcurrentAddAndTick(t *testing.T) {
	effects := &decayFakeEffects{}
	decay, _ := NewDecay(effects, nil)
	actors := make([]*decayFakeActor, 100)
	for i := range actors {
		actors[i] = &decayFakeActor{id: int32(i)}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for _, actor := range actors {
			decay.Add(actor, 0)
		}
	}()
	go func() {
		defer wg.Done()
		for _, actor := range actors {
			decay.Cancel(actor)
		}
	}()
	go func() {
		defer wg.Done()
		for range actors {
			decay.Tick()
		}
	}()
	wg.Wait()
}

type decayFakeSummon struct {
	id     int32
	linked bool
}

func (a *decayFakeSummon) ObjectID() int32 { return a.id }

func (a *decayFakeSummon) OwnerStillLinked() bool { return a.linked }

func TestDecayOrphanedSummonCancelledBeforeDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	summon := &decayFakeSummon{id: 200, linked: false}
	decay.Add(summon, time.Hour)
	if !decay.Tracked(summon) {
		t.Fatal("orphaned summon should be tracked immediately after Add")
	}

	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick cancelled orphaned summon = %v, want none", got)
	}
	if decay.Tracked(summon) {
		t.Fatal("orphaned summon should be untracked after Tick")
	}
}

func TestDecayLinkedSummonDecaysAtDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	summon := &decayFakeSummon{id: 201, linked: true}
	decay.Add(summon, time.Second)

	now = now.Add(time.Second)
	decay.Tick()
	if got, want := effects.take(), []string{"decay 201"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if decay.Tracked(summon) {
		t.Fatal("linked summon should be removed after decay fires")
	}
}

func TestDecayOrphanedSummonCancelledEvenWhenDue(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	summon := &decayFakeSummon{id: 202, linked: false}
	decay.Add(summon, -time.Second)

	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("due orphaned summon = %v, want cancel without decay", got)
	}
	if decay.Tracked(summon) {
		t.Fatal("due orphaned summon should be untracked after Tick")
	}
}

func TestDecayNonSummonActorsUnaffectedByLinkageCheck(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &decayFakeEffects{}
	decay, err := NewDecay(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDecay() error = %v", err)
	}

	npc := &decayFakeActor{id: 300}
	decay.Add(npc, time.Second)

	decay.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before NPC deadline = %v, want none", got)
	}
	if !decay.Tracked(npc) {
		t.Fatal("NPC should remain tracked before deadline")
	}

	now = now.Add(time.Second)
	decay.Tick()
	if got, want := effects.take(), []string{"decay 300"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at NPC deadline = %v, want %v", got, want)
	}
}

// ---- from gameclock_test.go ----
// Expected values in the tables below were generated by a probe running the
// reference Java implementation's game-time arithmetic
// (GameTimeTaskManager.java, openjdk 21).

func TestGameClockConversions(t *testing.T) {
	tests := []struct {
		minutes   int
		timeOfDay int
		hour      int
		minute    int
		day       int
		night     bool
		formatted string
	}{
		{0, 0, 0, 0, 0, true, "00:00"},
		{1, 1, 0, 1, 0, true, "00:01"},
		{59, 59, 0, 59, 0, true, "00:59"},
		{60, 60, 1, 0, 0, true, "01:00"},
		{61, 61, 1, 1, 0, true, "01:01"},
		{359, 359, 5, 59, 0, true, "05:59"},
		{360, 360, 6, 0, 0, false, "06:00"},
		{361, 361, 6, 1, 0, false, "06:01"},
		{719, 719, 11, 59, 0, false, "11:59"},
		{720, 720, 12, 0, 0, false, "12:00"},
		{1379, 1379, 22, 59, 0, false, "22:59"},
		{1439, 1439, 23, 59, 0, false, "23:59"},
		{1440, 0, 0, 0, 1, true, "00:00"},
		{1441, 1, 0, 1, 1, true, "00:01"},
		{1799, 359, 5, 59, 1, true, "05:59"},
		{1800, 360, 6, 0, 1, false, "06:00"},
		{2879, 1439, 23, 59, 1, false, "23:59"},
		{2880, 0, 0, 0, 2, true, "00:00"},
		{4319, 1439, 23, 59, 2, false, "23:59"},
		{4320, 0, 0, 0, 3, true, "00:00"},
		{100000, 640, 10, 40, 69, false, "10:40"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.minutes), func(t *testing.T) {
			c := &GameClock{minutes: tc.minutes}
			if got := c.TimeOfDay(); got != tc.timeOfDay {
				t.Errorf("TimeOfDay() = %d, want %d", got, tc.timeOfDay)
			}
			if got := c.Hour(); got != tc.hour {
				t.Errorf("Hour() = %d, want %d", got, tc.hour)
			}
			if got := c.Minute(); got != tc.minute {
				t.Errorf("Minute() = %d, want %d", got, tc.minute)
			}
			if got := c.Day(); got != tc.day {
				t.Errorf("Day() = %d, want %d", got, tc.day)
			}
			if got := c.IsNight(); got != tc.night {
				t.Errorf("IsNight() = %v, want %v", got, tc.night)
			}
			if got := c.String(); got != tc.formatted {
				t.Errorf("String() = %q, want %q", got, tc.formatted)
			}
		})
	}
}

func TestGameClockBootAlignment(t *testing.T) {
	midnight := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		offset  time.Duration // wall-clock time past midnight at boot
		minutes int
		night   bool
	}{
		{0, 0, true},
		{time.Millisecond, 0, true},
		{9999 * time.Millisecond, 0, true},
		{10000 * time.Millisecond, 1, true},
		{10001 * time.Millisecond, 1, true},
		{19999 * time.Millisecond, 1, true},
		{3599999 * time.Millisecond, 359, true},
		{3600000 * time.Millisecond, 360, false},
		{3600001 * time.Millisecond, 360, false},
		{21599999 * time.Millisecond, 2159, false},
		{21600000 * time.Millisecond, 2160, false},
		{43200000 * time.Millisecond, 4320, true},
		{86399999 * time.Millisecond, 8639, false},
	}
	for _, tc := range tests {
		t.Run(tc.offset.String(), func(t *testing.T) {
			boot := midnight.Add(tc.offset)
			c := NewGameClock(func() time.Time { return boot })
			if c.minutes != tc.minutes {
				t.Errorf("minutes = %d, want %d", c.minutes, tc.minutes)
			}
			if got := c.IsNight(); got != tc.night {
				t.Errorf("IsNight() = %v, want %v", got, tc.night)
			}
			if c.night != tc.night {
				t.Errorf("stored night = %v, want %v", c.night, tc.night)
			}
		})
	}
}

func TestGameClockBootAlignmentUsesLocalMidnight(t *testing.T) {
	zone := time.FixedZone("UTC+3", 3*3600)
	boot := time.Date(2026, 7, 10, 1, 0, 0, 0, zone)
	c := NewGameClock(func() time.Time { return boot })
	if c.minutes != 360 {
		t.Errorf("minutes = %d, want 360 (one real hour past the zone's midnight)", c.minutes)
	}
}

func TestGameClockTickDayNightTransitions(t *testing.T) {
	tests := []struct {
		name      string
		minutes   int
		wantNight bool
		wantTime  int // TimeOfDay a listener must observe when fired
	}{
		{"day breaks at 06:00", 359, false, 360},
		{"night falls at midnight", 1439, true, 0},
		{"day breaks on the second day", 1799, false, 360},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &GameClock{
				minutes: tc.minutes,
				night:   tc.minutes%minutesPerDay < nightEndMinute,
			}
			var fired []bool
			c.OnDayNight(func(night bool) {
				fired = append(fired, night)
				// The clock must be readable from a listener and already
				// show the post-crossing minute.
				if got := c.TimeOfDay(); got != tc.wantTime {
					t.Errorf("TimeOfDay() inside listener = %d, want %d", got, tc.wantTime)
				}
			})

			c.Tick()
			if len(fired) != 1 || fired[0] != tc.wantNight {
				t.Fatalf("after boundary tick fired = %v, want [%v]", fired, tc.wantNight)
			}

			c.Tick()
			if len(fired) != 1 {
				t.Fatalf("non-boundary tick fired listeners: %v", fired)
			}
		})
	}
}

func TestGameClockTickTransitionSequence(t *testing.T) {
	midnight := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	c := NewGameClock(func() time.Time { return midnight })
	var fired []bool
	c.OnDayNight(func(night bool) { fired = append(fired, night) })

	// Three in-game half-day boundaries fall within 1800 ticks from
	// midnight: day at 06:00, night at the next midnight, day again.
	for i := 0; i < 1800; i++ {
		c.Tick()
	}
	want := []bool{false, true, false}
	if len(fired) != len(want) {
		t.Fatalf("fired %v, want %v", fired, want)
	}
	for i := range want {
		if fired[i] != want[i] {
			t.Fatalf("fired %v, want %v", fired, want)
		}
	}
	if got := c.Day(); got != 1 {
		t.Errorf("Day() = %d, want 1", got)
	}
}

func TestGameClockUptime(t *testing.T) {
	base := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	cur := base
	c := NewGameClock(func() time.Time { return cur })

	cur = base.Add(95 * time.Second)
	if got := c.Uptime(); got != 95*time.Second {
		t.Errorf("Uptime() = %v, want %v", got, 95*time.Second)
	}
}

func TestGameClockConcurrentAccess(t *testing.T) {
	c := NewGameClock(nil)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			c.Tick()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = c.TimeOfDay()
			_ = c.Hour()
			_ = c.Minute()
			_ = c.Day()
			_ = c.IsNight()
			_ = c.String()
			_ = c.Uptime()
			c.OnDayNight(func(bool) {})
		}
	}()
	wg.Wait()
}

// ---- from inventoryupdates_test.go ----
func TestInventoryUpdatesTickSendsVisibleOwnersAndUpdatesWeight(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Weight: 2, Stackable: true}})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)
	inv.Add(&item.Instance{ObjectID: 1, TemplateID: 57, Count: 3})

	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()
	updates.Add(inv, owner)

	updates.Tick()

	if got, want := owner.sent, [][]itemcontainer.Update{{{ObjectID: 1, TemplateID: 57, Count: 3, State: itemcontainer.UpdateAdded}}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("sent updates = %+v, want %+v", got, want)
	}
	if got := inv.TotalWeight(); got != 6 {
		t.Fatalf("TotalWeight() = %d, want 6", got)
	}
	if got := inv.DrainUpdates(); len(got) != 0 {
		t.Fatalf("DrainUpdates() after send = %+v, want empty", got)
	}
}

// TestInventoryUpdatesTickBatchesMultipleMutationsIntoOneSend pins the
// batching half of the task: several mutations queued before a tick runs —
// here, two different items changing on the same inventory — reach the
// owner as exactly one SendInventoryUpdate call carrying both, not one call
// per mutation.
func TestInventoryUpdatesTickBatchesMultipleMutationsIntoOneSend(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Weight: 2, Stackable: true},
		{ID: 58, Kind: item.KindEtcItem, Weight: 1, Stackable: true},
	})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)

	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()
	inv.SetUpdateNotifier(func() { updates.Add(inv, owner) })

	inv.Add(&item.Instance{ObjectID: 1, TemplateID: 57, Count: 3})
	inv.Add(&item.Instance{ObjectID: 2, TemplateID: 58, Count: 1})

	updates.Tick()

	if len(owner.sent) != 1 {
		t.Fatalf("SendInventoryUpdate calls = %d, want 1 (one tick, one batch)", len(owner.sent))
	}
	if got := len(owner.sent[0]); got != 2 {
		t.Fatalf("updates in the single batch = %d, want 2", got)
	}
}

// TestInventoryUpdatesRemoveUnchangedKeepsReregisteredInventory pins the
// epoch guard against Tick's check-then-remove race: a mutation that lands
// between a tick observing an inventory as empty (or gated out) and the
// sweep that drops it must not orphan the update it just queued. Add bumps
// the inventory's epoch on every registration; removeUnchanged only drops
// an entry whose epoch still matches what the tick observed when it decided
// to remove it.
func TestInventoryUpdatesRemoveUnchangedKeepsReregisteredInventory(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Stackable: true}})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)
	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()

	updates.Add(inv, owner)
	seenEpoch := updates.epoch[inv]

	// A mutation "landing mid-tick" re-registers the inventory, bumping its
	// epoch past what this tick's snapshot observed.
	updates.Add(inv, owner)

	updates.removeUnchanged(map[*itemcontainer.Inventory]uint64{inv: seenEpoch})

	if !updates.Contains(inv) {
		t.Fatal("inventory re-registered mid-tick was dropped by the stale removal sweep")
	}

	// The ordinary case still removes: no re-registration happened, so the
	// epoch removeUnchanged sees still matches.
	updates.removeUnchanged(map[*itemcontainer.Inventory]uint64{inv: updates.epoch[inv]})
	if updates.Contains(inv) {
		t.Fatal("inventory with an unchanged epoch should have been removed")
	}
}

// TestInventoryUpdatesRemoveUnchangedClearsDroppedSlots pins that the
// in-place filter in removeUnchanged doesn't leak dropped *Inventory
// pointers past order's new length: order only ever appends and filters, so
// a leaked pointer there would keep a logged-out player's inventory (and
// every item it holds) reachable indefinitely.
func TestInventoryUpdatesRemoveUnchangedClearsDroppedSlots(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Stackable: true}})
	invA := itemcontainer.NewPlayerInventory(0x10000001, templates)
	invB := itemcontainer.NewPlayerInventory(0x10000002, templates)
	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()

	updates.Add(invA, owner)
	updates.Add(invB, owner)

	updates.removeUnchanged(map[*itemcontainer.Inventory]uint64{invB: updates.epoch[invB]})

	full := updates.order[:cap(updates.order)]
	for i := len(updates.order); i < len(full); i++ {
		if full[i] != nil {
			t.Fatalf("order's backing array at index %d still references a dropped inventory past its new length", i)
		}
	}
}

func TestInventoryUpdatesTickDropsInvisibleNonTeleportingOwners(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Stackable: true}})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)
	inv.Add(&item.Instance{ObjectID: 1, TemplateID: 57, Count: 3})

	owner := &inventoryUpdateOwnerStub{}
	updates := NewInventoryUpdates()
	updates.Add(inv, owner)

	updates.Tick()

	if len(owner.sent) != 0 {
		t.Fatalf("sent updates = %+v, want none", owner.sent)
	}
	if updates.Contains(inv) {
		t.Fatalf("invisible non-teleporting owner should be removed from the task")
	}
	if got := inv.DrainUpdates(); len(got) != 1 {
		t.Fatalf("DrainUpdates() = %+v, want the pending update to remain queued", got)
	}
}

type inventoryUpdateOwnerStub struct {
	visible     bool
	teleporting bool
	sent        [][]itemcontainer.Update
}

func (o *inventoryUpdateOwnerStub) Visible() bool { return o.visible }

func (o *inventoryUpdateOwnerStub) Teleporting() bool { return o.teleporting }

func (o *inventoryUpdateOwnerStub) SendInventoryUpdate(updates []itemcontainer.Update) {
	o.sent = append(o.sent, slices.Clone(updates))
}

// ---- from iteminstances_test.go ----
func TestItemInstancesSaveFlushesAndClearsPendingItems(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}},
		{ID: 20, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}},
		{ID: 30, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemPetCollar}},
	})
	flusher := &itemFlusherStub{}
	instances := NewItemInstances(flusher, templates)

	kept := &item.Instance{
		ObjectID: 1, TemplateID: 10, OwnerID: 100, Count: 5, Location: item.LocationInventory,
		Augmentation: &item.Augmentation{Attributes: 123, SkillID: 456, SkillLevel: 7},
	}
	deletedWeapon := &item.Instance{ObjectID: 2, TemplateID: 20, Count: 0, Location: item.LocationInventory}
	deletedPetCollar := &item.Instance{ObjectID: 3, TemplateID: 30, Count: 0, Location: item.LocationInventory}
	instances.Add(kept)
	instances.Add(deletedWeapon)
	instances.Add(deletedPetCollar)

	if !instances.Contains(&item.Instance{ObjectID: kept.ObjectID}) {
		t.Fatalf("Contains() should match pending items by object id")
	}
	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	batch := flusher.last()
	if got, want := savedIDs(batch.Saves), []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("saved item ids = %v, want %v", got, want)
	}
	if got, want := batch.Deletes, []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("deleted item ids = %v, want %v", got, want)
	}
	if got, want := augmentationSaveIDs(batch.AugmentationSaves), []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("saved augmentation ids = %v, want %v", got, want)
	}
	if got, want := batch.AugmentationDeletes, []int32{2}; !slices.Equal(got, want) {
		t.Fatalf("deleted augmentation ids = %v, want %v", got, want)
	}
	if got, want := batch.PetDeletes, []int32{3}; !slices.Equal(got, want) {
		t.Fatalf("deleted pet item ids = %v, want %v", got, want)
	}
	if instances.Contains(kept) {
		t.Fatalf("Save() should clear successfully flushed pending items")
	}
}

func TestItemInstancesSaveDeletesVoidItemsWithoutDeletingAugmentation(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}}})
	flusher := &itemFlusherStub{}
	instances := NewItemInstances(flusher, templates)

	instances.Add(&item.Instance{
		ObjectID: 1, TemplateID: 10, Count: 1, Location: item.LocationVoid,
		Augmentation: &item.Augmentation{Attributes: 123},
	})

	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	batch := flusher.last()
	if got, want := batch.Deletes, []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("deleted item ids = %v, want %v", got, want)
	}
	if len(batch.AugmentationDeletes) != 0 {
		t.Fatalf("void item with positive count should not delete augmentation, got %v", batch.AugmentationDeletes)
	}
}

func TestItemInstanceBackgroundAndInventoryMutationIsRaceFree(t *testing.T) {
	tmpl := &item.Template{ID: 10, Kind: item.KindEtcItem, Stackable: true, Duration: 100000, EtcItem: &item.EtcItemDetail{}}
	templates := item.NewTable([]*item.Template{tmpl})
	inv := itemcontainer.NewPlayerInventory(100, templates)
	inst := inv.AddNew(tmpl.ID, 100000, 1)

	effects := &shadowItemFakeEffects{}
	shadowItems, err := NewShadowItems(effects)
	if err != nil {
		t.Fatalf("NewShadowItems() error = %v", err)
	}
	shadowItems.Track(100, inst, tmpl)

	instances := NewItemInstances(&itemFlusherStub{}, templates)

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			shadowItems.Tick()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			instances.Add(inst)
			if err := instances.Save(context.Background()); err != nil {
				t.Errorf("Save() error = %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if inv.DestroyItem(inst, 1) == nil {
				t.Errorf("DestroyItem() returned nil")
			}
		}
	}()
	wg.Wait()
}

type itemFlusherStub struct {
	mu    sync.Mutex
	batch item.FlushBatch
}

// Flush reads every save's mutable fields directly (not through
// Snapshot()), the same way a real store's Flush does, so a race between
// this and a concurrent mutation of the live instance still trips -race:
// FlushBatch.Saves is meant to hold already-detached copies, and this is
// the assertion that they actually are.
func (s *itemFlusherStub) Flush(_ context.Context, batch item.FlushBatch) error {
	for _, inst := range batch.Saves {
		_, _, _ = inst.Count, inst.Location, inst.ManaLeft
	}
	s.mu.Lock()
	s.batch = batch
	s.mu.Unlock()
	return nil
}

func (s *itemFlusherStub) last() item.FlushBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batch
}

func savedIDs(saves []item.InstanceState) []int32 {
	ids := make([]int32, len(saves))
	for i, inst := range saves {
		ids[i] = inst.ObjectID
	}
	return ids
}

func augmentationSaveIDs(saves []item.FlushAugmentationSave) []int32 {
	ids := make([]int32, len(saves))
	for i, save := range saves {
		ids[i] = save.ObjectID
	}
	return ids
}

// ---- from pvpflag_test.go ----
type pvpFlagFakeActor struct {
	id     int32
	flag   PvPFlagState
	events []PvPFlagState
}

func (a *pvpFlagFakeActor) ObjectID() int32 { return a.id }

func (a *pvpFlagFakeActor) UpdatePvPFlag(flag PvPFlagState) {
	if a.flag == flag {
		return
	}
	a.flag = flag
	a.events = append(a.events, flag)
}

func TestPvPFlagsTickUpdatesBlinksAndExpires(t *testing.T) {
	now := time.UnixMilli(1_000)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	actor := &pvpFlagFakeActor{id: 1}

	flags.Add(actor, 10*time.Second)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("initial Tick events = %v, want %v", got, want)
	}

	now = now.Add(5 * time.Second)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("Tick at exactly five seconds left = %v, want unchanged %v", got, want)
	}

	now = now.Add(time.Millisecond)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn, PvPFlagBlinking}; !slices.Equal(got, want) {
		t.Fatalf("Tick inside blink window = %v, want %v", got, want)
	}

	now = time.UnixMilli(11_000)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn, PvPFlagBlinking}; !slices.Equal(got, want) {
		t.Fatalf("Tick at exact deadline = %v, want unchanged %v", got, want)
	}

	now = now.Add(time.Millisecond)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn, PvPFlagBlinking, PvPFlagNone}; !slices.Equal(got, want) {
		t.Fatalf("Tick after deadline = %v, want %v", got, want)
	}
	if flags.Len() != 0 {
		t.Fatalf("Len() after expiry = %d, want 0", flags.Len())
	}
}

// TestPvPFlagsTickDuePartitionIsNotPreSized guards against tickExpiry
// pre-sizing its due partition to the full tracked count on every sweep,
// instead of only allocating when something is actually due. A pre-sized
// due costs one constant extra allocation regardless of N, so an
// allocation-count (or count-ratio) assertion cannot see it — it shows up
// only as extra bytes. Measuring bytes/op via testing.Benchmark, not
// testing.AllocsPerRun, is what makes this test actually fail when that
// regression is reintroduced: with all 128 flags non-expiring, tickExpiry's
// own necessary work (appending every entry to pending) costs ~11.9 KB/op,
// while a due pre-sized to len(entries) would add ~5.1 KB more
// (128 * sizeof(deadlineEntry[PvPFlagActor])) on every single call.
func TestPvPFlagsTickDuePartitionIsNotPreSized(t *testing.T) {
	now := time.UnixMilli(0)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	for i := 0; i < 128; i++ {
		flags.Add(&pvpFlagFakeActor{id: int32(i + 1)}, time.Hour)
	}

	res := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			flags.Tick()
		}
	})

	if got := res.AllocedBytesPerOp(); got > 14_000 {
		t.Fatalf("PvPFlags.Tick = %d B/op at 128 non-expiring flags, want <= 14000; due partition may be pre-sized to the tracked count even though nothing is due", got)
	}
}

func TestPvPFlagsRemoveCanLeaveCurrentFlag(t *testing.T) {
	now := time.UnixMilli(1_000)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	actor := &pvpFlagFakeActor{id: 1}

	flags.Add(actor, 10*time.Second)
	flags.Tick()
	flags.Remove(actor, false)

	now = now.Add(11 * time.Second)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("events after non-reset remove = %v, want %v", got, want)
	}
}

func TestPvPFlagsConfiguredDurations(t *testing.T) {
	now := time.UnixMilli(1_000)
	flags := NewPvPFlags(PvPFlagOptions{Normal: 10 * time.Second, Flagged: 2 * time.Second}, func() time.Time { return now })
	normal := &pvpFlagFakeActor{id: 1}
	flagged := &pvpFlagFakeActor{id: 2}

	flags.AddNormal(normal)
	flags.AddFlagged(flagged)
	if got, want := normal.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("normal add events = %v, want %v", got, want)
	}
	if got, want := flagged.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("flagged add events = %v, want %v", got, want)
	}

	now = now.Add(2*time.Second + time.Millisecond)
	flags.Tick()
	if got, want := flagged.events, []PvPFlagState{PvPFlagOn, PvPFlagNone}; !slices.Equal(got, want) {
		t.Fatalf("flagged timeout events = %v, want %v", got, want)
	}
	if got, want := normal.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("normal timeout early events = %v, want %v", got, want)
	}

	now = time.UnixMilli(11_001)
	flags.Tick()
	if got, want := normal.events, []PvPFlagState{PvPFlagOn, PvPFlagNone}; !slices.Equal(got, want) {
		t.Fatalf("normal timeout events = %v, want %v", got, want)
	}
}

func TestPvPFlagOptionsFromProperties(t *testing.T) {
	props, err := config.ParseString(`
PvPVsNormalTime = 40000
PvPVsPvPTime = 20000
KarmaPlayerCanShop = False
AwardPKKillPVPPoint = False
`)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	opts, err := PvPFlagOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("PvPFlagOptionsFromProperties() error = %v", err)
	}
	if opts.Normal != 40*time.Second || opts.Flagged != 20*time.Second {
		t.Fatalf("durations = normal %s flagged %s, want 40s/20s", opts.Normal, opts.Flagged)
	}
	if opts.AwardPKKillPVPPoint {
		t.Fatal("AwardPKKillPVPPoint = true, want false")
	}
	wantUnsupported := []string{"KarmaPlayerCanShop"}
	if !slices.Equal(opts.UnsupportedKeys, wantUnsupported) {
		t.Fatalf("UnsupportedKeys = %v, want %v", opts.UnsupportedKeys, wantUnsupported)
	}
}

func TestPvPFlagOptionsDefaultsAndInvalidValues(t *testing.T) {
	opts, err := PvPFlagOptionsFromProperties(nil)
	if err != nil {
		t.Fatalf("PvPFlagOptionsFromProperties(nil) error = %v", err)
	}
	if opts.Normal != 40*time.Second || opts.Flagged != 20*time.Second {
		t.Fatalf("default durations = normal %s flagged %s, want 40s/20s", opts.Normal, opts.Flagged)
	}
	if !opts.AwardPKKillPVPPoint {
		t.Fatal("default AwardPKKillPVPPoint = false, want true")
	}

	props, err := config.ParseString(`PvPVsNormalTime = nope`)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	if _, err := PvPFlagOptionsFromProperties(props); err == nil {
		t.Fatal("PvPFlagOptionsFromProperties() with bad int: expected error")
	}
}

func BenchmarkPvPFlagsTickManyNonExpiringFlags(b *testing.B) {
	now := time.UnixMilli(0)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	for i := 0; i < 128; i++ {
		flags.Add(&pvpFlagFakeActor{id: int32(i + 1)}, time.Hour)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flags.Tick()
	}
}

// ---- from respawn_test.go ----
type respawnFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *respawnFakeEffects) Respawn(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, key)
}

func (e *respawnFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestNewRespawnRejectsNilEffects(t *testing.T) {
	if _, err := NewRespawn(nil, nil); err == nil {
		t.Fatal("NewRespawn() error = nil, want error for nil effects")
	}
}

func TestRespawnAddThenTickFiresAfterDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &respawnFakeEffects{}
	r, err := NewRespawn(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewRespawn() error = %v", err)
	}

	r.Add("slot-1", now.Add(7*time.Second))
	if !r.Tracked("slot-1") {
		t.Fatal("slot should be tracked after Add")
	}

	now = now.Add(6 * time.Second)
	r.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before deadline = %v, want none", got)
	}

	now = now.Add(time.Second)
	r.Tick()
	if got, want := effects.take(), []string{"slot-1"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at deadline = %v, want %v", got, want)
	}
	if r.Tracked("slot-1") {
		t.Fatal("slot should be removed after respawn fires")
	}
}

func TestRespawnAddWithPastDeadlineFiresOnNextTick(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &respawnFakeEffects{}
	r, _ := NewRespawn(effects, func() time.Time { return now })

	r.Add("slot-1", now.Add(-time.Minute))
	r.Tick()
	if got, want := effects.take(), []string{"slot-1"}; !slices.Equal(got, want) {
		t.Fatalf("Tick with past deadline = %v, want %v", got, want)
	}
}

func TestRespawnCancelStopsPendingRespawn(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &respawnFakeEffects{}
	r, _ := NewRespawn(effects, func() time.Time { return now })

	r.Add("slot-1", now.Add(time.Second))

	if !r.Cancel("slot-1") {
		t.Fatal("Cancel() = false, want true for tracked slot")
	}
	if r.Cancel("slot-1") {
		t.Fatal("Cancel() = true, want false for already-removed slot")
	}

	now = now.Add(time.Hour)
	r.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after cancel = %v, want none", got)
	}
}

func TestRespawnAddReplacesExistingDeadline(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &respawnFakeEffects{}
	r, _ := NewRespawn(effects, func() time.Time { return now })

	r.Add("slot-1", now.Add(time.Second))
	r.Add("slot-1", now.Add(10*time.Second))

	now = now.Add(time.Second)
	r.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before replaced deadline = %v, want none", got)
	}

	now = now.Add(9 * time.Second)
	r.Tick()
	if got, want := effects.take(), []string{"slot-1"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at replaced deadline = %v, want %v", got, want)
	}
}

func TestRespawnConcurrentAddAndTick(t *testing.T) {
	effects := &respawnFakeEffects{}
	r, _ := NewRespawn(effects, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "slot"
			r.Add(key, time.Now())
			r.Tick()
			r.Cancel(key)
		}(i)
	}
	wg.Wait()
}

// ---- from shadowitem_test.go ----
type shadowItemFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *shadowItemFakeEffects) ManaThreshold(actorID int32, inst *item.Instance, secondsLeft int) {
	e.record(fmt.Sprintf("%d threshold %d %d", actorID, inst.ObjectID, secondsLeft))
}

func (e *shadowItemFakeEffects) Expire(actorID int32, inst *item.Instance) {
	e.record(fmt.Sprintf("%d expire %d", actorID, inst.ObjectID))
}

func (e *shadowItemFakeEffects) record(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, s)
}

func (e *shadowItemFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestShadowItems_TrackDecaysManaEachTick(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, err := NewShadowItems(effects)
	if err != nil {
		t.Fatalf("NewShadowItems() error = %v", err)
	}

	tmpl := &item.Template{Duration: 5} // 300 seconds of mana
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}

	s.Track(10, inst, tmpl)
	if !s.Tracked(inst) {
		t.Fatalf("Track() should start tracking a shadow item")
	}
	if inst.ManaLeft != 300 {
		t.Fatalf("first Track() must not cost extra mana, ManaLeft = %d, want 300", inst.ManaLeft)
	}

	s.Tick()
	if inst.ManaLeft != 299 {
		t.Errorf("ManaLeft after one tick = %d, want 299", inst.ManaLeft)
	}
}

func TestShadowItems_Track_NonShadowItemIgnored(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: -1}
	inst := &item.Instance{ObjectID: 1, ManaLeft: -1}

	s.Track(10, inst, tmpl)
	if s.Tracked(inst) {
		t.Errorf("Track() should ignore a non-shadow item")
	}
}

func TestShadowItems_Track_ReequipCostsOneMinute(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: 5}
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}

	s.Track(10, inst, tmpl)
	s.Tick() // let a second elapse so mana drifts off the full-duration value
	s.Untrack(inst)
	if inst.ManaLeft != 299 {
		t.Fatalf("ManaLeft before re-equip = %d, want 299", inst.ManaLeft)
	}

	s.Track(10, inst, tmpl) // re-equip: mana no longer at full duration
	if inst.ManaLeft != 239 {
		t.Errorf("re-equipping after mana has already drifted should cost one extra minute, ManaLeft = %d, want 239", inst.ManaLeft)
	}
}

func TestShadowItems_Track_ReequipFreeWhenManaNeverMoved(t *testing.T) {
	// Equipping then immediately unequipping without a tick in between
	// leaves mana at the template's full duration, so re-equipping costs
	// nothing — the penalty only applies once mana has actually drifted.
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: 5}
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}

	s.Track(10, inst, tmpl)
	s.Untrack(inst)
	s.Track(10, inst, tmpl)
	if inst.ManaLeft != 300 {
		t.Errorf("ManaLeft = %d, want 300 (no tick elapsed, so re-equip is free)", inst.ManaLeft)
	}
}

func TestShadowItems_Tick_FiresThresholdsAndExpiry(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: 5} // 300 seconds of mana
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}
	s.Track(10, inst, tmpl)
	effects.take()

	// Fast-forward straight to just above the 1-minute threshold instead
	// of ticking 240 times to get there.
	inst.ManaLeft = 61

	s.Tick()
	if got := effects.take(); len(got) != 1 || got[0] != "10 threshold 1 60" {
		t.Fatalf("Tick() at the 1-minute mark = %v, want [10 threshold 1 60]", got)
	}

	for i := 0; i < 60; i++ {
		s.Tick()
	}
	got := effects.take()
	if len(got) == 0 || got[len(got)-1] != "10 expire 1" {
		t.Fatalf("Tick() at zero mana = %v, want an expiry event", got)
	}
	if s.Tracked(inst) {
		t.Errorf("an expired item should no longer be tracked")
	}
}

func TestShadowItems_Remove_StopsTrackingByActor(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: 5}
	instA := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}
	instB := &item.Instance{ObjectID: 2, ManaLeft: tmpl.InitialManaLeft()}
	s.Track(10, instA, tmpl)
	s.Track(20, instB, tmpl)

	s.Remove(10)
	if s.Tracked(instA) {
		t.Errorf("Remove(10) should stop tracking actor 10's item")
	}
	if !s.Tracked(instB) {
		t.Errorf("Remove(10) must not affect actor 20's item")
	}
}

// ---- from water_test.go ----
type waterFakeActor struct {
	id   int32
	dead bool
}

func (a *waterFakeActor) ObjectID() int32 { return a.id }
func (a *waterFakeActor) Dead() bool      { return a.dead }

type waterFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *waterFakeEffects) GaugeSet(actor WaterActor, remaining time.Duration) {
	e.record(fmt.Sprintf("%d gauge %s", actor.ObjectID(), remaining))
}

func (e *waterFakeEffects) Drown(actor WaterActor) {
	e.record(fmt.Sprintf("%d drown", actor.ObjectID()))
}

func (e *waterFakeEffects) record(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, s)
}

func (e *waterFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestWaterAddStartsCountdownAndDrownsAfterBreathElapses(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &waterFakeEffects{}
	w, err := NewWater(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}

	actor := &waterFakeActor{id: 1}
	w.Add(actor, 10*time.Second)
	if got, want := effects.take(), []string{"1 gauge 10s"}; !slices.Equal(got, want) {
		t.Fatalf("after Add = %v, want %v", got, want)
	}

	now = now.Add(9 * time.Second)
	w.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick before breath elapsed = %v, want none", got)
	}

	now = now.Add(time.Second)
	w.Tick()
	if got, want := effects.take(), []string{"1 drown"}; !slices.Equal(got, want) {
		t.Fatalf("Tick at breath limit = %v, want %v", got, want)
	}

	// Drowning repeats every tick until the actor is removed.
	now = now.Add(time.Second)
	w.Tick()
	if got, want := effects.take(), []string{"1 drown"}; !slices.Equal(got, want) {
		t.Fatalf("Tick after breath limit = %v, want %v", got, want)
	}
}

func TestWaterRemoveStopsDrowning(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, func() time.Time { return now })

	actor := &waterFakeActor{id: 1}
	w.Add(actor, time.Second)
	effects.take()

	now = now.Add(2 * time.Second)
	w.Remove(actor)
	if got, want := effects.take(), []string{"1 gauge 0s"}; !slices.Equal(got, want) {
		t.Fatalf("after Remove = %v, want %v", got, want)
	}

	w.Tick()
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Tick after Remove = %v, want none", got)
	}
}

func TestWaterRemoveUntrackedActorIsNoop(t *testing.T) {
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, nil)

	w.Remove(&waterFakeActor{id: 1})
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Remove on untracked actor = %v, want none", got)
	}
}

func TestWaterAddDeadActorIsNoop(t *testing.T) {
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, nil)

	w.Add(&waterFakeActor{id: 1, dead: true}, time.Second)
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("Add on dead actor = %v, want none", got)
	}
}

func TestWaterAddAlreadyTrackedActorIsNoop(t *testing.T) {
	now := time.UnixMilli(0)
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, func() time.Time { return now })

	actor := &waterFakeActor{id: 1}
	w.Add(actor, time.Second)
	effects.take()

	w.Add(actor, 5*time.Second)
	if got := effects.take(); len(got) != 0 {
		t.Fatalf("second Add on tracked actor = %v, want none", got)
	}

	// The original one-second deadline still applies, not the second call's.
	now = now.Add(time.Second)
	w.Tick()
	if got, want := effects.take(), []string{"1 drown"}; !slices.Equal(got, want) {
		t.Fatalf("Tick after re-Add = %v, want %v", got, want)
	}
}

func TestWaterConcurrentAccess(t *testing.T) {
	effects := &waterFakeEffects{}
	w, _ := NewWater(effects, nil)
	actors := make([]*waterFakeActor, 20)
	for i := range actors {
		actors[i] = &waterFakeActor{id: int32(i)}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, a := range actors {
				w.Add(a, time.Millisecond)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, a := range actors {
				w.Remove(a)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			w.Tick()
		}
	}()
	wg.Wait()
}

// ---- from grounditems_test.go ----

func newTestGroundItem(t *testing.T, id, templateID int32) *grounditem.Item {
	t.Helper()
	tmpl := &item.Template{ID: templateID, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}}
	inst := item.Instance{ObjectID: id, TemplateID: templateID, Count: 1, Location: item.LocationVoid}
	ground, err := grounditem.New(inst, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New() error = %v", err)
	}
	return ground
}

func TestGroundItemsDropLootProtectionLocksThenExpires(t *testing.T) {
	now := time.Unix(0, 0)
	g := NewGroundItems(nil, DefaultGroundItemOptions(), func() time.Time { return now })

	ground := newTestGroundItem(t, 1, 57)
	g.Drop(ground, DropOptions{X: 1, Y: 1, Z: 1, ProtectOwnerID: 42, ProtectFor: 15 * time.Second})

	if got := ground.Instance.Snapshot().OwnerID; got != 42 {
		t.Fatalf("OwnerID after drop = %d, want 42 (locked to killer)", got)
	}

	now = now.Add(10 * time.Second)
	g.Tick()
	if got := ground.Instance.Snapshot().OwnerID; got != 42 {
		t.Fatalf("OwnerID before protection deadline = %d, want still 42", got)
	}

	now = now.Add(6 * time.Second) // total 16s > 15s protection window
	g.Tick()
	if got := ground.Instance.Snapshot().OwnerID; got != 0 {
		t.Fatalf("OwnerID after protection deadline = %d, want 0 (unlocked)", got)
	}
}

func TestGroundItemsLoadClearsPersistedLootProtectionOwner(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}}})
	g := NewGroundItems(nil, DefaultGroundItemOptions(), func() time.Time { return time.Unix(0, 0) })

	// A snapshot carrying a nonzero OwnerID models a row flushed mid-protection
	// window; Load has no persisted lootExpiresAt to restore alongside it, so
	// Tick's unlock branch (gated on lootExpiresAt) would never fire again if
	// the owner survived the reload, permanently locking the item.
	rows := []item.GroundSnapshot{{
		Instance: item.Instance{ObjectID: 1, TemplateID: 57, Count: 1, OwnerID: 42, Location: item.LocationVoid},
		X:        1, Y: 1, Z: 1,
	}}
	if err := g.Load(rows, templates); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	snaps := g.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("Snapshots() len = %d, want 1", len(snaps))
	}
	if snaps[0].OwnerID != 0 {
		t.Fatalf("OwnerID after Load = %d, want 0 (a restart cannot resume a loot-protection lock it never persisted a deadline for)", snaps[0].OwnerID)
	}
}

func TestGroundItemsDropWithoutProtectOwnerLeavesItemUnowned(t *testing.T) {
	g := NewGroundItems(nil, DefaultGroundItemOptions(), func() time.Time { return time.Unix(0, 0) })

	ground := newTestGroundItem(t, 1, 57)
	g.Drop(ground, DropOptions{X: 1, Y: 1, Z: 1})

	if got := ground.Instance.Snapshot().OwnerID; got != 0 {
		t.Fatalf("OwnerID = %d, want 0 for an unprotected drop", got)
	}
}

func TestGroundItemOptionsFromPropertiesAbsentSpecialItemsUsesDefault(t *testing.T) {
	props, err := config.ParseString("")
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	opts, err := GroundItemOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("GroundItemOptionsFromProperties() error = %v", err)
	}
	want := map[int32]time.Duration{57: 0, 5575: 0, 6673: 0}
	if len(opts.SpecialAutoDestroy) != len(want) {
		t.Fatalf("SpecialAutoDestroy = %v, want %v when AutoDestroySpecialItemTime is absent", opts.SpecialAutoDestroy, want)
	}
	for id, delay := range want {
		if got, ok := opts.SpecialAutoDestroy[id]; !ok || got != delay {
			t.Errorf("SpecialAutoDestroy[%d] = %v, ok=%v, want %v", id, got, ok, delay)
		}
	}
}

func TestGroundItemOptionsFromPropertiesSpecialItemsOverridesDefault(t *testing.T) {
	props, err := config.ParseString("AutoDestroySpecialItemTime=1-10,2-20\n")
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	opts, err := GroundItemOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("GroundItemOptionsFromProperties() error = %v", err)
	}
	want := map[int32]time.Duration{1: 10 * time.Second, 2: 20 * time.Second}
	if len(opts.SpecialAutoDestroy) != len(want) {
		t.Fatalf("SpecialAutoDestroy = %v, want %v", opts.SpecialAutoDestroy, want)
	}
	for id, delay := range want {
		if got, ok := opts.SpecialAutoDestroy[id]; !ok || got != delay {
			t.Errorf("SpecialAutoDestroy[%d] = %v, ok=%v, want %v", id, got, ok, delay)
		}
	}
}
