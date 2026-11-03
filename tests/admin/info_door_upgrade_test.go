package admin

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminInfoUpgradedDoorPage: the door page shows an upgraded door's
// upgrade ratio, and HP over the maximum that ratio multiplied
// (DoorStatus.getMaxHp, getUpgradeHpRatio); the upgrade filled the door's
// HP to that maximum.
func TestAdminInfoUpgradedDoorPage(t *testing.T) {
	t.Parallel()
	gate := &door.Template{
		ID: infoGateID, Name: "info_gate", Kind: door.KindDoor, Level: 3,
		Position: location.Location{X: 100, Y: spawnY, Z: spawnZ},
		Coordinates: []location.Point{
			{X: 92, Y: spawnY - 8}, {X: 108, Y: spawnY - 8}, {X: 108, Y: spawnY + 8}, {X: 92, Y: spawnY + 8},
		},
		HP: 500, PDef: 100, MDef: 50, Height: 32,
	}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithDoors(gate),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "doorinfo.htm")),
	)
	gm := srv.Client
	enterWorld(t, gm)
	d, ok := srv.WorldObjects.Door(infoGateID)
	if !ok {
		t.Fatal("info gate not spawned")
	}
	if !srv.WorldObjects.UpgradeDoor(infoGateID, 3) {
		t.Fatal("info gate not upgraded")
	}
	gm.ReadQueued()
	exchange(t, gm, encodeAction(d.ObjectID()))

	assertInfoPage(t, gm, "info", shippedPage(t, "doorinfo.htm",
		"%name%", "info_gate",
		"%objid%", strconv.Itoa(int(d.ObjectID())),
		"%doorid%", strconv.Itoa(infoGateID),
		"%doortype%", "DOOR",
		"%doorlvl%", "3",
		"%residence%", "none",
		"%opentype%", "CLICK",
		"%initial%", "Closed",
		"%ot%", "0",
		"%ct%", "0",
		"%rt%", "0",
		"%controlid%", "0",
		"%hp%", "1500",
		"%hpmax%", "1500",
		"%hpratio%", "3",
		"%pdef%", "100",
		"%mdef%", "50",
		"%spawn%", "100, 20, 30, 0",
		"%height%", "32.0",
	))
}
