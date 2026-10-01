package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ---- from attackable_wander_test.go ----
// TestAttackableAIPromotesMoveToDesireAndKeepsItAtDestination pins
// NpcAI.thinkMoveTo at the destination: the desire is dropped but MOVE_TO
// stays current until desire selection idles it on the empty queue.
func TestAttackableAIPromotesMoveToDesireAndKeepsItAtDestination(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionMoveTo)
	}
	if move.home != home {
		t.Fatalf("MoveHome destination = %#v, want %#v", move.home, home)
	}

	owner.x, owner.y, owner.z = home.X, home.Y, home.Z
	move.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() after arrival error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after arrival Think = %v, want %v kept", got, IntentionMoveTo)
	}
	if brain.Desires().Len() != 0 {
		t.Fatalf("desires after arrival Think = %d, want the MOVE_TO desire dropped", brain.Desires().Len())
	}
	if move.home == home {
		t.Fatal("Think at the destination restarted MoveHome")
	}
	if err := tickThinkIdle(brain); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after TickThink = %v, want %v", got, IntentionIdle)
	}
}

// hitAnimationAttack is a recordingAttack that reports a hit animation
// window.
type hitAnimationAttack struct {
	recordingAttack
	inHitAnimation bool
}

func (a *hitAnimationAttack) InHitAnimation() bool { return a.inHitAnimation }

// TestAttackableAIHoldsPromotionInsideHitAnimation pins NpcAI.runAI's hit
// animation gate: a queued desire is not taken up while the attack
// controller reports its hit animation window open, and is taken up by the
// next pass once the window closes.
func TestAttackableAIHoldsPromotionInsideHitAnimation(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	strike := &hitAnimationAttack{inHitAnimation: true}
	brain := NewAttackable(owner, move, strike)
	dest := location.Location{X: 300}
	unmoved := location.Location{X: -1, Y: -1, Z: -1}
	move.home = unmoved

	brain.AddMoveToDesire(dest, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() inside the hit animation error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() inside the hit animation = %v, want %v", got, IntentionIdle)
	}
	if move.home != unmoved {
		t.Fatalf("MoveHome destination inside the hit animation = %#v, want no move", move.home)
	}

	strike.inHitAnimation = false
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() after the hit animation error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the hit animation = %v, want %v", got, IntentionMoveTo)
	}
	if move.home != dest {
		t.Fatalf("MoveHome destination after the hit animation = %#v, want %#v", move.home, dest)
	}
}

// TestAttackableAIArrivedClearsMoveToWhenGeoSnapsZ pins that desire
// selection idles a walk an arrival left short of its destination rather
// than restarting it: NpcAI.runAI steps a walk only through its queued
// desire.
func TestAttackableAIArrivedClearsMoveToWhenGeoSnapsZ(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionMoveTo)
	}

	// |ΔZ| > Desire.Equal's 30 so thinkMoveTo's fast path cannot idle.
	owner.x, owner.y, owner.z = home.X, home.Y, home.Z-40
	brain.Arrived()
	move.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() after Arrived error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after Arrived and RunAI = %v, want %v", got, IntentionIdle)
	}
	if move.home == home {
		t.Fatal("RunAI after Arrived restarted MoveHome")
	}
}

// TestAttackableAIArrivedShortThinkResumesMoveTo pins the reference THINK
// on a MOVE_TO an arrival left short of its destination with no swing in
// flight: AbstractAI.onEvtThink runs NpcAI.thinkMoveTo, which away from the
// destination calls CreatureAI.thinkMoveTo and walks again. The desire-less
// walk is not idled first.
func TestAttackableAIArrivedShortThinkResumesMoveTo(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	owner.x, owner.y, owner.z = home.X, home.Y, home.Z-40
	brain.Arrived()
	move.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() after Arrived error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after arrival Think = %v, want %v kept", got, IntentionMoveTo)
	}
	if move.home != home {
		t.Fatalf("MoveHome after arrival Think = %#v, want %#v", move.home, home)
	}
	if brain.Desires().Len() != 0 {
		t.Fatalf("desires after arrival Think = %d, want none re-queued", brain.Desires().Len())
	}

	move.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() after arrival Think error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionIdle)
	}
	if move.home == home {
		t.Fatal("RunAI after arrival Think restarted MoveHome")
	}
}

