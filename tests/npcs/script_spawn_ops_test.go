package npcs

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The spawn-operation fixture's NPC ids.
const (
	opsAltarID  = 21301 // a civilian at a fixed point, 36 hours to respawn
	opsRunnerID = 21302 // a monster placed anywhere in the field, with a flee point
	opsFarID    = 21303 // a monster of a territory far from the character
)

// opsAltarKey is the altar's spawn slot key.
const opsAltarKey = "ops_altar#0#0"

// opsSpawnlist declares the fixture's npcmakers. The field holds the
// character, at (10, 20); the far territory does not.
const opsSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="ops_field" minZ="-10000" maxZ="10000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<territory name="ops_far" minZ="-10000" maxZ="10000"><node x="5000" y="5000"/><node x="5400" y="5000"/><node x="5400" y="5400"/><node x="5000" y="5400"/></territory>
	<npcmaker name="ops_altar" territory="ops_field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21301" total="1" pos="60;20;30;1000" respawn="36hour"/>
	</npcmaker>
	<npcmaker name="ops_runner" territory="ops_field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21302" total="1" respawn="60sec"><ai><set name="flee_x" val="111"/><set name="flee_y" val="-222"/></ai></npc>
	</npcmaker>
	<npcmaker name="ops_far" territory="ops_far" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21303" total="1" respawn="60sec"/>
	</npcmaker>
