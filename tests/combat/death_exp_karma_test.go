package combat

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// deathLossTable gives level 5 a 1000→3000 experience band with the
// reference-shaped penalty parameters: ExpLossAtDeath 10%, KarmaModifier 2.
func deathLossTable(t *testing.T) *player.LevelTable {
	t.Helper()
	table, err := player.NewLevelTable(map[int]player.Level{
		1: {RequiredExpToLevelUp: 0},
		2: {RequiredExpToLevelUp: 1},
		3: {RequiredExpToLevelUp: 2},
		4: {RequiredExpToLevelUp: 3},
		5: {RequiredExpToLevelUp: 1000, KarmaModifier: 2, ExpLossAtDeath: 10},
		6: {RequiredExpToLevelUp: 3000},
		7: {RequiredExpToLevelUp: 1_000_000_000},
	})
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	return table
}

// seedExperiencedCharacter creates the account's single selectable character
// at a known experience position inside the penalty band, so the loss a death
// applies is computable exactly.
func seedExperiencedCharacter(exp int64, karma int) func(*gamesql.CharacterStore, *gamesql.ItemStore) {
	return func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		ch, err := player.NewCharacter(4242, gameservertest.ClassTemplate(), "player1", "Victim", 1, 0, 0, player.SexMale)
		if err != nil {
			panic(err)
		}
		ch.CharLevel = 5
		ch.Exp = exp
		ch.KarmaPoints = karma
		ctx := context.Background()
		if err := chars.Create(ctx, ch); err != nil {
			panic(err)
		}
		if err := chars.Save(ctx, ch.SaveState()); err != nil {
			panic(err)
		}
	}
}

// killPrimaryClient has the killer client force-cast the one-shot PDAM skill
// into the primary client's character and waits for the death to register.
func killPrimaryClient(t *testing.T, srv *gameservertest.Server, killer *scriptedClient, killerID, victimID int32) {
	t.Helper()
	selectPlayerTarget(t, killer, victimID)
	// An innocent victim is only attackable with force (ctrl), matching the
	// reference's isAttackableWithoutForce gate.
	castKillSkill(t, srv, killer, killerID, victimID, true)

	obj, ok := srv.State.Player(victimID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	dead, ok := obj.(interface{ Dead() bool })
	if !ok {
		t.Fatalf("world victim %T does not expose Dead()", obj)
	}
	srv.AdvanceUntil(t, "victim death", dead.Dead)
	// The dead flag flips at the start of the death sequence; the rest of
	// it (effect cleanup, penalties) finishes on an actor queue.
	srv.Settle(t)
}

// assertSilentExpLoss reads the victim's frames until they go quiet and
// checks the experience loss told the client the way the reference does: a
// death's loss is a negative experience add (Player.applyDeathPenalty,
// Player.java:2925), so it sends UserInfo (PlayerStatus.addExp,
// PlayerStatus.java:478-485) and no EXP_DECREASED_BY_S1 — only
// PlayerStatus.removeExpAndSp (PlayerStatus.java:583-603) sends that, and a
// death never calls it.
func assertSilentExpLoss(t *testing.T, c *scriptedClient) {
	t.Helper()
	frames := readQuiet(c)
	if lost := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageExpDecreasedByS1); lost >= 0 {
		t.Fatalf("death sent EXP_DECREASED_BY_S1 at frame %d; the reference sends none", lost)
	}
	if indexOf(frames, 0, serverpackets.OpcodeUserInfo, -1) < 0 {
		t.Fatal("death's experience loss sent no UserInfo")
	}
}

// TestDeathCostsConfiguredExperience walks the exp half of the death
// penalty: a karma-free victim dies to a forced cast and loses exactly
// round(span * ExpLossAtDeath / 100) — 200 of its 1500 — told to the client
// through UserInfo alone and persisted at logout.
func TestDeathCostsConfiguredExperience(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedExperiencedCharacter(1500, 0)),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithLevels(deathLossTable(t)),
		gameservertest.WithAllowDelevel(true),
	)
	c := srv.Client
	victimID := srv.SoleObjectID(t)
	startInWorld(t, c)

	killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 5, 0)
	seedKnownSkill(t, srv, killerChar.ID, 42, 1)
	killer := srv.DialClient(t, "killer", 1)
	startInWorld(t, killer)
	drainUntilQuiet(t, killer)
	drainUntilQuiet(t, c)

	killPrimaryClient(t, srv, killer, killerChar.ID, victimID)

	assertSilentExpLoss(t, c)

	// Logout persists the character; the reduced total must survive.
	logoutPersisted(t, srv, c)
	if ch, err := srv.Chars.Get(context.Background(), victimID); err != nil || ch.Exp != 1300 || ch.CharLevel != 5 {
		t.Fatalf("persisted victim = %+v, %v; want exp 1300 at level 5", ch, err)
	}
}

// TestKarmaDeathLosesKarma pins the karma half of the same flow: a
// karma-positive victim's loss percentage is scaled by RateKarmaExpLost
// (400 instead of 200) and its karma drops by floor(lostExp / modifier / 15).
func TestKarmaDeathLosesKarma(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedExperiencedCharacter(1500, 240)),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithLevels(deathLossTable(t)),
		gameservertest.WithAllowDelevel(true),
		gameservertest.WithRateKarmaExpLost(2.0),
	)
	c := srv.Client
	victimID := srv.SoleObjectID(t)
	startInWorld(t, c)

	killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 5, 0)
	seedKnownSkill(t, srv, killerChar.ID, 42, 1)
	killer := srv.DialClient(t, "killer", 1)
	startInWorld(t, killer)
	drainUntilQuiet(t, killer)
	drainUntilQuiet(t, c)

	killPrimaryClient(t, srv, killer, killerChar.ID, victimID)

	// The dying client learns its new karma total (the karma update precedes
	// the exp loss); the loss itself only refreshes UserInfo.
	assertKarmaChangeFrames(t, c, victimID, 227)
	assertSilentExpLoss(t, c)

	logoutPersisted(t, srv, c)
	if ch, err := srv.Chars.Get(context.Background(), victimID); err != nil || ch.Exp != 1100 || ch.KarmaPoints != 227 {
		t.Fatalf("persisted victim = %+v, %v; want exp 1100 and karma 227", ch, err)
	}
}
