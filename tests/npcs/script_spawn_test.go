package npcs

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The script spawn fixtures: a monster, a civilian, and a captain whose
// template lists two privates.
const (
	spawnWolfID    = 20120
	spawnGrocerID  = 30001
	spawnCaptainID = 20130
	spawnPupID     = 20131
	spawnScoutID   = 20132
)

// groundBelow is how far below any probe point sinkingGeo puts the ground.
const groundBelow = 5

// sinkingGeo is the harness geodata with the ground groundBelow under every
// point it is asked about.
type sinkingGeo struct{ gameservertest.Geo }

func (sinkingGeo) Height(_, _, z int) int16 { return int16(z - groundBelow) }

func (sinkingGeo) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz - groundBelow}
}

func spawnMonster(id int, name string) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "Monster", Name: name, Level: 1, HPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 10,
	}
}

func scriptSpawnTemplates() *npc.Table {
	captain := spawnMonster(spawnCaptainID, "Captain")
	captain.Privates = []npc.PrivateEntry{
		{NpcID: spawnPupID, Weight: 3, RespawnDelay: 30 * time.Second},
		{NpcID: spawnScoutID, Weight: 5, RespawnDelay: 40 * time.Second},
	}
	return npc.NewTable([]*npc.Template{
		spawnMonster(spawnWolfID, "Wolf"),
		gameservertest.FolkTemplate("Merchant", spawnGrocerID),
		captain,
		spawnMonster(spawnPupID, "Pup"),
		spawnMonster(spawnScoutID, "Scout"),
	})
}

// scriptSpawnWorld is a booted world with a script spawner over its NPC
// population.
type scriptSpawnWorld struct {
	*folkWorld
	spawner *script.Spawner
}

func bootScriptSpawns(t *testing.T, extra ...gameservertest.Option) *scriptSpawnWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithNPCs(scriptSpawnTemplates()),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithGeo(sinkingGeo{}),
	}, extra...)
	w := bootFolkWorld(t, nil, opts...)
	drainFrames(t, w.c)
	return &scriptSpawnWorld{folkWorld: w, spawner: script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop())}
}

// liveObject returns the creature of object id in the world.
func (w *scriptSpawnWorld) liveObject(t *testing.T, id int32) attackable.Combatant {
	t.Helper()
	obj, ok := w.srv.State.Object(id)
	if !ok {
		t.Fatalf("npc %d is not in the world", id)
	}
	return obj.(attackable.Combatant)
}

// spawnedNPC finds the one NPC of template id in the world.
func spawnedNPC(t *testing.T, w *scriptSpawnWorld, id int) attackable.Combatant {
	t.Helper()
	return liveOf(t, w.srv, id).(attackable.Combatant)
}

// npcInfoAt reads frames until the NpcInfo of objectID and returns the
// position and heading it shows.
func npcInfoAt(t *testing.T, frames [][]byte, objectID int32) (x, y, z, heading int32) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != objectID {
			continue
		}
		r.ReadInt32() // template
		r.ReadInt32() // attackable
		x, y, z, heading = r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if err := r.Err(); err != nil {
			t.Fatalf("read NpcInfo: %v", err)
		}
		return x, y, z, heading
	}
	t.Fatalf("no NpcInfo for %d among %x", objectID, opcodes(frames))
	return 0, 0, 0, 0
}

// deletedIn reports whether frames hold a DeleteObject of objectID.
func deletedIn(frames [][]byte, objectID int32) bool {
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeDeleteObject && wire.NewReader(frame[1:]).ReadInt32() == objectID {
			return true
		}
	}
	return false
}

// A script spawn of a monster or a civilian NPC returns a live handle and
// the NPC appears to the player standing by: at the asked point, put on
// the ground from 20 above it, facing the asked way.
func TestScriptSpawnAppearsToNearbyPlayer(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	x, y, z := w.at.X+60, w.at.Y, w.at.Z

	for _, id := range []int32{spawnWolfID, spawnGrocerID} {
		h := w.spawner.AddSpawn(id, script.Loc{X: x, Y: y, Z: z, Heading: 1000}, false, 0)
		if h == nil || h.Decayed() {
			t.Fatalf("spawn of %d returned %v, want a live handle", id, h)
		}
		live := spawnedNPC(t, w, int(id))
		if script.NPCOf(live).Decayed() {
			t.Fatalf("npc %d reports gone at spawn", id)
		}
		// Ground below z: z-5; then the ground below 20 above that: z+10.
		gx, gy, gz, heading := npcInfoAt(t, drainFrames(t, w.c), live.ObjectID())
		if gx != int32(x) || gy != int32(y) || gz != int32(z+20-2*groundBelow) || heading != 1000 {
			t.Fatalf("npc %d shown at %d,%d,%d heading %d; want %d,%d,%d heading 1000", id, gx, gy, gz, heading, x, y, z+20-2*groundBelow)
		}
		rec, ok := w.srv.NpcSpawns.SpawnOf(live.ObjectID())
		if !ok || !rec.Fixed || rec.MasterID != 0 {
			t.Fatalf("npc %d spawn = %+v (%v), want a standalone spawn", id, rec, ok)
		}
	}

	// A template that does not exist spawns nothing.
	if h := w.spawner.AddSpawn(4242, script.Loc{X: x, Y: y, Z: z}, false, 0); h != nil {
		t.Fatalf("spawn of a missing template returned %v", h)
	}
}

