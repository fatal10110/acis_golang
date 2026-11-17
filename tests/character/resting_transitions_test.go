package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootVitalForce enters the world as a level 1 player knowing Vital Force
// and returns the server, the character and its standing regeneration.
func bootVitalForce(t *testing.T) (*gameservertest.Server, *player.Character, regenRates) {
	t.Helper()
	srv := gameservertest.Boot(t,
		regenTermsTemplate(),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(vitalForceSkills(t)),
	)
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, vitalForceID, 1); err != nil {
		t.Fatalf("grant Vital Force: %v", err)
	}
	enterWorld(t, srv.Client)
	drainQuiet(t, srv.Client)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player not in the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	if !c.HasSkill(vitalForceID) {
		t.Fatal("Vital Force not known")
	}
	return srv, c, readRegenRates(c)
}

// wantVitalForce fails unless got carries Vital Force's resting bonus on top
// of the seated posture's regeneration (1.5 against the standing 1.1).
func wantVitalForce(t *testing.T, when string, got, standing regenRates) {
	t.Helper()
	noBonus := standing.scaled(1.5 / 1.1)
	if got.hp-noBonus.hp < 1.9 || got.mp-noBonus.mp < 0.9 {
		t.Fatalf("%s: regen %+v, want Vital Force's bonus on top of %+v", when, got, noBonus)
	}
}

// TestRestingConditionEndsAtTheStandUpStart pins the other end of
// ConditionPlayerState RESTING: standUp clears isSitting at once rather than
// when its 2.5 s transition ends (Player.java:1573-1577), so Vital Force's
// bonus is gone from the moment the player starts to stand.
func TestRestingConditionEndsAtTheStandUpStart(t *testing.T) {
	t.Parallel()
	srv, c, still := bootVitalForce(t)
	objID := srv.SoleObjectID(t)

	srv.Client.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, srv.Client, serverpackets.OpcodeChangeWaitType, "sit")
	srv.SettlePosture(t, objID)
	wantVitalForce(t, "seated", readRegenRates(c), still)

	srv.Client.Send(encodeRequestChangeWaitType(true))
	mustReadOpcode(t, srv.Client, serverpackets.OpcodeChangeWaitType, "stand")
	if !c.StandingNow() {
		t.Fatal("the stand-up transition has not started")
	}
	if got := readRegenRates(c); got.hp >= still.hp*(1.5/1.1) {
		t.Fatalf("regen while standing up = %+v, want no resting bonus (standing %+v)", got, still)
	}

	srv.SettlePosture(t, objID)
	if got := readRegenRates(c); !got.near(still) {
		t.Fatalf("regen standing = %+v, want %+v", got, still)
	}
}

// TestRestingConditionWaitsForTheLieDownToEnd pins startFakeDeath
// (Player.java:7017-7033): isSitting turns on only when its 3 s transition
// ends, like a sit-down, so Vital Force's bonus waits for it and then holds
// while the player feigns death.
func TestRestingConditionWaitsForTheLieDownToEnd(t *testing.T) {
	t.Parallel()
	srv, c, still := bootVitalForce(t)

	if !c.StartFakeDeath() {
		t.Fatal("StartFakeDeath reported no change")
	}
	mustReadOpcode(t, srv.Client, serverpackets.OpcodeChangeWaitType, "fake death")
	if got := readRegenRates(c); got.hp >= still.hp*(1.5/1.1) {
		t.Fatalf("regen while lying down = %+v, want no resting bonus (standing %+v)", got, still)
	}

	srv.AdvanceUntil(t, "lie-down end", func() bool { return !c.SittingNow() })
	wantVitalForce(t, "lying down", readRegenRates(c), still)
}
