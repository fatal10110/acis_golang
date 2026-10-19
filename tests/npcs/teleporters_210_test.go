package npcs

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/charmteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/dimensionalvortex"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/elmoredencemeteryteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/gatekeeperspirit"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/huntinggroundsteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/monsterracetrack"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The scripted teleporters of #210's second batch, driven through real
// Action and bypass packets against MariaDB: where each link or talk lands
// the player, what it takes from the inventory as it is saved, the player
// memo the Monster Race Track trip keeps, the Seven Signs reads of the
// gatekeeper spirits and the priests, and the exit spirit a raid boss's
// death brings. The V4 scenarios (tests/quest/testdata/scenarios) replay
// the reference traces of the pages and messages; a scenario asserts no
// landing, seal owner or spawned NPC (#3946), so those are here.

// The teleporters' NPCs, items and destinations, as the reference scripts
// have them.
const (
	tpWhirpy      = 30540
	tpTamil       = 30576
	tpVortex      = 30952
	tpChamberlain = 31919
	tpSpiritIn    = 31111
	tpSpiritOut   = 31112
	tpLilith      = 25283
	tpAnakim      = 25286
	tpDawnPriest  = 31078
	tpDuskPriest  = 31085
	tpTrisha      = 30059
	tpBilia       = 31964
	tpRaceManager = 30995

	tpOrcCharm   = 1658
	tpDwarfToken = 1659
	tpGreenStone = 4401
	tpBlueStone  = 4402
	tpRedStone   = 4403
	tpGravePass  = 7261
	tpBrooch     = 7262

	tpRaceMemo = "MonsterRaceTrack_Npc"
)

var (
	tpGludinVillage  = location.Location{X: -80826, Y: 149775, Z: -3043}
	tpFourSepulchers = location.Location{X: 178127, Y: -84435, Z: -7215}
	tpImperialTomb   = location.Location{X: 186699, Y: -75915, Z: -2826}
	tpRaceTrack      = location.Location{X: 12661, Y: 181687, Z: -3560}
)

// teleporterSetup is what a teleporter world's character brings into the
// world.
type teleporterSetup struct {
	// held is what the character holds, template id to count.
	held map[int32]int32
	// memo is the character's saved Monster Race Track memo, "" for none.
	memo string
	// cabal is the character's Seven Signs sign-up.
	cabal sevensigns.Cabal
	// status edits the Seven Signs status row before the state restores
	// it; nil keeps the installed row.
	status func(*sevensigns.StatusRow)
	// teleports are the gatekeepers' destination lists.
	teleports travel.TeleportTable
}

// teleporterItems is the fixture catalog with the teleporters' tokens,
// stones, pass and brooch.
func teleporterItems() *item.Table {
	etc := func(id int32) *item.Template {
		return &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		}
	}
	all := gameservertest.ItemTemplates().All()
	for _, id := range []int32{tpOrcCharm, tpDwarfToken, tpGreenStone, tpBlueStone, tpRedStone, tpGravePass, tpBrooch} {
		all = append(all, etc(id))
	}
	return item.NewTable(all)
}

// raidTemplate is a level 1 raid boss of 1000 HP that stands still.
func raidTemplate(id int) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "RaidBoss", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

