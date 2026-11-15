package character

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestZoneReloadAfterExitDoesNotReadmitPlayer pins that a player taken out
// of its zones by an exit stays out of them: a zone reload whose world
// snapshot still listed it (taken before the despawn, rebased after the
// exit) must not admit it into the reloaded zones. Admitted, it would stay
// an occupant past its session, and the next session's spawn could not
// enter the peace zone it stands in.
func TestZoneReloadAfterExitDoesNotReadmitPlayer(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-100, 100, -100, 100, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		// The relog selects the character again inside CharacterSelectTime.
		gameservertest.WithReuseDelays(0, 0),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	assertCompassCodes(t, readEnterWorldBurst(t, c), 0x0c) // PEACEZONE
	drainQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	departed, ok := obj.(interface{ ReloadZones() })
	if !ok {
		t.Fatalf("player %T has no zone reload", obj)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	if !slices.ContainsFunc(readUntilQuiet(c), func(f []byte) bool {
		return f[0] == serverpackets.OpcodeRestartResponse && f[1] == 1
	}) {
		t.Fatal("restart was not accepted")
	}
	// The reload reaches the departed session after its exit.
	departed.ReloadZones()

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	assertCompassCodes(t, readEnterWorldBurst(t, c), 0x0c)
}
