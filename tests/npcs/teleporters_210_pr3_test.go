package npcs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/elrokiteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/grandbossteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/oracleteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/paganteleporter"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The scripted teleporters of #210's third and fourth batches, driven
// through real Action and bypass packets against MariaDB: where each link
// or talk lands the player, what it takes from the inventory as it is
// saved, the player memo the oracles keep, the Seven Signs dungeon flag,
// the doors the gatekeepers open, the attack stance Orahochin refuses, the
// boss cubes' despawns and Klein's count of the players he sent to
// Valakas. The V4 scenarios (tests/quest/testdata/scenarios) replay the
// reference traces of the pages; a scenario seeds no player memo, door,
// zone, attack stance or quest list, and asserts no dungeon flag or
// despawn (#4048), so those are here.

// The teleporters' NPCs and items, as the reference scripts have them.
const (
	tpDawnTown    = 31078
	tpDuskTown    = 31085
	tpTemple      = 31127
	tpZiggurat    = 31095
	tpTempleGK    = 32034
	tpTempleOut   = 32035
	tpChapelGK    = 32036
	tpChapelOut   = 32037
	tpMirror      = 32039
	tpMirrorTown  = 32040
	tpOrahochin   = 32111
	tpGariachin   = 32112
	tpBenomCube   = 29055
	tpFrintCube   = 29061
	tpBaiumCube   = 31842
	tpAntharasCub = 31859
	tpValakasCube = 31759
	tpSailrenCube = 32107
	tpKlein       = 31540
	tpFireGK1     = 31384
	tpFireGK2     = 31686
	tpFireGK3     = 31687
	tpValakas     = 29028

	tpFragment     = 7079
	tpVisitorMark  = 8064
	tpPaganMark    = 8067
	tpFloatingStne = 7267

	tpOracleMemo = "OracleTeleport_Npc"
	tpBaiumZone  = 110002
)

var (
	tpDawnFestival = location.Location{X: -80157, Y: 111344, Z: -4901}
	tpDuskFestival = location.Location{X: -81261, Y: 86531, Z: -5157}
	tpRiftEntrance = location.Location{X: -114755, Y: -179466, Z: -6752}
	tpRiftRoom     = location.Location{X: -114796, Y: -179334, Z: -6752}
)

// oracleSetup is what an oracle world's character brings into the world.
type oracleSetup struct {
	// level is the character's level; 0 is the fixture's.
	level int
	// held is what the character holds, template id to count.
	held map[int32]int32
	// memo is the character's saved oracle memo, "" for none.
	memo string
	// quests is how many real quests the character has started.
	quests int
}

// oracleItems is the fixture catalog with the teleporters' fragment, marks
// and floating stone.
func oracleItems() *item.Table {
	etc := func(id int32) *item.Template {
		return &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		}
	}
	all := gameservertest.ItemTemplates().All()
	for _, id := range []int32{tpFragment, tpVisitorMark, tpPaganMark, tpFloatingStne} {
		all = append(all, etc(id))
	}
	return item.NewTable(all)
}

// stubQuest is the path of the i-th fixture quest a character can have
// started.
func stubQuest(i int) string { return fmt.Sprintf("quest.Q%d_StartedStub", 900+i) }