// bootTeleporters boots the six teleporters with their datapack pages and
// the admit-all page, seeds what setup asks, and enters the world.
func bootTeleporters(t *testing.T, setup teleporterSetup) *folkWorld {
	t.Helper()
	datapack.Require(t)
	pages := dialogPages()
	addPage := func(rel string) {
		data, err := os.ReadFile(datapack.Path(t, append([]string{"data", "html"}, strings.Split(rel, "/")...)...))
		if err != nil {
			t.Fatal(err)
		}
		pages[rel] = string(data)
	}
	addPage("gatekeeper/31111.htm")
	for _, name := range []string{"CharmTeleporter", "DimensionalVortex", "ElmoredenCemeteryTeleporter", "GatekeeperSpirit", "HuntingGroundsTeleporter"} {
		entries, err := os.ReadDir(datapack.Path(t, "data", "html", "script", "teleport", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			addPage("script/teleport/" + name + "/" + e.Name())
		}
	}
	catalog := script.Catalog{
		"script.teleport.CharmTeleporter":             charmteleporter.New,
		"script.teleport.DimensionalVortex":           dimensionalvortex.New,
		"script.teleport.ElmoredenCemeteryTeleporter": elmoredencemeteryteleporter.New,
		"script.teleport.GatekeeperSpirit":            gatekeeperspirit.New,
		"script.teleport.HuntingGroundsTeleporter":    huntinggroundsteleporter.New,
		"script.teleport.MonsterRaceTrack":            monsterracetrack.New,
	}
	var list []script.Listing
	for _, path := range slices.Sorted(maps.Keys(catalog)) {
		list = append(list, script.Listing{Path: path})
	}
	kinds := map[int32]script.NPCKind{tpLilith: script.KindHostile, tpAnakim: script.KindHostile}
	for _, id := range []int32{
		tpWhirpy, tpTamil, tpVortex, tpChamberlain, tpSpiritIn, tpSpiritOut, tpDawnPriest, tpDuskPriest,
		tpTrisha, tpBilia, tpRaceManager,
	} {
		kinds[id] = script.KindFolk
	}
	opts := []gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithItemTemplates(teleporterItems()),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("DungeonGatekeeper", tpSpiritOut), raidTemplate(tpLilith), raidTemplate(tpAnakim),
		})),
		gameservertest.WithNpcSpawns(nil),
		noBypassReuse,
	}
	if setup.status != nil {
		opts = append(opts, withSevenSignsStatus(t, setup.status))
	}
	if setup.teleports != nil {
		opts = append(opts, gameservertest.WithTeleports(setup.teleports, nil, false, newTeleportClock().now))
	}
	srv := gameservertest.Boot(t, opts...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	ctx := context.Background()
	if setup.memo != "" {
		if _, err := srv.DB.ExecContext(ctx, "INSERT INTO character_memo (charId,var,val) VALUES (?,?,?)", w.player, tpRaceMemo, setup.memo); err != nil {
			t.Fatal(err)
		}
	}
	if setup.cabal != sevensigns.NoCabal {
		if err := srv.SevenSigns.SetPlayerInfo(ctx, w.player, setup.cabal, sevensigns.Avarice); err != nil {
			t.Fatalf("sign up: %v", err)
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

// openThrough talks to n, so it is the last quest NPC, then opens the
// admit-all page.
func (w *folkWorld) openThrough(t *testing.T, n *npc.Folk) {
	t.Helper()
	w.talkTo(t, n)
	w.openAnyNpcPage(t)
}

// landedAt returns where the player's one TeleportToLocation among frames
// lands, and fails when there is not exactly one.
func (w *folkWorld) landedAt(t *testing.T, frames [][]byte) location.Location {
	t.Helper()
	var out []location.Location
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != w.player {
			continue
		}
		out = append(out, location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())})
	}
	if len(out) != 1 {
		t.Fatalf("player jumps = %+v among %x, want one", out, opcodes(frames))
	}
	return out[0]
}

// requireTrip requires frames to move the player to want, at no offset,
// and the player to stand there afterwards; the client then appears there,
// so it can act again.
func (w *folkWorld) requireTrip(t *testing.T, frames [][]byte, want location.Location) {
	t.Helper()
	if got := w.landedAt(t, frames); got != want {
		t.Fatalf("landed at %+v, want %+v", got, want)
	}
	x, y, z := w.srv.PlayerPosition(t, w.player)
	if got := (location.Location{X: x, Y: y, Z: z}); got != want {
		t.Fatalf("player at %+v, want %+v", got, want)
	}
	w.at = want
	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)
}

// requireSaved requires the persisted count of each template of want.
func (w *folkWorld) requireSaved(t *testing.T, want map[int32]int) {
	t.Helper()
	for _, id := range slices.Sorted(maps.Keys(want)) {
		if got := w.savedCount(t, id); got != want[id] {
			t.Fatalf("saved count of %d = %d, want %d", id, got, want[id])
		}
	}
}

// memoRow returns the player's saved Monster Race Track memo, "" for none.
func (w *folkWorld) memoRow(t *testing.T) string {
	t.Helper()
	w.srv.FlushPersistence(t)
	var val string
	err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT val FROM character_memo WHERE charId=? AND var=?", w.player, tpRaceMemo).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return val
}

