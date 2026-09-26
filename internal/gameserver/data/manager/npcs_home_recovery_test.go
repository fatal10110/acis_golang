package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// blockedHomeGeo blocks every route so return-home attempts accumulate
// geopath failures until the controller teleports.
type blockedHomeGeo struct{}

func (blockedHomeGeo) CanMove(_, _, _, _, _, _ int) bool { return false }
func (blockedHomeGeo) Height(_, _, z int) int16          { return int16(z) }
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
			ID:          9001,
			Type:        "Monster",
			RunSpeed:    100,
			AIParams:    commons.NewStatSet(),
			NoSleepMode: true,
		},
		Kind:    "SiegeGuard",
		HasHome: true,
		Home:    home,
	}

	state := world.New()
	positions := task.NewPositionUpdates(state)
	hostile, walkerRef, err := newLiveHostile(inst, 100, blockedHomeGeo{}, positions, zerolog.Nop(), nil, actorcast.EffectHandlers{}, nil, 20, 0, nil, effect.Env{}, npcQueues().NewQueue("npc"))
	if err != nil {
		t.Fatalf("newLiveHostile() error: %v", err)
	}
	hostile.Attach(npc.Runtime{World: state})

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

// TestHostileControlClosesAbortedCastWithCancelAnimation pins the NPC cast
// abort mapping newLiveHostile's controller sink owns: an aborted AI cast
// reports the NPC's own MagicSkillCanceled, matching CreatureCast.stop()'s
// isCastingNow()-gated broadcast (CreatureCast.java:416-419) that NpcCast
// inherits.
func TestHostileControlClosesAbortedCastWithCancelAnimation(t *testing.T) {
	inst := &npc.Instance{
		ObjectID: 7,
		Template: &npc.Template{ID: 9001, Type: "Monster", RunSpeed: 100, AIParams: commons.NewStatSet()},
		Kind:     "Monster",
	}
	state := world.New()
	hostile, _, err := newLiveHostile(inst, 100, blockedHomeGeo{}, task.NewPositionUpdates(state), zerolog.Nop(), nil, actorcast.EffectHandlers{}, nil, 20, 0, nil, effect.Env{}, npcQueues().NewQueue("npc"))
	if err != nil {
		t.Fatalf("newLiveHostile() error: %v", err)
	}
	rec := &event.Recorder{}
	hostile.Attach(npc.Runtime{World: state, Sink: rec})

	(&hostileControl{hostile: hostile}).Emit(event.CastAborted{Interrupted: true})

	if got := event.Of[event.SkillCanceled](rec); len(got) != 1 || got[0].ObjectID != 7 {
		t.Fatalf("SkillCanceled events = %+v, want one for the NPC itself (7)", got)
	}
}

// floorGeo is blockedHomeGeo with a fixed ground height, so a grounded
// teleport destination is distinguishable from the one requested.
type floorGeo struct {
	blockedHomeGeo
	floor int16
}

func (g floorGeo) Height(int, int, int) int16 { return g.floor }

// wallGeo is blockedHomeGeo whose destination validation stops every walk
// at a wall cell and records the last query.
type wallGeo struct {
	blockedHomeGeo
	wall location.Location
	last *[6]int
}

func (g wallGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	*g.last = [6]int{ox, oy, oz, tx, ty, tz}
	return g.wall
}

func newTeleportTestHostile(t *testing.T, id int32, geo move.Geo, zones *zone.Index, state *world.State) (*npc.Hostile, *event.Recorder) {
	t.Helper()
	inst := &npc.Instance{
		ObjectID: id,
		Template: &npc.Template{ID: 9001, Type: "Monster", RunSpeed: 100, CanMove: true, AIParams: commons.NewStatSet()},
		Kind:     "Monster",
	}
	hostile, _, err := newLiveHostile(inst, 100, geo, task.NewPositionUpdates(state), zerolog.Nop(), nil, actorcast.EffectHandlers{}, nil, 20, 0, zones, effect.Env{}, npcQueues().NewQueue("npc"))
	if err != nil {
		t.Fatalf("newLiveHostile() error: %v", err)
	}
	rec := &event.Recorder{}
	hostile.Attach(npc.Runtime{World: state, Sink: rec})
	return hostile, rec
}

