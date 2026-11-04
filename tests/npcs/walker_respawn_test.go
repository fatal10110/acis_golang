package npcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// reverseWalkerSpawnlist places the porter, from an npcmaker spawn that
// brings it back 90 seconds after it decays, by the last node of its route.
const reverseWalkerSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="porter_field" minZ="-1000" maxZ="1000"><node x="-1000" y="-1000"/><node x="2000" y="-1000"/><node x="0" y="1000"/></territory>
	<npcmaker name="porter_maker" territory="porter_field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="31356" total="1" pos="700;20;30;0" respawn="90sec"/>
	</npcmaker>
</list>`

func reverseWalkerTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "porter.xml"), []byte(reverseWalkerSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// A walker its spawn brings back is the same NPC in a new life
// (ASpawn.doRespawn reuses it; Npc.onSpawn clears only its being on a
// route): it heads for the node nearest to it, then walks on the way its
// last life walked. A porter that turned back at the end of its route, its
// first node out of reach, dies, decays and comes back by its last node:
// there it walks back toward the middle node, not on round to the first,
// though the first is reachable again by then.
func TestRespawnedWalkerWalksOnInReverse(t *testing.T) {
	t.Parallel()
	executors(t, testRespawnedWalkerWalksOnInReverse)
}

func testRespawnedWalkerWalksOnInReverse(t *testing.T, opts ...gameservertest.Option) {
	at := location.Location{X: 10, Y: 20, Z: gameservertest.SpawnZ}
	nodes := []location.Location{
		{X: at.X + 100, Y: at.Y, Z: at.Z},
		{X: at.X + 400, Y: at.Y, Z: at.Z},
		{X: at.X + 700, Y: at.Y, Z: at.Z},
	}
	// The first node is out of reach once: as the porter first turns
	// round the end of its route.
	path := &blockedNode{node: nodes[0]}
	path.left.Store(1)
	tmpl := gameservertest.FolkTemplate("Folk", porterID)
	tmpl.Alias, tmpl.CanMove = porterAlias, true
	var walk []route.WalkerLocation
	for _, n := range nodes {
		walk = append(walk, route.WalkerLocation{Location: n})
	}
	list, catalog := walkersList()
	w := bootFolkWorld(t, nil, append([]gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
		gameservertest.WithNpcSpawns(reverseWalkerTable(t)),
		gameservertest.WithWalkerRoutes(route.WalkerRoutes{porterAlias: {porterAlias: walk}}),
		gameservertest.WithWalkerPath(path),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{porterID: script.KindFolk}, list, catalog),
		gameservertest.WithAITask(),
	}, opts...)...)
	if w.at != at {
		t.Fatalf("character stands at %v, want %v", w.at, at)
	}
	drainFrames(t, w.c)

	// takeRoute runs the porter's AI until it takes up its route, on its
	// second tick, and requires it to head for the last node.
	takeRoute := func(when string) *npc.Folk {
		t.Helper()
		porter := liveOf(t, w.srv, porterID).(*npc.Folk)
		w.tickAI(t)
		dests, _ := legs(aboutNPC(w.tickAI(t), porter.ObjectID()))
		if len(dests) != 1 || dests[0] != nodes[2] {
			t.Fatalf("%s: porter legs taking up its route = %v, want one to %v", when, dests, nodes[2])
		}
		return porter
	}
	// arrive lets the porter reach the last node and returns where it heads
	// on from there.
	arrive := func(when string, porter *npc.Folk) []location.Location {
		t.Helper()
		w.srv.AdvanceUntil(t, when+": porter reaches the last node", func() bool {
			p := folkAt(porter)
			return p.X == nodes[2].X && p.Y == nodes[2].Y
		})
		dests, _ := legs(aboutNPC(drainFrames(t, w.c), porter.ObjectID()))
		return dests
	}

	porter := takeRoute("first life")
	// Round the end its first node is out of reach: it turns back.
	if dests := arrive("first life", porter); len(dests) != 1 || dests[0] != nodes[1] {
		t.Fatalf("first life: porter legs at the last node = %v, want one back to %v", dests, nodes[1])
	}

	if !porter.Kill(nil) {
		t.Fatal("porter did not die")
	}
	onQueueOf(t, porter, func() { porter.DecayWithRespawn(w.srv.State, w.srv.NpcSpawns.RespawnHook) })
	w.srv.Settle(t)
	if _, inWorld := w.srv.State.Object(porter.ObjectID()); inWorld {
		t.Fatal("decayed porter is still in the world")
	}
	w.srv.RespawnSlot(t, "porter_maker#0#0")
	w.srv.Settle(t)
	drainFrames(t, w.c)

	back := takeRoute("next life")
	if back.ObjectID() == porter.ObjectID() {
		t.Fatal("the respawned porter kept its object id")
	}
	if dests := arrive("next life", back); len(dests) != 1 || dests[0] != nodes[1] {
		t.Fatalf("next life: porter legs at the last node = %v, want one back to %v, still walking in reverse", dests, nodes[1])
	}
}