// With a random offset the NPC stands within 100 of the asked point on
// each axis, on the ground geodata gives there; a spawn at a creature
// takes its position and heading.
func TestScriptSpawnRandomOffsetAndAtCreature(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	player := w.liveObject(t, w.player)

	for range 20 {
		h := w.spawner.AddSpawn(spawnWolfID, script.PlayerOf(player), true, 0)
		if h == nil {
			t.Fatal("spawn at the player returned no handle")
		}
	}
	px, py, pz := player.Position()
	frames := drainFrames(t, w.c)
	shown := 0
	for _, obj := range w.srv.State.Objects() {
		h, ok := obj.(*npc.Hostile)
		if !ok || h.Instance.Template.ID != spawnWolfID {
			continue
		}
		x, y, z, heading := npcInfoAt(t, frames, h.ObjectID())
		if dx, dy := int(x)-px, int(y)-py; dx < -100 || dx > 100 || dy < -100 || dy > 100 {
			t.Fatalf("wolf at %d,%d; want within 100 of %d,%d", x, y, px, py)
		}
		if z != int32(pz+20-2*groundBelow) || int(heading) != player.Heading() {
			t.Fatalf("wolf at z %d heading %d; want z %d and the player's heading %d", z, heading, pz+20-2*groundBelow, player.Heading())
		}
		shown++
	}
	if shown != 20 {
		t.Fatalf("wolves shown = %d, want 20", shown)
	}
}

// A timed despawn deletes the NPC once the delay has passed: the player
// sees it go and its handle reports it gone. Monsters and civilians alike.
func TestScriptSpawnTimedDespawn(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)

	wolf := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 30*time.Second)
	grocer := w.spawner.AddSpawn(spawnGrocerID, script.Loc{X: w.at.X - 50, Y: w.at.Y, Z: w.at.Z}, false, 30*time.Second)
	wolfID := spawnedNPC(t, w, spawnWolfID).ObjectID()
	grocerID := spawnedNPC(t, w, spawnGrocerID).ObjectID()
	drainFrames(t, w.c)

	w.srv.Advance(t, 29*time.Second)
	if wolf.Decayed() || grocer.Decayed() {
		t.Fatal("an NPC left before its despawn delay")
	}
	w.srv.Advance(t, time.Second)
	if !wolf.Decayed() || !grocer.Decayed() {
		t.Fatalf("after the delay wolf gone %v, grocer gone %v; want both", wolf.Decayed(), grocer.Decayed())
	}
	frames := drainFrames(t, w.c)
	for _, id := range []int32{wolfID, grocerID} {
		if !deletedIn(frames, id) {
			t.Fatalf("no DeleteObject for %d", id)
		}
		if _, ok := w.srv.State.Object(id); ok {
			t.Fatalf("npc %d is still in the world", id)
		}
		if _, ok := w.srv.NpcSpawns.SpawnOf(id); ok {
			t.Fatalf("npc %d still has its spawn", id)
		}
	}
}