// TestCharmTeleporterTakesItsTokenAndMoves: Whirpy takes one dwarf
// gatekeeper token and Tamil one orc gatekeeper charm, each leaving the
// other gatekeeper's item alone, and both send the player to the Village
// of Gludin at no offset, answering no page.
func TestCharmTeleporterTakesItsTokenAndMoves(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		npcID int
		saved map[int32]int
	}{
		{name: "whirpy", npcID: tpWhirpy, saved: map[int32]int{tpDwarfToken: 1, tpOrcCharm: 2}},
		{name: "tamil", npcID: tpTamil, saved: map[int32]int{tpDwarfToken: 2, tpOrcCharm: 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootTeleporters(t, teleporterSetup{held: map[int32]int32{tpDwarfToken: 2, tpOrcCharm: 2}})
			gk := w.spawnFolk(t, folkTemplate("Gatekeeper", tt.npcID), 50)
			w.openThrough(t, gk)
			frames := w.tripFrames(t, npcCommand(gk, "Quest CharmTeleporter"))
			if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
				t.Fatalf("the trip answered a page: %x", opcodes(frames))
			}
			w.requireTrip(t, frames, tpGludinVillage)
			w.requireSaved(t, tt.saved)
		})
	}
}

// TestDimensionalVortexLinksTakeTheirStone: each colour's link, in any
// case, takes one stone of that colour and sends the player to its floor;
// the other stones stay.
func TestDimensionalVortexLinksTakeTheirStone(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		event string
		stone int32
		want  location.Location
	}{
		{event: "blue", stone: tpBlueStone, want: location.Location{X: 114097, Y: 19935, Z: 935}},
		{event: "GREEN", stone: tpGreenStone, want: location.Location{X: 110930, Y: 15963, Z: -4378}},
		{event: "Red", stone: tpRedStone, want: location.Location{X: 118558, Y: 16659, Z: 5987}},
	} {
		t.Run(tt.event, func(t *testing.T) {
			t.Parallel()
			w := bootTeleporters(t, teleporterSetup{held: map[int32]int32{tpGreenStone: 1, tpBlueStone: 1, tpRedStone: 1}})
			vortex := w.spawnFolk(t, folkTemplate("Folk", tpVortex), 50)
			w.openThrough(t, vortex)
			w.requireTrip(t, w.tripFrames(t, "Quest DimensionalVortex "+tt.event), tt.want)
			saved := map[int32]int{tpGreenStone: 1, tpBlueStone: 1, tpRedStone: 1}
			saved[tt.stone] = 0
			w.requireSaved(t, saved)
		})
	}
}

// TestElmoredenCemeteryTeleporterDestinations: an antique brooch goes to
// either destination for free, its holder's grave pass kept; without one a
// used grave pass is taken; the event's case does not matter.
func TestElmoredenCemeteryTeleporterDestinations(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		event string
		held  map[int32]int32
		want  location.Location
		saved map[int32]int
	}{
		{
			name: "brooch", event: "4S", held: map[int32]int32{tpBrooch: 1, tpGravePass: 1},
			want: tpFourSepulchers, saved: map[int32]int{tpBrooch: 1, tpGravePass: 1},
		},
		{
			name: "brooch lower case", event: "it", held: map[int32]int32{tpBrooch: 1, tpGravePass: 1},
			want: tpImperialTomb, saved: map[int32]int{tpBrooch: 1, tpGravePass: 1},
		},
		{
			name: "grave pass", event: "4s", held: map[int32]int32{tpGravePass: 2},
			want: tpFourSepulchers, saved: map[int32]int{tpBrooch: 0, tpGravePass: 1},
		},
		{
			name: "last grave pass", event: "IT", held: map[int32]int32{tpGravePass: 1},
			want: tpImperialTomb, saved: map[int32]int{tpBrooch: 0, tpGravePass: 0},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootTeleporters(t, teleporterSetup{held: tt.held})
			chamberlain := w.spawnFolk(t, folkTemplate("Folk", tpChamberlain), 50)
			w.openThrough(t, chamberlain)
			w.requireTrip(t, w.tripFrames(t, "Quest ElmoredenCemeteryTeleporter "+tt.event), tt.want)
			w.requireSaved(t, tt.saved)
		})
	}
}

