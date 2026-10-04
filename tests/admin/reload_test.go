package admin

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// reloadUsage is AdminReload.sendUsage.
var reloadUsage = []string{
	"Usage : //reload <admin|announcement|buylist|config>",
	"Usage : //reload <crest|cw|door|htm|item|multisell|npc>",
	"Usage : //reload <npcwalker|script|skill|teleport|zone>",
}

// countedReloads are reload hooks that only count their runs, in order.
type countedReloads struct {
	mu   sync.Mutex
	runs []string
	fail string // the hook that fails
}

func (c *countedReloads) hook(name string) func() error {
	return func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.runs = append(c.runs, name)
		if name == c.fail {
			return errors.New("broken file")
		}
		return nil
	}
}

func (c *countedReloads) take() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	runs := c.runs
	c.runs = nil
	return runs
}

func (c *countedReloads) reloads() network.DataReloads {
	teleports := c.hook("teleport")
	return network.DataReloads{
		Admin:        c.hook("admin"),
		Crests:       c.hook("crest"),
		HTML:         c.hook("htm"),
		Multisells:   c.hook("multisell"),
		NPCs:         c.hook("npc"),
		WalkerRoutes: c.hook("npcwalker"),
		Teleports: func() (travel.TeleportTable, travel.InstantTable, error) {
			return travel.TeleportTable{}, travel.InstantTable{}, teleports()
		},
	}
}

// TestAdminReload pins AdminReload.java's //reload: each token is matched
// against the types in the reference's order, by prefix but for multisell,
// npc and script, which must be spelled out; each type reloads and answers
// its own message, tokens stacking left to right. A token naming no type
// answers the usage and the next token is still read; no token, or a
// reload that fails, answers the usage and ends the command. A type whose
// reload is not ported yet only releases the client.
func TestAdminReload(t *testing.T) {
	t.Parallel()
	hooks := &countedReloads{}
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithDataReloads(hooks.reloads()))
	gm := srv.Client
	enterWorld(t, gm)

	for _, tc := range []struct {
		cmd  string
		runs []string
		want []string
	}{
		{"reload administrator", []string{"admin"}, []string{"Admin data has been reloaded."}},
		{"reload announcement", nil, []string{"The content of announcements.xml has been reloaded."}},
		{"reload crests", []string{"crest"}, []string{"Crests have been reloaded."}},
		{"reload html", []string{"htm"}, []string{"The HTM cache has been reloaded."}},
		{"reload multisell", []string{"multisell"}, []string{"The multisell instance has been reloaded."}},
		{"reload npc", []string{"npc"}, []string{"NPCs templates and Scripts have been reloaded."}},
		{"reload npcwalkers", []string{"npcwalker"}, []string{"Walking routes have been reloaded."}},
		{"reload teleports", []string{"teleport"}, []string{"Teleport locations have been reloaded."}},
		{"reload htm crest admin", []string{"htm", "crest", "admin"}, []string{"The HTM cache has been reloaded.", "Crests have been reloaded.", "Admin data has been reloaded."}},
		// Spelled-out types take no suffix.
		{"reload multisells", nil, reloadUsage},
		{"reload npcs", nil, reloadUsage},
		{"reload scripts", nil, reloadUsage},
		// A token naming no type does not stop the next one.
		{"reload nothing htm", []string{"htm"}, append(slices.Clone(reloadUsage), "The HTM cache has been reloaded.")},
		{"reload", nil, reloadUsage},
		{"reload ADMIN", nil, reloadUsage},
	} {
		frames := exchange(t, gm, encodeBuildCmd(tc.cmd))
		assertTexts(t, frames, tc.want...)
		if got := hooks.take(); !slices.Equal(got, tc.runs) {
			t.Fatalf("//%s reloaded %q, want %q", tc.cmd, got, tc.runs)
		}
	}

	// Not ported yet: released, nothing reloaded, nothing said.
	for _, word := range []string{"boat", "buylist", "config", "cw", "door", "item", "script", "skill", "zone"} {
		frames := exchange(t, gm, encodeBuildCmd("reload "+word))
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("//reload %s frames = %x, want ActionFailed", word, testsupport.FrameOpcodes(frames))
		}
	}

	// A failed reload answers the usage and stops there.
	hooks.fail = "crest"
	frames := exchange(t, gm, encodeBuildCmd("reload htm crest admin"))
	assertTexts(t, frames, append([]string{"The HTM cache has been reloaded."}, reloadUsage...)...)
	if got := hooks.take(); !slices.Equal(got, []string{"htm", "crest"}) {
		t.Fatalf("reloads after the failed one ran: %q", got)
	}
}

