package combat

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"

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
	srv.AdvanceUntil(t, "fake-death lie-down ended", victim.Seated)
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
// want, and that the victim ends standing and no longer playing dead.
func assertDeathFrames(t *testing.T, self, observer *scriptedClient, victim *player.Character, want []fakeDeathFrame) {
	t.Helper()
	id := victim.ObjectID()
	for _, rc := range []struct {
		who string
		c   *scriptedClient
	}{{"self", self}, {"observer", observer}} {
		if got := postureLifeFrames(readQuiet(rc.c), id); !slices.Equal(got, want) {
			t.Errorf("%s frames = %v, want %v", rc.who, got, want)
		}
	}
	if !victim.Dead() || !victim.Standing() || victim.FakeDead() {
		t.Fatalf("victim Dead=%v Standing=%v FakeDead=%v, want dead, standing, not playing dead",
			victim.Dead(), victim.Standing(), victim.FakeDead())
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