// TestGatekeeperSpiritCabalPages: the entrance spirit gives the cabal's
// page only to a member of the cabal that both owns the Seal of Avarice and
// wins; anyone else gets the spirit's own chat window.
func TestGatekeeperSpiritCabalPages(t *testing.T) {
	t.Parallel()
	status := func(owner sevensigns.Cabal, dawn, dusk float64) func(*sevensigns.StatusRow) {
		return func(row *sevensigns.StatusRow) {
			row.SealOwners[0] = owner
			row.DawnStoneScore, row.DuskStoneScore = dawn, dusk
		}
	}
	for _, tt := range []struct {
		name   string
		cabal  sevensigns.Cabal
		status func(*sevensigns.StatusRow)
		page   string
		// releases follow the page: the hook's answer releases once; the
		// chat window releases, then the empty answer once more.
		releases int
	}{
		{name: "dawn", cabal: sevensigns.Dawn, status: status(sevensigns.Dawn, 100, 0), page: "script/teleport/GatekeeperSpirit/dawn.htm", releases: 1},
		{name: "dusk", cabal: sevensigns.Dusk, status: status(sevensigns.Dusk, 0, 100), page: "script/teleport/GatekeeperSpirit/dusk.htm", releases: 1},
		{name: "owner losing", cabal: sevensigns.Dawn, status: status(sevensigns.Dawn, 0, 100), page: "gatekeeper/31111.htm", releases: 2},
		{name: "winner not owning", cabal: sevensigns.Dusk, status: status(sevensigns.Dawn, 0, 100), page: "gatekeeper/31111.htm", releases: 2},
		{name: "no cabal", status: status(sevensigns.NoCabal, 0, 0), page: "gatekeeper/31111.htm", releases: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootTeleporters(t, teleporterSetup{cabal: tt.cabal, status: tt.status})
			spirit := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", tpSpiritIn), 50)
			w.selectFolk(t, spirit)
			frames := dialogFrames(w.talk(t, spirit, false))
			if len(frames) > 0 && frames[0][0] == serverpackets.OpcodeActionFailed {
				frames = frames[1:] // the talk's own release
			}
			wantPage(t, frames, spirit, scriptPage(t, tt.page, spirit), tt.releases)
		})
	}
}

// liveCount counts the NPCs of template id in the world.
func liveCount(w *folkWorld, id int) int {
	n := 0
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.Template().ID == id {
			n++
		}
	}
	return n
}

// TestGatekeeperSpiritExitAfterRaidDeath: the dying hook runs three
// seconds after Lilith or Anakim dies, as every dying hook does, and ten
// seconds later, on a timer bound to no NPC, the exit spirit appears at the
// raid boss's point; fifteen minutes later it is gone.
func TestGatekeeperSpiritExitAfterRaidDeath(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		boss int
		want location.Location
	}{
		{name: "lilith", boss: tpLilith, want: location.Location{X: 184446, Y: -10112, Z: -5488}},
		{name: "anakim", boss: tpAnakim, want: location.Location{X: 184466, Y: -13106, Z: -5488}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootTeleporters(t, teleporterSetup{})
			boss := w.srv.SpawnHostileNPCTemplateAt(t, raidTemplate(tt.boss), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			obj, ok := w.srv.State.Player(w.player)
			if !ok {
				t.Fatal("the character is not in the world")
			}
			p := obj.(attackable.Combatant)
			done := make(chan struct{})
			w.srv.PlayerQueue(t, w.player).Post(func() {
				defer close(done)
				boss.TakeDamage(boss.MaxHP()+1, p)
			})
			<-done
			w.srv.Advance(t, 12900*time.Millisecond)
			if n := liveCount(w, tpSpiritOut); n != 0 {
				t.Fatalf("exit spirits before thirteen seconds = %d, want 0", n)
			}
			w.srv.Advance(t, 100*time.Millisecond)
			if n := liveCount(w, tpSpiritOut); n != 1 {
				t.Fatalf("exit spirits after thirteen seconds = %d, want 1", n)
			}
			exit := liveOf(t, w.srv, tpSpiritOut).(*npc.Folk)
			x, y, _ := exit.Position()
			if x != tt.want.X || y != tt.want.Y {
				t.Fatalf("exit spirit at %d,%d, want %d,%d", x, y, tt.want.X, tt.want.Y)
			}
			w.srv.Advance(t, 15*time.Minute-time.Second)
			if n := liveCount(w, tpSpiritOut); n != 1 {
				t.Fatalf("exit spirits before fifteen minutes = %d, want 1", n)
			}
			w.srv.Advance(t, time.Second)
			if n := liveCount(w, tpSpiritOut); n != 0 {
				t.Fatalf("exit spirits after fifteen minutes = %d, want 0", n)
			}
		})
	}
}