// TestAdminReloadSwapsTheTables pins what a reload changes: the pages,
// templates and access rights read after it are the reloaded ones, while
// an NPC already in the world keeps the template it was built from.
func TestAdminReloadSwapsTheTables(t *testing.T) {
	t.Parallel()
	templates := spawnTemplates()
	adminData := shippedAdminData(t)
	var srv *gameservertest.Server
	reloadedPages := gameservertest.HTMLCache(t, map[string]string{"admin/main_menu.htm": "<html><body>reloaded menu</body></html>"})
	reloads := network.DataReloads{
		HTML: func() error {
			srv.HTML.Replace(reloadedPages)
			return nil
		},
		NPCs: func() error {
			templates.Replace(renamedWolfTemplates())
			return nil
		},
		Admin: func() error {
			// The reloaded table asks for the master level to reload.
			fresh, err := admin.NewData([]admin.AccessLevel{{Level: 0}, {Level: adminLevel, IsGM: true}, {Level: masterLevel, IsGM: true}},
				[]admin.Command{{Name: "admin_reload", AccessLevel: masterLevel}, {Name: "admin_admin", AccessLevel: adminLevel}})
			if err != nil {
				return err
			}
			adminData.Replace(fresh)
			return nil
		},
	}
	srv, _ = bootAdmin(t, adminLevel, gameservertest.WithAdmin(adminData), gameservertest.WithNPCs(templates), gameservertest.WithNpcSpawns(nil), gameservertest.WithDataReloads(reloads))
	gm := srv.Client
	enterWorld(t, gm)

	exchange(t, gm, encodeBuildCmd("spawn 20120"))
	before := npcsOf(srv, wolfID)
	if len(before) != 1 {
		t.Fatalf("wolves = %d, want 1", len(before))
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload htm npc")), "The HTM cache has been reloaded.", "NPCs templates and Scripts have been reloaded.")
	assertPage(t, exchange(t, gm, encodeBuildCmd("admin")), "reloaded menu")
	frames := exchange(t, gm, encodeBuildCmd("spawn 20120"))
	if got := textsIn(t, frames); !slices.Equal(got, []string{"You spawned Dire Wolf. - Cmd: admin_spawn"}) {
		t.Fatalf("//spawn after the npc reload = %q, want the reloaded name", got)
	}
	if name := before[0].(*npc.Hostile).Instance.Name(); name != "Wolf" {
		t.Fatalf("the wolf spawned before the reload is now %q, want it to keep its template", name)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload admin")), "Admin data has been reloaded.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload htm")), "You don't have the access right to use this command.")
}

// renamedWolfTemplates are spawnTemplates with the wolf renamed.
func renamedWolfTemplates() *npc.Table {
	out := []*npc.Template{{ID: wolfID, TemplateID: wolfID, Type: "Monster", Name: "Dire Wolf", Level: 1, HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, AIParams: commons.NewStatSet()}}
	for _, tmpl := range spawnTemplates().All() {
		if tmpl.ID != wolfID {
			out = append(out, tmpl)
		}
	}
	return npc.NewTable(out)
}

// TestAdminReloadRaceFree runs reloads through one game master while
// another reads the same tables through its own commands: under -race,
// every read sees one table or the other and nothing races.
func TestAdminReloadRaceFree(t *testing.T) {
	t.Parallel()
	templates := spawnTemplates()
	adminData := shippedAdminData(t)
	var srv *gameservertest.Server
	pages := gameservertest.HTMLCache(t, shippedAdminPages(t, "main_menu.htm"))
	reloadedAdmin := shippedAdminData(t)
	var flip atomic.Bool
	reloads := network.DataReloads{
		HTML: func() error {
			srv.HTML.Replace(pages)
			return nil
		},
		NPCs: func() error {
			if flip.Load() {
				templates.Replace(spawnTemplates())
			} else {
				templates.Replace(renamedWolfTemplates())
			}
			flip.Store(!flip.Load())
			return nil
		},
		Admin: func() error {
			adminData.Replace(reloadedAdmin)
			return nil
		},
	}
	srv, _ = bootAdmin(t, adminLevel, gameservertest.WithAdmin(adminData), gameservertest.WithNPCs(templates), gameservertest.WithNpcSpawns(nil), gameservertest.WithDataReloads(reloads))
	gm := srv.Client
	enterWorld(t, gm)
	reader, _ := addPlayer(t, srv, "reader", "Reader", adminLevel)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 10 {
			for _, frame := range exchange(t, reader, encodeBuildCmd("admin")) {
				if frame[0] != serverpackets.OpcodeNpcHtmlMessage {
					t.Errorf("//admin during reloads answered %#x", frame[0])
					return
				}
			}
			for _, text := range textsIn(t, exchange(t, reader, encodeBuildCmd("spawn 20120"))) {
				if text != "You spawned Wolf. - Cmd: admin_spawn" && text != "You spawned Dire Wolf. - Cmd: admin_spawn" {
					t.Errorf("//spawn during reloads answered %q", text)
					return
				}
			}
		}
	}()
	// The reader's moves and spawns reach the game master too; only its
	// messages count.
	want := []string{"The HTM cache has been reloaded.", "NPCs templates and Scripts have been reloaded.", "Admin data has been reloaded."}
	for range 10 {
		if got := textsIn(t, exchange(t, gm, encodeBuildCmd("reload htm npc admin"))); !slices.Equal(got, want) {
			t.Fatalf("//reload during reads answered %q, want %q", got, want)
		}
	}
	wg.Wait()
}
