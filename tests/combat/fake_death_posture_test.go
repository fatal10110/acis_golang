package combat

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference posture-end handling: Player.startFakeDeath and
// Player.stopFakeDeath (Player.java:7017-7056) each schedule their own
// uncancelled task, the lie-down's notifying SAT_DOWN and every get-up's
// STOOD_UP. PlayerAI.onEvtSatDown (PlayerAI.java:99-106) runs the queued
// next intention through doIntention at once, past the isSittingNow /
// isStandingNow gate the tryTo* entry points (PlayableAI.java:439-475)
// queue behind, so it runs while another transition is still under way.
// PlayerAI.onEvtStoodUp (PlayerAI.java:108-124) does the same after it
// frees the throne (StaticObject.setBusy(false), throneId 0), whatever the
// posture is by then.

// frameStartFake is the lie-down into fake death.
var frameStartFake = fakeDeathFrame(fmt.Sprintf("ChangeWaitType(%d)", serverpackets.WaitFakeDeathStart))

// TestQueuedStandRunsAtLieDownEndBesideGetUp lies a player down into fake
// death and has it ask to restart late in the lie-down, so the get-ups
// outlast it, then press stand, which queues. The lie-down's end (SAT_DOWN)
// runs the stand at once: the player is seated and still playing dead, so
// PlayerAI.thinkStand (PlayerAI.java:490-504) gets it up out of fake death
// a third time; the Fake Death effect is already gone, so no nested get-up
// precedes it. Nobody ever sees ChangeWaitType(STANDING).
func TestQueuedStandRunsAtLieDownEndBesideGetUp(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	lieDown, getUp := victimFakeDeathDelays(t, srv, victim)
	if lieDown <= getUp {
		t.Fatalf("lie-down %v not longer than get-up %v; the scenario needs a get-up that outlasts it", lieDown, getUp)
	}
	start := vc.Now()
	startFakeDeath(t, srv, victim, false)
	// Restarting at lieDown - getUp/2 leaves half a get-up of margin on
	// both sides: the get-ups end that long after the lie-down does.
	srv.Advance(t, lieDown-getUp/2-vc.Now().Sub(start))
	if !victim.SittingNow() {
		t.Fatal("the lie-down had already ended; the scenario proves nothing")
	}

	vc.Send(encodeRequestRestartPoint(0))
	srv.Settle(t)
	vc.Send(encodeRequestChangeWaitType(true))
	srv.Settle(t)
	if !victim.SittingNow() || !victim.StandingNow() || !victim.FakeDead() {
		t.Fatalf("after the restart and stand requests SittingNow=%v StandingNow=%v FakeDead=%v, want the lie-down and a get-up both under way",
			victim.SittingNow(), victim.StandingNow(), victim.FakeDead())
	}

	srv.AdvanceUntil(t, "lie-down ended", func() bool { return !victim.SittingNow() })
	if !victim.Standing() || !victim.StandingNow() || !victim.FakeDead() {
		t.Fatalf("at the lie-down's end Standing=%v StandingNow=%v FakeDead=%v, want the queued stand to have got the player up again while it still plays dead",
			victim.Standing(), victim.StandingNow(), victim.FakeDead())
	}

	srv.Advance(t, getUp+10*time.Millisecond)
	if !victim.Standing() || victim.StandingNow() || victim.SittingNow() || victim.FakeDead() {
		t.Fatalf("after every get-up Standing=%v StandingNow=%v SittingNow=%v FakeDead=%v, want standing, out of fake death",
			victim.Standing(), victim.StandingNow(), victim.SittingNow(), victim.FakeDead())
	}
	assertFramesOnly(t, vc, c, id, []fakeDeathFrame{
		frameStartFake,
		frameStopFake, frameRevive, frameStopFake, frameRevive, // restart request
		frameStopFake, frameRevive, // queued stand at the lie-down's end
	})
}

