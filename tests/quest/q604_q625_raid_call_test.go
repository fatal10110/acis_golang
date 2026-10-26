package quest

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q604"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q625"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The altar of Daimon (Q604_DaimonTheWhiteEyed_Part2 onAdvEvent "31541-02.htm"),
// the Yeti's table (Q625_TheFinestIngredients_Part2 "31542-02.htm") and the
// Varka's holy altar (Q610_MagicalPowerOfWater_Part2 "31560-02.htm") call a
// raid boss for 1200 s when the player holds the summon item: the boss
// appears, says an NpcString, the altar leaves the world and the quest moves
// to cond 2; while the boss stands the other altars refuse the call; when it
// leaves, whether it despawns or dies, they call again. The raid's death
// moves the player on cond 2 to cond 3 with the quest's trophy. A scenario
// cannot show a script-spawned NPC appearing or leaving, nor act on one
// (#4020 item 5), so these Boot tests pin the calls; the scenarios play the
// dialogs, the hunt and the trade.

// raidQuest is what tells the two quests apart.
type raidQuest struct {
	name, path       string
	newQuest         func() script.Script
	eye, altar, raid int
	summon, trophy   int32
	called, left     int32
	at               [3]int
}

var (
	q604Raid = raidQuest{
		name: "Q604_DaimonTheWhiteEyed_Part2", path: "quest.Q604_DaimonTheWhiteEyed_Part2", newQuest: q604.New,
		eye: 31683, altar: 31541, raid: 25290, summon: 7193, trophy: 7194, called: 60403, left: 60404,
		at: [3]int{186320, -43904, -3175},
	}
	q625Raid = raidQuest{
		name: "Q625_TheFinestIngredients_Part2", path: "quest.Q625_TheFinestIngredients_Part2", newQuest: q625.New,
		eye: 31521, altar: 31542, raid: 25296, summon: 7209, trophy: 7210, called: 62503, left: 62504,
		at: [3]int{158240, -121536, -2222},
	}
)

// Two altars of the quest stand at the raid's spot, the west one 20 east of
// it and the east one 40 east of it, each its own npcmaker's with a 36 hours
// respawn, and the player stands 50 east of it.
const (
	westAltarDX = 20
	eastAltarDX = 40
)

