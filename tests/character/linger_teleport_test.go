package character

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// lingerWatcherID is the monster template the see-creature recorder is
// bound to.
const lingerWatcherID = int32(20062)

// seenPlayers records the object ids of the players the watching monsters'
// see-creature hooks were raised for. Hooks run on whichever goroutine
// raises them, so the ids are guarded by mu.
type seenPlayers struct {
	mu  sync.Mutex
	ids []int32
}

func (s *seenPlayers) see(_ *script.Script, e script.SeeCreature) {
	p, ok := e.Creature.(*script.Player)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids = append(s.ids, p.ObjectID())
}

func (s *seenPlayers) take() []int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.ids
	s.ids = nil
	return ids
}

// playerPeace is a peace zone over the given box that counts the players
// entering and leaving it.
type playerPeace struct {
	*zone.Peace
	enters, exits atomic.Int32
}

func newPlayerPeace(t *testing.T, minX, maxX, minY, maxY int) *playerPeace {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, minY, maxY, -10_000, 10_000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	z := &playerPeace{Peace: zone.NewPeace(1, form)}
	z.OnEnter(func(a zone.Actor) {
		if a.Class() == zone.ClassPlayer {
			z.enters.Add(1)
		}
	})
	z.OnExit(func(a zone.Actor) {
		if a.Class() == zone.ClassPlayer {
			z.exits.Add(1)
		}
	})
	return z
}

// assert checks the zone's player enters and exits, and whether objID is
// among its occupants.
func (z *playerPeace) assert(t *testing.T, when string, enters, exits int32, objID int32, inside bool) {
	t.Helper()
	if got := z.enters.Load(); got != enters {
		t.Fatalf("%s: zone player enters = %d, want %d", when, got, enters)
	}
	if got := z.exits.Load(); got != exits {
		t.Fatalf("%s: zone player exits = %d, want %d", when, got, exits)
	}
	got := slices.ContainsFunc(z.Occupants(), func(a zone.Actor) bool { return a.ObjectID() == objID })
	if got != inside {
		t.Fatalf("%s: player inside the zone = %v, want %v", when, got, inside)
	}
}

// enterQuiet takes c's sole character into the world and drains the login
// burst instead of matching it.
func enterQuiet(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	drainQuiet(t, c)
}

// lingerTeleport is a player that dropped its connection in combat and
// lingers in the world, an observer standing next to a peace zone far
// away, and a watching monster inside that zone.
type lingerTeleport struct {
	srv      *gameservertest.Server
	observer *testsupport.ScriptedClient
	peace    *playerPeace
	seen     *seenPlayers
	lingerer *player.Character
	objID    int32
	dest     location.Location
}

func bootLingerTeleport(t *testing.T, opts ...gameservertest.Option) *lingerTeleport {
	t.Helper()
	dest := location.Location{X: spawnOrigin.X + 20_000, Y: spawnOrigin.Y, Z: spawnOrigin.Z}
	peace := newPlayerPeace(t, dest.X-300, dest.X+300, dest.Y-300, dest.Y+300)
	zones := zone.NewIndex()
	zones.Add(peace)
	seen := &seenPlayers{}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{lingerWatcherID: script.KindHostile},
			[]script.Listing{{Path: "ai.LingerWatcher"}},
			script.Catalog{"ai.LingerWatcher": func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{lingerWatcherID}, Hooks: script.Hooks{OnSeeCreature: seen.see}}
			}}),
	}, opts...)...)
	c := srv.Client
	enterQuiet(t, c)
	objID := srv.SoleObjectID(t)

	watched := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	enterQuiet(t, observer)
	drainQuiet(t, c)
	// The observer waits next to the zone, outside it, with the lingering
	// player out of its sight.
	watcherObj, ok := srv.State.Player(watched.ID)
	if !ok {
		t.Fatal("observer missing from the world")
	}
	watcher, ok := network.OnlineCharacter(watcherObj)
	if !ok {
		t.Fatalf("observer = %T, want an online character", watcherObj)
	}
	watcher.TeleportTo(dest.X+800, dest.Y, dest.Z, 0)
	mustReadOpcode(t, observer, serverpackets.OpcodeTeleportToLocation, "observer TeleportToLocation")
	appear(t, observer)
	drainQuiet(t, observer)
	drainQuiet(t, c)

	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = int(lingerWatcherID), int(lingerWatcherID)
	srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: dest.X + 150, Y: dest.Y, Z: dest.Z})
	srv.Settle(t)
	drainQuiet(t, observer)
	seen.take()
	peace.assert(t, "before the drop", 0, 0, objID, false)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from the world")
	}
	lingerer, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player = %T, want an online character", obj)
	}
	srv.SetPlayerInCombat(t, objID, true)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "the server noticing the connection drop", func() bool {
		return network.ClientDetached(obj)
	})
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("a player dropped in combat left the world at once")
	}
	return &lingerTeleport{srv: srv, observer: observer, peace: peace, seen: seen, lingerer: lingerer, objID: objID, dest: dest}
}

