package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const walkerPorterID, walkerPorterAlias = 31357, "porter_remy"

// walkerPorterRoute is the porter's two-node route; a porter placed near
// its second node heads for that one first.
var walkerPorterRoute = []route.WalkerLocation{
	{Location: location.Location{X: 110, Y: 20, Z: 50}},
	{Location: location.Location{X: 410, Y: 20, Z: 50}},
}

const (
	// walkerPorterHeadingOn is the panel row of the porter heading for its
	// second node the regular way.
	walkerPorterHeadingOn = `<table width=280 bgcolor=000000><tr><td width=150><a action="bypass admin_teleport 410 20 50">Porter Remy</a></td><td width=40>1 / 2</td><td width=50 align=right>Regular</td><td width=40 align=right><a action="bypass admin_show walker 1 porter_remy">Show</a></td></tr></table>`
	walkerPorterUnspawned = `<table width=280 bgcolor=000000><tr><td width=150><a action="bypass admin_teleport 110 20 50">Unspawned</a></td><td width=40>-</td><td width=50 align=right>-</td><td width=40 align=right><a action="bypass admin_show walker 1 porter_remy">Show</a></td></tr></table>`
)

// bootWalkerPorter boots a GM with the porter walking route and spawns,
// nil for none.
func bootWalkerPorter(t *testing.T, spawns *spawn.Table) (*gameservertest.Server, *testsupport.ScriptedClient) {
	t.Helper()
	tmpl := gameservertest.FolkTemplate("Folk", walkerPorterID)
	tmpl.Alias, tmpl.Name, tmpl.CanMove = walkerPorterAlias, "Porter Remy", true
	return bootShowAdmin(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithWalkerRoutes(route.WalkerRoutes{walkerPorterAlias: {walkerPorterAlias: walkerPorterRoute}}),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{walkerPorterID: script.KindFolk},
			[]script.Listing{{Path: "script.ai.group.Walkers"}}, script.Catalog{"script.ai.group.Walkers": group.Walkers}),
		gameservertest.WithAITask(),
	)
}

// walkerPorterMakers is a spawn list whose one npcmaker places the porter
// near its second node, respawning after respawn ("" for never).
func walkerPorterMakers(t *testing.T, respawn string) *spawn.Table {
	t.Helper()
	if respawn != "" {
		respawn = ` respawn="` + respawn + `"`
	}
	list := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="porter_field" minZ="-1000" maxZ="1000"><node x="-1000" y="-1000"/><node x="1000" y="-1000"/><node x="0" y="1000"/></territory>
	<npcmaker name="porter_maker" territory="porter_field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="31357" total="1" pos="400;20;30;0"` + respawn + `/>
	</npcmaker>
</list>`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "makers.xml"), []byte(list), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// walkPorter runs the AI until the porter takes up its route, on its
// second tick, and returns it.
func walkPorter(t *testing.T, srv *gameservertest.Server, gm *testsupport.ScriptedClient) *npc.Folk {
	t.Helper()
	for range 2 {
		if err := srv.AI.Tick(); err != nil {
			t.Fatalf("AI tick: %v", err)
		}
	}
	drain(t, gm)
	var porter *npc.Folk
	for _, obj := range srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.Instance.Template.ID == walkerPorterID {
			porter = f
		}
	}
	if porter == nil {
		t.Fatal("no porter in the world")
	}
	return porter
}

// assertWalkerRow requires the walker panel to hold row.
func assertWalkerRow(t *testing.T, gm *testsupport.ScriptedClient, when, row string) {
	t.Helper()
	frames := exchangeZones(t, gm, encodeBuildCmd("show walker 1"))
	if len(frames) != 1 {
		t.Fatalf("//show walker %s = %x, want the panel", when, opcodes(frames))
	}
	if body := htmlBody(t, frames[0]); !strings.Contains(body, row) {
		t.Fatalf("//show walker %s: want the row\n%s\ngot\n%s", when, row, body)
	}
}

// TestAdminShowWalkerDeadWalkerKeepsItsNode pins the walker panel row of a
// walker that died mid-route: its corpse still names the node it headed
// for and its direction, not the route's first node.
func TestAdminShowWalkerDeadWalkerKeepsItsNode(t *testing.T) {
	t.Parallel()
	srv, gm := bootWalkerPorter(t, nil)
	if _, err := srv.NpcSpawns.AddSpawn(walkerPorterID, 400, 20, 30, 0, false, 0); err != nil {
		t.Fatalf("spawn porter: %v", err)
	}
	porter := walkPorter(t, srv, gm)
	assertWalkerRow(t, gm, "with the porter walking", walkerPorterHeadingOn)

	if !porter.Kill(nil) {
		t.Fatal("porter did not die")
	}
	srv.Settle(t)
	drain(t, gm)
	assertWalkerRow(t, gm, "over the porter's corpse", walkerPorterHeadingOn)
}

// TestAdminShowWalkerDecayedMakerNPC pins the walker panel row of a
// walker of an npcmaker spawn that died and decayed: while its spawn
// waits to respawn it, the row still names it with the node it headed for
// and its direction; a spawn that never respawns lets it go, and the row
// is unspawned.
func TestAdminShowWalkerDecayedMakerNPC(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, respawn, row string
		armed              int
	}{
		{name: "respawning", respawn: "90sec", row: walkerPorterHeadingOn, armed: 1},
		{name: "never respawning", row: walkerPorterUnspawned},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, gm := bootWalkerPorter(t, walkerPorterMakers(t, tc.respawn))
			porter := walkPorter(t, srv, gm)
			assertWalkerRow(t, gm, "with the porter walking", walkerPorterHeadingOn)

			if !porter.Kill(nil) {
				t.Fatal("porter did not die")
			}
			porter.Queue().Post(func() { porter.DecayWithRespawn(srv.State, srv.NpcSpawns.RespawnHook) })
			srv.Settle(t)
			drain(t, gm)
			if _, inWorld := srv.State.Object(porter.ObjectID()); inWorld {
				t.Fatal("decayed porter is still in the world")
			}
			if got := srv.RespawnsArmed(); got != tc.armed {
				t.Fatalf("respawns armed = %d, want %d", got, tc.armed)
			}
			assertWalkerRow(t, gm, "with the porter decayed", tc.row)
		})
	}
}
