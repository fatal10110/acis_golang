package npcs

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/classmaster"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// classMasterTowns are the towns the shipped spawnlist stands the class
// master in: the five starting villages and the main towns, by town zone
// id, each beside the NPC (by template id) the town's class change masters
// gather around.
var classMasterTowns = []struct {
	name   string
	townID int
	beside int32
}{
	{"Dark Elven Village", 1, 30143},
	{"Talking Island", 2, 30026},
	{"Elven Village", 3, 30154},
	{"Orc Village", 4, 30569},
	{"Gludin Village", 5, 30373},
	{"Dwarven Village", 6, 30520},
	{"Gludio", 7, 30288},
	{"Dion", 8, 30066},
	{"Giran", 9, 30109},
	{"Oren", 10, 30195},
	{"Hunters Village", 11, 30689},
	{"Aden", 12, 30849},
	{"Goddard", 13, 31276},
	{"Rune", 14, 31321},
	{"Heine", 15, 30900},
	{"Schuttgart", 17, 31974},
}

// classMasterBeside bounds how far the class master stands from the NPC it
// is placed beside.
const classMasterBeside = 150

// freeClassMasterJobs is the shipped ConfigClassMaster: every occupation
// change offered, free and without rewards.
const freeClassMasterJobs = "1;[];[];2;[];[];3;[];[]"