</list>`

func opsTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ops.xml"), []byte(opsSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// opsTemplates are the fixture's templates. The altar's template names a
// flee point of its own, which a flee point read never consults.
func opsTemplates() *npc.Table {
	altar := gameservertest.FolkTemplate("Merchant", opsAltarID)
	altar.AIParams = npc.AIParams{"flee_x": "7", "flee_y": "8", "flee_z": "9"}
	return npc.NewTable([]*npc.Template{altar, spawnMonster(opsRunnerID, "Runner"), spawnMonster(opsFarID, "Far")})
}

// opsWorld is the booted fixture: a script spawner over its population and
// the registered script a territory teleport is asked through.
type opsWorld struct {
	*folkWorld
	spawner *script.Spawner
	script  *script.Script
	// created counts the altar's created hooks.
	created *atomic.Int32
}

// bootOps boots the fixture with a plain script bound to the altar's
// created event, which keeps the registered script it is called with.
func bootOps(t *testing.T) *opsWorld {
	t.Helper()
	var held atomic.Pointer[script.Script]
	created := new(atomic.Int32)
	catalog := script.Catalog{"quest.OpsKeeper": func() script.Script {
		return script.Script{Bind: script.Bindings{script.EventCreated: {opsAltarID}}, Hooks: script.Hooks{
			OnCreated: func(s *script.Script, _ script.Created) {
				held.CompareAndSwap(nil, s)
				created.Add(1)
			},
		}}
	}}
	w := bootFolkWorld(t, nil,
		gameservertest.WithNPCs(opsTemplates()),
		gameservertest.WithNpcSpawns(opsTable(t)),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{opsAltarID: script.KindFolk}, []script.Listing{{Path: "quest.OpsKeeper"}}, catalog),
	)
	drainFrames(t, w.c)
	s := held.Load()
	if s == nil {
		t.Fatal("the altar's created hook did not run at boot")
	}
	return &opsWorld{folkWorld: w, spawner: script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop()), script: s, created: created}
}

func (w *opsWorld) handle(t *testing.T, id int) *script.NPC {
	t.Helper()
	return script.NPCOf(liveOf(t, w.srv, id).(attackable.Combatant))
}

// An npcmaker NPC a script deleted, whose maker has armed its respawn, is
// brought back at once by DoRespawn: the pending respawn is cancelled, the
// NPC comes back as a new life at its spawn's declared point and heading,
// the player standing by sees it, its created hooks run again, and the
// handle follows it. DoRespawn on the live NPC changes nothing.
func TestScriptDoRespawnBringsAMakerNPCBackAtOnce(t *testing.T) {
	t.Parallel()
	w := bootOps(t)
	altar := w.handle(t, opsAltarID)
	first := altar.ObjectID()

	w.spawner.DoRespawn(altar)
	w.srv.Settle(t)
	if frames := drainFrames(t, w.c); len(frames) != 0 || altar.ObjectID() != first || altar.Decayed() {
		t.Fatalf("DoRespawn on the live altar: frames %x, id %d (was %d), decayed %v; want nothing changed", opcodes(frames), altar.ObjectID(), first, altar.Decayed())
	}

	altar.DeleteMe()
	w.srv.Settle(t)
	if frames := drainFrames(t, w.c); !deletedIn(frames, first) {
		t.Fatalf("deleting the altar sent %x, want its DeleteObject", opcodes(frames))
	}
	if !w.srv.NpcRespawns.Tracked(opsAltarKey) {
		t.Fatal("the maker armed no respawn for the deleted altar")
	}

	w.spawner.DoRespawn(altar)
	w.srv.Settle(t)
	if w.srv.NpcRespawns.Tracked(opsAltarKey) {
		t.Fatal("the altar's pending respawn is still armed after DoRespawn")
	}
	back := liveOf(t, w.srv, opsAltarID).(*npc.Folk)
	if back.ObjectID() == first {
		t.Fatal("the respawned altar kept the deleted one's object id")
	}
	frames := drainFrames(t, w.c)
	if x, y, _, heading := npcInfoAt(t, frames, back.ObjectID()); x != 60 || y != 20 || heading != 1000 {
		t.Fatalf("respawned altar shown at %d,%d heading %d; want its point 60,20 heading 1000", x, y, heading)
	}
	if altar.ObjectID() != back.ObjectID() || altar.Decayed() {
		t.Fatalf("kept handle: id %d, decayed %v; want the respawned altar %d", altar.ObjectID(), altar.Decayed(), back.ObjectID())
	}
	if got := w.created.Load(); got != 2 {
		t.Fatalf("altar created hooks = %d, want 2: at boot and at the respawn", got)
	}
}

// A spawn with one declared point gives that point and its heading on
// every draw; a spawn with no declared point gives a new random point of
// its territory on each draw, which is not necessarily where its NPC
// stands.
func TestScriptDrawSpawnLocation(t *testing.T) {
	t.Parallel()
	w := bootOps(t)
	altar := w.handle(t, opsAltarID)
	for range 3 {
		if got := w.spawner.DrawSpawnLocation(altar); got.X != 60 || got.Y != 20 || got.Heading != 1000 {
			t.Fatalf("altar spawn location = %+v, want 60,20 heading 1000", got)
		}
	}

	runner := w.handle(t, opsRunnerID)
	seen := map[[2]int]bool{}
	for range 20 {
		got := w.spawner.DrawSpawnLocation(runner)
		if got.X < 0 || got.X > 400 || got.Y < 0 || got.Y > 400 {
			t.Fatalf("runner spawn location %+v lies outside its territory", got)
		}
		seen[[2]int{got.X, got.Y}] = true
	}
	if len(seen) < 2 {
		t.Fatalf("20 draws of the runner's territory gave %d distinct points, want a new point per draw", len(seen))
	}
}

// The flee point is read from the npcmaker spawn's own AI parameters, an
// unset one counting as 0; with all three 0 or unset there is none, even
// when the template names one.
func TestScriptFleeLocationReadsTheSpawnOnly(t *testing.T) {
	t.Parallel()
	w := bootOps(t)
	if got, ok := w.handle(t, opsRunnerID).FleeLocation(); !ok || got != (script.Loc{X: 111, Y: -222}) {
		t.Fatalf("runner flee location = %+v, %v; want 111,-222,0", got, ok)
	}
	if got, ok := w.handle(t, opsAltarID).FleeLocation(); ok {
		t.Fatalf("altar flee location = %+v, want none: its spawn names none and the template's is not read", got)
	}
}

// Every player standing in the spawn's territory is teleported, each on
// its own queue; a spawn whose territory holds nobody teleports nobody.
func TestScriptTeleportPlayersInTerritory(t *testing.T) {
	t.Parallel()
	w := bootOps(t)
	dest := script.Loc{X: w.at.X + 300, Y: w.at.Y + 200, Z: w.at.Z}

	w.script.TeleportPlayersInTerritory(w.handle(t, opsFarID), dest, 0)
	w.srv.Settle(t)
	if frames := drainFrames(t, w.c); teleportsOf(frames, w.player) != 0 {
		t.Fatalf("a territory without the character teleported it: %x", opcodes(frames))
	}

	w.script.TeleportPlayersInTerritory(w.handle(t, opsRunnerID), dest, 0)
	w.srv.Settle(t)
	if frames := drainFrames(t, w.c); teleportsOf(frames, w.player) != 1 {
		t.Fatalf("TeleportToLocation frames for the character = %d in %x, want 1", teleportsOf(frames, w.player), opcodes(frames))
	}
	if x, y, _ := w.srv.PlayerPosition(t, w.player); x != dest.X || y != dest.Y {
		t.Fatalf("character at %d,%d, want %d,%d", x, y, dest.X, dest.Y)
	}
}

// teleportsOf counts the TeleportToLocation frames of objectID.
func teleportsOf(frames [][]byte, objectID int32) int {
	n := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeTeleportToLocation && wire.NewReader(f[1:]).ReadInt32() == objectID {
			n++
		}
	}
	return n
}

// SetXYZ puts a live NPC at the point, facing its way, without a packet
// while it stays in its region. A decayed NPC is left as it is.
func TestScriptSetXYZMovesALiveNPCSilently(t *testing.T) {
	t.Parallel()
	w := bootOps(t)
	runner := w.handle(t, opsRunnerID)
	live := liveOf(t, w.srv, opsRunnerID).(*npc.Hostile)
	to := script.Loc{X: w.at.X + 40, Y: w.at.Y + 30, Z: w.at.Z, Heading: 4321}

	runner.SetXYZ(to)
	w.srv.Settle(t)
	if x, y, z := live.Position(); x != to.X || y != to.Y || z != to.Z || live.Heading() != to.Heading {
		t.Fatalf("runner at %d,%d,%d heading %d; want %+v", x, y, z, live.Heading(), to)
	}
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("SetXYZ within the region sent %x, want nothing", opcodes(frames))
	}

	onQueueOf(t, live, func() { live.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(live.ObjectID())) })
	drainFrames(t, w.c)
	runner.SetXYZ(script.Loc{X: w.at.X + 90, Y: w.at.Y, Z: w.at.Z})
	if x, y, _ := live.Position(); x != to.X || y != to.Y {
		t.Fatalf("decayed runner moved to %d,%d, want it left at %d,%d", x, y, to.X, to.Y)
	}
}
