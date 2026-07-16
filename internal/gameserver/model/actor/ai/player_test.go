package ai

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

type fakePlayerActor struct {
	id          int32
	alikeDead   bool
	disabled    bool
	attackRange int
	known       map[int32]bool
}

func playerActor(id int32) *fakePlayerActor {
	return &fakePlayerActor{id: id, attackRange: 40, known: make(map[int32]bool)}
}

func (a *fakePlayerActor) ObjectID() int32      { return a.id }
func (a *fakePlayerActor) SiegeGuard() bool     { return false }
func (a *fakePlayerActor) AlikeDead() bool      { return a.alikeDead }
func (a *fakePlayerActor) AttackDisabled() bool { return a.disabled }
func (a *fakePlayerActor) Knows(target attackable.Combatant) bool {
	return a.known[target.ObjectID()]
}
func (a *fakePlayerActor) PhysicalAttackRange() int { return a.attackRange }

func TestPlayerAttackStartsSwingWhenAlreadyInRange(t *testing.T) {
	owner := playerActor(1)
	target := actor(2)
	owner.known[target.ObjectID()] = true
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	p := NewPlayerAttack(owner, move, strike)

	if !p.Start(target) {
		t.Fatal("Start() = false, want true for an in-range target")
	}
	if strike.target != target {
		t.Fatalf("attacked target = %v, want %v", strike.target, target)
	}
	if move.stopCount != 1 {
		t.Fatalf("move stop count = %d, want 1", move.stopCount)
	}
}

func TestPlayerAttackMovesFirstWhenOutOfRange(t *testing.T) {
	owner := playerActor(1)
	target := actor(2)
	owner.known[target.ObjectID()] = true
	move := &recordingMove{followStarted: true}
	strike := &recordingAttack{canAttack: true}
	p := NewPlayerAttack(owner, move, strike)

	if !p.Start(target) {
		t.Fatal("Start() = false, want true: closing distance is not a failure")
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while still closing distance", strike.target)
	}
	if move.followTarget != target || move.followRange != owner.attackRange {
		t.Fatalf("follow check = (%v, %d), want (%v, %d)", move.followTarget, move.followRange, target, owner.attackRange)
	}
}

func TestPlayerAttackReportsFailureWhenAlreadyAttacking(t *testing.T) {
	owner := playerActor(1)
	target := actor(2)
	owner.known[target.ObjectID()] = true
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	p := NewPlayerAttack(owner, move, strike)

	if p.Start(target) {
		t.Fatal("Start() = true, want false while a swing is already in progress")
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while busy", strike.target)
	}
	// A busy rejection must not clear the intention: the next Think (fired
	// once the in-progress swing finishes) should retry the same target.
	if p.Target() != target {
		t.Fatalf("Target() = %v after a busy rejection, want %v retained", p.Target(), target)
	}
}

func TestPlayerAttackClearsIntentionWhenTargetLost(t *testing.T) {
	owner := playerActor(1)
	target := actor(2)
	// Not marked known.
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	p := NewPlayerAttack(owner, move, strike)

	if p.Start(target) {
		t.Fatal("Start() = true, want false for an unknown target")
	}
	if p.Target() != nil {
		t.Fatalf("Target() = %v after target lost, want nil", p.Target())
	}
	if move.stopCount != 1 {
		t.Fatalf("move stop count = %d, want 1", move.stopCount)
	}
}

func TestPlayerAttackClearsIntentionWhenCanAttackFails(t *testing.T) {
	owner := playerActor(1)
	target := actor(2)
	owner.known[target.ObjectID()] = true
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: false}
	p := NewPlayerAttack(owner, move, strike)

	if p.Start(target) {
		t.Fatal("Start() = true, want false when CanAttack rejects the target")
	}
	if p.Target() != nil {
		t.Fatalf("Target() = %v after CanAttack rejection, want nil", p.Target())
	}
}

