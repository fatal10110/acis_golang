package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference fake-death timeline:
//   - Player.startFakeDeath (Player.java:7017-7033) schedules the lie-down
//     task, which sets _isSitting after (int)(3000 / mult) ms;
//   - Player.stopFakeDeath (Player.java:7035-7056) clears _isSitting,
//     broadcasts ChangeWaitType(WT_STOP_FAKEDEATH) then Revive, and
//     schedules the get-up task, which clears _isFakeDeath after
//     (int)(2500 / mult) ms, dead or alive. Neither task is ever cancelled,
//     so a get-up during the lie-down still ends seated, and a second
//     stopFakeDeath does not move the first get-up's end;
//   - RequestRestartPoint.runImpl (RequestRestartPoint.java:38-42) checks
//     isFakeDeath() before isDead(): while it holds, the request calls
//     stopFakeDeath(true) and returns. With the Fake Death effect still on,
//     that call's stopEffects(FAKE_DEATH) exits the effect first, whose
//     EffectFakeDeath.onExit (EffectFakeDeath.java:33-37) nests one more
//     stopFakeDeath, so two get-ups go out;
//   - mult is CreatureStatus.getMovementSpeedMultiplier, the DEX run-speed
//     bonus for an unequipped player on land;
//   - CharInfo writes isSitting() ? 0 : 1 (CharInfo.java:130), and a hit on
//     a seated player not playing dead broadcasts ChangeWaitType(WT_STANDING)
//     once: its ATTACKED stand intention (PlayerAI.onEvtAttacked/thinkStand,
//     PlayerAI.java:148-158, 490-504) stands it up before the damage, whose
//     own stand-up (PlayerStatus.java:124-125) then finds it standing.

// victimFakeDeathDelays are the reference lie-down and get-up lengths for
// victim: (int)(3000 / DEXBonus[dex]) and (int)(2500 / DEXBonus[dex]) ms.
func victimFakeDeathDelays(t *testing.T, srv *gameservertest.Server, victim *player.Character) (lieDown, getUp time.Duration) {
	t.Helper()
	var dex int
	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() { dex = victim.DEX() })
	mult := float32(statbonus.DEXBonus[dex])
	if mult == 1 {
		t.Fatalf("DEX %d gives multiplier 1; the lengths would not tell the multiplier apart", dex)
	}
	return time.Duration(int32(3000/mult)) * time.Millisecond, time.Duration(int32(2500/mult)) * time.Millisecond
}

// TestRestartWithinFakeDeathGetUpOnCorpseOnlyGetsUp kills a player lying in
// fake death and has it ask to restart: inside the death's get-up each
// request only sends the get-up again and the player stays dead; the
// repeats do not move the get-up's end, and a request after it respawns.
func TestRestartWithinFakeDeathGetUpOnCorpseOnlyGetsUp(t *testing.T) {
	t.Parallel()
	srv, c, vc, attacker, iv := bootPvPPair(t, gameservertest.WithRestartPoints(restartTable()))
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	_, getUp := victimFakeDeathDelays(t, srv, victim)
	lieInFakeDeath(t, srv, victim, false)
	// Read before the death: its get-up starts no earlier.
	diedAt := vc.Now()
	onQueue(t, srv.PlayerQueue(t, id), func() {
		if !victim.Kill(attacker) {
			t.Error("Kill() = false on a living player")
		}
	})
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	// Each quiet read lets the clock run, so the state is checked before the
	// frames are read.
	restartInsideGetUp := func(what string) {
		t.Helper()
		vc.Send(encodeRequestRestartPoint(0))
		srv.Settle(t)
		if !victim.Dead() || !victim.FakeDead() {
			t.Fatalf("%s: Dead=%v FakeDead=%v, want still dead and playing dead", what, victim.Dead(), victim.FakeDead())
		}
		assertFramesOnly(t, vc, c, id, []fakeDeathFrame{frameStopFake, frameRevive})
	}
	restartInsideGetUp("restart right after the death")
	if srv.DrivesClock() {
		srv.Advance(t, getUp-vc.Now().Sub(diedAt)-10*time.Millisecond)
		restartInsideGetUp("restart 10ms before the get-up ends")
		// The frame reads ran the clock past the death's get-up end, but
		// well short of a get-up restarted by the last request.
		if elapsed := vc.Now().Sub(diedAt); elapsed >= 2*getUp-10*time.Millisecond {
			t.Fatalf("frame reads ran %v past the death; the check below proves nothing", elapsed)
		}
		if victim.FakeDead() {
			t.Fatal("FakeDead() = true after the death's get-up end: a repeated request moved it")
		}
	}
	srv.AdvanceUntil(t, "death's get-up ended", func() bool { return !victim.FakeDead() })
	if elapsed := vc.Now().Sub(diedAt); elapsed < getUp {
		t.Fatalf("fake death ended %v after the death, want no earlier than %v", elapsed, getUp)
	}

	vc.Send(encodeRequestRestartPoint(0))
	srv.AdvanceUntil(t, "respawn after the get-up", func() bool { return !victim.Dead() })
}