// TestQueuedStandRunsAtGetUpEndBesideLieDown has a player ask to restart
// early in the fake-death lie-down, so the get-ups end before it, then
// press stand, which queues. The first get-up's end (STOOD_UP) runs the
// stand at once: the player took the standing posture with the get-up and
// is not seated, so PlayerAI.thinkStand (PlayerAI.java:490-504) refuses it
// with ActionFailed. The lie-down's end then seats the player, who stays
// seated: no ChangeWaitType(STANDING) ever goes out.
func TestQueuedStandRunsAtGetUpEndBesideLieDown(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	if !srv.DrivesClock() {
		t.Skip("the get-up must end inside the lie-down, a margin the wall clock does not hold")
	}
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	startFakeDeath(t, srv, victim, false)

	vc.Send(encodeRequestRestartPoint(0))
	srv.Settle(t)
	vc.Send(encodeRequestChangeWaitType(true))
	srv.Settle(t)
	if !victim.SittingNow() || !victim.StandingNow() {
		t.Fatalf("after the restart and stand requests SittingNow=%v StandingNow=%v, want the lie-down and a get-up both under way",
			victim.SittingNow(), victim.StandingNow())
	}

	srv.AdvanceUntil(t, "get-ups ended", func() bool { return !victim.StandingNow() })
	if !victim.SittingNow() || !victim.Standing() || victim.FakeDead() {
		t.Fatalf("at the get-ups' end SittingNow=%v Standing=%v FakeDead=%v, want the lie-down still under way, standing, out of fake death",
			victim.SittingNow(), victim.Standing(), victim.FakeDead())
	}

	srv.AdvanceUntil(t, "lie-down ended", func() bool { return !victim.SittingNow() })
	self := readQuiet(vc)
	if !victim.Seated() || victim.StandingNow() || victim.FakeDead() {
		t.Fatalf("after the lie-down Seated=%v StandingNow=%v FakeDead=%v, want seated, not playing dead",
			victim.Seated(), victim.StandingNow(), victim.FakeDead())
	}
	want := []fakeDeathFrame{frameStartFake, frameStopFake, frameRevive, frameStopFake, frameRevive}
	if got := postureLifeFrames(self, id); !slices.Equal(got, want) {
		t.Errorf("self frames = %v, want %v", got, want)
	}
	if got := postureLifeFrames(readQuiet(c), id); !slices.Equal(got, want) {
		t.Errorf("observer frames = %v, want %v", got, want)
	}
	if n := countOpcode(self, len(self), serverpackets.OpcodeActionFailed); n != 1 {
		t.Errorf("victim got %d ActionFailed, want 1: the queued stand refused at the get-up's end", n)
	}
}

// TestLateGetUpEndFreesThroneOfLaterSit has a player lying in fake death
// target a throne, press stand (two get-ups, ending together) and press sit,
// which queues. The first get-up's end runs the sit, which claims the throne
// (ChairSit); the second get-up's end then frees it although the player is
// sitting down on it (PlayerAI.onEvtStoodUp, PlayerAI.java:108-124).
func TestLateGetUpEndFreesThroneOfLaterSit(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	lieInFakeDeath(t, srv, victim, false)
	x, y, z := victim.Position()
	throne, err := staticobject.NewObject(srv.NewObjectID(), &staticobject.Template{
		ID: 24180017, Location: location.Location{X: x, Y: y, Z: z}, Type: staticobject.ChairType,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	srv.State.Spawn(throne, x, y, z, 0)
	readUntil(t, vc, serverpackets.OpcodeStaticObjectInfo, "throne StaticObjectInfo")
	vc.Send(encodeAction(throne.ObjectID(), int32(x), int32(y), int32(z), false))
	readUntil(t, vc, serverpackets.OpcodeMyTargetSelected, "throne selected")
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	if !victim.Seated() || !victim.FakeDead() {
		t.Fatalf("before the stand Seated=%v FakeDead=%v, want lying in fake death", victim.Seated(), victim.FakeDead())
	}

	vc.Send(encodeRequestChangeWaitType(true))
	srv.Settle(t)
	vc.Send(encodeRequestChangeWaitType(false))
	srv.Settle(t)
	if !victim.StandingNow() || throne.Busy() {
		t.Fatalf("after the stand and sit requests StandingNow=%v throne busy=%v, want getting up, the sit queued", victim.StandingNow(), throne.Busy())
	}

	srv.AdvanceUntil(t, "get-ups ended", func() bool { return !victim.StandingNow() })
	if !victim.SittingNow() || victim.Standing() {
		t.Fatalf("after the get-ups SittingNow=%v Standing=%v, want the queued sit under way", victim.SittingNow(), victim.Standing())
	}
	if throne.Busy() {
		t.Fatal("throne still busy: the later get-up's end did not free it")
	}
	self := readQuiet(vc)
	if countOpcode(self, len(self), serverpackets.OpcodeChairSit) != 1 {
		t.Fatalf("victim got %d ChairSit, want 1: the queued sit claims the throne", countOpcode(self, len(self), serverpackets.OpcodeChairSit))
	}
	want := []fakeDeathFrame{frameStopFake, frameRevive, frameStopFake, frameRevive, fakeDeathFrame(fmt.Sprintf("ChangeWaitType(%d)", serverpackets.WaitSitting))}
	if got := postureLifeFrames(self, id); !slices.Equal(got, want) {
		t.Errorf("self frames = %v, want %v", got, want)
	}

	srv.AdvanceUntil(t, "sit-down ended", victim.Seated)
	if throne.Busy() {
		t.Fatal("the sit-down's end claimed the throne again")
	}
}