// bootOracles boots the four teleporters with their datapack pages, the
// admit-all page and 25 fixture quests, seeds what setup asks, and enters
// the world.
func bootOracles(t *testing.T, setup oracleSetup) *folkWorld {
	t.Helper()
	datapack.Require(t)
	pages := dialogPages()
	for _, name := range []string{"OracleTeleporter", "PaganTeleporter", "ElrokiTeleporter", "GrandBossTeleporter"} {
		dir := datapack.Path(t, "data", "html", "script", "teleport", name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(dir + "/" + e.Name())
			if err != nil {
				t.Fatal(err)
			}
			pages["script/teleport/"+name+"/"+e.Name()] = string(data)
		}
	}
	catalog := script.Catalog{
		"script.teleport.ElrokiTeleporter":    elrokiteleporter.New,
		"script.teleport.GrandBossTeleporter": grandbossteleporter.New,
		"script.teleport.OracleTeleporter":    oracleteleporter.New,
		"script.teleport.PaganTeleporter":     paganteleporter.New,
	}
	for i := range 25 {
		catalog[stubQuest(i)] = func() script.Script { return script.Script{QuestID: int32(900 + i), Title: "Started stub"} }
	}
	var list []script.Listing
	for _, path := range slices.Sorted(maps.Keys(catalog)) {
		list = append(list, script.Listing{Path: path})
	}
	kinds := map[int32]script.NPCKind{tpValakas: script.KindHostile}
	for _, id := range []int32{
		tpDawnTown, tpDuskTown, tpTemple, tpZiggurat, tpTempleGK, tpTempleOut, tpChapelGK, tpChapelOut, tpMirror, tpMirrorTown,
		tpOrahochin, tpGariachin, tpBenomCube, tpFrintCube, tpBaiumCube, tpAntharasCub, tpValakasCube, tpSailrenCube,
		tpKlein, tpFireGK1, tpFireGK2, tpFireGK3,
	} {
		kinds[id] = script.KindFolk
	}
	level := setup.level
	if level == 0 {
		level = playerLevel
	}
	opts := []gameservertest.Option{
		gameservertest.WithCharacter("Talker", level, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithItemTemplates(oracleItems()),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{raidTemplate(tpValakas)})),
		gameservertest.WithNpcSpawns(nil),
		noBypassReuse,
	}
	srv := gameservertest.Boot(t, opts...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	ctx := context.Background()
	if setup.memo != "" {
		if _, err := srv.DB.ExecContext(ctx, "INSERT INTO character_memo (charId,var,val) VALUES (?,?,?)", w.player, tpOracleMemo, setup.memo); err != nil {
			t.Fatal(err)
		}
	}
	for i := range setup.quests {
		name := stubQuest(i)[len("quest."):]
		if _, err := srv.DB.ExecContext(ctx, "INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)", w.player, name, "<state>", "STARTED"); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(setup.held)) {
		srv.GiveItem(t, w.player, id, setup.held[id])
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w
}

// oracleMemo returns the player's saved oracle memo, "" for none.
func (w *folkWorld) oracleMemo(t *testing.T) string {
	t.Helper()
	w.srv.FlushPersistence(t)
	var val string
	err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT val FROM character_memo WHERE charId=? AND var=?", w.player, tpOracleMemo).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return val
}

// in7sDungeon reports the player's Seven Signs dungeon flag.
func (w *folkWorld) in7sDungeon(t *testing.T) bool {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	return obj.(player.CharacterHolder).PlayerCharacter().In7sDungeon()
}

// teleportPage is the teleporter's datapack page file, as n shows it.
func teleportPage(t *testing.T, n *npc.Folk, name, file string) string {
	t.Helper()
	return scriptPage(t, "script/teleport/"+name+"/"+file, n)
}

// wantTalkAnswer requires frames, after the releases a talk may send
// first, to be page shown through n and the two releases of a talk's
// answer.
func wantTalkAnswer(t *testing.T, frames [][]byte, n *npc.Folk, page string) {
	t.Helper()
	frames = dialogFrames(frames)
	if i := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeNpcHtmlMessage }); i > 0 {
		frames = frames[i:]
	}
	wantPage(t, frames, n, page, 2)
}

// requireNoTrip requires frames to hold no jump of the player.
func (w *folkWorld) requireNoTrip(t *testing.T, frames [][]byte) {
	t.Helper()
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeTeleportToLocation && int32(wire.NewReader(f[1:]).ReadInt32()) == w.player {
			t.Fatalf("the player jumped: %x", opcodes(frames))
		}
	}
}

// requireTripNear requires frames to move the player to within offset of
// one of spots on each axis, at its Z, and returns where it landed.
func (w *folkWorld) requireTripNear(t *testing.T, frames [][]byte, offset int, spots ...location.Location) location.Location {
	t.Helper()
	got := w.landedAt(t, frames)
	ok := slices.ContainsFunc(spots, func(s location.Location) bool {
		return got.X >= s.X-offset && got.X <= s.X+offset && got.Y >= s.Y-offset && got.Y <= s.Y+offset && got.Z == s.Z
	})
	if !ok {
		t.Fatalf("landed at %+v, want within %d of one of %+v", got, offset, spots)
	}
	w.at = got
	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)
	return got
}