// TestRestartDuringFakeDeathLieDownEndsSeated has a living player ask to
// restart while still lying down into fake death: the Fake Death effect's
// exit and the request each send a get-up, and once both the get-up and the
// lie-down have ended the player sits — observers get a sitting CharInfo
// and the next hit stands it up.
func TestRestartDuringFakeDeathLieDownEndsSeated(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	startFakeDeath(t, srv, victim, false)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	if !victim.SittingNow() {
		t.Fatal("the lie-down had already ended; the scenario proves nothing")
	}

	vc.Send(encodeRequestRestartPoint(0))
	srv.Settle(t)
	assertFramesOnly(t, vc, c, id, []fakeDeathFrame{frameStopFake, frameRevive, frameStopFake, frameRevive})
	if victim.Dead() || !victim.Standing() || !victim.SittingNow() {
		t.Fatalf("after the restart request Dead=%v Standing=%v SittingNow=%v, want alive, standing, lie-down still under way",
			victim.Dead(), victim.Standing(), victim.SittingNow())
	}

	srv.AdvanceUntil(t, "lie-down and get-up ended", func() bool { return !victim.SittingNow() && !victim.StandingNow() })
	if victim.Standing() || !victim.Seated() || victim.FakeDead() {
		t.Fatalf("after the lie-down Standing=%v Seated=%v FakeDead=%v, want seated, not playing dead",
			victim.Standing(), victim.Seated(), victim.FakeDead())
	}
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	vc.Send(encodeRequestChangeMoveType(false))
	srv.Settle(t)
	seen := readQuiet(c)
	i := indexOfCharInfo(seen, id)
	if i < 0 {
		t.Fatalf("observer got no CharInfo for the victim after its walk toggle (opcodes %v)", opcodes(seen))
	}
	// With no cubics, CharInfo's standing byte sits 74 bytes from the end:
	// it is followed by six bytes, the cubic count, the party match room,
	// then a fixed 64-byte tail starting at the abnormal effect.
	if got := seen[i][len(seen[i])-74]; got != 0 {
		t.Fatalf("CharInfo standing byte = %d, want 0 (sitting)", got)
	}
	drainUntilQuiet(t, vc)

	before := srv.PlayerCurrentHP(t, id)
	attackPlayer(t, c, id)
	srv.AdvanceUntil(t, "first hit on the seated victim", func() bool { return srv.PlayerCurrentHP(t, id) < before })
	// The stand intention stands the player up ahead of the damage, which
	// then finds it standing: one stand-up, never a get-up out of fake death.
	assertFramesOnly(t, vc, c, id, []fakeDeathFrame{frameStand})
}

// TestStandWhileLyingInFakeDeathGetsUpTwice has a player lying in fake
// death press stand: PlayerAI.thinkStand (PlayerAI.java:489-502) calls
// stopFakeDeath(true), whose stopEffects(FAKE_DEATH) exits the effect and
// nests one get-up before its own, so both clients see two.
func TestStandWhileLyingInFakeDeathGetsUpTwice(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	lieInFakeDeath(t, srv, victim, false)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	vc.Send(encodeRequestChangeWaitType(true))
	srv.Settle(t)
	if !victim.FakeDead() || !victim.StandingNow() {
		t.Fatalf("after the stand request FakeDead=%v StandingNow=%v, want getting up, still playing dead", victim.FakeDead(), victim.StandingNow())
	}
	assertFramesOnly(t, vc, c, victim.ObjectID(), []fakeDeathFrame{frameStopFake, frameRevive, frameStopFake, frameRevive})
}
