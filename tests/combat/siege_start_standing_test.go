package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// TestSiegeStartingAroundStandingPlayersRulesTheirFight: a siege starting
// around two players already standing on its battlefield puts their fight
// under the battlefield's rules at once: the killer is not flagged for the
// kill, and the victim loses a quarter of the normal exp (50 of 200), as on
// a battlefield it walked onto with the siege running.
func TestSiegeStartingAroundStandingPlayersRulesTheirFight(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-100_000, 100_000, -100_000, 100_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("siege form: %v", err)
	}
	siege := zone.NewSiege(1, form)
	zones := zone.NewIndex()
	zones.Add(siege)
	srv, killer, killerID, victimID := pvpZoneDeathScene(t, zones, seedCharacterWithDeathSnapshot(1500, 1700))

	siege.SetActive(true)
	drainUntilQuiet(t, killer)
	drainUntilQuiet(t, srv.Client)
	killPrimaryClient(t, srv, killer, killerID, victimID)
	obj, ok := srv.State.Player(killerID)
	if !ok {
		t.Fatal("killer missing from world state")
	}
	k, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatal("killer has no live character")
	}
	if k.PvPFlagState() != 0 || k.Karma() != 0 {
		t.Fatalf("killer after the siege kill: pvp flag %d, karma %d; want neither", k.PvPFlagState(), k.Karma())
	}
	if exp, before := persistedExp(t, srv, srv.Client, victimID); exp != 1450 || before != 1500 {
		t.Fatalf("persisted exp/before-death = %d/%d, want 1450/1500", exp, before)
	}
}
