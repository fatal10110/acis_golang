package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A player teleport resets the selection inside its abortAll(true)
// (Creature.teleportTo, Creature.java:386-393; Creature.abortAll,
// Creature.java:1298-1306), before TeleportToLocation (:412).
// Player.setTarget(null) (Player.java:2440-2510) answers ActionFailed even
// for an empty selection, then broadcasts TargetUnselected when one was held,
// the player itself included.

// TestTeleportResetsSelectedMonsterBeforeJump: with a monster selected, the
// fifth ActionFailed is followed by the player's TargetUnselected, to itself
// and to a watcher, both ahead of TeleportToLocation.
func TestTeleportResetsSelectedMonsterBeforeJump(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	w := joinWatcher(t, srv)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)

	const (
		af  byte = serverpackets.OpcodeActionFailed
		tu  byte = serverpackets.OpcodeTargetUnselected
		ttl byte = serverpackets.OpcodeTeleportToLocation
	)
	got := teleportOpcodes(t, srv, onlinePlayer(t, srv, objID))
	if want := []byte{af, af, af, af, af, tu, ttl}; string(got) != string(want) {
		t.Fatalf("teleport with a monster selected sent opcodes %x up to TeleportToLocation, want %x", got, want)
	}
	if target := onlineTarget(t, srv, objID); target != nil {
		t.Fatalf("Target() = %d after the teleport, want none", target.ObjectID())
	}

	watched := readQuiet(w)
	unselected := indexOf(watched, 0, tu, objID)
	jump := indexOf(watched, 0, ttl, objID)
	if unselected < 0 || jump < unselected {
		t.Fatalf("watcher got %s, want the player's TargetUnselected before its TeleportToLocation", opcodes(watched))
	}
	if again := indexOf(watched, unselected+1, tu, objID); again >= 0 {
		t.Fatalf("watcher got a second TargetUnselected for the player: %s", opcodes(watched))
	}
}

// TestTeleportClearsSelfSelection: a player that selected itself has no
// target after the teleport; the reset answers ActionFailed, then
// TargetUnselected, before TeleportToLocation.
func TestTeleportClearsSelfSelection(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	drainUntilQuiet(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	c.Send(encodeAction(objID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, c)
	if target := onlineTarget(t, srv, objID); target == nil || target.ObjectID() != objID {
		t.Fatalf("Target() = %v after clicking itself, want the player %d", target, objID)
	}

	const (
		af  byte = serverpackets.OpcodeActionFailed
		tu  byte = serverpackets.OpcodeTargetUnselected
		ttl byte = serverpackets.OpcodeTeleportToLocation
	)
	got := teleportOpcodes(t, srv, onlinePlayer(t, srv, objID))
	if want := []byte{af, af, af, af, af, tu, ttl}; string(got) != string(want) {
		t.Fatalf("teleport with itself selected sent opcodes %x up to TeleportToLocation, want %x", got, want)
	}
	if target := onlineTarget(t, srv, objID); target != nil {
		t.Fatalf("Target() = %d after the teleport, want none", target.ObjectID())
	}
}