// TestAttackableAIArrivedBlockedThinkResumesMoveTo pins
// NpcAI.onEvtArrivedBlocked dropping only the MOVE_TO desire: a THINK then
// runs thinkMoveTo and walks for the destination again.
func TestAttackableAIArrivedBlockedThinkResumesMoveTo(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}

	brain.ArrivedBlocked()
	if brain.Desires().Len() != 0 {
		t.Fatalf("desires after ArrivedBlocked = %d, want the MOVE_TO desire dropped", brain.Desires().Len())
	}
	move.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() after ArrivedBlocked error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after ArrivedBlocked Think = %v, want %v kept", got, IntentionMoveTo)
	}
	if move.home != home {
		t.Fatalf("MoveHome after ArrivedBlocked Think = %#v, want %#v", move.home, home)
	}
}

// TestAttackableAIArrivedKeepsMoveToWhileAttackingNow pins NpcAI.onEvtArrived:
// an arrival short of the MOVE_TO destination drops only the desire. Desire
// selection does not restart the finished walk, even while a swing keeps
// dropCurrentIfUnqueued from idling it, but a THINK runs thinkMoveTo and
// sets off for the destination again.
func TestAttackableAIArrivedKeepsMoveToWhileAttackingNow(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	mv := &recordingMove{}
	atk := &recordingAttack{attackingNow: true}
	brain := NewAttackable(owner, mv, atk)
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionMoveTo)
	}

	owner.x, owner.y, owner.z = home.X, home.Y, home.Z-40
	brain.Arrived()
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after Arrived = %v, want %v kept", got, IntentionMoveTo)
	}
	if brain.Desires().Len() != 0 {
		t.Fatalf("desires after Arrived = %d, want the MOVE_TO desire dropped", brain.Desires().Len())
	}
	mv.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() after Arrived error: %v", err)
	}
	if mv.home == home {
		t.Fatal("RunAI after Arrived restarted MoveHome while attacking")
	}
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() after Arrived error: %v", err)
	}
	if mv.home != home {
		t.Fatalf("MoveHome after arrival Think = %#v, want %#v", mv.home, home)
	}
}

func TestAttackableAIArrivedRestoresSpawnHeading(t *testing.T) {
	owner := actor(1)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.Arrived()

	if owner.headingRestores != 1 {
		t.Fatalf("heading restores = %d, want 1", owner.headingRestores)
	}
}

func TestAttackableAIFollowArrivedSkipsSpawnHeadingAndOOTSweep(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionFollow, FinalTarget: target, Weight: 5})
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want follow", got)
	}

	addAttackHate(ai, target, 0, 20)
	start := time.Now()
	ai.now = func() time.Time { return start }
	owner.inTerritory = false
	ai.Arrived()

	if owner.headingRestores != 0 {
		t.Fatalf("heading restores = %d, want 0 on FOLLOW arrival", owner.headingRestores)
	}

	tickOOTSweepDue(ai, start.Add(91*time.Second))
	if got := ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}); !got {
		t.Fatal("FOLLOW arrival armed the OOT sweep, want it skipped")
	}
}

func TestAttackableAIAttackArrivedRestoresHeadingAndArmsOOTSweep(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})
	addAttackHate(ai, target, 0, 1000)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack", got)
	}

	start := time.Now()
	ai.now = func() time.Time { return start }
	owner.inTerritory = false
	ai.Arrived()

	if owner.headingRestores != 1 {
		t.Fatalf("heading restores = %d, want 1 on ATTACK arrival", owner.headingRestores)
	}

	tickOOTSweepDue(ai, start.Add(91*time.Second))
	if got := ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}); got {
		t.Fatal("ATTACK arrival did not arm the OOT sweep")
	}
}

func TestAttackableAIAddMoveToDesireSkipsUnreachable(t *testing.T) {
	owner := actor(1)
	move := &recordingMove{denyMove: true}
	brain := NewAttackable(owner, move, &recordingAttack{})

	if brain.AddMoveToDesire(location.Location{X: 50, Y: 0, Z: 0}, 1_000_000) {
		t.Fatal("AddMoveToDesire() = true, want false when unreachable")
	}
	if got := brain.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0", got)
	}
}