// shippedSpawnlist loads the shipped spawnlist.
func shippedSpawnlist(t *testing.T) *spawn.Table {
	t.Helper()
	table, err := gamexml.LoadSpawnlist(datapack.Path(t, "data", "xml", "spawnlist"), zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// fixedPositions lists every fixed position the table gives template id.
func fixedPositions(table *spawn.Table, id int32) []location.Location {
	var out []location.Location
	for _, m := range table.Makers() {
		for _, e := range m.Entries {
			if e.NPCID != id {
				continue
			}
			for _, p := range e.Positions {
				out = append(out, p.Location)
			}
		}
	}
	return out
}

func TestShippedSpawnlistStandsTheClassMasterInEveryTown(t *testing.T) {
	table := shippedSpawnlist(t)
	zones, err := gamexml.LoadZones(datapack.Path(t, "data", "xml", "zones"))
	if err != nil {
		t.Fatalf("load zones: %v", err)
	}
	towns := map[int][]*zone.Town{}
	for _, k := range zones.All() {
		if town, ok := k.(*zone.Town); ok {
			towns[town.TownID] = append(towns[town.TownID], town)
		}
	}

	masters := fixedPositions(table, classManagerID)
	if len(masters) != len(classMasterTowns) {
		t.Fatalf("class master spawns = %d %v, want one per town (%d)", len(masters), masters, len(classMasterTowns))
	}
	for _, town := range classMasterTowns {
		var inTown []location.Location
		for _, at := range masters {
			if slices.ContainsFunc(towns[town.townID], func(z *zone.Town) bool { return z.ContainsPoint(at.X, at.Y, at.Z) }) {
				inTown = append(inTown, at)
			}
		}
		if len(inTown) != 1 {
			t.Errorf("%s: class masters in town zone %d = %v, want one", town.name, town.townID, inTown)
			continue
		}
		at := inTown[0]
		besides := fixedPositions(table, town.beside)
		if !slices.ContainsFunc(besides, func(b location.Location) bool {
			return math.Hypot(float64(at.X-b.X), float64(at.Y-b.Y)) <= classMasterBeside && abs(at.Z-b.Z) <= 32
		}) {
			t.Errorf("%s: class master at %v stands farther than %d from NPC %d at %v", town.name, at, classMasterBeside, town.beside, besides)
		}
	}
}

// shippedClassMaster boots the character seeded by character, saved beside
// the shipped spawnlist's Gludio class master, spawned from its own maker with
// the shipped NPC template, with the shipped class master pages and the
// shipped free occupation changes, and opens the master's first page.
func shippedClassMaster(t *testing.T, character gameservertest.Option) *classManagerWorld {
	t.Helper()
	full := shippedSpawnlist(t)
	maker, ok := full.Maker("classmaster_gludio")
	if !ok || len(maker.Entries) != 1 || maker.Entries[0].NPCID != classManagerID || len(maker.Entries[0].Positions) != 1 {
		t.Fatal("shipped spawnlist has no Gludio class master maker")
	}
	table, err := spawn.NewTable(maker.Territories, []*spawn.Maker{maker})
	if err != nil {
		t.Fatalf("class master table: %v", err)
	}

	data, err := os.ReadFile(datapack.Path(t, "data", "xml", "npcs", "50000-50999.xml"))
	if err != nil {
		t.Fatalf("read class master template: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "50000-50999.xml"), data, 0o644); err != nil {
		t.Fatalf("copy class master template: %v", err)
	}
	templates, err := gamexml.LoadNPCTemplates(dir, nil, modelskill.NewTable(nil), zerolog.Nop())
	if err != nil {
		t.Fatalf("load class master template: %v", err)
	}

	jobs, err := classmaster.ParseJobs(freeClassMasterJobs)
	if err != nil {
		t.Fatal(err)
	}
	spawnAt := maker.Entries[0].Positions[0].Location
	srv := gameservertest.Boot(t,
		character,
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(classManagerPages(t)),
		gameservertest.WithClassMaster(classmaster.NewConfig(false, jobs)),
		gameservertest.WithClassTemplates(occupationTemplate(warriorOccupation), occupationTemplate(gladiatorOccupation)),
		gameservertest.WithNPCs(templates),
		gameservertest.WithNpcSpawns(table),
		classManagerSkills(t),
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if _, err := srv.DB.Exec(`UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?`, spawnAt.X+40, spawnAt.Y, spawnAt.Z, w.player); err != nil {
		t.Fatal(err)
	}
	startInWorld(t, srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}

	var found []*npc.Folk
	for _, obj := range srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.Instance.Template.ID == classManagerID {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		t.Fatalf("live class masters = %d, want the one the maker spawns", len(found))
	}
	master := found[0]
	if x, y, _ := master.Position(); x != spawnAt.X || y != spawnAt.Y {
		t.Fatalf("class master stands at %d,%d, want %v", x, y, spawnAt)
	}
	w.selectFolk(t, master)
	if _, html := pageOf(t, w.talk(t, master, false)); !strings.Contains(html, "_1stClass") {
		t.Fatalf("first page = %q, want the first class transfer link", html)
	}
	return &classManagerWorld{folkWorld: w, folk: master}
}

// gladiatorOccupation is the Warrior's second occupation.
const gladiatorOccupation = 2

// occupationTemplate is the fixture class template as occupation id.
func occupationTemplate(id int) *player.Template {
	tmpl := gameservertest.ClassTemplate()
	tmpl.ID = id
	tmpl.Skills = nil
	return tmpl
}

// changeForFree changes the class master's talker to occupation from the
// menu command opens, and checks the change took nothing and gave nothing,
// showed the new class and is saved.
func changeForFree(t *testing.T, w *classManagerWorld, menu string, occupation int, name string) {
	t.Helper()
	w.command(t, menu)
	frames := w.command(t, "change_class "+strconv.Itoa(occupation))
	if _, messages := changeOrder(frames); !slices.Equal(messages, []int32{smClassTransfer}) {
		t.Fatalf("change messages = %v, want only the class transfer %d", messages, smClassTransfer)
	}
	if info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)]); info.visibleClass != int32(occupation) {
		t.Fatalf("UserInfo class = %d, want %d", info.visibleClass, occupation)
	}
	if _, html := pageOf(t, frames); !strings.Contains(html, `You have now become a <font color="LEVEL">`+name+`</font>.`) {
		t.Fatalf("page = %q, want the %s confirmation", html, name)
	}
	if adena := w.srv.PlayerInventory(t, w.player).ItemCount(item.AdenaID, -1, true); adena != 0 {
		t.Fatalf("adena = %d after a free change, want 0", adena)
	}
	w.srv.TickAutosave(t)
	w.srv.FlushPersistence(t)
	var classID, baseClass int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT classid, base_class FROM characters WHERE obj_Id = ?`, w.player).Scan(&classID, &baseClass); err != nil {
		t.Fatal(err)
	}
	if classID != occupation || baseClass != occupation {
		t.Fatalf("saved class/base class = %d/%d, want %d/%d", classID, baseClass, occupation, occupation)
	}
}

func TestShippedClassMasterGivesTheFirstOccupationForFree(t *testing.T) {
	w := shippedClassMaster(t, gameservertest.WithCharacter("Talker", 20, 0))
	changeForFree(t, w, "1stClass", warriorOccupation, "Warrior")
}

func TestShippedClassMasterGivesTheSecondOccupationForFree(t *testing.T) {
	w := shippedClassMaster(t, gameservertest.WithClassCharacter("Talker", 40, 0, warriorOccupation))
	changeForFree(t, w, "2ndClass", gladiatorOccupation, "Gladiator")
}
