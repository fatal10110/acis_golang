package ai

import (
	"encoding/binary"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// scatterOffset is the random offset the scattering hook teleports
	// with.
	scatterOffset = 200
	// scatterFolkID is a civilian NPC fixture that walks.
	scatterFolkID = 30921
)

// scatterWallGeo is open terrain with a wall across x at wallX: a walk
// that crosses it stops on the last point before it, on the line walked.
// It records the walks toward watch. The ground at dip lies dipZ below the
// height asked.
type scatterWallGeo struct {
	gameservertest.Geo
	mu    sync.Mutex
	wallX int
	watch location.Location
	walks [][6]int
	dip   location.Location
	dipZ  int
}

func (g *scatterWallGeo) Height(x, y, z int) int16 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.dipZ != 0 && x == g.dip.X && y == g.dip.Y {
		return int16(z - g.dipZ)
	}
	return g.Geo.Height(x, y, z)
}

func (g *scatterWallGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	g.mu.Lock()
	defer g.mu.Unlock()
	if tx == g.watch.X && ty == g.watch.Y {
		g.walks = append(g.walks, [6]int{ox, oy, oz, tx, ty, tz})
	}
	if ox < g.wallX && tx >= g.wallX {
		x := g.wallX - 1
		return location.Location{X: x, Y: oy + (ty-oy)*(x-ox)/(tx-ox), Z: tz}
	}
	return location.Location{X: tx, Y: ty, Z: tz}
}

// An NPC a script teleports with a random offset, a hostile one, a
// civilian one and a civilian one that cannot move alike: the script
// random source draws the x offset, then the y one, each in [-200, 200];
// the teleport walks from the point toward the drawn one on the geodata,
// stops before the wall the line crosses, and lands on the ground there.
// The player sees one ordinary TeleportToLocation there, and the NPC
// stands there.
func TestScriptTeleportNearScattersAnNPC(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		civilian, immobile bool
	}{{"hostile", false, false}, {"civilian", true, false}, {"immobile civilian", true, true}} {
		civilian := tc.civilian
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			geo := &scatterWallGeo{wallX: 1 << 30}
			var mu sync.Mutex
			var draws, bounds []int
			var dest location.Location
			rand := func(n int) int {
				mu.Lock()
				defer mu.Unlock()
				bounds = append(bounds, n)
				if len(draws) == 0 {
					return 0
				}
				v := draws[0]
				draws = draws[1:]
				return v
			}
			scatter := func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{warriorID, scatterFolkID}, Hooks: script.Hooks{
					OnAttacked: func(s *script.Script, e script.Attacked) {
						mu.Lock()
						to := dest
						mu.Unlock()
						s.TeleportNear(e.NPC, script.Loc{X: to.X, Y: to.Y, Z: to.Z}, scatterOffset)
					},
				}}
			}
			folkTmpl := gameservertest.FolkTemplate("Folk", scatterFolkID)
			folkTmpl.CanMove = !tc.immobile
			w := bootWarriorsWith(t, []gameservertest.Option{
				gameservertest.WithGeo(geo),
				gameservertest.WithScriptRand(rand),
				gameservertest.WithNPCScripts(map[int32]script.NPCKind{warriorID: script.KindHostile, scatterFolkID: script.KindFolk},
					[]script.Listing{{Path: "ai.Scatter"}}, script.Catalog{"ai.Scatter": scatter}),
			}, warriorTemplate(warriorID, nil), folkTmpl)
			var target attackable.Combatant
			if civilian {
				n, err := w.srv.NpcSpawns.AddSpawn(scatterFolkID, w.at.X+100, w.at.Y, w.at.Z, 0, false, 0)
				if err != nil {
					t.Fatalf("spawn %d: %v", scatterFolkID, err)
				}
				w.srv.Settle(t)
				w.srv.ReadQueued(t, w.srv.Client)
				target = n.(*npc.Folk)
			} else {
				target = w.spawn(t, warriorID, 100, 0)
			}

			// The draws 300 and 50 aim 100 east and 150 south of the
			// point; the wall 50 east of it stops the walk 49 east, 73
			// south.
			to := location.Location{X: w.at.X + 300, Y: w.at.Y + 100, Z: w.at.Z}
			toward := location.Location{X: to.X + 100, Y: to.Y - 150, Z: to.Z}
			landing := location.Location{X: to.X + 49, Y: to.Y - 73, Z: to.Z - 20}
			mu.Lock()
			dest, draws, bounds = to, []int{300, 50}, nil
			mu.Unlock()
			geo.mu.Lock()
			geo.wallX, geo.watch = to.X+50, toward
			geo.dip, geo.dipZ = landing, 20
			geo.mu.Unlock()

			done := make(chan struct{})
			if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
				defer close(done)
				target.TakeDamage(10, w.player)
			}) {
				t.Fatal("the character's queue is closed")
			}
			<-done
			w.srv.Settle(t)
			frames := w.srv.ReadQueued(t, w.srv.Client)

			mu.Lock()
			gotBounds := bounds
			mu.Unlock()
			if want := []int{2*scatterOffset + 1, 2*scatterOffset + 1}; !slices.Equal(gotBounds, want) {
				t.Fatalf("script draws = %v, want %v", gotBounds, want)
			}
			geo.mu.Lock()
			walks := geo.walks
			geo.mu.Unlock()
			if want := [][6]int{{to.X, to.Y, to.Z, toward.X, toward.Y, to.Z}}; !slices.Equal(walks, want) {
				t.Fatalf("geodata walks = %v, want %v", walks, want)
			}
			var got []location.Location
			for _, f := range framesOf(frames, serverpackets.OpcodeTeleportToLocation, target.ObjectID()) {
				v := func(i int) int { return int(int32(binary.LittleEndian.Uint32(f[5+4*i:]))) }
				if v(3) != 0 {
					t.Fatal("the NPC's TeleportToLocation is the fast one, want the ordinary one")
				}
				got = append(got, location.Location{X: v(0), Y: v(1), Z: v(2)})
			}
			if !slices.Equal(got, []location.Location{landing}) {
				t.Fatalf("NPC teleports = %v, want one to %v", got, landing)
			}
			if x, y, z := target.Position(); (location.Location{X: x, Y: y, Z: z}) != landing {
				t.Fatalf("NPC stands at %d,%d,%d, want %v", x, y, z, landing)
			}
		})
	}
}