// TestHuntingGroundsTeleporterOpensWindowForCabalMember: a priest of either
// side opens its standard teleport window for a player of either cabal.
func TestHuntingGroundsTeleporterOpensWindowForCabalMember(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		kind  string
		npcID int
		cabal sevensigns.Cabal
	}{
		{name: "dawn priest", kind: "DawnPriest", npcID: tpDawnPriest, cabal: sevensigns.Dawn},
		{name: "dusk priest", kind: "DuskPriest", npcID: tpDuskPriest, cabal: sevensigns.Dawn},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spot := location.Location{X: -41567, Y: 209463, Z: -5080}
			teleports := travel.TeleportTable{tt.npcID: {
				{Location: spot, Description: "Necropolis of Sacrifice", Kind: travel.KindStandard, PriceID: int(item.AdenaID), PriceCount: 2000},
			}}
			w := bootTeleporters(t, teleporterSetup{cabal: tt.cabal, teleports: teleports})
			priest := w.spawnFolk(t, folkTemplate(tt.kind, tt.npcID), 50)
			w.openThrough(t, priest)
			window := "<html><body>&$556;<br><br>" +
				`<a action="bypass -h npc_` + strconv.Itoa(int(priest.ObjectID())) +
				`_teleport 0" msg="811;Necropolis of Sacrifice">Necropolis of Sacrifice - 2000 &#57;</a><br1>` +
				"</body></html>"
			wantPage(t, w.bypass(t, npcCommand(priest, "Quest HuntingGroundsTeleporter")), priest, window, 1)
		})
	}
}

// TestMonsterRaceTrackRoundTrip: a town gatekeeper sends the player to the
// track and saves itself as the player's memo; the race manager sends the
// player back to that gatekeeper's town and deletes the memo.
func TestMonsterRaceTrackRoundTrip(t *testing.T) {
	t.Parallel()
	w := bootTeleporters(t, teleporterSetup{})
	trisha := w.spawnFolk(t, folkTemplate("Gatekeeper", tpTrisha), 50)
	w.openThrough(t, trisha)
	w.requireTrip(t, w.tripFrames(t, npcCommand(trisha, "Quest MonsterRaceTrack")), tpRaceTrack)
	if got := w.memoRow(t); got != "30059" {
		t.Fatalf("saved memo = %q, want 30059", got)
	}

	manager := w.spawnFolk(t, folkTemplate("Folk", tpRaceManager), 50)
	w.openThrough(t, manager)
	w.requireTrip(t, w.tripFrames(t, npcCommand(manager, "Quest MonsterRaceTrack")), location.Location{X: 15670, Y: 142983, Z: -2705})
	if got := w.memoRow(t); got != "" {
		t.Fatalf("saved memo after the trip back = %q, want none", got)
	}
}

// TestMonsterRaceTrackManagerReadsSavedMemo: a memo saved before the
// character entered the world sends it back to that gatekeeper's town; a
// saved value past int32 fails the parse as the reference's parseInt
// does, so the talk aborts before the trip and the memo stays; with no
// memo the race manager moves no one and keeps no memo.
func TestMonsterRaceTrackManagerReadsSavedMemo(t *testing.T) {
	t.Parallel()
	t.Run("past int32", func(t *testing.T) {
		t.Parallel()
		const past = "4294997355" // 1<<32 + Trisha's id
		w := bootTeleporters(t, teleporterSetup{memo: past})
		manager := w.spawnFolk(t, folkTemplate("Folk", tpRaceManager), 50)
		w.openThrough(t, manager)
		frames := w.bypass(t, npcCommand(manager, "Quest MonsterRaceTrack"))
		if _, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation); ok {
			t.Fatalf("a memo past int32 moved the player: %x", opcodes(frames))
		}
		if got := w.memoRow(t); got != past {
			t.Fatalf("saved memo = %q, want %q kept", got, past)
		}
	})
	t.Run("saved", func(t *testing.T) {
		t.Parallel()
		w := bootTeleporters(t, teleporterSetup{memo: strconv.Itoa(tpBilia)})
		manager := w.spawnFolk(t, folkTemplate("Folk", tpRaceManager), 50)
		w.openThrough(t, manager)
		w.requireTrip(t, w.tripFrames(t, npcCommand(manager, "Quest MonsterRaceTrack")), location.Location{X: 87386, Y: -143246, Z: -1293})
		if got := w.memoRow(t); got != "" {
			t.Fatalf("saved memo after the trip = %q, want none", got)
		}
	})
	t.Run("none", func(t *testing.T) {
		t.Parallel()
		w := bootTeleporters(t, teleporterSetup{})
		manager := w.spawnFolk(t, folkTemplate("Folk", tpRaceManager), 50)
		w.openThrough(t, manager)
		frames := w.bypass(t, npcCommand(manager, "Quest MonsterRaceTrack"))
		if _, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation); ok {
			t.Fatalf("the race manager moved a player with no memo: %x", opcodes(frames))
		}
		if got := opcodes(dialogFrames(frames)); !slices.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
			t.Fatalf("answer = %x, want the dispatcher's release alone", got)
		}
		if got := w.memoRow(t); got != "" {
			t.Fatalf("saved memo = %q, want none", got)
		}
	})
}
