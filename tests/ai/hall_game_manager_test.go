package ai

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The first circle's hot springs game manager, which the tests bind to
// AzitWateringGameManager (it binds no NPC itself, its circles do), the
// yeti it raises, and the NPCs bound by the tests: three that tell a script
// event as they appear, and one that runs Rainbow Springs' mini game.
const (
	gameManagerID      = int32(35596)
	enragedYetiID      = int32(35592)
	wordZeroCallerID   = int32(29982)
	wordThreeCallerID  = int32(29983)
	gourdBrokeCallerID = int32(29984)
	rainbowKeeperID    = int32(29985)
	rainbowSpringsHall = int32(62)
)

// The hot springs waters: the character's nectar, and the nectar the
// manager casts at its gourd.
const (
	hotSpringNectar = modelskill.ID(2240)
	npcNectar       = modelskill.ID(4989)
)

// managerDir is where the manager's pages are, under data/html.
const managerDir = "script/siegablehall/RainbowSpringsChateau/"

// boundGameManager is AzitWateringGameManager bound to the first circle's
// manager, its talk included, as its circles bind it.
func boundGameManager() script.Script {
	sc := hall.GameManager(hall.NewWateringGame())
	sc.NPCs = []int32{gameManagerID}
	sc.Bind = script.Bindings{script.EventTalked: {gameManagerID}}
	return sc
}

// hallCaller is a behavior of id that, as its NPC appears, sends the
// script event eventID with arg1 and arg2 to the NPCs within 10000.
func hallCaller(id, eventID, arg1, arg2 int32) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{id}, Hooks: script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				s.BroadcastScriptEventEx(e.NPC, eventID, arg1, arg2, 10000)
			},
		}}
	}
}

// rainbowKeeper is a behavior that, as its NPC appears, runs Rainbow
// Springs' siege with a mini game whose winners it sends to won, as
// "<gourd object id> <winner object id>", or "unbound" when the hall's
// siege takes no script.
func rainbowKeeper(won chan<- string) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{rainbowKeeperID}, Hooks: script.Hooks{
			OnCreated: func(s *script.Script, _ script.Created) {
				h := s.BindHallSiege(rainbowSpringsHall, script.HallSiegeHooks{
					MiniGameWinner: func(_ *script.Hall, gourd *script.NPC, winner *script.Player) {
						won <- fmt.Sprintf("%d %d", gourd.ObjectID(), winner.ObjectID())
					},
				})
				if h == nil {
					won <- "unbound"
				}
			},
		}}
	}
}

// managerPages are the manager's pages: its first page links its three
// commands, and the current word page shows the word in place of
// %quizstring%.
func managerPages() map[string]string {
	links := `<a action="bypass -h Quest GameManager giveWord">Give</a>` +
		`<a action="bypass -h Quest GameManager showCurrentWord">Word</a>` +
		`<a action="bypass -h Quest GameManager howTo">How</a>`
	return map[string]string{
		managerDir + "35596/watering_manager001.htm": "<html><body>manager001 " + links + "</body></html>",
		managerDir + "watering_manager002.htm":       "<html><body>manager002</body></html>",
		managerDir + "watering_manager003.htm":       "<html><body>manager003</body></html>",
		managerDir + "watering_manager004.htm":       "<html><body>manager004</body></html>",
		managerDir + "watering_manager005.htm":       "<html><body>word %quizstring%</body></html>",
	}
}

// letterItems is the shared item catalog with the word game's letters and
// prizes.
func letterItems() *item.Table {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{8030, 8031, 8032, 8033, 8035, 8036, 8037, 8038, 8039, 8040, 8041, 8042, 8043, 8045, 8046, 8047, 8048, 8049, 8050, 8051, 8052, 8053, 8054, 8055} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Hot Springs " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Dropable: true, Tradable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return item.NewTable(templates)
}

// gameManagerSetup is a game manager test's boot: its script draw, the
// letters the character starts with, whether it knows the nectar, the
// extra scripts and options.
type gameManagerSetup struct {
	draw    int
	letters map[int32]int32
	nectar  bool
	scripts script.Catalog
	kinds   map[int32]script.NPCKind
	opts    []gameservertest.Option
}

