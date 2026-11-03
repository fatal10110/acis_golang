package quest

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The flows of Fate's Whispers (Q234), Mimir's Elixir (Q235) and Repent
// Your Sins (Q422) a scenario cannot play: it seeds no held item and no PK
// count. Each boots the quest alone over its shipped pages, every NPC a
// plain civilian whose chat window links to its quests.

// x1Weapon is a weapon template of the X1 flows.
type x1Weapon struct {
	id   int32
	name string
}

// x1Items is the shared item catalog plus an item of each of etc and a
// right-hand weapon of each of weapons.
func x1Items(etc []int32, weapons ...x1Weapon) gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range etc {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	for _, w := range weapons {
		templates = append(templates, &item.Template{
			ID: w.id, Name: w.name, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1,
			Destroyable: true, Weapon: &item.WeaponDetail{Type: item.WeaponDagger},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// shippedQuestPages returns every shipped page of quest, keyed as the page
// cache serves them.
func shippedQuestPages(t *testing.T, quest string) map[string]string {
	t.Helper()
	dir := "script/quest/" + quest
	entries, err := os.ReadDir(datapack.Path(t, append([]string{"data", "html"}, strings.Split(dir, "/")...)...))
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	for _, e := range entries {
		pages[dir+"/"+e.Name()] = datapackPage(t, dir+"/"+e.Name())
	}
	return pages
}

// bootX1Quest boots a character of level with the quest path (its name
// after "quest.") built by ctor over its shipped pages, the civilians
// npcs and the hostiles hostiles; before runs ahead of the selection.
func bootX1Quest(t *testing.T, level int, path string, ctor func() script.Script, npcs, hostiles []int32,
	before func(srv *gameservertest.Server, objID int32), opts ...gameservertest.Option,
) *dialogWorld {
	t.Helper()
	quest := strings.TrimPrefix(path, "quest.")
	kinds := map[int32]script.NPCKind{}
	ids := make([]int, 0, len(npcs))
	for _, id := range npcs {
		kinds[id] = script.KindFolk
		ids = append(ids, int(id))
	}
	for _, id := range hostiles {
		kinds[id] = script.KindHostile
	}
	w := bootSeamQuest(t, level, chatPages(shippedQuestPages(t, quest), ids...), before, append([]gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: path}}, script.Catalog{path: ctor}),
		gameservertest.WithLevels(q607Levels(t)),
	}, opts...)...)
	w.roles[w.player] = "player"
	return w
}

// seedLevelExp gives objID the experience level starts at in q607Levels,
// so a reward of experience keeps the level.
func seedLevelExp(t *testing.T, srv *gameservertest.Server, objID int32, level int) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET exp = ? WHERE obj_Id = ?", int64(level-1)*q607LevelExp, objID); err != nil {
		t.Fatalf("seed experience: %v", err)
	}
}

// page is the rendered NpcHtmlMessage of the shipped quest page file
// shown by the NPC role.
func (w *dialogWorld) page(t *testing.T, role, quest, file string) string {
	t.Helper()
	return "S NpcHtmlMessage obj=" + role + " item=0 html=" + scriptcontract.Quote(datapackPage(t, "script/quest/"+quest+"/"+file))
}

// keep returns the lines of got that start with one of prefixes.
func keep(got []string, prefixes ...string) []string {
	var out []string
	for _, l := range got {
		if slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(l, p) }) {
			out = append(out, l)
		}
	}
	return out
}

// bypassAll sends command and renders every frame it is answered with,
// deferred updates left out, any other packet the goldens do not render as its opcode.
func (w *dialogWorld) bypassAll(t *testing.T, command string) []string {
	t.Helper()
	w.srv.Client.Send(encodeBypass(command))
	var out []string
	for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
		if deferredUpdate(f) {
			continue
		}
		if line, err := scriptcontract.Packet(f, w.roles); err == nil {
			out = append(out, line)
			continue
		}
		out = append(out, fmt.Sprintf("S 0x%02x", f[0]))
	}
	return out
}

// started is the rows of a started quest at cond with vars.
func started(cond int, vars ...string) map[string]string {
	m := map[string]string{"<state>": "STARTED", "<cond>": strconv.Itoa(cond)}
	for i := 0; i+1 < len(vars); i += 2 {
		m[vars[i]] = vars[i+1]
	}
	return m
}

// wantPage fails unless got shows the stand-in page file.
func wantPage(t *testing.T, step string, got []string, file string) {
	t.Helper()
	if !shows(got, file) {
		t.Fatalf("%s: got %q, want the page %s", step, got, file)
	}
}
