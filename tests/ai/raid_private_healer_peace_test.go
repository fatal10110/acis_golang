package ai

import (
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRaidPrivateHealerFellowHitInPeaceGoesHome ports the peace return of
// RaidPrivateHealer's party-attacked hook, the only one a healer fellow
// gets since that hook skips its parent's. Two healers of one master in a
// peace zone, the second moved off its spawn location: a hit on the first
// puts the second back on its spawn location with no attack desire.
func TestRaidPrivateHealerFellowHitInPeaceGoesHome(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(200, 3_000, -1_000, 1_000, -1_000, 1_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	w := bootRhw(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithZones(zones)}})
	w.draws.set(1)
	master := w.spawn(t, rhwMasterID, 400, 0)
	first := w.raise(t, master, rhwHealerID, 600, 0, script.Params{})
	second := w.raise(t, master, rhwHealerID, 800, 0, script.Params{})

	home := script.NPCOf(second).SpawnLocation()
	homeAt := location.Location{X: home.X, Y: home.Y, Z: home.Z}
	w.teleport(t, second, 300, 0)
	if positionOf(second) == homeAt {
		t.Fatal("setup: the second healer still stands on its spawn location")
	}
	if !second.InsideZone(zone.FlagPeace) {
		t.Fatal("setup: the second healer is not in the peace zone")
	}

	w.hit(t, first, 10)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := positionOf(second); got != homeAt {
		t.Fatalf("second healer at %v after a hit on its fellow in peace, want its spawn location %v", got, homeAt)
	}
	if d, ok := desire(second, actorai.IntentionAttack); ok {
		t.Fatalf("second healer still wants to attack after a hit on its fellow in peace: %+v", d)
	}
}