func TestPlayerAttackThinkRetriesAfterMovementArrives(t *testing.T) {
	owner := playerActor(1)
	target := actor(2)
	owner.known[target.ObjectID()] = true
	move := &recordingMove{followStarted: true}
	strike := &recordingAttack{canAttack: true}
	p := NewPlayerAttack(owner, move, strike)

	if !p.Start(target) {
		t.Fatal("Start() = false, want true while closing distance")
	}
	if strike.target != nil {
		t.Fatal("attacked before arrival, want no swing yet")
	}

	// Movement has now closed the distance; the arrived hook calls Think.
	move.followStarted = false
	p.Think()

	if strike.target != target {
		t.Fatalf("attacked target after arrival = %v, want %v", strike.target, target)
	}
}

func TestPlayerAttackStopClearsTargetAndStopsMovement(t *testing.T) {
	owner := playerActor(1)
	target := actor(2)
	owner.known[target.ObjectID()] = true
	move := &recordingMove{followStarted: true}
	strike := &recordingAttack{canAttack: true}
	p := NewPlayerAttack(owner, move, strike)
	p.Start(target)

	p.Stop()

	if p.Target() != nil {
		t.Fatalf("Target() = %v after Stop, want nil", p.Target())
	}
	if move.stopCount != 1 {
		t.Fatalf("move stop count = %d, want 1", move.stopCount)
	}
}

func TestPlayerAttackStartRejectsWhenAttackDisabled(t *testing.T) {
	owner := playerActor(1)
	owner.disabled = true
	target := actor(2)
	owner.known[target.ObjectID()] = true
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	p := NewPlayerAttack(owner, move, strike)

	if p.Start(target) {
		t.Fatal("Start() = true, want false while attacks are disabled")
	}
}

// stagedAttack detects overlapping DoAttack calls directly: AttackingNow
// always reports false (simulating the window before attack.Controller has
// updated its own busy flag), and DoAttack blocks until released, so a
// second call reaching DoAttack while the first is still blocked flips
// overlapped via a failed compare-and-swap.
type stagedAttack struct {
	canAttack  bool
	entered    chan struct{}
	release    chan struct{}
	inFlight   int32
	doAttackN  int32
	overlapped int32
}

func (a *stagedAttack) BowCoolingDown() bool { return false }
func (a *stagedAttack) AttackingNow() bool   { return false }
func (a *stagedAttack) CanAttack(attackable.Combatant) bool {
	return a.canAttack
}
func (a *stagedAttack) DoAttack(attackable.Combatant) {
	if !atomic.CompareAndSwapInt32(&a.inFlight, 0, 1) {
		atomic.StoreInt32(&a.overlapped, 1)
	}
	atomic.AddInt32(&a.doAttackN, 1)
	select {
	case <-a.entered:
	default:
		close(a.entered)
	}
	<-a.release
	atomic.StoreInt32(&a.inFlight, 0)
}

// TestPlayerAttackThinkDoesNotDoubleAttackWhileASwingIsInFlight proves
// thinkLocked's mutex spans the whole decision, not just the target read.
// AttackingNow() here always reports false, so a version that only locked
// the target field would let a concurrent Think reach DoAttack while the
// first call is still blocked inside it — caught here as an overlap via
// compare-and-swap, deterministically (not a timing-dependent race).
func TestPlayerAttackThinkDoesNotDoubleAttackWhileASwingIsInFlight(t *testing.T) {
	owner := playerActor(1)
	target := actor(2)
	owner.known[target.ObjectID()] = true
	move := &recordingMove{}
	strike := &stagedAttack{canAttack: true, entered: make(chan struct{}), release: make(chan struct{})}
	p := NewPlayerAttack(owner, move, strike)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.Start(target) // holds mu for the whole decision, including DoAttack
	}()
	<-strike.entered // first goroutine is now inside DoAttack, still holding mu
	go func() {
		defer wg.Done()
		p.Think() // must block on mu until the first decision finishes
	}()

	close(strike.release)
	wg.Wait()

	if atomic.LoadInt32(&strike.overlapped) != 0 {
		t.Fatal("a second DoAttack call overlapped one still in flight — thinkLocked did not hold mu for the whole decision")
	}
	if got := atomic.LoadInt32(&strike.doAttackN); got != 2 {
		t.Fatalf("DoAttack calls = %d, want 2 (both goroutines eventually attack, sequentially)", got)
	}
}