// readCharInfoOf reads c until objID's CharInfo, failing when none comes.
func readCharInfoOf(t *testing.T, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	for deadline := c.Now().Add(5 * time.Second); c.Now().Before(deadline); {
		frame := c.ReadWithTimeout(500 * time.Millisecond)
		if frame == nil || frame[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		r.ReadInt32() // x
		r.ReadInt32() // y
		r.ReadInt32() // z
		r.ReadInt32() // boat
		if r.ReadInt32() == objID {
			return
		}
	}
	t.Fatalf("no CharInfo of %d within 5s", objID)
}

// TestLingeringPlayerTeleportLandsAtOnce pins Creature.teleportTo for a
// player whose client detached (Creature.java:415-426): no Appearing will
// ever come from it, so it lands at once. It rejoins the grid, where the
// observer at the destination sees it appear, and enters the destination's
// zones; the monsters around it then run their see-creature hooks for it
// (forceSeeCreature), unless spawn protection turned on as it landed
// (Player.onTeleported).
func TestLingeringPlayerTeleportLandsAtOnce(t *testing.T) {
	t.Parallel()
	t.Run("protection off", func(t *testing.T) {
		t.Parallel()
		h := bootLingerTeleport(t)
		h.lingerer.TeleportTo(h.dest.X, h.dest.Y, h.dest.Z, 0)
		h.srv.Settle(t)
		readCharInfoOf(t, h.observer, h.objID)
		if x, y, _ := h.srv.PlayerPosition(t, h.objID); x != h.dest.X || y != h.dest.Y {
			t.Fatalf("lingering player position = %d %d, want %d %d", x, y, h.dest.X, h.dest.Y)
		}
		if !h.lingerer.InPeaceZone() {
			t.Fatal("lingering player not in the destination's peace zone")
		}
		h.peace.assert(t, "after the teleport", 1, 0, h.objID, true)
		if got := h.seen.take(); !slices.Equal(got, []int32{h.objID}) {
			t.Fatalf("see-creature calls on the landing = %v, want one for %d", got, h.objID)
		}
	})
	t.Run("protection on", func(t *testing.T) {
		t.Parallel()
		h := bootLingerTeleport(t, gameservertest.WithSpawnProtection(time.Minute))
		h.lingerer.TeleportTo(h.dest.X, h.dest.Y, h.dest.Z, 0)
		h.srv.Settle(t)
		readCharInfoOf(t, h.observer, h.objID)
		h.peace.assert(t, "after the teleport", 1, 0, h.objID, true)
		if !h.lingerer.SpawnProtected() {
			t.Fatal("lingering player landed without spawn protection")
		}
		if got := h.seen.take(); len(got) != 0 {
			t.Fatalf("see-creature calls on a spawn-protected landing = %v, want none", got)
		}
	})
}
