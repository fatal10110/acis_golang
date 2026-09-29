package combat

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: CreatureAttack.onHitTimer stops only on a target that is
// unknown or isDead() (CreatureAttack.java:115); PlayerStatus.reduceHp
// returns only on isDead() (PlayerStatus.java:103) and stands the player up
// only on isSitting() && !isInStoreMode() (PlayerStatus.java:124-125), where
// isSitting() turns true when the sit-down or fake-death lie-down ends
// (Player.sitDown / startFakeDeath, Player.java:1542-1552, 7017-7033);
// AttackableAttack.canAttack refuses a fake-dead target
// (AttackableAttack.java:18-25).

// intakePlayer is the posture and vitals surface these scenarios drive on a
// live player.
type intakePlayer interface {
	attackable.Combatant
	StartFakeDeath() bool
	FakeDead() bool
	SittingNow() bool
	Seated() bool
	Standing() bool
	SetHP(float64)
	SetCP(float64)
	SetRollSource(func(int) int)
}

func intakePlayerOf(t *testing.T, srv *gameservertest.Server, id int32) intakePlayer {
	t.Helper()
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d not in world", id)
	}
	return obj.(intakePlayer)
}

// bootPvPPair boots a level-5 attacker whose every swing lands without a
// critical, and a level-40 victim at full HP and CP carrying karma, which the attacker's auto-attack keeps
// going against without force.
func bootPvPPair(t *testing.T) (srv *gameservertest.Server, c, vc *scriptedClient, attacker, victim intakePlayer) {
	t.Helper()
	srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Attacker", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c = srv.Client
	attackerID := srv.SoleObjectID(t)
	victimID := srv.SeedCharacterFor(t, "victim", "Victim", 40, 0).ID
	ch, err := srv.Chars.Get(context.Background(), victimID)
	if err != nil {
		t.Fatalf("load victim: %v", err)
	}
	ch.KarmaPoints = 500
	if err := srv.Chars.Save(context.Background(), ch.SaveState()); err != nil {
		t.Fatalf("save victim karma: %v", err)
	}
	vc = srv.DialClient(t, "victim", 1)
	startInWorld(t, vc)
	startInWorld(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	attacker = intakePlayerOf(t, srv, attackerID)
	victim = intakePlayerOf(t, srv, victimID)
	onQueue(t, srv.PlayerQueue(t, attackerID), func() { attacker.SetRollSource(landNoCrit()) })
	maxHP, maxCP := srv.PlayerMaxHP(t, victimID), srv.PlayerMaxCP(t, victimID)
	onQueue(t, srv.PlayerQueue(t, victimID), func() {
		victim.SetHP(float64(maxHP))
		victim.SetCP(float64(maxCP))
	})
	return srv, c, vc, attacker, victim
}

// attackPlayer targets victim and starts an auto-attack on it.
func attackPlayer(t *testing.T, c *scriptedClient, victimID int32) {
	t.Helper()
	selectPlayerTarget(t, c, victimID)
	c.Send(encodeAttackRequest(victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
}

// TestPlayerHitsLandOnFakeDeadPlayer has a player auto-attack another player
// lying in fake death: each hit lands and costs HP, and the auto-attack goes
// on swing after swing while the victim still plays dead.
func TestPlayerHitsLandOnFakeDeadPlayer(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, victim := bootPvPPair(t)
	victimID := victim.ObjectID()
	onQueue(t, srv.PlayerQueue(t, victimID), func() { victim.StartFakeDeath() })
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	before := srv.PlayerCurrentHP(t, victimID)

	attackPlayer(t, c, victimID)
	srv.AdvanceUntil(t, "first hit on the fake-dead victim", func() bool {
		return srv.PlayerCurrentHP(t, victimID) < before
	})
	first := srv.PlayerCurrentHP(t, victimID)
	srv.AdvanceUntil(t, "a further swing on the fake-dead victim", func() bool {
		return srv.PlayerCurrentHP(t, victimID) < first
	})
	if !victim.FakeDead() || victim.Dead() {
		t.Fatalf("victim FakeDead=%v Dead=%v after the hits, want still playing dead", victim.FakeDead(), victim.Dead())
	}
}

// TestPlayerAttackStopsOnDeadPlayer kills the victim with the first hit:
// the auto-attack stops there, and no further swing goes out.
func TestPlayerAttackStopsOnDeadPlayer(t *testing.T) {
	t.Parallel()
	srv, c, _, attacker, victim := bootPvPPair(t)
	victimID := victim.ObjectID()
	onQueue(t, srv.PlayerQueue(t, victimID), func() {
		victim.SetCP(0)
		victim.SetHP(1)
	})

	attackPlayer(t, c, victimID)
	srv.AdvanceUntil(t, "victim killed", func() bool { return srv.PlayerDead(t, victimID) })
	srv.Advance(t, 5*time.Second)
	swings := 0
	for _, frame := range readQuiet(c) {
		if frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == attacker.ObjectID() {
			swings++
		}
	}
	if swings != 1 {
		t.Fatalf("attacker swings = %d, want the killing swing alone", swings)
	}
}

// TestHostileRefusesToAttackFakeDeadPlayer: a monster starts no swing at a
// player in fake death, and does at the same player standing.
func TestHostileRefusesToAttackFakeDeadPlayer(t *testing.T) {
	t.Parallel()
	srv, c, player, hostile := bootHostilePair(t)
	if !hostile.CanAttack(player) {
		t.Fatal("monster refuses to attack a standing player, want the control swing allowed")
	}
	onQueue(t, srv.PlayerQueue(t, player.ObjectID()), func() { player.StartFakeDeath() })
	drainUntilQuiet(t, c)
	if hostile.CanAttack(player) {
		t.Fatal("monster may attack a fake-dead player, want it refused")
	}
}

// bootHostilePair boots a level-5 player next to a monster that swings only
// when told to.
func bootHostilePair(t *testing.T) (*gameservertest.Server, *scriptedClient, intakePlayer, *gameservertest.AttackingHostile) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	player := intakePlayerOf(t, srv, srv.SoleObjectID(t))
	// Within melee range: the parked monster never closes distance itself.
	tmpl := gameservertest.AttackingHostileTemplate()
	tmpl.BaseAttackRange = 40
	hostile := srv.SpawnAttackingHostileNPCTemplate(t, tmpl, location.Location{X: playerOrigin.X + 20, Y: playerOrigin.Y, Z: playerOrigin.Z})
	drainUntilQuiet(t, c)
	return srv, c, player, hostile
}

// standingBroadcast reports whether frames carry player id's
// ChangeWaitType(STANDING).
func standingBroadcast(frames [][]byte, id int32) bool {
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeChangeWaitType &&
			int32(binary.LittleEndian.Uint32(frame[1:5])) == id &&
			serverpackets.WaitType(binary.LittleEndian.Uint32(frame[5:9])) == serverpackets.WaitStanding {
			return true
		}
	}
	return false
}

// hitMidTransition lands one monster hit on player while it is still going
// down, and asserts the hit left it going down with no standing broadcast.
func hitMidTransition(t *testing.T, srv *gameservertest.Server, c *scriptedClient, player intakePlayer, hostile *gameservertest.AttackingHostile) {
	t.Helper()
	before := srv.PlayerCurrentHP(t, player.ObjectID())
	hostile.DoAttack(t, player)
	if srv.PlayerCurrentHP(t, player.ObjectID()) >= before {
		t.Fatal("monster hit did not land")
	}
	if player.Standing() || standingBroadcast(readQuiet(c), player.ObjectID()) {
		t.Fatal("a hit while going down stood the player up, want the transition left running")
	}
	if !player.SittingNow() {
		t.Fatal("going down had already ended when the hit landed; the scenario proves nothing")
	}
}

// TestDamageStandsUpOnlySeatedPlayer lands monster hits on a sitting
// player. A hit during the 2.5s sit-down leaves it running; the sit-down
// ends on time. A hit once seated stands the player up, unless it runs a
// shop. A hit during the fake-death lie-down leaves it running too.
func TestDamageStandsUpOnlySeatedPlayer(t *testing.T) {
	t.Parallel()
	t.Run("sit-down, then seated", func(t *testing.T) {
		t.Parallel()
		srv, c, player, hostile := bootHostilePair(t)
		c.Send(encodeRequestChangeWaitType(false))
		srv.AdvanceUntil(t, "sit-down started", func() bool { return player.SittingNow() })
		hitMidTransition(t, srv, c, player, hostile)
		srv.AdvanceUntil(t, "sit-down ended", func() bool { return player.Seated() })
		drainUntilQuiet(t, c)

		hostile.DoAttack(t, player)
		if !player.Standing() || !standingBroadcast(readQuiet(c), player.ObjectID()) {
			t.Fatal("a hit on the seated player did not stand it up")
		}
	})
	t.Run("seated in a shop", func(t *testing.T) {
		t.Parallel()
		srv, c, player, hostile := bootHostilePair(t)
		c.Send(encodeRequestChangeWaitType(false))
		srv.AdvanceUntil(t, "sit-down ended", func() bool { return player.Seated() })
		srv.SetPlayerOperating(t, player.ObjectID(), true)
		drainUntilQuiet(t, c)

		hostile.DoAttack(t, player)
		if !player.Seated() || standingBroadcast(readQuiet(c), player.ObjectID()) {
			t.Fatal("a hit stood up a player running a shop, want it left seated")
		}
	})
	t.Run("fake-death lie-down", func(t *testing.T) {
		t.Parallel()
		srv, c, player, hostile := bootHostilePair(t)
		onQueue(t, srv.PlayerQueue(t, player.ObjectID()), func() { player.StartFakeDeath() })
		hitMidTransition(t, srv, c, player, hostile)
		if !player.FakeDead() {
			t.Fatal("a hit during the lie-down ended fake death")
		}
	})
}
