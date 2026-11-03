package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminReloadDoorKeepsSealOfStrife pins that a door //reload door
// builds reads the Seal of Strife as a door built at boot does
// (DoorStatus.getPDef/getMDef): under Dusk its defences are cut to 30%.
func TestAdminReloadDoorKeepsSealOfStrife(t *testing.T) {
	t.Parallel()
	reloads := network.DataReloads{Doors: func() (*door.Table, error) {
		return door.NewTable([]*door.Template{reloadDoor(reloadGateID, 100, false)})
	}}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithDoors(reloadDoor(reloadGateID, 100, false)),
		gameservertest.WithDataReloads(reloads),
		seedStrifeOwner(t, sevensigns.Dusk),
	)
	gm := srv.Client
	enterWorld(t, gm)
	old, _ := srv.WorldObjects.Door(reloadGateID)
	if old.PhysicalDefence() != 30 || old.MagicDefence() != 30 {
		t.Fatalf("fixture: boot door P.Def %d, M.Def %d under Dusk; want 30, 30", old.PhysicalDefence(), old.MagicDefence())
	}

	exchange(t, gm, encodeBuildCmd("reload door"))

	gate, ok := srv.WorldObjects.Door(reloadGateID)
	if !ok || gate == old {
		t.Fatalf("door %d after the reload = %v, want a new door", reloadGateID, gate)
	}
	if gate.PhysicalDefence() != 30 || gate.MagicDefence() != 30 {
		t.Fatalf("reloaded door P.Def %d, M.Def %d under Dusk; want 30, 30", gate.PhysicalDefence(), gate.MagicDefence())
	}
}
