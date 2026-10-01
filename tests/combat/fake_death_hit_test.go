package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference for a hit or offensive skill reaching a player seated in fake
// death:
//   - CreatureAttack.doHit notifies the target ATTACKED (CreatureAttack.java:240)
//     before reduceCurrentHp (:263); CreatureCast.callSkill notifies each
//     target of an offensive skill (CreatureCast.java:488) before its
//     handler deals the damage;
//   - PlayerAI.onEvtAttacked (PlayerAI.java:148-158) runs the stand
//     intention on a sitting player, and thinkStand (PlayerAI.java:490-504)
//     calls stopFakeDeath(true) on one playing dead: with the Fake Death
//     effect still on, its stopEffects(FAKE_DEATH) exits the effect, whose
//     EffectFakeDeath.onExit (EffectFakeDeath.java:33-37) nests one get-up
//     before the stand's own; with the effect gone, one get-up goes out;
//   - an invulnerable target still runs the stand intention: only its
//     damage is dropped (PlayerStatus.reduceHp returns on isInvul);
//   - the damage then finds the player standing, so PlayerStatus.reduceHp
//     (PlayerStatus.java:124-125) sends no ChangeWaitType(WT_STANDING).

// fakeDeathHitSkillID is a damaging offensive skill too weak to kill.
const fakeDeathHitSkillID = 43

// TestHitWhilePlayingDeadGetsUpTwice lands a swing on a player lying in
// fake death: its stand intention, ahead of the damage, gets it up twice
// and no stand-up follows. It is left standing, still playing dead through
// the get-up, with its Fake Death gone.
func TestHitWhilePlayingDeadGetsUpTwice(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	lieInFakeDeath(t, srv, victim, false)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	before := srv.PlayerCurrentHP(t, id)

	attackPlayer(t, c, id)
	srv.AdvanceUntil(t, "first hit on the fake-dead victim", func() bool { return srv.PlayerCurrentHP(t, id) < before })
	assertFramesOnly(t, vc, c, id, []fakeDeathFrame{frameStopFake, frameRevive, frameStopFake, frameRevive})
	assertGettingUpFromFakeDeath(t, victim)
}

// TestInvulnerableHitWhilePlayingDeadGetsUpTwice lands a swing on an
// invulnerable player lying in fake death: the damage is dropped, but the
// stand intention still gets it up twice.
func TestInvulnerableHitWhilePlayingDeadGetsUpTwice(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	lieInFakeDeath(t, srv, victim, false)
	victim.SetInvul(true)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	before := srv.PlayerCurrentHP(t, id)

	attackPlayer(t, c, id)
	srv.AdvanceUntil(t, "hit on the invulnerable victim", victim.Standing)
	assertFramesOnly(t, vc, c, id, []fakeDeathFrame{frameStopFake, frameRevive, frameStopFake, frameRevive})
	assertGettingUpFromFakeDeath(t, victim)
	if got := srv.PlayerCurrentHP(t, id); got != before {
		t.Fatalf("invulnerable victim HP = %d, want unchanged %d", got, before)
	}
}

// TestHitWhileSeatedInFakeDeathGetUpGetsUpOnce lands a swing on a player
// whose Fake Death ended during the lie-down: the lie-down seated it while
// the get-up still runs, so it plays dead with no effect left, and the
// stand intention gets it up once.
func TestHitWhileSeatedInFakeDeathGetUpGetsUpOnce(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	lieDown, _ := victimFakeDeathDelays(t, srv, victim)
	startFakeDeath(t, srv, victim, false)
	// Ended here, just before the lie-down ends, the get-up outlasts the
	// lie-down by nearly its whole length, room for the swing to land.
	srv.Advance(t, lieDown-50*time.Millisecond)
	if !victim.SittingNow() {
		t.Fatal("the lie-down had already ended; the scenario proves nothing")
	}
	onQueue(t, srv.PlayerQueue(t, id), func() { victim.EffectList().StopByType(effect.TypeFakeDeath) })
	srv.AdvanceUntil(t, "lie-down ended", victim.Seated)
	if !victim.FakeDead() || !victim.StandingNow() {
		t.Fatalf("seated victim FakeDead=%v StandingNow=%v, want still getting up", victim.FakeDead(), victim.StandingNow())
	}
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	before := srv.PlayerCurrentHP(t, id)

	attackPlayer(t, c, id)
	srv.AdvanceUntil(t, "hit on the seated victim", func() bool { return srv.PlayerCurrentHP(t, id) < before })
	assertFramesOnly(t, vc, c, id, []fakeDeathFrame{frameStopFake, frameRevive})
	// The earlier get-up, not the hit's, ends fake death, which may already
	// have happened by now.
	if victim.Dead() || !victim.Standing() {
		t.Fatalf("victim Dead=%v Standing=%v, want alive and standing", victim.Dead(), victim.Standing())
	}
}

// TestOffensiveSkillWhilePlayingDeadGetsUpTwice casts a damaging offensive
// skill on a player lying in fake death: the skill notifies its target
// before dealing damage, so the stand intention gets it up twice and the
// damage stands nobody up.
func TestOffensiveSkillWhilePlayingDeadGetsUpTwice(t *testing.T) {
	t.Parallel()
	defs := []modelskill.Definition{{
		ID: fakeDeathHitSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "PDAM", Power: 1, Offensive: true,
	}}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Caster", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, defs)),
	)
	c, casterID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, casterID, fakeDeathHitSkillID, 1)
	id := seedPlayer(t, srv, "victim", "Victim", 40, 500)
	vc := srv.DialClient(t, "victim", 1)
	startInWorld(t, vc)
	startInWorld(t, c)
	victim := onlineVictim(t, srv, id)
	selectPlayerTarget(t, c, id)
	lieInFakeDeath(t, srv, victim, false)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	before := srv.PlayerCurrentHP(t, id)

	c.Send(encodeRequestMagicSkillUse(fakeDeathHitSkillID, false, false))
	srv.AdvanceUntil(t, "skill damage on the fake-dead victim", func() bool { return srv.PlayerCurrentHP(t, id) < before })
	assertFramesOnly(t, vc, c, id, []fakeDeathFrame{frameStopFake, frameRevive, frameStopFake, frameRevive})
	assertGettingUpFromFakeDeath(t, victim)
}

// assertGettingUpFromFakeDeath checks victim is alive and standing, getting
// up out of fake death with its Fake Death effect gone.
func assertGettingUpFromFakeDeath(t *testing.T, victim *player.Character) {
	t.Helper()
	if victim.Dead() || !victim.Standing() || !victim.StandingNow() || !victim.FakeDead() {
		t.Fatalf("victim Dead=%v Standing=%v StandingNow=%v FakeDead=%v, want alive, standing, getting up out of fake death",
			victim.Dead(), victim.Standing(), victim.StandingNow(), victim.FakeDead())
	}
	if victim.EffectList().IsAffected(effect.FlagFakeDeath) {
		t.Fatal("Fake Death effect outlived the get-up")
	}
}