// A teleport grounds its destination unless the destination lies inside a
// water zone, where the requested height is kept (the creature swims).
func TestLiveHostileTeleportKeepsHeightOnlyInsideWater(t *testing.T) {
	form, err := zone.NewCuboid(0, 1_000, 0, 1_000, -1_000, 150)
	if err != nil {
		t.Fatalf("NewCuboid() error: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	state := world.New()
	hostile, rec := newTeleportTestHostile(t, 1, floorGeo{floor: -200}, zones, state)
	state.Spawn(hostile, 5_000, 5_000, -200, 0)

	for _, tc := range []struct {
		name string
		to   location.Location
		want location.Location
	}{
		{"in water", location.Location{X: 500, Y: 500, Z: 100}, location.Location{X: 500, Y: 500, Z: 100}},
		{"on land", location.Location{X: 5_000, Y: 6_000, Z: 100}, location.Location{X: 5_000, Y: 6_000, Z: -200}},
	} {
		hostile.TeleportTo(tc.to)
		teleports := event.Of[event.Teleported](rec)
		if len(teleports) == 0 || teleports[len(teleports)-1].To != tc.want {
			t.Fatalf("%s: Teleported events = %+v, want last to %+v", tc.name, teleports, tc.want)
		}
		if x, y, z := hostile.Position(); (location.Location{X: x, Y: y, Z: z}) != tc.want {
			t.Fatalf("%s: Position() = (%d,%d,%d), want %+v", tc.name, x, y, z, tc.want)
		}
	}
}

// A stuck escort teleports beside its leader: the random offset is walked
// from the leader's cell through geodata, so a wall beside the leader stops
// it instead of placing the escort inside the wall.
func TestLiveHostileEscortTeleportValidatesOffset(t *testing.T) {
	state := world.New()
	leaderAt := location.Location{X: 2_000, Y: 2_000, Z: 0}
	leader, _ := newTeleportTestHostile(t, 1, blockedHomeGeo{}, nil, state)
	state.Spawn(leader, leaderAt.X, leaderAt.Y, leaderAt.Z, 0)
	leader.SetXYZ(leaderAt.X, leaderAt.Y, leaderAt.Z)

	var last [6]int
	wall := location.Location{X: leaderAt.X - 1, Y: leaderAt.Y, Z: leaderAt.Z}
	escort, rec := newTeleportTestHostile(t, 2, wallGeo{wall: wall, last: &last}, nil, state)
	state.Spawn(escort, 2_600, 2_000, 0, 0)
	escort.SetXYZ(2_600, 2_000, 0)
	for range 10 {
		escort.AddGeoPathFailCount()
	}

	escort.ThinkFollow(leader, false)

	if last[0] != leaderAt.X || last[1] != leaderAt.Y || last[2] != leaderAt.Z || last[5] != leaderAt.Z {
		t.Fatalf("ValidLocation origin/height = %v, want from leader %+v at its height", last, leaderAt)
	}
	if dx, dy := last[3]-leaderAt.X, last[4]-leaderAt.Y; dx < -10 || dx > 10 || dy < -10 || dy > 10 {
		t.Fatalf("ValidLocation target offset = (%d,%d), want within 10 of the leader", dx, dy)
	}
	teleports := event.Of[event.Teleported](rec)
	if len(teleports) != 1 || teleports[0].To != wall {
		t.Fatalf("Teleported events = %+v, want one to the wall cell %+v", teleports, wall)
	}
	if got := escort.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after escort teleport = %d, want 0", got)
	}
}
