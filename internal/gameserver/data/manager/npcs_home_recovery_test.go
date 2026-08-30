package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// blockedHomeGeo blocks every route so return-home attempts accumulate
// geopath failures until the controller teleports.
type blockedHomeGeo struct{}

func (blockedHomeGeo) CanMove(_, _, _, _, _, _ int) bool { return false }
func (blockedHomeGeo) Height(_, _, z int) int16           { return int16(z) }
func (blockedHomeGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (blockedHomeGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}
func (blockedHomeGeo) Walkable(int, int, int) bool { return true }

type homeRecovery interface {
	GeoPathFailCount() int
	ResetGeoPathFailCount()
	AddGeoPathFailCount()
	TeleportTo(location.Location)
}

var _ homeRecovery = (*locatedRef)(nil)

// TestLiveHostileMoveHomeTeleportsThroughLocatedRef pins the production
// wiring from newLiveHostile: move.Controller.self is *locatedRef, which
// must forward home-path recovery methods to the embedded *npc.Hostile.
func TestLiveHostileMoveHomeTeleportsThroughLocatedRef(t *testing.T) {
	home := location.Location{X: 0, Y: 0, Z: 0}
	inst := &npc.Instance{
		ObjectID: 1,
		Template: &npc.Template{
			ID:         9001,
			Type:       "Monster",
			RunSpeed:   100,
			AIParams:   commons.NewStatSet(),
			NoSleepMode: true,
		},
		Kind:    "SiegeGuard",
		HasHome: true,
		Home:    home,
	}

	state := world.New()
	positions := task.NewPositionUpdates(state)
	hostile, walkerRef, err := newLiveHostile(inst, 100, blockedHomeGeo{}, positions, zerolog.Nop(), nil, actorcast.EffectHandlers{}, nil, nil)
	if err != nil {
		t.Fatalf("newLiveHostile() error: %v", err)
	}
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	hostile.SetWorld(state)

	locRef := &locatedRef{Actor: hostile}
	locRef.AddGeoPathFailCount()
	if got := hostile.GeoPathFailCount(); got != 1 {
		t.Fatalf("locatedRef.AddGeoPathFailCount() = %d on hostile, want 1", got)
	}
	locRef.ResetGeoPathFailCount()

	state.Spawn(hostile, 1000, 0, 0, 0)
	hostile.SetXYZ(1000, 0, 0)

	const failLimit = 10
	for range failLimit {
		if err := walkerRef.moveCtl.MoveHome(home); err != nil {
			t.Fatalf("MoveHome() error: %v", err)
		}
	}
	if got := hostile.GeoPathFailCount(); got != failLimit {
		t.Fatalf("GeoPathFailCount() = %d, want %d before teleport recovery", got, failLimit)
	}
	stranded := location.Location{X: 1000, Y: 0, Z: 0}
	x, y, z := hostile.Position()
	got := location.Location{X: x, Y: y, Z: z}
	if got != stranded {
		t.Fatalf("Position() = %+v, want still stranded before teleport", got)
	}

	if err := walkerRef.moveCtl.MoveHome(home); err != nil {
		t.Fatalf("MoveHome() after limit error: %v", err)
	}
	x, y, z = hostile.Position()
	got = location.Location{X: x, Y: y, Z: z}
	if got != home {
		t.Fatalf("Position() = %+v, want teleported home %+v", got, home)
	}
	if got := hostile.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after teleport = %d, want 0", got)
	}
}
