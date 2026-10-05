package combat

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// sitDownDelay is Player.sitDown's 2.5s task delay (Player.java:1547-1554).
const sitDownDelay = 2500 * time.Millisecond

// frameSit is an ordinary sit-down.
var frameSit = fakeDeathFrame(fmt.Sprintf("ChangeWaitType(%d)", serverpackets.WaitSitting))

// TestQueuedSitAtGetUpEndKeepsLieDownEnd pins issue #3353's steps 1-4. A
// player lies down into fake death and asks to restart at once, so the
// get-ups end inside the lie-down, then presses sit while getting up, which
// queues (PlayableAI.tryToSit, isStandingNow). The get-up's end (STOOD_UP)
// runs the sit: the get-up took the standing posture, so PlayerAI.thinkSit
// sits the player down. The lie-down's own task is never cancelled
// (Player.startFakeDeath, Player.java:7017-7033): at its time it clears
// isSittingNow, seats the player and notifies SAT_DOWN, which runs a move
// queued during the sit-down; the seated player is refused it
// (PlayerAI.thinkMoveTo) long before the sit-down's own end
// (Player.sitDown, Player.java:1542-1565), which finds nothing queued.
func TestQueuedSitAtGetUpEndKeepsLieDownEnd(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	if !srv.DrivesClock() {
		t.Skip("the get-up must end inside the lie-down, a margin the wall clock does not hold")
	}
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	lieDown, getUp := victimFakeDeathDelays(t, srv, victim)
	if getUp >= lieDown {
		t.Fatalf("get-up %v not shorter than lie-down %v; the scenario needs the get-up to end first", getUp, lieDown)
	}
	start := vc.Now()
	startFakeDeath(t, srv, victim, false)

	vc.Send(encodeRequestRestartPoint(0))
	srv.Settle(t)
	vc.Send(encodeRequestChangeWaitType(false))
	srv.Settle(t)
	if !victim.SittingNow() || !victim.StandingNow() || !victim.Standing() {
		t.Fatalf("after the restart and sit requests SittingNow=%v StandingNow=%v Standing=%v, want the lie-down and a get-up both under way, the sit queued",
			victim.SittingNow(), victim.StandingNow(), victim.Standing())
	}

	srv.AdvanceUntil(t, "get-ups ended", func() bool { return !victim.StandingNow() })
	sitAt := vc.Now()
	if victim.Standing() || !victim.SittingNow() || victim.Seated() || victim.FakeDead() {
		t.Fatalf("at the get-ups' end Standing=%v SittingNow=%v Seated=%v FakeDead=%v, want the queued sit sitting the player down, out of fake death",
			victim.Standing(), victim.SittingNow(), victim.Seated(), victim.FakeDead())
	}
	// No frame is read until the lie-down has ended: each quiet read lets
	// readQuietWindow pass on the driven clock.
	x, y, z := victim.Position()
	vc.Send(encodeMoveFrom(int32(x)+100, int32(y), int32(z), int32(x), int32(y), int32(z)))
	srv.Settle(t)

	srv.AdvanceUntil(t, "lie-down ended", func() bool { return !victim.SittingNow() })
	if at := vc.Now().Sub(start); at > lieDown+10*time.Millisecond {
		t.Fatalf("SittingNow cleared %v after the lie-down began, want at its end %v", at, lieDown)
	}
	if !victim.Seated() || victim.Standing() {
		t.Fatalf("at the lie-down's end Seated=%v Standing=%v, want seated", victim.Seated(), victim.Standing())
	}
	if sitEnd := sitAt.Add(sitDownDelay); !vc.Now().Add(readQuietWindow).Before(sitEnd) {
		t.Fatalf("the sit-down ends %v after the lie-down, inside one quiet read; the scenario cannot tell the two ends apart", sitEnd.Sub(vc.Now()))
	}
	// Player.tryToPassBoatEntrance answers the move with ActionFailed (no
	// boat known), PlayableAI.tryToMoveTo queues it with another, and the
	// lie-down's SAT_DOWN refuses it, the player now seated, with a third.
	untilLieDownEnd := readQuiet(vc)
	if n := countOpcode(untilLieDownEnd, len(untilLieDownEnd), serverpackets.OpcodeActionFailed); n != 3 {
		t.Fatalf("by the lie-down's end the victim got %d ActionFailed, want 3: boat probe, queued move, move refused while seated", n)
	}
	if countOpcode(untilLieDownEnd, len(untilLieDownEnd), serverpackets.OpcodeMoveToLocation) != 0 {
		t.Fatal("the victim started the queued move")
	}

	srv.Advance(t, sitAt.Add(sitDownDelay).Sub(vc.Now())+10*time.Millisecond)
	afterSit := readQuiet(vc)
	if !victim.Seated() || victim.SittingNow() || victim.StandingNow() {
		t.Fatalf("after the sit-down Seated=%v SittingNow=%v StandingNow=%v, want seated", victim.Seated(), victim.SittingNow(), victim.StandingNow())
	}
	if n := countOpcode(afterSit, len(afterSit), serverpackets.OpcodeActionFailed); n != 0 {
		t.Fatalf("at the sit-down's end the victim got %d ActionFailed, want none: nothing was left queued", n)
	}

	want := []fakeDeathFrame{frameStartFake, frameStopFake, frameRevive, frameStopFake, frameRevive, frameSit}
	if got := postureLifeFrames(slices.Concat(untilLieDownEnd, afterSit), id); !slices.Equal(got, want) {
		t.Errorf("self frames = %v, want %v", got, want)
	}
	if got := postureLifeFrames(readQuiet(c), id); !slices.Equal(got, want) {
		t.Errorf("observer frames = %v, want %v", got, want)
	}
}

// encodeMoveFrom is a mouse-click MoveBackwardToLocation to target from the
// client's origin.
func encodeMoveFrom(targetX, targetY, targetZ, originX, originY, originZ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range []int32{targetX, targetY, targetZ, originX, originY, originZ, 1} {
		w.WriteInt32(v)
	}
	return w.Bytes()
}
