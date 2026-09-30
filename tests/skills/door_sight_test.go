package skills

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Line of sight to a door follows GeoEngine.canSeeTarget
// (geoengine/GeoEngine.java:523-548): a door is a Creature there, so its
// sight point rises by getCollisionHeight() * 2 * PART_OF_CHARACTER_HEIGHT
// / 100, and Door.getCollisionHeight() (Door.java:282-285) is half the
// template height, so the line ends at 75% of the door's height above its
// Z. A target that is itself a geo object (a closed door) is left out of
// the query, whoever looks.

// engineSightGeo is passable movement geo whose line-of-sight queries run on
// a real geodata engine, recording the target eye heights they were asked.
type engineSightGeo struct {
	gameservertest.Geo
	eng *engine.Engine

	mu            sync.Mutex
	targetHeights []float64
}

func (g *engineSightGeo) CanSeeActor(ox, oy, oz int, oh float64, tx, ty, tz int, th float64) bool {
	g.record(th)
	return g.eng.CanSeeActor(ox, oy, oz, oh, tx, ty, tz, th)
}

func (g *engineSightGeo) CanSeeActorIgnoring(ox, oy, oz int, oh float64, tx, ty, tz int, th float64, ignore dynamic.Object) bool {
	g.record(th)
	return g.eng.CanSeeActorIgnoring(ox, oy, oz, oh, tx, ty, tz, th, ignore)
}

func (g *engineSightGeo) record(th float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.targetHeights = append(g.targetHeights, th)
}

func (g *engineSightGeo) heights() []float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]float64(nil), g.targetHeights...)
}

// sightEngine builds a geodata engine over the region holding (x, y): flat
// ground at groundZ, except that when wallZ is non-zero every cell in the
// geodata column of world X wallX rises to wallZ.
func sightEngine(t *testing.T, x, y int, groundZ int16, wallX int, wallZ int16) *engine.Engine {
	t.Helper()
	eng := engine.New()
	region := block.NewRegion()
	for i := range block.RegionBlockCount {
		region.SetFlat(i, groundZ)
	}
	if wallZ != 0 {
		const regionCells = block.RegionBlocksX * block.CellsX
		wallCell := engine.GeoX(wallX) % regionCells
		blockX := wallCell / block.CellsX
		blockY := (engine.GeoY(y) % regionCells) / block.CellsY
		var cells [block.CellCount]block.Cell
		for cx := range block.CellsX {
			for cy := range block.CellsY {
				cell := block.Cell{Height: groundZ, NSWE: block.AllDirections}
				if blockX*block.CellsX+cx == wallCell {
					cell.Height = wallZ
				}
				cells[cx*block.CellsY+cy] = cell
			}
		}
		if err := region.SetComplex(blockX*block.RegionBlocksY+blockY, cells); err != nil {
			t.Fatalf("wall block: %v", err)
		}
	}
	regionX := engine.TileXMin + (x-engine.WorldXMin)/engine.TileSize
	regionY := engine.TileYMin + (y-engine.WorldYMin)/engine.TileSize
	if err := eng.SetRegion(regionX, regionY, region); err != nil {
		t.Fatalf("sight geodata: %v", err)
	}
	return eng
}

// sightDoorTemplate is a skill-unlockable door of the given height standing
// at (x, y, z).
func sightDoorTemplate(x, y, z, height int) *door.Template {
	tmpl := unlockDoorTemplate(false)
	tmpl.Position = location.Location{X: x, Y: y, Z: z}
	tmpl.Coordinates = []location.Point{
		{X: x - 8, Y: y - 8}, {X: x + 8, Y: y - 8}, {X: x + 8, Y: y + 8}, {X: x - 8, Y: y + 8},
	}
	tmpl.Height = height
	return tmpl
}

