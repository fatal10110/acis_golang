package npcs

import (
	"os"
	"path/filepath"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// civilianMakerSpawnlist places the civilian tablet through a default
// maker, beside the fixture character.
const civilianMakerSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="field" minZ="-200" maxZ="200"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="tablet_m" territory="field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="30010" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

func civilianMakerTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "makers.xml"), []byte(civilianMakerSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// A civilian NPC placed by a maker creates a hostile and a civilian
// private; the maker's delete-all event (1000) takes the master and both
// privates out of the world, and the player is told of all three.
func TestMakerDeleteAllTakesCivilianMastersPrivates(t *testing.T) {
	t.Parallel()
	w := bootCivilianParty(t, &partyLog{}, gameservertest.WithNpcSpawns(civilianMakerTable(t)))
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)
	master := script.NPCOf(tablet)

	knight := w.spawner.CreateOnePrivate(master, spawnPupID, 0)
	clerk := w.spawner.CreateOnePrivate(master, spawnClerkID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	folk := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	assertPrivateOf(t, w, knight, pup, tablet)
	assertPrivateOf(t, w, clerk, folk, tablet)
	drainFrames(t, w.c)

	if !w.srv.NpcSpawns.MakerEvent("tablet_m", "1000", 0, 0) {
		t.Fatal("maker tablet_m not found by name")
	}
	w.srv.Settle(t)
	if !master.Decayed() || !knight.Decayed() || !clerk.Decayed() {
		t.Fatalf("master gone %v, privates gone %v %v; want all", master.Decayed(), knight.Decayed(), clerk.Decayed())
	}
	frames := drainFrames(t, w.c)
	for _, id := range []int32{tablet.ObjectID(), pup.ObjectID(), folk.ObjectID()} {
		if !deletedIn(frames, id) {
			t.Fatalf("frames %x miss the DeleteObject of %d", opcodes(frames), id)
		}
	}
	if pup.Master() != nil || folk.Master() != nil || len(tablet.Minions()) != 0 {
		t.Fatal("a deleted private is still linked to its master")
	}
}