// TestOracleTeleporterReturn: a temple priest's return sends the player
// back to the point of the NPC its memo names and clears the dungeon flag;
// another NPC's return keeps the flag. With no memo nothing happens; a
// memo naming an NPC of no return point aborts before the trip.
func TestOracleTeleporterReturn(t *testing.T) {
	t.Parallel()
	t.Run("temple priest", func(t *testing.T) {
		t.Parallel()
		w := bootOracles(t, oracleSetup{})
		town := w.spawnFolk(t, folkTemplate("DawnPriest", tpDawnTown), 50)
		w.openThrough(t, town)
		w.requireTrip(t, w.tripFrames(t, npcCommand(town, "Quest OracleTeleporter")), tpDawnFestival)
		temple := w.spawnFolk(t, folkTemplate("FestivalGuide", tpTemple), 50)
		w.openThrough(t, temple)
		w.requireTrip(t, w.tripFrames(t, "Quest OracleTeleporter Return"), location.Location{X: -80555, Y: 150337, Z: -3040})
		if w.in7sDungeon(t) {
			t.Fatal("the temple priest's return kept the dungeon flag")
		}
		if got := w.oracleMemo(t); got != strconv.Itoa(tpDawnTown) {
			t.Fatalf("saved memo after the return = %q, want %d kept", got, tpDawnTown)
		}
	})
	t.Run("ziggurat gatekeeper", func(t *testing.T) {
		t.Parallel()
		w := bootOracles(t, oracleSetup{})
		town := w.spawnFolk(t, folkTemplate("DuskPriest", tpDuskTown), 50)
		w.openThrough(t, town)
		w.requireTrip(t, w.tripFrames(t, npcCommand(town, "Quest OracleTeleporter")), tpDuskFestival)
		gk := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", tpZiggurat), 50)
		w.openThrough(t, gk)
		w.requireTrip(t, w.tripFrames(t, "Quest OracleTeleporter RETURN"), location.Location{X: -82368, Y: 151568, Z: -3120})
		if !w.in7sDungeon(t) {
			t.Fatal("a gatekeeper's return cleared the dungeon flag")
		}
	})
	t.Run("no memo", func(t *testing.T) {
		t.Parallel()
		w := bootOracles(t, oracleSetup{})
		temple := w.spawnFolk(t, folkTemplate("FestivalGuide", tpTemple), 50)
		w.openThrough(t, temple)
		frames := w.tripFrames(t, "Quest OracleTeleporter Return")
		w.requireNoTrip(t, frames)
		if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
			t.Fatalf("the return answered a page: %x", opcodes(frames))
		}
	})
	t.Run("no return point", func(t *testing.T) {
		t.Parallel()
		w := bootOracles(t, oracleSetup{memo: strconv.Itoa(tpTemple)})
		temple := w.spawnFolk(t, folkTemplate("FestivalGuide", tpTemple), 50)
		w.openThrough(t, temple)
		w.requireNoTrip(t, w.tripFrames(t, "Quest OracleTeleporter Return"))
		if got := w.oracleMemo(t); got != strconv.Itoa(tpTemple) {
			t.Fatalf("saved memo = %q, want %d kept", got, tpTemple)
		}
	})
}

// TestOracleTeleporterFestival: the festival link sends a player whose
// memo names a town priest into that priest's side's necropolis and marks
// it inside a Seven Signs dungeon; any other player gets oracle1.htm.
func TestOracleTeleporterFestival(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		memo string
		want *location.Location
	}{
		{name: "dawn", memo: "31692", want: &tpDawnFestival},
		{name: "dusk", memo: "31169", want: &tpDuskFestival},
		{name: "ziggurat", memo: strconv.Itoa(tpZiggurat)},
		{name: "none"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootOracles(t, oracleSetup{memo: tt.memo})
			temple := w.spawnFolk(t, folkTemplate("FestivalGuide", tpTemple), 50)
			w.openThrough(t, temple)
			frames := w.tripFrames(t, "Quest OracleTeleporter festival")
			if tt.want == nil {
				w.requireNoTrip(t, frames)
				wantPage(t, frames, temple, teleportPage(t, temple, "OracleTeleporter", "oracle1.htm"), 1)
				if w.in7sDungeon(t) {
					t.Fatal("a refused player is marked inside a Seven Signs dungeon")
				}
				return
			}
			w.requireTrip(t, frames, *tt.want)
			if !w.in7sDungeon(t) {
				t.Fatal("the player is not marked inside a Seven Signs dungeon")
			}
		})
	}
}

