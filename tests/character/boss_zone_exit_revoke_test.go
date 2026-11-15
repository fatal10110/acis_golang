package character

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBossZoneRestartRevokesPermissionAndRelogEjects pins an accepted
// RequestRestart inside a lair (RequestRestart.java:55,
// Player.removeFromBossZone): the permission goes before the player leaves,
// so selecting the character again spawns it in the lair without one, and
// the zone ejects it to its oust location.
func TestBossZoneRestartRevokesPermissionAndRelogEjects(t *testing.T) {
	t.Parallel()
	oust := location.Location{X: -5000, Y: 20, Z: 30}
	zones, boss := bossEjectZone(t, oust, -100)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		// The relog selects the character again inside CharacterSelectTime.
		gameservertest.WithReuseDelays(0, 0),
	)
	objID := srv.SoleObjectID(t)
	boss.AllowEntry(objID, time.Minute)
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	if !slices.Contains(boss.AllowedPlayers(), objID) {
		t.Fatal("permitted player lost its permission on entry")
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	frames := readUntilQuiet(c)
	if !slices.ContainsFunc(frames, func(f []byte) bool {
		return f[0] == serverpackets.OpcodeRestartResponse && f[1] == 1
	}) {
		t.Fatal("restart inside the boss zone was not accepted")
	}
	if slices.Contains(boss.AllowedPlayers(), objID) {
		t.Fatal("player restarting inside the boss zone kept its entry permission")
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	if to := gameservertest.AssertSpawnTeleport(t, readUntilQuiet(c), objID); to != oust {
		t.Fatalf("relog inside the lair ejected to %+v, want %+v", to, oust)
	}
	waitForWorldPosition(t, srv, objID, oust)
}

// TestBossZoneDroppedConnectionKeepsPermission pins the other side: a lost
// connection is no Logout, so the player keeps its permission and the zone
// exit grants its re-entry window (BossZone.java:148-151).
func TestBossZoneDroppedConnectionKeepsPermission(t *testing.T) {
	t.Parallel()
	srv, boss, objID := bootInBossZone(t)

	if err := srv.Client.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "player left the world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	if !slices.Contains(boss.AllowedPlayers(), objID) {
		t.Fatal("player whose connection dropped inside the boss zone lost its entry permission")
	}
}