// A private created through the API follows its master: it is linked to
// it, starts with its spawn parameters, and leaves the world when the
// master's spawn is deleted. One that cannot respawn stops being the
// master's private when it decays.
func TestScriptPrivateFollowsMaster(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	boss := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)

	placed := w.spawner.CreateOnePrivateEx(master, spawnPupID, script.Loc{X: w.at.X + 80, Y: w.at.Y + 10, Z: w.at.Z, Heading: 500}, 0, script.Params{P1: 7, P2: 8, P3: 9})
	around := w.spawner.CreateOnePrivate(master, spawnScoutID, 10*time.Second)
	if placed == nil || around == nil {
		t.Fatalf("private handles = %v, %v; want both", placed, around)
	}
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	scout := spawnedNPC(t, w, spawnScoutID).(*npc.Hostile)
	for _, p := range []*npc.Hostile{pup, scout} {
		if p.Master() != boss || !slices.Contains(boss.Minions(), p) {
			t.Fatalf("private %d is not linked to its master", p.ObjectID())
		}
		if rec, ok := w.srv.NpcSpawns.SpawnOf(p.ObjectID()); !ok || rec.MasterID != boss.ObjectID() {
			t.Fatalf("private %d spawn = %+v (%v), want the master's private", p.ObjectID(), rec, ok)
		}
	}
	s := pup.Scratch()
	if a, b, c := s.Int(npc.IntParam1), s.Int(npc.IntParam2), s.Int(npc.IntParam3); a != 7 || b != 8 || c != 9 {
		t.Fatalf("pup params = %d, %d, %d; want 7, 8, 9", a, b, c)
	}
	frames := drainFrames(t, w.c)
	if x, y, z, heading := npcInfoAt(t, frames, pup.ObjectID()); x != int32(w.at.X+80) || y != int32(w.at.Y+10) || z != int32(w.at.Z-groundBelow) || heading != 500 {
		t.Fatalf("pup shown at %d,%d,%d heading %d; want the asked point on the ground", x, y, z, heading)
	}
	bx, by, _ := boss.Position()
	sx, sy, _ := scout.Position()
	if d := math.Hypot(float64(sx-bx), float64(sy-by)); d < 39 || d > 121 {
		t.Fatalf("scout %.0f from its master; want 40 to 120", d)
	}

	// The scout's despawn passes: it is no longer the master's private.
	w.srv.Advance(t, 10*time.Second)
	if !around.Decayed() || slices.Contains(boss.Minions(), scout) || scout.Master() != nil {
		t.Fatalf("despawned scout gone %v, still a private %v", around.Decayed(), slices.Contains(boss.Minions(), scout))
	}

	// Deleting the master's spawn deletes its private with it.
	if !w.srv.NpcSpawns.DeleteFixed(boss.ObjectID()) {
		t.Fatal("the master's spawn was not deleted")
	}
	w.srv.Settle(t)
	if !master.Decayed() || !placed.Decayed() {
		t.Fatalf("master gone %v, private gone %v; want both", master.Decayed(), placed.Decayed())
	}
	if frames := drainFrames(t, w.c); !deletedIn(frames, pup.ObjectID()) || !deletedIn(frames, boss.ObjectID()) {
		t.Fatalf("frames %x miss the master's or the private's DeleteObject", opcodes(frames))
	}
}

// Privates from the template: each listed private is spawned around the
// master with its declared weight point; the master first forgets the
// privates it had, which stay. A created private that dies does not come
// back on its own.
func TestScriptCreatePrivatesFromTemplate(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	master := w.spawner.AddSpawn(spawnCaptainID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	captain := spawnedNPC(t, w, spawnCaptainID).(*npc.Hostile)
	earlier := w.spawner.CreateOnePrivate(master, spawnWolfID, 0)
	wolf := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)

	w.spawner.CreatePrivates(master)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	scout := spawnedNPC(t, w, spawnScoutID).(*npc.Hostile)
	minions := captain.Minions()
	if len(minions) != 2 || !slices.Contains(minions, pup) || !slices.Contains(minions, scout) {
		t.Fatalf("captain's privates = %d, want the pup and the scout", len(minions))
	}
	if earlier.Decayed() || wolf.Master() != captain {
		t.Fatal("the earlier private left the world or lost its master")
	}
	if a, b := pup.Scratch().Int(npc.IntWeightPoint), scout.Scratch().Int(npc.IntWeightPoint); a != 3 || b != 5 {
		t.Fatalf("weight points = %d, %d; want 3, 5", a, b)
	}

	// A private that dies stays the master's, and no respawn comes.
	if !pup.Die(nil, nil) {
		t.Fatal("pup did not die")
	}
	pupID := pup.ObjectID()
	armed := make(chan bool, 1)
	pup.Queue().Post(func() {
		respawn := w.srv.NpcSpawns.RespawnHook(pupID)
		armed <- respawn != nil
		pup.Decay(w.srv.State, respawn)
	})
	w.srv.Settle(t)
	if <-armed {
		t.Fatal("the decayed private armed a respawn of its own")
	}
	if pup.Master() != captain || !slices.Contains(captain.Minions(), pup) {
		t.Fatal("a private with a respawn delay lost its master at decay")
	}
	w.srv.Advance(t, time.Minute)
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == spawnPupID {
			t.Fatalf("a pup came back on its own: %d", h.ObjectID())
		}
	}
}

// The spawn's declared privates come before the template's.
func TestScriptCreatePrivatesFromSpawn(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t, gameservertest.WithNpcSpawns(captainDen(t)))
	captain := spawnedNPC(t, w, spawnCaptainID).(*npc.Hostile)
	w.spawner.CreatePrivates(script.NPCOf(captain))

	minions := captain.Minions()
	if len(minions) != 1 || minions[0].Instance.Template.ID != spawnWolfID {
		t.Fatalf("captain's privates = %d, want the spawn's one wolf", len(minions))
	}
	if got := minions[0].Scratch().Int(npc.IntWeightPoint); got != 9 {
		t.Fatalf("wolf weight point = %d, want the spawn's 9", got)
	}
}

// captainDen is a maker of one captain whose spawn entry declares one
// wolf private of its own.
func captainDen(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="den" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="captain_den" territory="den" maximumNpcs="1">
		<npc id="20130" total="1" pos="200;300;30;0" respawn="60sec"><privates><private id="20120" weight="9" respawn="20sec"/></privates></npc>
	</npcmaker>
</list>`
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}