// TestOracleTeleporterRiftLinks: 5.htm sends the player to the rift's
// entrance, answering 5a.htm to a player that already had a memo and no
// page to one that had none, and remembers the NPC; Dimensional answers
// oracle.htm and sends the player to the rift's waiting room, memo left
// alone.
func TestOracleTeleporterRiftLinks(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		memo  string
		event string
		want  location.Location
		page  string
		saved string
	}{
		{name: "5.htm without memo", event: "5.htm", want: tpRiftEntrance, saved: strconv.Itoa(tpZiggurat)},
		{name: "5.htm with memo", memo: "31081", event: "5.HTM", want: tpRiftEntrance, page: "5a.htm", saved: strconv.Itoa(tpZiggurat)},
		{name: "dimensional", memo: "31081", event: "Dimensional", want: tpRiftRoom, page: "oracle.htm", saved: "31081"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootOracles(t, oracleSetup{memo: tt.memo})
			gk := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", tpZiggurat), 50)
			w.openThrough(t, gk)
			frames := w.tripFrames(t, "Quest OracleTeleporter "+tt.event)
			if tt.page == "" {
				if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
					t.Fatalf("the trip answered a page: %x", opcodes(frames))
				}
			} else {
				page, _ := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
				if page == nil {
					t.Fatalf("the trip answered no page: %x", opcodes(frames))
				}
				if obj, html, _ := htmlMessage(t, page); obj != gk.ObjectID() || html != teleportPage(t, gk, "OracleTeleporter", tt.page) {
					t.Fatalf("page through %d = %q, want %s through %d", obj, html, tt.page, gk.ObjectID())
				}
			}
			w.requireTrip(t, frames, tt.want)
			if got := w.oracleMemo(t); got != tt.saved {
				t.Fatalf("saved memo = %q, want %q", got, tt.saved)
			}
		})
	}
}

// TestOracleTeleporterZigguratFees: the rift link takes the fee of the
// player's level band, whatever it holds, answers ziggurat_rift.htm,
// remembers the gatekeeper and sends the player to the rift's entrance; a
// player short of the fee loses all it holds, and one below level 20 pays
// nothing.
func TestOracleTeleporterZigguratFees(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		level int
		held  int32
		left  int
	}{
		{level: 19, held: 30000, left: 30000},
		{level: 20, held: 30000, left: 28000},
		{level: 39, held: 30000, left: 25500},
		{level: 40, held: 30000, left: 22000},
		{level: 59, held: 30000, left: 17500},
		{level: 60, held: 30000, left: 12000},
		{level: 70, held: 30000, left: 5500},
		{level: 70, held: 1000, left: 0},
	} {
		t.Run(fmt.Sprintf("level %d holding %d", tt.level, tt.held), func(t *testing.T) {
			t.Parallel()
			w := bootOracles(t, oracleSetup{level: tt.level, held: map[int32]int32{int32(item.AdenaID): tt.held}})
			gk := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", tpZiggurat), 50)
			w.openThrough(t, gk)
			frames := w.tripFrames(t, "Quest OracleTeleporter zigurratDimensional")
			page, _ := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
			if page == nil {
				t.Fatalf("the rift link answered no page: %x", opcodes(frames))
			}
			if _, html, _ := htmlMessage(t, page); html != teleportPage(t, gk, "OracleTeleporter", "ziggurat_rift.htm") {
				t.Fatalf("page = %q, want ziggurat_rift.htm", html)
			}
			w.requireTrip(t, frames, tpRiftEntrance)
			w.requireSaved(t, map[int32]int{int32(item.AdenaID): tt.left})
			if got := w.oracleMemo(t); got != strconv.Itoa(tpZiggurat) {
				t.Fatalf("saved memo = %q, want %d", got, tpZiggurat)
			}
		})
	}
}

