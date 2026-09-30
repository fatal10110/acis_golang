package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// An AREA skill centered on a door filters its splash victims by
// GeoEngine.canSeeTarget(door, victim) (TargetArea.java:31,
// geoengine/GeoEngine.java:523-548): the sight line starts at the door's
// position raised by its eye height, a victim that is itself a geo object is
// left out of the query, and the anchor door's own geodata stays in it. A
// closed door's footprint cells are lifted to its top (BlockComplexDynamic
// update), so the origin lookup below its own Z + CELL_HEIGHT finds no layer
// and the closed door sees nothing.

// TestAreaSkillOnDoorFiltersVictimsByDoorGeodata centers an AREA skill on a
// door with one monster in the open and one behind a second closed door. The
// closed anchor splashes no one; opened, it reaches the monster in the open
// and sees the closed door but not past it; once that door opens too, the
// hidden monster is reached as well.
func TestAreaSkillOnDoorFiltersVictimsByDoorGeodata(t *testing.T) {
	t.Parallel()
	const (
		anchorX  = 40
		blockerX = 160
		blocker  = unlockDoorID + 1
	)
	blockerTmpl := sightDoorTemplate(blockerX, 0, 0, 200)
	blockerTmpl.ID = blocker
	blockerTmpl.Name = "blocking_gate"
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithDoors(sightDoorTemplate(anchorX, 0, 0, 200), blockerTmpl),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)

	anchor, ok := srv.WorldObjects.Door(unlockDoorID)
	if !ok {
		t.Fatal("anchor door not spawned")
	}
	wall, ok := srv.WorldObjects.Door(blocker)
	if !ok {
		t.Fatal("blocking door not spawned")
	}
	p, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("caster not in the world")
	}
	caster, ok := p.(skilltarget.Actor)
	if !ok {
		t.Fatalf("caster %T is not a skill actor", p)
	}
	open := srv.SpawnHostileNPCAt(t, location.Location{X: anchorX, Y: 150, Z: 0})
	hidden := srv.SpawnHostileNPCAt(t, location.Location{X: 280, Y: 0, Z: 0})

	handler, ok := skilltarget.NewRegistry(skilltarget.WorldKnown{State: srv.State}).Handler(modelskill.TargetArea)
	if !ok {
		t.Fatal("no AREA target handler")
	}
	def := modelskill.Definition{
		ID: 1000, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetArea, Radius: 300, Offensive: true,
	}
	victims := func() []int32 {
		var ids []int32
		for _, a := range handler.Targets(caster, anchor, &def) {
			ids = append(ids, a.ObjectID())
		}
		slices.Sort(ids)
		return ids
	}
	want := func(ids ...int32) []int32 {
		ids = append(ids, anchor.ObjectID())
		slices.Sort(ids)
		return ids
	}

	if anchor.CanSeeTarget(open) {
		t.Fatal("closed door sees a monster in the open, want its own geodata to block its sight origin")
	}
	if got := victims(); !slices.Equal(got, want()) {
		t.Fatalf("AREA on a closed door = %v, want only the door %v", got, want())
	}

	srv.WorldObjects.SetDoorOpen(unlockDoorID, true)
	if !anchor.CanSeeTarget(open) {
		t.Fatal("open door cannot see the monster in the open")
	}
	if !anchor.CanSeeTarget(wall) {
		t.Fatal("open door cannot see the closed door it looks at, want that door left out of the query")
	}
	if anchor.CanSeeTarget(hidden) {
		t.Fatal("open door sees through a closed door, want the closed door's geodata to block the line")
	}
	if got := victims(); !slices.Equal(got, want(open.ObjectID())) {
		t.Fatalf("AREA on an open door = %v, want the door and the monster in the open %v", got, want(open.ObjectID()))
	}

	srv.WorldObjects.SetDoorOpen(blocker, true)
	if got := victims(); !slices.Equal(got, want(open.ObjectID(), hidden.ObjectID())) {
		t.Fatalf("AREA with both doors open = %v, want the door and both monsters %v", got, want(open.ObjectID(), hidden.ObjectID()))
	}
}

// TestDoorSightStartsAtDoorEyeHeight pins the door's sight origin: a low wall
// between an open door and a player hides the door's base, so whether the door
// sees the player depends on how high its line starts. A 64-high door starts
// at 48 (0.75 x 64) and clears the wall; a 16-high door starts at 12 and does
// not. This is the geometry TestRangedCastAtDoorSeesItAtDoorEyeHeight pins
// from the player's side; the query is mutual, so it answers the same from
// the door's side.
func TestDoorSightStartsAtDoorEyeHeight(t *testing.T) {
	t.Parallel()
	const (
		groundZ = 30
		doorX   = 300
		doorY   = 20
		wallX   = 48
		wallZ   = 64
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
	p, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player not in the world")
	}
	victim, ok := p.(skilltarget.Actor)
	if !ok {
		t.Fatalf("player %T is not a skill actor", p)
	}
	px, py, pz := victim.Position()
	if px != 10 || py != 20 || pz != groundZ {
		t.Fatalf("player at (%d, %d, %d), want the fixture spawn (10, 20, %d)", px, py, pz, groundZ)
	}

	for _, tc := range []struct {
		name   string
		height int
		sees   bool
	}{
		{name: "tall door clears the wall", height: 64, sees: true},
		{name: "short door stays hidden", height: 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eng := sightEngine(t, doorX, doorY, groundZ, wallX, wallZ)
			// The fixture is only sensitive to the door's eye height if the
			// wall hides the door's base from the player.
			if eng.CanSeeActor(doorX, doorY, groundZ, 0, px, py, pz, victim.CollisionHeight()) {
				t.Fatal("fixture: the wall does not hide the door's base")
			}
			tmpl := sightDoorTemplate(doorX, doorY, groundZ, tc.height)
			tmpl.Opened = true
			gate := spawnDoorOn(t, eng, tmpl)
			if got := gate.CanSeeTarget(victim); got != tc.sees {
				t.Fatalf("door %d high sees the player = %v, want %v", tc.height, got, tc.sees)
			}
		})
	}
}

// spawnDoorOn spawns tmpl through the production world-object owner on eng,
// in a world of its own.
func spawnDoorOn(t *testing.T, eng *engine.Engine, tmpl *door.Template) *door.Object {
	t.Helper()
	doors, err := door.NewTable([]*door.Template{tmpl})
	if err != nil {
		t.Fatalf("door table: %v", err)
	}
	statics, err := staticobject.NewTable(nil)
	if err != nil {
		t.Fatalf("static object table: %v", err)
	}
	timers, err := task.NewDoor(idleDoorToggle{}, time.Now)
	if err != nil {
		t.Fatalf("door timers: %v", err)
	}
	objs, err := gamemanager.NewWorldObjects(doors, statics, &doorIDs{next: 1 << 28}, eng, world.New(), timers, nil, zerolog.Nop())
	if err != nil {
		t.Fatalf("world objects: %v", err)
	}
	gate, ok := objs.Door(tmpl.ID)
	if !ok {
		t.Fatal("door not spawned")
	}
	return gate
}

type idleDoorToggle struct{}

func (idleDoorToggle) ToggleDoor(int) {}

type doorIDs struct{ next int32 }

func (d *doorIDs) NextID() (int32, error) {
	d.next++
	return d.next, nil
}
