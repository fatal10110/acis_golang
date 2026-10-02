package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminSendHomeJailed pins that //sendhome takes a jailed player to its
// nearest town (AdminTeleport.java:158 → Player.teleportTo(RestartType.TOWN)
// → RestartPointData.getLocationToTeleport(TOWN), which has no jail branch):
// only a restart request forces the jail (RequestRestartPoint.java:71-72).
func TestAdminSendHomeJailed(t *testing.T) {
	t.Parallel()
	town := location.Location{X: 20000, Y: 20000, Z: 300}
	table := &restart.Table{Points: []restart.Point{{
		Name:       "JailTown",
		Points:     []location.Location{town},
		ChaoPoints: []location.Location{town},
		MapRegions: []location.Point{{
			X: (int(jailAt[0])-world.MinX)/world.TileSize + world.TileXMin,
			Y: (int(jailAt[1])-world.MinY)/world.TileSize + world.TileYMin,
		}},
	}}}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(punishPages(t)),
		gameservertest.WithZones(shippedJailZones(t)),
		gameservertest.WithRestartPoints(table))
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	exchange(t, gm, encodeBuildCmd("jail Player 30"))
	if at := teleportOf(t, settle(t, user), userID); at != jailAt {
		t.Fatalf("jail teleport = %v, want %v", at, jailAt)
	}
	appear(t, user)
	drain(t, gm)

	exchange(t, gm, encodeBuildCmd("sendhome Player"))
	if at := readTeleport(t, user, userID); !nearTown(at, town) {
		t.Fatalf("//sendhome of a jailed player = %v, want within 20 of the town %v", at, town)
	}
}
