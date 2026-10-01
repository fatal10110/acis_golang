package combat

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference for a player dying while it plays dead:
//   - Playable.doDie (Playable.java:127-185) strips every effect that does
//     not last through death unless a Phoenix or Noblesse Blessing is on,
//     then broadcasts StatusUpdate and notifies DEAD, whose
//     CreatureAI.onEvtDead broadcasts Die (CreatureAI.java:78-86);
//   - the stripped Fake Death's EffectFakeDeath.onExit calls
//     stopFakeDeath(true) (EffectFakeDeath.java:33-37); the effect is
//     already removed when onExit runs (AbstractEffect.scheduleEffect
//     FINISHING, AbstractEffect.java:310-320), so it sends one pair;
//   - Player.doDie then calls stopFakeDeath(true) again while isFakeDeath()
//     still holds (Player.java:2609-2613), since only the delayed get-up
//     clears it; with the effect kept by a blessing, that call's own
//     stopEffects(FAKE_DEATH) exits it first, nesting one more pair;
//   - stopFakeDeath always broadcasts ChangeWaitType(WT_STOP_FAKEDEATH)
//     then Revive, dead or not (Player.java:7035-7056);
//   - a hit on a seated player broadcasts ChangeWaitType(WT_STANDING)
//     first (PlayerStatus.java:124-125, Player.standUp Player.java:1574-1591),
//     leaving fake death and its effect in place.
//
// Each frame is broadcast to the dying player's own client and to every
// player that knows it.

// fakeDeathFrame names one of the dying player's posture and life frames.
type fakeDeathFrame string

const (
	frameStand    fakeDeathFrame = "ChangeWaitType(STANDING)"
	frameStopFake fakeDeathFrame = "ChangeWaitType(STOP_FAKEDEATH)"
	frameRevive   fakeDeathFrame = "Revive"
	frameDie      fakeDeathFrame = "Die"
)

// postureLifeFrames reduces frames to id's ChangeWaitType, Revive and Die
// frames, in wire order.
func postureLifeFrames(frames [][]byte, id int32) []fakeDeathFrame {
	var out []fakeDeathFrame
	for _, f := range frames {
		if len(f) < 5 || int32(binary.LittleEndian.Uint32(f[1:5])) != id {
			continue
		}
		switch f[0] {
		case serverpackets.OpcodeChangeWaitType:
			switch serverpackets.WaitType(binary.LittleEndian.Uint32(f[5:9])) {
			case serverpackets.WaitStanding:
				out = append(out, frameStand)
			case serverpackets.WaitFakeDeathStop:
				out = append(out, frameStopFake)
			default:
				out = append(out, fakeDeathFrame(fmt.Sprintf("ChangeWaitType(%d)", binary.LittleEndian.Uint32(f[5:9]))))
			}
		case serverpackets.OpcodeRevive:
			out = append(out, frameRevive)
		case serverpackets.OpcodeDie:
			out = append(out, frameDie)
		}
	}
	return out
}

// lieInFakeDeath puts victim in a Fake Death effect, preceded by a
// Noblesse Blessing when blessed, and waits for the lie-down to end.
func lieInFakeDeath(t *testing.T, srv *gameservertest.Server, victim *player.Character, blessed bool) {
	t.Helper()
	startFakeDeath(t, srv, victim, blessed)
	srv.AdvanceUntil(t, "fake-death lie-down ended", victim.Seated)
}

// startFakeDeath puts victim in a Fake Death effect, preceded by a Noblesse
// Blessing when blessed, and leaves it lying down.
func startFakeDeath(t *testing.T, srv *gameservertest.Server, victim *player.Character, blessed bool) {
	t.Helper()
	names := []string{"FakeDeath"}
	if blessed {
		names = []string{"NoblesseBless", "FakeDeath"}
	}
	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() {
		for _, name := range names {
			e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
			if err != nil {
				t.Fatalf("new %s effect: %v", name, err)
			}
			e.Effected = victim
			victim.EffectList().Add(e)
		}
	})
	if !victim.FakeDead() {
		t.Fatal("victim is not playing dead")
	}
}

func onlineVictim(t *testing.T, srv *gameservertest.Server, id int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d not in world", id)
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return ch
}

// assertDeathFrames checks the victim's own client and the observer saw
// want, and that the victim is dead, standing, and still playing dead: the
// death's get-up holds fake death until it ends (Player.stopFakeDeath,
// Player.java:7035-7056).
func assertDeathFrames(t *testing.T, self, observer *scriptedClient, victim *player.Character, want []fakeDeathFrame) {
	t.Helper()
	assertFramesOnly(t, self, observer, victim.ObjectID(), want)
	if !victim.Dead() || !victim.Standing() || !victim.FakeDead() {
		t.Fatalf("victim Dead=%v Standing=%v FakeDead=%v, want dead, standing, still playing dead",
			victim.Dead(), victim.Standing(), victim.FakeDead())
	}
}

