package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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

// TestHitWhileSeatedInFakeDeathGetUpGetsUpOnce lands a hit on a player
// whose Fake Death ended during the lie-down: the lie-down seated it while
// the get-up still runs, so it plays dead with no effect left, and the
// stand intention gets it up once.
func TestHitWhileSeatedInFakeDeathGetUpGetsUpOnce(t *testing.T) {
	t.Parallel()
	srv, c, vc, attacker, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	seatWhileGettingUp(t, srv, victim)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	landHit(t, srv, attacker, victim)
	assertFramesOnly(t, vc, c, id, []fakeDeathFrame{frameStopFake, frameRevive})
	// The earlier get-up, not the hit's, ends fake death, which may already
	// have happened by now.
	if victim.Dead() || !victim.Standing() {
		t.Fatalf("victim Dead=%v Standing=%v, want alive and standing", victim.Dead(), victim.Standing())
	}
}

// TestHitWhileSeatedInFakeDeathGetUpDropsQueuedFollow has a player seated
// while still getting up out of fake death queue a follow behind the get-up
// (PlayableAI.tryToFollow, PlayableAI.java:344-348), then takes a hit: the
// stand intention's prepareIntention (AbstractAI.java:130-136) cancels the
// follow and clears the queued intention on its fake-death branch too, so
// once every get-up has ended the player is not following.
func TestHitWhileSeatedInFakeDeathGetUpDropsQueuedFollow(t *testing.T) {
	t.Parallel()
	srv, _, vc, attacker, iv := bootPvPPair(t)
	victim := onlineVictim(t, srv, iv.ObjectID())
	id := victim.ObjectID()
	selectPlayerTarget(t, vc, attacker.ObjectID())
	seatWhileGettingUp(t, srv, victim)
	drainUntilQuiet(t, vc)

	vc.Send(encodeAction(attacker.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	for i := 0; ; i++ {
		f := mustRead(t, vc, "follow click answer")
		if f[0] == serverpackets.OpcodeActionFailed {
			break
		}
		if i == 4 {
			t.Fatal("follow click during the get-up got no ActionFailed; the follow is not queued")
		}
	}
	landHit(t, srv, attacker, victim)
	srv.AdvanceUntil(t, "every get-up ended", func() bool { return !victim.FakeDead() && !victim.StandingNow() })
	srv.Settle(t)
	if srv.PlayerMove(t, id).Following() {
		t.Fatal("the follow queued behind the get-up survived the hit's stand")
	}
}

// seatWhileGettingUp ends victim's Fake Death during its lie-down, a third
// of a get-up before the lie-down ends: the lie-down then seats victim while
// the get-up still runs for two thirds of its length. victim plays dead,
// seated, with no effect left. The margins leave room for the wall clock.
func seatWhileGettingUp(t *testing.T, srv *gameservertest.Server, victim *player.Character) {
	t.Helper()
	lieDown, getUp := victimFakeDeathDelays(t, srv, victim)
	startFakeDeath(t, srv, victim, false)
	srv.Advance(t, lieDown-getUp/3)
	if !victim.SittingNow() {
		t.Fatal("the lie-down had already ended; the scenario proves nothing")
	}
	onQueue(t, srv.PlayerQueue(t, victim.ObjectID()), func() { victim.EffectList().StopByType(effect.TypeFakeDeath) })
	srv.AdvanceUntil(t, "lie-down ended", victim.Seated)
	if !victim.FakeDead() || !victim.StandingNow() {
		t.Fatalf("seated victim FakeDead=%v StandingNow=%v, want still getting up", victim.FakeDead(), victim.StandingNow())
	}
}

// landHit lands a damaging hit from attacker on victim on the attacker's
// queue, as a landed swing does: the victim is notified, then takes the
// damage. Unlike a swing it takes no time, so it lands inside a short
// posture window on the wall clock too.
func landHit(t *testing.T, srv *gameservertest.Server, attacker attackable.Combatant, victim *player.Character) {
	t.Helper()
	id := victim.ObjectID()
	before := srv.PlayerCurrentHP(t, id) + srv.PlayerCurrentCP(t, id)
	onQueue(t, srv.PlayerQueue(t, attacker.ObjectID()), func() {
		victim.NotifyAttacked(attacker)
		victim.TakeDamage(10, attacker)
	})
	srv.Settle(t)
	if after := srv.PlayerCurrentHP(t, id) + srv.PlayerCurrentCP(t, id); after >= before {
		t.Fatalf("victim HP+CP %d after the hit, want below %d", after, before)
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
