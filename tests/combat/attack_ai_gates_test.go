package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// wyvernNPCID is the wyvern mount; riding it makes the player fly.
const wyvernNPCID = 12621

// onlinePlayer resolves the online player objID to its character.
func onlinePlayer(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an online character", objID, obj)
	}
	return ch
}

// attackFrameBy reports whether frame is an Attack whose attacker is id.
func attackFrameBy(frame []byte, id int32) bool {
	return frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == id
}

// advanceUntilActionFailed lets time pass until an ActionFailed reaches c,
// failing on any Attack frame by attackerID before it.
func advanceUntilActionFailed(t *testing.T, srv *gameservertest.Server, c *scriptedClient, attackerID int32, what string) {
	t.Helper()
	for range 60 {
		for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
			if attackFrameBy(frame, attackerID) {
				t.Fatalf("%s: Attack by %d before ActionFailed", what, attackerID)
			}
			if frame[0] == serverpackets.OpcodeActionFailed {
				return
			}
		}
		srv.Advance(t, 100*time.Millisecond)
	}
	t.Fatalf("%s: no ActionFailed", what)
}

// assertNoAttackBy lets d pass and fails on any Attack frame by attackerID.
func assertNoAttackBy(t *testing.T, srv *gameservertest.Server, c *scriptedClient, attackerID int32, d time.Duration, what string) {
	t.Helper()
	for passed := time.Duration(0); passed < d; passed += 100 * time.Millisecond {
		srv.Advance(t, 100*time.Millisecond)
		for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
			if attackFrameBy(frame, attackerID) {
				t.Fatalf("%s: Attack by %d", what, attackerID)
			}
		}
	}
}

// TestWyvernRiderApproachesThenFailsAttackInRange pins the attack think's
// gate order for a flying player: flying does not deny the AI action, so the
// rider first walks into range (MoveToPawn) and only then fails the attack
// itself, answering ActionFailed with no swing.
func TestWyvernRiderApproachesThenFailsAttackInRange(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 150, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	rider := onlinePlayer(t, srv, objID)
	if !rider.Mount(wyvernNPCID, 1) || !rider.Flying() {
		t.Fatal("wyvern mount did not make the rider fly")
	}

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "MoveToPawn"), serverpackets.OpcodeMoveToPawn, "rider approach")

	advanceUntilActionFailed(t, srv, c, objID, "rider arrival")
	assertNoAttackBy(t, srv, c, objID, 1500*time.Millisecond, "after rider ActionFailed")
	if got := hostile.CurrentHP(); got != hostile.MaxHP() {
		t.Fatalf("hostile HP = %d after a wyvern rider's attack, want untouched %d", got, hostile.MaxHP())
	}
}

// TestTeleportingPlayerDropsAttackIntention pins the AI-action gate on a
// re-think: a player whose attack intention is thought again while
// teleporting (here, when the in-flight swing ends) drops it with
// ActionFailed and swings no more.
func TestTeleportingPlayerDropsAttackIntention(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, objID)
	assertAttackBy(t, c, objID)

	if !onlinePlayer(t, srv, objID).SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}
	advanceUntilActionFailed(t, srv, c, objID, "teleporting re-think")
	hp := hostile.CurrentHP()
	assertNoAttackBy(t, srv, c, objID, 2*time.Second, "after teleporting ActionFailed")
	if got := hostile.CurrentHP(); got != hp {
		t.Fatalf("hostile HP = %d after the intention dropped, want frozen at %d", got, hp)
	}
}

// TestBoxChestNeverSwingsBack pins the core-AI-disabled attack gate: a
// treasure-box chest with hate on a player never opens a physical attack,
// while a mimic chest (a Chest id outside the box range) does.
func TestBoxChestNeverSwingsBack(t *testing.T) {
	for _, tt := range []struct {
		name  string
		id    int
		swing bool
	}{
		{"box", 18265, false},
		{"mimic", 21671, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)

			tmpl := gameservertest.MovingHostileTemplate("Chest")
			tmpl.ID, tmpl.TemplateID = tt.id, tt.id
			home := location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ}
			chest := srv.SpawnMovingHostileNPCTemplate(t, tmpl, home, home)
			drainUntilQuiet(t, c)

			obj, _ := srv.State.Player(objID)
			victim, ok := obj.(attackable.Combatant)
			if !ok {
				t.Fatalf("world.Player(%d) = %T is not a combatant", objID, obj)
			}
			chest.AddCombatDamageHate(victim, 50)

			if !tt.swing {
				assertNoAttackBy(t, srv, c, chest.ObjectID(), 3*time.Second, "box chest with hate")
				return
			}
			for range 30 {
				srv.Advance(t, 100*time.Millisecond)
				for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
					if attackFrameBy(frame, chest.ObjectID()) {
						return
					}
				}
			}
			t.Fatal("mimic chest with hate never swung")
		})
	}
}

// TestQueuedAttackIdledAtCastEndAnswersActionFailed pins the cast-finished
// resume of an attack queued mid-cast: when the caster is teleporting, or
// the target has left the world, by the time the cast ends, the resumed
// think drops the attack and answers exactly one ActionFailed, with no
// swing.
func TestQueuedAttackIdledAtCastEndAnswersActionFailed(t *testing.T) {
	for _, tt := range []struct {
		name string
		idle func(t *testing.T, srv *gameservertest.Server, pc *player.Character, hostileID int32)
	}{
		{"caster teleporting", func(t *testing.T, _ *gameservertest.Server, pc *player.Character, _ int32) {
			if !pc.SetTeleporting(true) {
				t.Fatal("SetTeleporting(true) reported no change")
			}
		}},
		{"target left", func(t *testing.T, srv *gameservertest.Server, _ *player.Character, hostileID int32) {
			hostile, ok := srv.State.Object(hostileID)
			if !ok {
				t.Fatal("fixture monster missing from world state")
			}
			srv.State.Despawn(hostile)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, pc, hostileID := bootMidCastBesideHostile(t)
			c, objID := srv.Client, pc.ObjectID()

			requestAttackMidCast(t, c, hostileID)
			tt.idle(t, srv, pc, hostileID)
			drainUntilQuiet(t, c)
			if !pc.CastingNow() {
				t.Fatal("long cast ended before the queued attack was made to idle")
			}

			srv.AdvanceUntil(t, "cast end", func() bool { return !pc.CastingNow() })
			failed := 0
			for end := c.Now().Add(2 * time.Second); c.Now().Before(end); {
				frame := c.ReadWithTimeout(300 * time.Millisecond)
				if frame == nil {
					continue
				}
				if attackFrameBy(frame, objID) {
					t.Fatal("queued attack swung after the cast")
				}
				if frame[0] == serverpackets.OpcodeActionFailed {
					failed++
				}
			}
			if failed != 1 {
				t.Fatalf("ActionFailed after the cast = %d, want 1", failed)
			}
		})
	}
}