// TestOracleTeleporterZigguratGatekeeperPages: the gatekeeper refuses a
// player below level 20, tells one at the quest limit so, refuses one
// without a dimension fragment and one short of its level band's fee, and
// offers the rift to anyone else. The checks of the higher bands apply to
// a lower level too, so only a player holding the highest band's fee is
// offered the rift. Nothing is taken and no one moves.
func TestOracleTeleporterZigguratGatekeeperPages(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		level  int
		quests int
		adena  int32
		frag   bool
		page   string
	}{
		{name: "low level", level: 19, adena: 30000, frag: true, page: "ziggurat_lowlevel.htm"},
		{name: "quest limit", level: 20, quests: 25, adena: 30000, frag: true},
		{name: "24 quests", level: 20, quests: 24, adena: 30000, frag: true, page: "ziggurat.htm"},
		{name: "no fragment", level: 20, adena: 30000, page: "ziggurat_nofrag.htm"},
		{name: "level 20 short", level: 20, adena: 1999, frag: true, page: "ziggurat_noadena.htm"},
		{name: "level 20 fee", level: 20, adena: 2000, frag: true, page: "ziggurat_noadena.htm"},
		{name: "level 20 highest fee", level: 20, adena: 24500, frag: true, page: "ziggurat.htm"},
		{name: "level 35 short", level: 35, adena: 4499, frag: true, page: "ziggurat_noadena.htm"},
		{name: "level 45 short", level: 45, adena: 7999, frag: true, page: "ziggurat_noadena.htm"},
		{name: "level 55 short", level: 55, adena: 12499, frag: true, page: "ziggurat_noadena.htm"},
		{name: "level 65 short", level: 65, adena: 17999, frag: true, page: "ziggurat_noadena.htm"},
		{name: "level 75 short", level: 75, adena: 24499, frag: true, page: "ziggurat_noadena.htm"},
		{name: "level 75 fee", level: 75, adena: 24500, frag: true, page: "ziggurat.htm"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			held := map[int32]int32{int32(item.AdenaID): tt.adena}
			if tt.frag {
				held[tpFragment] = 1
			}
			w := bootOracles(t, oracleSetup{level: tt.level, held: held, quests: tt.quests})
			gk := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", tpZiggurat), 50)
			w.openThrough(t, gk)
			frames := w.tripFrames(t, npcCommand(gk, "Quest OracleTeleporter"))
			w.requireNoTrip(t, frames)
			if tt.page == "" {
				if got := systemMessageIDs(frames); !slices.Equal(got, []int32{401}) {
					t.Fatalf("system messages = %v, want [401]", got)
				}
				if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
					t.Fatalf("the quest limit answered a page: %x", opcodes(frames))
				}
			} else {
				wantTalkAnswer(t, frames, gk, teleportPage(t, gk, "OracleTeleporter", tt.page))
			}
			want := map[int32]int{int32(item.AdenaID): int(tt.adena)}
			if tt.frag {
				want[tpFragment] = 1
			}
			w.requireSaved(t, want)
		})
	}
}

// TestPaganTeleporterMirrors: Triol's mirrors send the player between the
// temple and the town, answering no page.
func TestPaganTeleporterMirrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		npcID int
		want  location.Location
	}{
		{npcID: tpMirror, want: location.Location{X: 34962, Y: -49758, Z: -763}},
		{npcID: tpMirrorTown, want: location.Location{X: -12766, Y: -35840, Z: -10856}},
	} {
		t.Run(strconv.Itoa(tt.npcID), func(t *testing.T) {
			t.Parallel()
			w := bootOracles(t, oracleSetup{})
			mirror := w.spawnFolk(t, folkTemplate("Folk", tt.npcID), 50)
			w.openThrough(t, mirror)
			frames := w.tripFrames(t, npcCommand(mirror, "Quest PaganTeleporter"))
			if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
				t.Fatalf("the trip answered a page: %x", opcodes(frames))
			}
			w.requireTrip(t, frames, tt.want)
		})
	}
}
