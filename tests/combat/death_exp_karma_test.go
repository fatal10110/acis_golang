package combat

import (
	"context"
	"testing"
	"time"

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
		if err := chars.Save(ctx, ch); err != nil {
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
	killer.Send(encodeRequestMagicSkillUse(42, true, false))
	readCastStartFrames(t, killer, killerID, 42, 1, 500, 60_000, victimID)

	obj, ok := srv.State.Player(victimID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	dead, ok := obj.(interface{ Dead() bool })
	if !ok {
		t.Fatalf("world victim %T does not expose Dead()", obj)
	}
	waitFor(t, "victim death", dead.Dead)
}

// readExpLossMessage scans the victim's frames until the experience-loss
// announcement arrives and asserts its amount.
func readExpLossMessage(t *testing.T, c *scriptedClient, wantLost int64) {
	t.Helper()
	for i := 0; i < 50; i++ {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatalf("experience-loss message for %d never arrived", wantLost)
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		if id := r.ReadInt32(); id != int32(serverpackets.SystemMessageExpDecreasedByS1) {
			continue
		}
		if params := r.ReadInt32(); params != 1 {
			t.Fatalf("exp-loss message params = %d, want 1", params)
		}
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
			t.Fatalf("exp-loss message param type = %d, want number", typ)
		}
		if got := r.ReadInt32(); got != int32(wantLost) {
			t.Fatalf("exp-loss message amount = %d, want %d", got, wantLost)
		}
		return
	}
	t.Fatal("experience-loss message not found within 50 frames")
}

// TestDeathCostsConfiguredExperience walks the exp half of the death
// penalty: a karma-free victim dies to a forced cast and loses exactly
// round(span * ExpLossAtDeath / 100) — 200 of its 1500 — announced on the
// wire and persisted at logout.
func TestDeathCostsConfiguredExperience(t *testing.T) {
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

	readExpLossMessage(t, c, 200)
	drainUntilQuiet(t, c)

	// Logout persists the character; the reduced total must survive.
	c.Send(encodeLogout())
	waitFor(t, "persisted exp loss", func() bool {
		ch, err := srv.Chars.Get(context.Background(), victimID)
		return err == nil && ch.Exp == 1300 && ch.CharLevel == 5
	})
}

// TestKarmaDeathLosesKarma pins the karma half of the same flow: a
// karma-positive victim's loss percentage is scaled by RateKarmaExpLost
// (400 instead of 200) and its karma drops by floor(lostExp / modifier / 15).
func TestKarmaDeathLosesKarma(t *testing.T) {
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

	// The dying client learns both costs: its new karma total first (the
	// karma update precedes the exp removal), then the exp loss.
	assertKarmaChangeFrames(t, c, victimID, 227)
	readExpLossMessage(t, c, 400)
	drainUntilQuiet(t, c)

	c.Send(encodeLogout())
	waitFor(t, "persisted karma loss", func() bool {
		ch, err := srv.Chars.Get(context.Background(), victimID)
		return err == nil && ch.Exp == 1100 && ch.KarmaPoints == 227
	})
}