func (rq raidQuest) spawns(t *testing.T) *spawn.Table {
	t.Helper()
	list := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="altar_field" minZ="-10000" maxZ="10000"><node x="%[4]d" y="%[5]d"/><node x="%[6]d" y="%[5]d"/><node x="%[6]d" y="%[7]d"/><node x="%[4]d" y="%[7]d"/></territory>
	<npcmaker name="altar_west" territory="altar_field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="%[1]d" total="1" pos="%[2]d;%[8]d;%[9]d;1000" respawn="36hour"/>
	</npcmaker>
	<npcmaker name="altar_east" territory="altar_field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="%[1]d" total="1" pos="%[3]d;%[8]d;%[9]d;2000" respawn="36hour"/>
	</npcmaker>
</list>`, rq.altar, rq.at[0]+westAltarDX, rq.at[0]+eastAltarDX, rq.at[0]-200, rq.at[1]-200, rq.at[0]+200, rq.at[1]+200, rq.at[1], rq.at[2])
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "altars.xml"), []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func (rq raidQuest) page(npcID, n int) string { return fmt.Sprintf("%d-%02d.htm", npcID, n) }

// boot boots a level 75 character whose quest is at cond with summons of the
// summon item, the Eye beside it and both altars standing.
func (rq raidQuest) boot(t *testing.T, cond string, summons int32) *dialogWorld {
	t.Helper()
	// The pages the call is offered on link it; the page of a standing altar at
	// cond 2 stands for the window a player holds from before the call.
	pages := map[string][]string{}
	for n := 1; n <= 5; n++ {
		pages[rq.page(rq.altar, n)] = nil
	}
	pages[rq.page(rq.altar, 1)] = []string{rq.page(rq.altar, 2)}
	pages[rq.page(rq.altar, 5)] = []string{rq.page(rq.altar, 2)}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{rq.summon, rq.trophy} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	w := bootSeamQuest(t, 75, chatPages(questPages(rq.name, pages), rq.altar), func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, rq.name, "<state>", val("STARTED")},
			journalRow{objID, rq.name, "<cond>", val(cond)})
		if summons > 0 {
			srv.GiveItem(t, objID, rq.summon, summons)
		}
	},
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("Folk", rq.eye), gameservertest.FolkTemplate("Folk", rq.altar), proofMonster(rq.raid),
		})),
		gameservertest.WithNpcSpawns(rq.spawns(t)),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{int32(rq.eye): script.KindFolk, int32(rq.altar): script.KindFolk, int32(rq.raid): script.KindHostile},
			[]script.Listing{{Path: rq.path}},
			script.Catalog{rq.path: rq.newQuest},
		),
	)
	w.roles[w.player] = "player"
	w.nameAltars(t, rq)
	w.stepTo(t, rq.at)
	return w
}

// nameAltars names the standing altars west and east in the rendered packets.
func (w *dialogWorld) nameAltars(t *testing.T, rq raidQuest) {
	t.Helper()
	for _, n := range npcsOf(w.srv, int32(rq.altar)) {
		switch n.SpawnLocation().X - rq.at[0] {
		case westAltarDX:
			w.roles[n.ObjectID()], w.npcs["west"] = "west", n.ObjectID()
		case eastAltarDX:
			w.roles[n.ObjectID()], w.npcs["east"] = "east", n.ObjectID()
		}
	}
}

func (rq raidQuest) wantCond(t *testing.T, w *dialogWorld, step, want string) {
	t.Helper()
	if rows := questRows(t, w.srv, w.player, rq.name); rows["<cond>"] != want {
		t.Fatalf("%s: rows %v, want cond %s", step, rows, want)
	}
}

func (rq raidQuest) wantItems(t *testing.T, w *dialogWorld, step string, summons, trophies int) {
	t.Helper()
	if n := w.srv.PlayerItemCount(t, w.player, rq.summon); n != summons {
		t.Fatalf("%s: summon items %d, want %d", step, n, summons)
	}
	if n := w.srv.PlayerItemCount(t, w.player, rq.trophy); n != trophies {
		t.Fatalf("%s: trophies %d, want %d", step, n, trophies)
	}
}

// call talks to the altar role, expecting its page, and sends the call event,
// returning the frames the event was answered with.
func (rq raidQuest) call(t *testing.T, w *dialogWorld, role string, talkPage int) [][]byte {
	t.Helper()
	if got := w.talk(t, role); !shows(got, rq.page(rq.altar, talkPage)) {
		t.Fatalf("%s: got %q, want %s", role, got, rq.page(rq.altar, talkPage))
	}
	return rq.event(t, w)
}

// event sends the call event from the page the player holds.
func (rq raidQuest) event(t *testing.T, w *dialogWorld) [][]byte {
	t.Helper()
	w.srv.Client.Send(encodeBypass("Quest " + rq.name + " " + rq.page(rq.altar, 2)))
	return w.srv.ReadQueued(t, w.srv.Client)
}

func opcodeIndex(frames [][]byte, opcode byte) int {
	return slices.IndexFunc(frames, func(f []byte) bool { return f[0] == opcode })
}

func raids(w *dialogWorld, rq raidQuest) []*script.NPC { return npcsOf(w.srv, int32(rq.raid)) }

func altars(w *dialogWorld, rq raidQuest) int { return len(npcsOf(w.srv, int32(rq.altar))) }

// TestRaidCallAppearsSaysAndTakesTheAltar: the call with the summon item puts
// the raid at its spot, in front of the player, then makes it say its
// NpcString, then removes the altar; cond becomes 2 and the summon item goes.
func TestRaidCallAppearsSaysAndTakesTheAltar(t *testing.T) {
	t.Parallel()
	for _, rq := range raidQuests {
		t.Run(rq.name, func(t *testing.T) {
			t.Parallel()
			w := rq.boot(t, "1", 1)
			if n := altars(w, rq); n != 2 {
				t.Fatalf("%d altars standing, want 2", n)
			}
			frames := rq.call(t, w, "west", 1)
			found := raids(w, rq)
			if len(found) != 1 {
				t.Fatalf("%d raids after the call, want 1", len(found))
			}
			raid := found[0]
			w.roles[raid.ObjectID()], w.npcs["raid"] = "raid", raid.ObjectID()
			loc := raid.SpawnLocation()
			if loc.X != rq.at[0] || loc.Y != rq.at[1] || abs(loc.Z-rq.at[2]) > 32 || loc.Heading < 0 || loc.Heading >= 65536 {
				t.Fatalf("raid at %+v, want %v with a heading below 65536", loc, rq.at)
			}
			appears, speaks, leaves := opcodeIndex(frames, serverpackets.OpcodeNPCInfo), opcodeIndex(frames, serverpackets.OpcodeNpcSay),
				opcodeIndex(frames, serverpackets.OpcodeDeleteObject)
			if appears < 0 || speaks < 0 || leaves < 0 || appears > speaks || speaks > leaves {
				t.Fatalf("raid shown at %d, said at %d, altar deleted at %d, want that order", appears, speaks, leaves)
			}
			lines := w.render(t, frames)
			if !slices.Contains(lines, say("raid", int32(rq.raid), rq.called)) || !shows(lines, rq.page(rq.altar, 2)) {
				t.Fatalf("lines %q, want the raid's %d and page %s", lines, rq.called, rq.page(rq.altar, 2))
			}
			if n := altars(w, rq); n != 1 || npcsOf(w.srv, int32(rq.altar))[0].ObjectID() != w.npcs["east"] {
				t.Fatalf("%d altars standing after the call, want the east one only", n)
			}
			rq.wantCond(t, w, "the call", "2")
			rq.wantItems(t, w, "the call", 0, 0)
		})
	}
}

// TestRaidCallRefusedWhileTheRaidStands: another altar refuses the call with
// its fourth page, taking nothing, while the raid stands; once the raid has
// despawned it calls a new one. The altar the first call removed stays away,
// its own respawn being 36 hours.
func TestRaidCallRefusedWhileTheRaidStands(t *testing.T) {
	t.Parallel()
	for _, rq := range raidQuests {
		t.Run(rq.name, func(t *testing.T) {
			t.Parallel()
			w := rq.boot(t, "1", 2)
			rq.call(t, w, "west", 1)
			first := raids(w, rq)[0].ObjectID()
			w.roles[first] = "raid"

			lines := w.render(t, rq.call(t, w, "east", 5))
			if !shows(lines, rq.page(rq.altar, 4)) {
				t.Fatalf("lines %q, want page %s", lines, rq.page(rq.altar, 4))
			}
			if n := len(raids(w, rq)); n != 1 || altars(w, rq) != 1 {
				t.Fatalf("%d raids and %d altars after the refused call, want 1 and 1", n, altars(w, rq))
			}
			rq.wantItems(t, w, "the refused call", 1, 0)

			w.advance(t, 1199*time.Second)
			if n := len(raids(w, rq)); n != 1 {
				t.Fatalf("%d raids 1199 s on, want 1", n)
			}
			lines = w.advance(t, 2*time.Second)
			if n := len(raids(w, rq)); n != 0 {
				t.Fatalf("%d raids 1201 s on, want 0", n)
			}
			if n := altars(w, rq); n != 1 {
				t.Fatalf("%d altars after the raid left, want the east one only", n)
			}
			if slices.Contains(lines, say("raid", int32(rq.raid), rq.left)) {
				t.Fatalf("lines %q: the raid spoke %d as it left", lines, rq.left)
			}

			frames := rq.call(t, w, "east", 5)
			if n := len(raids(w, rq)); n != 1 || raids(w, rq)[0].ObjectID() == first {
				t.Fatalf("%d raids after the new call, want a new one", n)
			}
			if opcodeIndex(frames, serverpackets.OpcodeDeleteObject) < 0 {
				t.Fatal("the new call did not remove the east altar")
			}
			rq.wantItems(t, w, "the new call", 0, 0)
		})
	}
}

// TestRaidKilledMovesPlayerOnAndFreesTheCall: the player on cond 2 who kills
// the raid is given the trophy and cond 3; once the corpse has left the world
// (the fixture's monsters keep theirs until the raid's 1200 s are up) an altar
// calls a new raid, and the altar the first call removed stays away.
func TestRaidKilledMovesPlayerOnAndFreesTheCall(t *testing.T) {
	t.Parallel()
	for _, rq := range raidQuests {
		t.Run(rq.name, func(t *testing.T) {
			t.Parallel()
			w := rq.boot(t, "1", 2)
			rq.call(t, w, "west", 1)
			raid := raids(w, rq)[0]
			w.roles[raid.ObjectID()], w.npcs["raid"] = "raid", raid.ObjectID()
			// The player keeps the east altar's window open through the hunt.
			if got := w.talk(t, "east"); !shows(got, rq.page(rq.altar, 5)) {
				t.Fatalf("east: got %q, want %s", got, rq.page(rq.altar, 5))
			}
			h := w.calledHostile(t, "raid", int32(rq.raid), rq.at)
			w.stepTo(t, rq.at)
			w.slay(t, h)
			rq.wantCond(t, w, "the kill", "3")
			rq.wantItems(t, w, "the kill", 1, 1)

			w.advance(t, 1200*time.Second)
			if n := len(raids(w, rq)); n != 0 {
				t.Fatalf("%d raids after the corpse decayed, want 0", n)
			}
			if n := altars(w, rq); n != 1 {
				t.Fatalf("%d altars after the raid died, want the east one only", n)
			}
			rq.event(t, w)
			if n := len(raids(w, rq)); n != 1 {
				t.Fatalf("%d raids after the new call, want 1", n)
			}
			rq.wantItems(t, w, "the new call", 0, 1)
		})
	}
}