// bootGameManager boots the character beside the manager, a gourd, the
// yeti, the event listener and the setup's NPCs.
func bootGameManager(t *testing.T, s gameManagerSetup) *warriorWorld {
	t.Helper()
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc": defaultnpc.New,
		"test.GameManager":                boundGameManager,
		"test.HallListener":               hallListener,
	}
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "test.GameManager"}, {Path: "test.HallListener"}}
	kinds := map[int32]script.NPCKind{
		gameManagerID: script.KindFolk, gourdID: script.KindFolk, enragedYetiID: script.KindFolk, hallListenerID: script.KindHostile,
	}
	templates := []*npc.Template{
		gameservertest.FolkTemplate("Folk", int(gameManagerID)), gameservertest.FolkTemplate("Folk", int(gourdID)),
		gameservertest.FolkTemplate("Folk", int(enragedYetiID)), agitTemplate(hallListenerID),
	}
	for path, ctor := range s.scripts {
		catalog[path] = ctor
		list = append(list, script.Listing{Path: path})
	}
	slices.SortFunc(list[3:], func(a, b script.Listing) int { return strings.Compare(a.Path, b.Path) })
	for id, kind := range s.kinds {
		kinds[id] = kind
		templates = append(templates, agitTemplate(id))
	}
	npcSkills := modelskill.NewTable([]modelskill.Definition{{
		ID: npcNectar, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}})
	opts := append([]gameservertest.Option{
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithHTMLPages(managerPages()),
		gameservertest.WithItemTemplates(letterItems()),
		gameservertest.WithNpcSpawnSkills(npcSkills),
	}, s.opts...)
	var nectar []modelskill.Definition
	if s.nectar {
		db := sqltest.SharedDB(t)
		nectar = []modelskill.Definition{{
			ID: hotSpringNectar, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 400, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		}}
		table := modelskill.NewTable(nectar)
		opts = append(opts,
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(nectar...))
	}
	ready := func(srv *gameservertest.Server, objID int32) {
		for id, count := range s.letters {
			srv.GiveItem(t, objID, id, count)
		}
		for _, def := range nectar {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(def.ID), 1); err != nil {
				t.Fatalf("seed skill %d: %v", def.ID, err)
			}
		}
	}
	return bootHalls(t, 0, s.draw, location.Location{}, raidBoot{opts: opts, ready: ready}, templates...)
}

// encodeManagerBypass is a RequestBypassToServer of command.
func encodeManagerBypass(command string) []byte {
	p := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	p.WriteString(command)
	return p.Bytes()
}

// managerHTML returns the text of each NpcHtmlMessage among frames.
func managerHTML(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNpcHtmlMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32()
		out = append(out, r.ReadString())
	}
	return out
}

// screenTexts returns the text and duration of each ExShowScreenMessage
// among frames, as "text@duration".
func screenTexts(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		if len(f) < 3 || f[0] != serverpackets.OpcodeExtended || binary.LittleEndian.Uint16(f[1:3]) != serverpackets.OpcodeExShowScreenMessage {
			continue
		}
		r := wire.NewReader(f[3:])
		for range 8 {
			r.ReadInt32()
		}
		duration := r.ReadInt32()
		r.ReadInt32()
		out = append(out, r.ReadString()+"@"+strconv.Itoa(int(duration)))
	}
	return out
}

// folkCasts returns the targets of the casts of skill id the NPC casterID
// started among frames.
func folkCasts(frames [][]byte, casterID int32, id modelskill.ID) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, casterID) {
		if modelskill.ID(binary.LittleEndian.Uint32(f[9:13])) == id {
			out = append(out, int32(binary.LittleEndian.Uint32(f[5:9])))
		}
	}
	return out
}

