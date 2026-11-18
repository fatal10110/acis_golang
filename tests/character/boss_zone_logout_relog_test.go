package character

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBossZoneLogoutRelogEjects pins the relog half of an accepted Logout
// inside a lair (Logout.java:52, Player.removeFromBossZone): the permission
// is gone with the session, so logging in again spawns the character in the
// lair without one and the zone ejects it to its oust location.
func TestBossZoneLogoutRelogEjects(t *testing.T) {
	t.Parallel()
	oust := location.Location{X: -5000, Y: 20, Z: 30}
	zones, boss := bossEjectZone(t, oust, -100)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	objID := srv.SoleObjectID(t)
	boss.AllowEntry(objID, time.Minute)
	enterWorld(t, srv.Client)
	drainQuiet(t, srv.Client)

	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	if !srv.Client.AwaitClose(5 * time.Second) {
		t.Fatal("server kept the connection open after Logout")
	}
	srv.AdvanceUntil(t, "player left the world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	if slices.Contains(boss.AllowedPlayers(), objID) {
		t.Fatal("player logging out inside the boss zone kept its entry permission")
	}

	c := srv.DialClient(t, srv.Account(), 1)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	if to := gameservertest.AssertSpawnTeleport(t, readUntilQuiet(c), objID); to != oust {
		t.Fatalf("relog inside the lair ejected to %+v, want %+v", to, oust)
	}
	waitForWorldPosition(t, srv, objID, oust)
}