// assertFramesOnly checks the victim's own client and the observer saw
// want of id's posture and life frames.
func assertFramesOnly(t *testing.T, self, observer *scriptedClient, id int32, want []fakeDeathFrame) {
	t.Helper()
	for _, rc := range []struct {
		who string
		c   *scriptedClient
	}{{"self", self}, {"observer", observer}} {
		if got := postureLifeFrames(readQuiet(rc.c), id); !slices.Equal(got, want) {
			t.Errorf("%s frames = %v, want %v", rc.who, got, want)
		}
	}
}

// TestKillWhilePlayingDeadSendsGetUps kills a player lying in fake death
// with no hit: the stripped Fake Death gets it up before Die, and the death
// gets it up once more after.
func TestKillWhilePlayingDeadSendsGetUps(t *testing.T) {
	t.Parallel()
	srv, c, vc, attacker, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	lieInFakeDeath(t, srv, victim, false)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() {
		if !victim.Kill(attacker) {
			t.Error("Kill() = false on a living player")
		}
	})
	assertDeathFrames(t, vc, c, victim, []fakeDeathFrame{frameStopFake, frameRevive, frameDie, frameStopFake, frameRevive})
}

// TestKillWhilePlayingDeadBlessedSendsGetUpsAfterDie kills a player lying
// in fake death under a Noblesse Blessing: the blessing keeps the Fake
// Death through the strip, so both get-ups follow Die — the effect's own,
// ended by the death's get-up, then the death's.
func TestKillWhilePlayingDeadBlessedSendsGetUpsAfterDie(t *testing.T) {
	t.Parallel()
	srv, c, vc, attacker, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	lieInFakeDeath(t, srv, victim, true)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() {
		if !victim.Kill(attacker) {
			t.Error("Kill() = false on a living player")
		}
	})
	assertDeathFrames(t, vc, c, victim, []fakeDeathFrame{frameDie, frameStopFake, frameRevive, frameStopFake, frameRevive})
	if victim.EffectList().IsAffected(effect.FlagFakeDeath) {
		t.Fatal("Fake Death outlived the death")
	}
}

// TestLethalHitWhilePlayingDeadSendsGetUps lands a killing swing on a
// player lying in fake death: the hit stands it up, the stripped Fake
// Death gets it up before Die, and the death gets it up once more after.
func TestLethalHitWhilePlayingDeadSendsGetUps(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	lieInFakeDeath(t, srv, victim, false)
	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() {
		victim.SetCP(0)
		victim.SetHP(1)
	})
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	attackPlayer(t, c, victim.ObjectID())
	srv.AdvanceUntil(t, "victim killed", victim.Dead)
	assertDeathFrames(t, vc, c, victim, []fakeDeathFrame{frameStand, frameStopFake, frameRevive, frameDie, frameStopFake, frameRevive})
}

// TestKillDuringLieDownLeavesSeatedCorpse kills a player still lying down
// into fake death: the get-ups go out and the corpse takes the standing
// posture, but the lie-down runs on beside the death's get-up and seats it
// when it ends; the get-up's end ends fake death.
func TestKillDuringLieDownLeavesSeatedCorpse(t *testing.T) {
	t.Parallel()
	srv, c, vc, attacker, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	startFakeDeath(t, srv, victim, false)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	if !victim.SittingNow() {
		t.Fatal("the lie-down had already ended; the scenario proves nothing")
	}

	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() {
		if !victim.Kill(attacker) {
			t.Error("Kill() = false on a living player")
		}
	})
	assertDeathFrames(t, vc, c, victim, []fakeDeathFrame{frameStopFake, frameRevive, frameDie, frameStopFake, frameRevive})
	srv.AdvanceUntil(t, "lie-down and get-up ended", func() bool { return !victim.SittingNow() && !victim.StandingNow() })
	if victim.Standing() || !victim.Seated() || victim.FakeDead() {
		t.Fatalf("corpse after the lie-down Standing=%v Seated=%v FakeDead=%v, want seated, not playing dead",
			victim.Standing(), victim.Seated(), victim.FakeDead())
	}
}

// TestKillDuringFakeDeathGetUpSendsOneGetUp kills a player already getting
// up out of fake death: its Fake Death is gone, so Die comes first, then
// the death's one get-up.
func TestKillDuringFakeDeathGetUpSendsOneGetUp(t *testing.T) {
	t.Parallel()
	srv, c, vc, attacker, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	lieInFakeDeath(t, srv, victim, false)
	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() {
		victim.EffectList().StopByType(effect.TypeFakeDeath)
	})
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	srv.Advance(t, time.Second)
	if !victim.FakeDead() || !victim.StandingNow() {
		t.Fatalf("victim FakeDead=%v StandingNow=%v, want still getting up", victim.FakeDead(), victim.StandingNow())
	}

	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() {
		if !victim.Kill(attacker) {
			t.Error("Kill() = false on a living player")
		}
	})
	assertFramesOnly(t, vc, c, victim.ObjectID(), []fakeDeathFrame{frameDie, frameStopFake, frameRevive})
}