// talkTo opens the manager's first page, the manager being selected, and
// returns the frames sent.
func (w *warriorWorld) talkTo(t *testing.T, f *npc.Folk) [][]byte {
	t.Helper()
	w.srv.Client.Send(encodeFolkAction(f.ObjectID(), w.at))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// command talks to the manager, then sends it the bypass of command and
// returns the frames the command sent. The boot sets no bypass reuse
// delay, so commands may follow each other at once.
func (w *warriorWorld) command(t *testing.T, f *npc.Folk, command string) [][]byte {
	t.Helper()
	w.talkTo(t, f)
	w.srv.Client.Send(encodeManagerBypass("Quest GameManager " + command))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// wantPage fails unless frames show exactly one page, containing text.
func wantPage(t *testing.T, step string, frames [][]byte, text string) {
	t.Helper()
	if got := managerHTML(frames); len(got) != 1 || !strings.Contains(got[0], text) {
		t.Fatalf("%s: pages %q, want one showing %q", step, got, text)
	}
}

// inventoryRows reads the character's stored inventory: count by item id.
func inventoryRows(t *testing.T, db *sql.DB, objID int32) map[int32]int64 {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "SELECT item_id, count FROM items WHERE owner_id = ? AND loc = 'INVENTORY'", objID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int32]int64{}
	for rows.Next() {
		var id int32
		var count int64
		if err := rows.Scan(&id, &count); err != nil {
			t.Fatal(err)
		}
		out[id] += count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// kingfisher are the letters of KINGFISHER, word 3, by item: K, I twice,
// N, G, F, S, H, E and R.
func kingfisher() map[int32]int32 {
	return map[int32]int32{8045: 1, 8043: 2, 8047: 1, 8041: 1, 8040: 1, 8051: 1, 8042: 1, 8039: 1, 8050: 1}
}

// TestGameManagerWordGame ports AzitWateringGameManager's first talk,
// commands and word: the manager answers a talk with its own first page,
// and howTo with the rules page. Before any word, showCurrentWord names
// none and giveWord makes it say there is none. Script event 1 with 10000
// sets the word to its argument and shows it on the screens around for
// 10 s. giveWord without the word's letters shows the wrong-word page and
// takes nothing; with them it takes each letter the word spells, as often
// as it does, and gives the prize the draw names (75 of 100 is the second
// prize for words 1 to 6), stored with the letters gone.
func TestGameManagerWordGame(t *testing.T) {
	t.Parallel()
	w := bootGameManager(t, gameManagerSetup{
		draw:    75,
		letters: kingfisher(),
		scripts: script.Catalog{
			"test.WordZeroCaller":  hallCaller(wordZeroCallerID, 1, 10000, 0),
			"test.WordThreeCaller": hallCaller(wordThreeCallerID, 1, 10000, 3),
		},
		kinds: map[int32]script.NPCKind{wordZeroCallerID: script.KindHostile, wordThreeCallerID: script.KindHostile},
	})
	objID := w.player.ObjectID()
	manager := w.spawnFolk(t, gameManagerID, 100, 0)
	w.talkTo(t, manager) // selects it

	wantPage(t, "talk", w.talkTo(t, manager), "manager001")
	wantPage(t, "howTo", w.command(t, manager, "howTo"), "manager003")
	wantPage(t, "showCurrentWord before a word", w.command(t, manager, "showCurrentWord"), "word "+script.NpcString(1010635))
	frames := w.command(t, manager, "giveWord")
	if got := managerHTML(frames); len(got) != 0 {
		t.Fatalf("giveWord before a word showed %q, want no page", got)
	}
	if got, want := says(frames, manager.ObjectID()), []string{script.NpcString(1010628)}; !slices.Equal(got, want) {
		t.Fatalf("giveWord before a word: manager said %q, want %q", got, want)
	}

	_, frames = w.spawnSeen(t, wordZeroCallerID, location.Location{X: w.at.X - 100, Y: w.at.Y, Z: w.at.Z})
	if got, want := screenTexts(frames), []string{"BABYDUCK@10000"}; !slices.Equal(got, want) {
		t.Fatalf("word 0: screen messages %q, want %q", got, want)
	}
	wantPage(t, "giveWord lacking the letters", w.command(t, manager, "giveWord"), "manager002")
	for id, count := range kingfisher() {
		if got := w.srv.PlayerItemCount(t, objID, id); got != int(count) {
			t.Fatalf("after a wrong word: %d of item %d, want %d", got, id, count)
		}
	}

	_, frames = w.spawnSeen(t, wordThreeCallerID, location.Location{X: w.at.X - 100, Y: w.at.Y + 50, Z: w.at.Z})
	if got, want := screenTexts(frames), []string{"KINGFISHER@10000"}; !slices.Equal(got, want) {
		t.Fatalf("word 3: screen messages %q, want %q", got, want)
	}
	wantPage(t, "showCurrentWord", w.command(t, manager, "showCurrentWord"), "word KINGFISHER")
	wantPage(t, "giveWord with the letters", w.command(t, manager, "giveWord"), "manager004")
	for id := range kingfisher() {
		if got := w.srv.PlayerItemCount(t, objID, id); got != 0 {
			t.Fatalf("after the word: %d of item %d, want none", got, id)
		}
	}
	if got := w.srv.PlayerItemCount(t, objID, 8031); got != 1 {
		t.Fatalf("prize 8031 = %d, want 1", got)
	}
	w.srv.FlushItems(t)
	stored := inventoryRows(t, w.srv.DB, objID)
	for id := range kingfisher() {
		if stored[id] != 0 {
			t.Fatalf("stored %d of letter %d, want none", stored[id], id)
		}
	}
	if stored[8031] != 1 {
		t.Fatalf("stored prize 8031 = %d, want 1", stored[8031])
	}
}

// pourNectar spawns the manager with a gourd 50 from it, lets its first
// look (10 s after it appears) find the gourd, and has the character pour
// the nectar on it.
func (w *warriorWorld) pourNectar(t *testing.T) (manager, gourd *npc.Folk) {
	t.Helper()
	manager = w.spawnFolk(t, gameManagerID, 100, 0)
	gourd = w.spawnFolk(t, gourdID, 150, 0)
	w.srv.Advance(t, 10*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	w.talkTo(t, manager) // selects it
	w.cast(t, hotSpringNectar)
	return manager, gourd
}

// TestGameManagerNectar ports the manager's spelled hook for the nectar:
// on a draw under 90 of 100 it wants to cast its own nectar at the gourd
// it saw first, weight 10000000, standing where it is; on a draw of 90 or
// more it raises an enraged yeti 10 east and 10 south of itself.
func TestGameManagerNectar(t *testing.T) {
	t.Parallel()
	t.Run("cast at the gourd", func(t *testing.T) {
		t.Parallel()
		w := bootGameManager(t, gameManagerSetup{nectar: true})
		manager, gourd := w.pourNectar(t)
		var frames [][]byte
		w.srv.AdvanceUntil(t, "the manager's nectar on its gourd", func() bool {
			frames = append(frames, cycle(t, w.srv)...)
			return len(folkCasts(frames, manager.ObjectID(), npcNectar)) > 0
		})
		if got := folkCasts(frames, manager.ObjectID(), npcNectar); !slices.Equal(got, []int32{gourd.ObjectID()}) {
			t.Fatalf("manager nectar casts = %v, want one on the gourd %d", got, gourd.ObjectID())
		}
	})
	t.Run("enraged yeti", func(t *testing.T) {
		t.Parallel()
		w := bootGameManager(t, gameManagerSetup{draw: 95, nectar: true})
		manager, _ := w.pourNectar(t)
		mx, my, mz := manager.Position()
		var yetis []string
		for _, o := range w.srv.State.Objects() {
			if f, ok := o.(*npc.Folk); ok && int32(f.NpcID()) == enragedYetiID {
				x, y, z := f.Position()
				yetis = append(yetis, fmt.Sprintf("%d,%d,%d", x-mx, y-my, z-mz))
			}
		}
		if want := []string{"10,10,0"}; !slices.Equal(yetis, want) {
			t.Fatalf("enraged yetis at %q from the manager, want %q", yetis, want)
		}
	})
}

// TestGameManagerNamesTheMiniGameWinner ports the manager's answer to a
// broken gourd (script event 5 with 40000): the player who last poured
// nectar on it, within 1000 of it, wins Rainbow Springs' mini game with the
// manager's gourd, and the NPCs within 8000 hear script event 0 with 20002.
func TestGameManagerNamesTheMiniGameWinner(t *testing.T) {
	t.Parallel()
	won := make(chan string, 4)
	rainbow, err := hallmodel.NewHall(hallmodel.HallAttrs{
		ID: int(rainbowSpringsHall), ParentID: -1, Alias: "rainbow_spring", Name: "Rainbow Springs Chateau",
		Description: "Contestable Clan Hall", Town: "Goddard", Grade: 3, SiegeLength: 3600000,
		Siegable: true, ScheduleConfig: []int{14, 0, 0, 12, 0},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	halls, err := hallmodel.NewTable([]*hallmodel.Hall{rainbow})
	if err != nil {
		t.Fatal(err)
	}
	w := bootGameManager(t, gameManagerSetup{
		nectar: true,
		scripts: script.Catalog{
			"test.GourdBrokeCaller": hallCaller(gourdBrokeCallerID, 5, 40000, 0),
			"test.RainbowKeeper":    rainbowKeeper(won),
		},
		kinds: map[int32]script.NPCKind{gourdBrokeCallerID: script.KindHostile, rainbowKeeperID: script.KindHostile},
		opts: []gameservertest.Option{
			gameservertest.WithClanHalls(halls, nil),
			gameservertest.WithResidences(halls, nil),
			gameservertest.WithClanSeed(func(db *sql.DB) {
				if _, err := db.ExecContext(context.Background(), "INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (?, 0, 0, 1, ?)",
					rainbowSpringsHall, time.Now().Add(7*24*time.Hour).UnixMilli()); err != nil {
					t.Fatalf("seed Rainbow Springs: %v", err)
				}
			}),
		},
	})
	w.spawn(t, rainbowKeeperID, -200, 0)
	select {
	case got := <-won:
		t.Fatalf("binding Rainbow Springs: %s", got)
	default:
	}
	listener := w.spawn(t, hallListenerID, -100, 100)
	_, gourd := w.pourNectar(t)

	_, frames := w.spawnSeen(t, gourdBrokeCallerID, location.Location{X: w.at.X - 100, Y: w.at.Y - 100, Z: w.at.Z})
	select {
	case got := <-won:
		if want := fmt.Sprintf("%d %d", gourd.ObjectID(), w.player.ObjectID()); got != want {
			t.Fatalf("mini game winner = %q, want %q", got, want)
		}
	default:
		t.Fatal("the manager named no mini game winner")
	}
	if got := says(frames, listener.ObjectID()); !slices.Contains(got, "event 0 20002 0") {
		t.Fatalf("listener heard %q, want event 0 20002 0", got)
	}
}
