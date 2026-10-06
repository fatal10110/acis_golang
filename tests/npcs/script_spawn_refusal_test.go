package npcs

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// spawnBrokenCaptainID is a captain whose template lists a pup, then an NPC
// id no template has.
const (
	spawnBrokenCaptainID = 20133
	spawnMissingID       = 4242
)

// brokenCaptainTemplates is the script spawn table with the broken captain.
func brokenCaptainTemplates() *npc.Table {
	broken := spawnMonster(spawnBrokenCaptainID, "Broken Captain")
	broken.Privates = []npc.PrivateEntry{
		{NpcID: spawnPupID, Weight: 3, RespawnDelay: 30 * time.Second},
		{NpcID: spawnMissingID, Weight: 5, RespawnDelay: 40 * time.Second},
	}
	return npc.NewTable([]*npc.Template{
		spawnMonster(spawnWolfID, "Wolf"),
		gameservertest.FolkTemplate("Merchant", spawnGrocerID),
		spawnMonster(spawnPupID, "Pup"),
		broken,
	})
}

// mustPanic runs fn and fails the test unless it panics.
func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", what)
		}
	}()
	fn()
}

// A spawn asking for a negative heading faces a random way: the NpcInfo
// heading is a real one, never the -1 asked for.
func TestScriptSpawnNegativeHeadingIsRandom(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)

	for _, id := range []int32{spawnWolfID, spawnGrocerID} {
		if h := w.spawner.AddSpawn(id, script.Loc{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z, Heading: -1}, false, 0); h == nil {
			t.Fatalf("spawn of %d returned no handle", id)
		}
		live := spawnedNPC(t, w, int(id))
		_, _, _, heading := npcInfoAt(t, drainFrames(t, w.c), live.ObjectID())
		if heading < 0 || heading >= 65536 {
			t.Fatalf("npc %d shown with heading %d, want one in [0, 65536)", id, heading)
		}
		if live.Heading() != int(heading) {
			t.Fatalf("npc %d faces %d, NpcInfo shows %d", id, live.Heading(), heading)
		}
	}
}

// Privates from a template listing an NPC id no template has: the master
// forgets its earlier privates, the privates listed before the missing id
// are placed and linked, and the call panics there.
func TestScriptCreatePrivatesPanicsOnMissingTemplate(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t, gameservertest.WithNPCs(brokenCaptainTemplates()))
	master := w.spawner.AddSpawn(spawnBrokenCaptainID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	captain := spawnedNPC(t, w, spawnBrokenCaptainID).(*npc.Hostile)
	if w.spawner.CreateOnePrivate(master, spawnWolfID, 0) == nil {
		t.Fatal("the earlier private was not placed")
	}
	wolf := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)

	mustPanic(t, "privates naming a missing template", func() { w.spawner.CreatePrivates(master) })

	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	if pup.Master() != captain || !slices.Equal(captain.Minions(), []*npc.Hostile{pup}) {
		t.Fatalf("captain's privates = %d, want the pup alone, linked", len(captain.Minions()))
	}
	if _, ok := w.srv.State.Object(wolf.ObjectID()); !ok {
		t.Fatal("the earlier private left the world")
	}
}

// Only a hostile NPC keeps privates: a civilian private is refused with no
// handle, and a civilian master panics.
func TestScriptPrivateKindRefusals(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	var logged bytes.Buffer
	spawner := script.NewSpawner(w.srv.NpcSpawns, zerolog.New(&logged))

	master := spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	boss := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)
	if h := spawner.CreateOnePrivate(master, spawnGrocerID, 0); h != nil {
		t.Fatalf("civilian private returned %v, want no handle", h)
	}
	if h := spawner.CreateOnePrivateEx(master, spawnGrocerID, script.Loc{X: w.at.X + 70, Y: w.at.Y, Z: w.at.Z}, 0, script.Params{}); h != nil {
		t.Fatalf("civilian private at a point returned %v, want no handle", h)
	}
	if len(boss.Minions()) != 0 {
		t.Fatalf("master's privates = %d after refusals, want none", len(boss.Minions()))
	}
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.Instance.Template.ID == spawnGrocerID {
			t.Fatalf("a civilian private was placed: %d", f.ObjectID())
		}
	}
	if got := strings.Count(logged.String(), "spawn placed nothing"); got != 2 {
		t.Fatalf("refusals logged %d times, want 2: %s", got, logged.String())
	}

	grocer := spawner.AddSpawn(spawnGrocerID, script.Loc{X: w.at.X - 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	if grocer == nil {
		t.Fatal("civilian spawn returned no handle")
	}
	mustPanic(t, "a private of a civilian master", func() { spawner.CreateOnePrivate(grocer, spawnPupID, 0) })
	mustPanic(t, "a private at a point of a civilian master", func() {
		spawner.CreateOnePrivateEx(grocer, spawnPupID, script.Loc{X: w.at.X, Y: w.at.Y, Z: w.at.Z}, 0, script.Params{})
	})
	mustPanic(t, "privates of a civilian master", func() { spawner.CreatePrivates(grocer) })
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == spawnPupID {
			t.Fatalf("a private of a civilian master was placed: %d", h.ObjectID())
		}
	}
}