// TestRangedCastAtDoorSeesItAtDoorEyeHeight pins the door's sight point: a
// low wall between the caster and the door hides the door's base, so the
// cast depends on how high the line ends on the door. A 64-high door is seen
// over the wall at 48 (0.75 x 64) and opens; a 16-high door ends at 12, stays
// hidden, and the cast answers CANT_SEE_TARGET alone. A door seen at its full
// height (24 for the short one) would clear the wall too.
func TestRangedCastAtDoorSeesItAtDoorEyeHeight(t *testing.T) {
	t.Parallel()
	// The fixture character stands at the class spawn (10, 20, 30); the door
	// stands 290 units east on the same ground, with a wall rising to Z 64 in
	// the geodata column at X 48-63, just past the 32-unit obstacle allowance
	// over a ground-level line.
	const (
		groundZ = 30
		doorX   = 300
		doorY   = 20
		wallX   = 48
		wallZ   = 64
	)
	for _, tc := range []struct {
		name   string
		height int
		opens  bool
	}{
		{name: "tall door clears the wall", height: 64, opens: true},
		{name: "short door stays hidden", height: 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			geo := &engineSightGeo{eng: sightEngine(t, doorX, doorY, groundZ, wallX, wallZ)}
			def := unlockSkill(2235, "UNLOCK_SPECIAL", unlockSkillLevel, 100)
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithGeo(geo),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
				gameservertest.WithDoors(sightDoorTemplate(doorX, doorY, groundZ, tc.height)),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
			c.Send(encodeRequestGameStart(0))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
			c.Send(encodeEnterWorld())
			drainUntilQuiet(t, c)
			gate, ok := srv.WorldObjects.Door(unlockDoorID)
			if !ok {
				t.Fatal("door not spawned")
			}

			px, py, pz := srv.PlayerPosition(t, objID)
			if px != 10 || py != 20 || pz != groundZ {
				t.Fatalf("caster at (%d, %d, %d), want the fixture spawn (10, 20, %d)", px, py, pz, groundZ)
			}
			// The fixture is only sensitive to the door's eye height if the
			// wall hides the door's base from the caster.
			if geo.eng.CanSeeActor(px, py, pz, 0, doorX, doorY, groundZ, 0) {
				t.Fatal("fixture: the wall does not hide the door's base")
			}

			selectTarget(t, c, gate.ObjectID())
			if tc.opens {
				castUnlock(t, c, objID, def, gate.ObjectID())
				if !gate.Opened() {
					t.Fatal("door still closed after a guaranteed unlock it can be seen for")
				}
			} else {
				c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
				assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCantSeeTarget)
				assertNoActionFailedUntilQuiet(t, c, tc.name)
				if gate.Opened() {
					t.Fatal("door opened by a cast that cannot see it")
				}
			}

			heights := geo.heights()
			if len(heights) == 0 {
				t.Fatal("the cast never queried line of sight")
			}
			for _, th := range heights {
				if th != float64(tc.height)/2 {
					t.Fatalf("door collision height = %v, want half its %d height", th, tc.height)
				}
				if got, want := geo.eng.SightHeight(th), 0.75*float64(tc.height); got != want {
					t.Fatalf("door sight point = %v above its Z, want %v", got, want)
				}
			}
		})
	}
}

// TestNPCSightToClosedDoorIgnoresThatDoor pins that an NPC's sight to a
// closed door, like a player's or summon's, leaves that door out of the
// geodata query: the door never hides itself.
func TestNPCSightToClosedDoorIgnoresThatDoor(t *testing.T) {
	t.Parallel()
	const (
		doorX = 40
		npcX  = 160
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithDoors(sightDoorTemplate(doorX, 0, 0, 200)),
	)
	gate, ok := srv.WorldObjects.Door(unlockDoorID)
	if !ok {
		t.Fatal("door not spawned")
	}
	eng := sightEngine(t, doorX, 0, 0, 0, 0)
	eng.AddObject(gate)
	home := location.Location{X: npcX, Y: 0, Z: 0}
	monster := srv.SpawnMovingHostileNPCAtGeo(t, "Monster", home, home, &engineSightGeo{eng: eng})

	// The fixture only means something if the closed door blocks a plain
	// sight query to itself.
	if eng.CanSeeActor(npcX, 0, 0, monster.CollisionHeight(), doorX, 0, 0, gate.CollisionHeight()) {
		t.Fatal("fixture: the closed door does not block a plain sight query")
	}
	if !monster.CanSeeTarget(gate) {
		t.Fatal("NPC cannot see the closed door it looks at, want the door left out of its own sight query")
	}
}
