package scenario

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Ext is the scenario file extension RunDir reads.
const Ext = ".scenario"

// killWait is how long a kill step lets pass after each lethal hit, so the
// dead NPC's dying hooks (three seconds after the death) have run: the
// reference probe's seven 500 ms ticks.
const killWait = 3500 * time.Millisecond

// Config is what a suite's generic test hands the runner.
type Config struct {
	// Catalog maps the scripts.xml paths the scenarios list to their
	// constructors.
	Catalog script.Catalog
}

// RunDir runs every scenario file of dir as a parallel subtest named after
// the file. A dir with no scenario file fails.
func RunDir(t *testing.T, dir string, cfg Config) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*"+Ext))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no %s file in %s", Ext, dir)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sc, err := Parse(filepath.Base(file), data)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(strings.TrimSuffix(sc.Name, Ext), func(t *testing.T) {
			t.Parallel()
			Run(t, sc, cfg)
		})
	}
}

// run is one scenario's booted server and the state the steps share.
type run struct {
	sc     *Scenario
	srv    *gameservertest.Server
	player int32
	// origin is where the character first entered the world; NPCs stand
	// at their offsets from it. spawned reports whether they stand there.
	origin  location.Location
	spawned bool
	// pages are the shipped pages as read, texts as the server shows them.
	pages, texts map[string]string
	// roles names object ids in rendered packets; objs and npcs are each
	// role's current object id and NPC.
	roles map[int32]string
	// rd renders the server packets of the steps.
	rd   *renderer
	objs map[string]int32
	npcs map[string]NPC
	live map[string]*npc.Hostile
	// makers are the territory lines' npcmakers, built at the first world
	// entry; grounds holds the ground lines' items' object ids.
	makers  map[string]*spawn.Maker
	grounds map[int32]bool
	// members are the member lines' characters and their clients.
	members []member
	// mates are the mate lines' characters; a client is set once the mate
	// entered the world.
	mates []mateClient
	// summon is the summon line's summon once called.
	summon *summon.Actor
	// stance is the clock of the attack stances when the scenario ends the
	// character's: it moves with the advance steps.
	stance stanceClock
	// items is the item catalog the server runs with.
	items *item.Table
	// draws is the random line's source; traceDraws the draws the
	// reference pinned in each traced step.
	draws      draws
	traceDraws [][]string
	// heard records the script events the NPCs' scripts receive;
	// traceHeard those the reference's NPCs heard in each traced step.
	heard      heardEvents
	traceHeard [][]string
}

// heardEvents records the script events the NPCs' scripts receive, for the
// step that caused them to take. Scripts deliver on the goroutine of their
// sender, so it is guarded.
type heardEvents struct {
	mu     sync.Mutex
	events []heardEvent
}

type heardEvent struct{ npcID, object, id, arg1, arg2 int32 }

func (h *heardEvents) add(npcID, object, id, arg1, arg2 int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, heardEvent{npcID, object, id, arg1, arg2})
}

// take returns the events recorded since the last take, as heardLine
// renders them with roles.
func (h *heardEvents) take(roles map[int32]string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, e := range h.events {
		out = append(out, heardLine(roles, e.object, e.npcID, e.id, e.arg1, e.arg2))
	}
	h.events = nil
	return out
}

// member is one member line's character in the world.
type member struct {
	obj    int32
	client *testsupport.ScriptedClient
}

// draws hands the script draws the random line lists, recording each draw
// as "<bound>:<value>" and each it cannot serve for the step that drew it.
type draws struct {
	mu     sync.Mutex
	random *Random
	next   int
	drawn  []string
	bad    []string
	// saved are the sources a step's own draws replaced.
	saved []savedDraws
}

// draw is the script random source: the next listed draw, which must be
// inside [0, n).
func (d *draws) draw(n int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.next
	d.next++
	var v int
	switch {
	case d.random == nil:
		// No random line, and no draws of this step's own: the draw is random.
		return anyDraw(n)
	case d.random.Always:
		v = d.random.Draws[0]
	case i < len(d.random.Draws):
		v = d.random.Draws[i]
	default:
		d.bad = append(d.bad, fmt.Sprintf("draw %d of [0, %d): the random line lists only %d", i+1, n, len(d.random.Draws)))
		return 0
	}
	if v >= n {
		d.bad = append(d.bad, fmt.Sprintf("draw %d is %d, outside [0, %d)", i+1, v, n))
		return 0
	}
	d.drawn = append(d.drawn, fmt.Sprintf("%d:%d", n, v))
	return v
}

// take returns the draws served and those not served since the last call.
func (d *draws) take() (drawn, bad []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	drawn, bad = d.drawn, d.bad
	d.drawn, d.bad = nil, nil
	return drawn, bad
}

// unused is how many listed draws were never drawn.
func (d *draws) unused() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.random == nil || d.random.Always {
		return 0
	}
	return max(len(d.random.Draws)-d.next, 0)
}

// objectIDsAfter is where a run's object ids start: in the server's own id
// range, as the reference trace's do, so no small number a page shows (a
// multisell or buy list id) is the id of a run's object, which a rendered
// page names by its role.
const objectIDsAfter = idfactory.FirstObjectID - 1

// Run boots the scenario's server, enters the world and runs every step,
// failing at the first expectation that does not hold.
func Run(t *testing.T, sc *Scenario, cfg Config) {
	t.Helper()
	r := &run{
		sc: sc, pages: loadPages(t, sc.Pages), roles: map[int32]string{}, objs: map[string]int32{}, npcs: map[string]NPC{},
		live: map[string]*npc.Hostile{}, makers: map[string]*spawn.Maker{}, grounds: map[int32]bool{},
	}
	r.rd = newRenderer(r.roles)
	r.rd.nameTemplates(sc.NPCs)
	r.draws.random = sc.Random
	r.texts = pageTexts(t, r.pages)
	items := sc.Items
	if sc.Creates() {
		items = append(slices.Clone(items), starterItemIDs(t)...)
	}
	r.items = itemTemplates(t, items)
	if sc.Trace != "" {
		data, err := os.ReadFile(filepath.Join(moduleRoot(t), filepath.FromSlash(sc.Trace)))
		if err != nil {
			t.Fatal(err)
		}
		if r.traceDraws, r.traceHeard, err = checkTrace(sc, data, r.texts); err != nil {
			t.Fatalf("%s does not replay %s: %v", sc.Name, sc.Trace, err)
		}
	}
	r.boot(t, cfg)
	for i, st := range sc.Steps {
		r.step(t, i, st)
	}
	if sc.Random != nil {
		if n := r.draws.unused(); n > 0 {
			t.Fatalf("%s: %d draws of the random line were never drawn", sc.Name, n)
		}
	}
}

func (r *run) boot(t *testing.T, cfg Config) {
	t.Helper()
	list := make([]script.Listing, 0, len(r.sc.Scripts))
	catalog := script.Catalog{}
	for _, l := range r.sc.Scripts {
		ctor, ok := cfg.Catalog[l.Path]
		switch {
		case l.Stub && ok:
			t.Fatalf("%s: %s is ported; list it as a script, not a stub", r.sc.Name, l.Path)
		case l.Stub:
			stub, err := manifestStub(filepath.Join(moduleRoot(t), filepath.FromSlash(manifestPath)), l.Path)
			if err != nil {
				t.Fatalf("%s: %v", r.sc.Name, err)
			}
			ctor = func() script.Script { return stub }
		case !ok:
			t.Fatalf("%s: script %s is not in the suite's catalog", r.sc.Name, l.Path)
		}
		list = append(list, script.Listing{Path: l.Path})
		catalog[l.Path] = ctor
	}
	kinds := map[int32]script.NPCKind{}
	var templates []*npc.Template
	for _, n := range r.sc.NPCs {
		tmpl := template(n)
		if _, dup := kinds[n.ID]; !dup {
			templates = append(templates, tmpl)
		}
		inst := &npc.Instance{Template: tmpl}
		switch {
		case npc.FolkKind(inst):
			kinds[n.ID] = script.KindFolk
		case npc.Attackable(inst):
			kinds[n.ID] = script.KindHostile
		default:
			t.Fatalf("%s: NPC %s type %s is neither a civilian nor an attackable", r.sc.Name, n.Role, n.Type)
		}
		r.npcs[n.Role] = n
	}
	if s := r.sc.Summon; s != nil {
		templates = append(templates, summonTemplate(s))
	}
	opts := []gameservertest.Option{
		gameservertest.WithObjectIDsAfter(objectIDsAfter),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(r.pages),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithItemTemplates(r.items),
		// The AI task ticks only on an approach or a tick step.
		gameservertest.WithAITask(),
		// A creature that attacks or is attacked holds an attack stance, which
		// the behaviors read (in combat); nothing ends it during a run.
		gameservertest.WithAttackStanceClock(r.stanceNow()),
		gameservertest.WithDrivenClockStart(r.clockStart()),
		gameservertest.WithLevels(scenarioLevels(t)),
		// The NPC population, with no maker, over the scenario's templates:
		// the scripts' spawner places NPCs through it.
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
	}
	opts = append(opts, r.fightOptions(t)...)
	switch {
	case r.sc.Creates():
		// The create step makes the character at the character select, with
		// the shipped starter items.
		opts = append(opts, gameservertest.WithWantChars(0), gameservertest.WithClassTemplate(createdTemplate(t)))
	case r.sc.Class != 0 || r.sc.Subclass != 0:
		opts = append(opts, gameservertest.WithClassTemplates(classTemplates()...),
			gameservertest.WithClassCharacter(r.sc.Character, r.sc.Level, 0, r.sc.Class))
	default:
		opts = append(opts, gameservertest.WithCharacter(r.sc.Character, r.sc.Level, 0))
	}
	if r.sc.Clan >= 0 || r.sc.usesHalls() {
		opts = append(opts, gameservertest.WithClanSeed(func(db *sql.DB) {
			if r.sc.Clan >= 0 {
				seedClan(t, db, r.sc.Character, r.sc.Clan)
				seedMates(t, db, r.sc)
			}
			if r.sc.Siege != nil {
				seedSiege(t, db, r.sc.Siege)
			}
			seedHalls(t, db, r.sc)
		}))
	}
	if r.sc.usesHalls() {
		opts = append(opts, hallOptions(t)...)
	}
	if r.sc.Siege != nil {
		opts = append(opts, siegeOptions(t)...)
	}
	if r.sc.Random != nil || r.sc.usesRandomSteps() {
		opts = append(opts, gameservertest.WithScriptRand(r.draws.draw))
	}
	opts = append(opts, gameservertest.WithScriptEventsHeard(r.heard.add))
	if r.sc.Teleports {
		opts = append(opts, r.teleportOptions(t)...)
	}
	if r.sc.LaysCursed() {
		opts = append(opts, gameservertest.WithCursedWeapons(shippedCursedWeapons(t)))
	}
	if r.sc.Hero {
		opts = append(opts, gameservertest.WithOlympiadSeed(func(db *sql.DB) { seedHero(t, db, r.sc.Character) }))
	}
	if r.sc.Festival != nil || r.sc.SealOwners != [3]sevensigns.Cabal{} {
		opts = append(opts, gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) { seedSevenSigns(t, store, r.sc) }))
	}
	r.srv = gameservertest.Boot(t, opts...)
	if r.sc.HasClock && !r.srv.DrivesClock() {
		t.Skipf("%s: the clock line needs the driven clock; the real pool runs on the wall clock", r.sc.Name)
	}
	if r.sc.Creates() {
		return
	}
	r.setPlayer(t)
	if r.sc.Clan >= 0 {
		// The seeded clan must be restored as written.
		if cl, ok := r.srv.Clans.Table().Get(scenarioClan); !ok || cl.Level() != r.sc.Clan {
			t.Fatalf("%s: the clan line's clan is not restored at level %d", r.sc.Name, r.sc.Clan)
		}
	}
	r.checkSiege(t)
	r.checkMates(t)
	r.setHalls(t)
	r.seedMateHolds(t)
	// The journal and the experience are read when the character is
	// selected.
	seedQuests(t, r.srv.DB, r.player, r.sc.Quests)
	seedMemos(t, r.srv.DB, r.player, r.sc.Memos)
	seedExp(t, r.srv.DB, r.player, r.sc.Level)
	r.seedSubclass(t)
	r.seedHoldings(t)
	r.seedSkills(t)
	for _, m := range r.sc.Members {
		c := r.srv.SeedCharacterFor(t, memberAccount(m), m.Name, m.Level, 0)
		seedExp(t, r.srv.DB, c.ID, m.Level)
		r.seedMember(t, c.ID, m)
		r.roles[c.ID] = m.Name
		r.members = append(r.members, member{obj: c.ID})
	}
	r.enterWorld(t)
	r.srv.FlushPersistence(t)
	r.srv.TakeJournalWrites()
	r.srv.TakeMemoWrites()
}

// setPlayer names the account's one character the player.
func (r *run) setPlayer(t *testing.T) {
	t.Helper()
	r.player = r.srv.SoleObjectID(t)
	r.roles[r.player] = "player"
}

// spawnNPCs places the NPC lines' NPCs around where the character stands,
// the first time it enters the world, and drops what their spawning sent.
func (r *run) spawnNPCs(t *testing.T) {
	t.Helper()
	if r.spawned {
		return
	}
	r.spawned = true
	x, y, z := r.srv.PlayerPosition(t, r.player)
	r.origin = location.Location{X: x, Y: y, Z: z}
	r.enterMembers(t)
	r.enterOnlineMates(t)
	for _, tr := range r.sc.Territories {
		r.makers[tr.Name] = r.maker(t, tr)
	}
	for _, n := range r.sc.NPCs {
		r.spawn(t, n)
	}
	for _, g := range r.sc.Grounds {
		r.lay(t, g)
	}
	r.placeFight(t)
	r.srv.ReadQueued(t, r.srv.Client)
	r.drainMembers(t)
	r.drainMates(t)
	// The draws the spawning made (a created hook's) are not a step's: the
	// reference probe does not trace them either.
	if _, bad := r.draws.take(); len(bad) > 0 {
		t.Fatalf("%s: script random at spawn: %s", r.sc.Name, strings.Join(bad, "; "))
	}
	r.heard.take(nil)
}

// seedHero makes the character an active hero of the running era, before
// the server restores the heroes.
func seedHero(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, 1, 1 FROM characters WHERE char_name = ?", name); err != nil {
		t.Fatalf("seed hero: %v", err)
	}
}

// scenarioClan is the object id of the clan a clan line seeds.
const scenarioClan = 900000

// seedClan stores a clan of level led by the character name.
func seedClan(t *testing.T, db *sql.DB, name string, level int) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) SELECT ?, 'Scenario', ?, 0, obj_Id FROM characters WHERE char_name = ?",
		scenarioClan, level, name); err != nil {
		t.Fatalf("seed clan: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET clanid = ? WHERE char_name = ?", scenarioClan, name); err != nil {
		t.Fatalf("seed clan member: %v", err)
	}
}

// shippedLevels is the shipped player level table, loaded once per test
// binary.
var shippedLevels struct {
	once  sync.Once
	table *player.LevelTable
	err   error
}

// scenarioLevels is the level table scenarios run with: the shipped one, as
// the reference trace's server runs with, so experience a script rewards
// raises the level as it does there.
func scenarioLevels(t *testing.T) *player.LevelTable {
	t.Helper()
	path := datapack.Path(t, "data", "xml", "playerLevels.xml")
	shippedLevels.once.Do(func() { shippedLevels.table, shippedLevels.err = gamexml.LoadPlayerLevels(path) })
	if shippedLevels.err != nil {
		t.Fatalf("load shipped levels: %v", shippedLevels.err)
	}
	return shippedLevels.table
}

// seedExp gives the character obj the experience its level starts at, so
// the experience a kill earns (none, from the fixture monsters) keeps the
// level the character line set.
func seedExp(t *testing.T, db *sql.DB, obj int32, level int) {
	t.Helper()
	exp := scenarioLevels(t).RequiredExpForLevel(level)
	if _, err := db.ExecContext(context.Background(), "UPDATE characters SET exp = ? WHERE obj_Id = ?", exp, obj); err != nil {
		t.Fatalf("seed experience: %v", err)
	}
}

// seedSubclass stores the subclass line's subclass in slot 1 and makes it the
// character's active class, as a character that logged out playing it: the
// characters row keeps the base class's level and experience.
func (r *run) seedSubclass(t *testing.T) {
	t.Helper()
	if r.sc.Subclass == 0 {
		return
	}
	levels := scenarioLevels(t)
	if r.sc.SubclassLevel > levels.RealMaxLevel() {
		t.Fatalf("%s: subclass level %d is past the highest level %d", r.sc.Name, r.sc.SubclassLevel, levels.RealMaxLevel())
	}
	ctx := context.Background()
	if _, err := r.srv.DB.ExecContext(ctx,
		"INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, ?, 0, ?, 1)",
		r.player, r.sc.Subclass, levels.RequiredExpForLevel(r.sc.SubclassLevel), r.sc.SubclassLevel); err != nil {
		t.Fatalf("seed subclass: %v", err)
	}
	if _, err := r.srv.DB.ExecContext(ctx, "UPDATE characters SET classid = ? WHERE obj_Id = ?", r.sc.Subclass, r.player); err != nil {
		t.Fatalf("seed active subclass: %v", err)
	}
}

// seedQuests stores the quest lines' rows of the character obj.
func seedQuests(t *testing.T, db *sql.DB, obj int32, quests []QuestRows) {
	t.Helper()
	for _, q := range quests {
		for _, kv := range q.Vars {
			if _, err := db.ExecContext(context.Background(),
				"INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)", obj, q.Quest, kv[0], kv[1]); err != nil {
				t.Fatalf("seed quest %s %s=%s: %v", q.Quest, kv[0], kv[1], err)
			}
		}
	}
}

// seedMemos stores the memo lines' character_memo rows of the character obj.
func seedMemos(t *testing.T, db *sql.DB, obj int32, memos []Memo) {
	t.Helper()
	for _, m := range memos {
		if _, err := db.ExecContext(context.Background(),
			"INSERT INTO character_memo (charId,var,val) VALUES (?,?,?)", obj, m.Var, m.Value); err != nil {
			t.Fatalf("seed memo %s=%s: %v", m.Var, m.Value, err)
		}
	}
}

// seedSevenSigns writes the festival line's scores and the sealowner lines'
// owners into the Seven Signs status row the server restores.
func seedSevenSigns(t *testing.T, store *gamesql.SevenSignsStore, sc *Scenario) {
	t.Helper()
	ctx := context.Background()
	row, found, err := store.LoadStatus(ctx)
	if err != nil || !found {
		t.Fatalf("seed seven signs: load the status row: found=%v err=%v", found, err)
	}
	if f := sc.Festival; f != nil {
		row.DawnFestivalScore, row.DuskFestivalScore = f[0], f[1]
	}
	for i, owner := range sc.SealOwners {
		if owner != sevensigns.NoCabal {
			row.SealOwners[i] = owner
		}
	}
	if err := store.SaveStatus(ctx, row); err != nil {
		t.Fatalf("seed seven signs: %v", err)
	}
}

// seedHoldings stores what the hold, equip, standing, pk and cabal lines
// say the character holds and stands for, before it is selected.
func (r *run) seedHoldings(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, h := range r.sc.Holds {
		tmpl, ok := r.items.Get(h.Item)
		switch {
		case !ok:
			t.Fatalf("%s: hold %d: no such template in the catalog; add an item line", r.sc.Name, h.Item)
		case tmpl.Stackable:
			r.srv.GiveItem(t, r.player, h.Item, h.Count)
		default:
			for range h.Count {
				r.srv.GiveItem(t, r.player, h.Item, 1)
			}
		}
	}
	for _, id := range r.sc.Equips {
		tmpl, ok := r.items.Get(id)
		if !ok {
			t.Fatalf("%s: equip %d: no such template in the catalog; add an item line", r.sc.Name, id)
		}
		inst := item.NewStackOrEquip(r.srv.NewObjectID(), tmpl, 1, true)
		if !inst.Equipped() {
			t.Fatalf("%s: equip %d: the item is not worn in any paperdoll slot", r.sc.Name, id)
		}
		inst.OwnerID = r.player
		if err := r.srv.Items.Create(ctx, r.player, inst); err != nil {
			t.Fatalf("%s: equip %d: %v", r.sc.Name, id, err)
		}
	}
	if r.sc.Standing != 0 || r.sc.PK != 0 {
		if _, err := r.srv.DB.ExecContext(ctx, "UPDATE characters SET varka_ketra_ally = ?, pkkills = ? WHERE obj_Id = ?",
			r.sc.Standing, r.sc.PK, r.player); err != nil {
			t.Fatalf("seed standing and pk: %v", err)
		}
	}
	if r.sc.Karma != 0 {
		if _, err := r.srv.DB.ExecContext(ctx, "UPDATE characters SET karma = ? WHERE obj_Id = ?", r.sc.Karma, r.player); err != nil {
			t.Fatalf("seed karma: %v", err)
		}
	}
	for _, h := range r.sc.Warehouse {
		tmpl, ok := r.items.Get(h.Item)
		if !ok {
			t.Fatalf("%s: warehouse %d: no such template in the catalog; add an item line", r.sc.Name, h.Item)
		}
		// A non-stackable item is one row per piece, as the hold line.
		stacks, count := h.Count, 1
		if tmpl.Stackable {
			stacks, count = 1, int(h.Count)
		}
		for range stacks {
			inst := item.Instance{
				ObjectID: r.srv.NewObjectID(), TemplateID: h.Item, OwnerID: r.player, Count: count, Location: item.LocationWarehouse,
			}
			if err := r.srv.Items.Create(ctx, r.player, inst); err != nil {
				t.Fatalf("%s: warehouse %d: %v", r.sc.Name, h.Item, err)
			}
		}
	}
	if r.sc.Noble {
		if _, err := r.srv.DB.ExecContext(ctx, "UPDATE characters SET nobless = 1 WHERE obj_Id = ?", r.player); err != nil {
			t.Fatalf("seed noble: %v", err)
		}
	}
	if r.sc.Cabal != sevensigns.NoCabal {
		if err := r.srv.SevenSigns.SetPlayerInfo(ctx, r.player, r.sc.Cabal, r.sc.Seal); err != nil {
			t.Fatalf("seed cabal: %v", err)
		}
	}
}

// spawn places a fresh NPC of n's template at n's offset under n's role,
// then runs its created hooks, as the spawn pass does. A civilian casts
// the shipped skills.
func (r *run) spawn(t *testing.T, n NPC) {
	t.Helper()
	at := location.Location{X: r.origin.X + n.DX, Y: r.origin.Y + n.DY, Z: r.origin.Z + n.DZ}
	tmpl := template(n)
	var live attackable.Combatant
	maker := r.makers[n.Maker]
	if npc.FolkKind(&npc.Instance{Template: tmpl}) {
		live = r.srv.SpawnCastingFolkNPCOfMakerAt(t, tmpl, at, shippedSkills(t), maker)
	} else {
		var h *npc.Hostile
		if n.Walks {
			h = r.srv.SpawnMovingHostileNPCOfMakerAt(t, tmpl, at, maker)
		} else {
			h = r.srv.SpawnHostileNPCOfMakerAt(t, tmpl, at, maker)
		}
		if n.Skills != nil {
			r.srv.InstallNPCCastSeam(t, h, shippedSkills(t))
		}
		r.srv.AI.Add(h)
		r.live[n.Role], live = h, h
	}
	r.roles[live.ObjectID()], r.objs[n.Role], r.rd.placed[live.ObjectID()] = n.Role, live.ObjectID(), true
	r.spoil(t, n.Role)
	if draw, ok := r.sc.Rolls[n.Role]; ok {
		r.pinRoll(t, n.Role, draw)
	}
	if v, ok := r.sc.Gauss[n.Role]; ok {
		r.pinGauss(t, n.Role, v)
	}
	r.setParams(t, live, n)
	r.link(t, n)
	r.srv.RunCreated(t, live)
	r.setMemory(t, n)
	if n.HasMP && !npc.FolkKind(&npc.Instance{Template: tmpl}) {
		h, post := r.hostile(t, n.Role)
		r.on(t, n.Role, post, func() { h.SetCurrentMP(n.MP) })
	}
}

// clockEpoch is the day the clock line's time of day is on.
var clockEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// clockStart is where the driven clock starts: the clock line's time, else
// the start of the epoch day, so a scenario never depends on the time of day
// it runs at.
func (r *run) clockStart() time.Time {
	if r.sc.HasClock {
		return clockEpoch.Add(r.sc.Clock)
	}
	return clockEpoch
}

// objectRef resolves an NPC parameter or event argument: a number, the
// character's object id or the object id of the NPC of a role.
func (r *run) objectRef(t *testing.T, ref string) int32 {
	t.Helper()
	switch ref {
	case "":
		return 0
	case characterRef:
		return r.player
	}
	if v, err := strconv.ParseInt(ref, 10, 32); err == nil {
		return int32(v)
	}
	if r.objs[ref] == 0 {
		t.Fatalf("%s: %s is not in the world yet", r.sc.Name, ref)
	}
	return r.objs[ref]
}

// setParams gives the fresh NPC its line's spawn parameters, as the
// private-creating master gives them before its created hook runs.
func (r *run) setParams(t *testing.T, live attackable.Combatant, n NPC) {
	t.Helper()
	if n.Params == [3]string{} {
		return
	}
	s, ok := live.(interface{ Scratch() *npc.Scratch })
	if !ok {
		t.Fatalf("%s: NPC %s (%T) keeps no script memory", r.sc.Name, n.Role, live)
	}
	for i, ref := range n.Params {
		s.Scratch().SetInt(npc.IntParam1+npc.IntSlot(i), r.objectRef(t, ref))
	}
}

// event sends the NPC of a role a script event on the NPC's own queue.
func (r *run) event(t *testing.T, args []string) {
	t.Helper()
	id, _ := strconv.ParseInt(args[1], 10, 32)
	a1, a2 := r.objectRef(t, args[2]), r.objectRef(t, args[3])
	n, post := r.npcQueue(t, args[0])
	done := make(chan struct{})
	if !post(func() {
		defer close(done)
		r.srv.ScriptEvent(n, int32(id), a1, a2)
	}) {
		t.Fatalf("%s: NPC %s's queue is closed", r.sc.Name, args[0])
	}
	<-done
	r.srv.Settle(t)
}

// otherSpoiler is the object id of the other player a spoil line's other
// names: nobody in the world.
const otherSpoiler = 0x7fff0001

// spoil marks the fresh monster of role as its spoil line says: a spawn
// clears the spoil state, so each one is marked anew.
func (r *run) spoil(t *testing.T, role string) {
	t.Helper()
	for _, s := range r.sc.Spoils {
		if s.Role != role {
			continue
		}
		h := r.live[role]
		if h == nil {
			t.Fatalf("%s: spoil %s: the NPC is not an attackable", r.sc.Name, role)
		}
		spoiler := r.player
		if s.Other {
			spoiler = otherSpoiler
		}
		if !h.SpoilPool().Mark(spoiler) {
			t.Fatalf("%s: spoil %s: the NPC is already spoiled", r.sc.Name, role)
		}
	}
}

// template is the fixture template of n: a civilian one, undying like the
// shipped service NPCs, or a level 1 monster of 1000 HP and 1000 MP (what
// its typed skills cost); it can move when n walks, and has n's clans and
// typed skills.
func template(n NPC) *npc.Template {
	tmpl := gameservertest.FolkTemplate(n.Type, int(n.ID))
	if npc.FolkKind(&npc.Instance{Template: tmpl}) {
		tmpl.CanMove = n.Walks
		if n.HasMP {
			// A civilian starts each life with its full MP: the mp option
			// sizes it.
			tmpl.MPMax = float64(n.MP)
		}
	} else {
		tmpl = &npc.Template{
			ID: int(n.ID), TemplateID: int(n.ID), Type: n.Type, Level: 1, HPMax: 1000, MPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
			CanMove: n.Walks,
		}
	}
	if n.Level != 0 {
		tmpl.Level = n.Level
	}
	tmpl.AggroRange, tmpl.Privates = n.Aggro, n.Privates
	tmpl.Clans, tmpl.ClanRange = n.Clans, n.ClanRange
	if n.AI != nil {
		tmpl.AIParams = n.AI
	}
	if n.Skills != nil {
		tmpl = npc.NewTemplate(*tmpl, npc.TemplateSkills{ByType: n.Skills})
	}
	return tmpl
}

func (r *run) step(t *testing.T, i int, st Step) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("%s step %d (line %d) %q: %s", r.sc.Name, i+1, st.Line, st, fmt.Sprintf(format, args...))
	}
	c, actor := r.actor(t, st.As)
	r.rd.talk = talkVerb(st.Verb)
	if st.Random != nil {
		r.draws.push(st.Random)
	}
	// frames is what the step sent, when the verb reads it itself.
	var frames [][]byte
	switch st.Verb {
	case "action":
		x, y, z := r.srv.PlayerPosition(t, actor)
		w := wire.NewPacketWriter(clientpackets.OpcodeAction)
		w.WriteInt32(r.objs[st.Args[0]])
		w.WriteInt32(int32(x))
		w.WriteInt32(int32(y))
		w.WriteInt32(int32(z))
		w.WriteUint8(0)
		c.Send(w.Bytes())
	case "bypass":
		w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
		w.WriteString(expandRoles(st.Args[0], r.objs))
		for range st.Repeat - 1 {
			// An xN bypass: the sends before the last are not compared.
			c.Send(w.Bytes())
			r.srv.ReadQueued(t, c)
			if st.As == "" && r.character(t).Teleporting() {
				// A trip the next send follows: the client has arrived.
				c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
				r.srv.ReadQueued(t, c)
			}
		}
		c.Send(w.Bytes())
	case "appear":
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	case "link":
		w := wire.NewPacketWriter(clientpackets.OpcodeRequestLinkHtml)
		w.WriteString(st.Args[0])
		c.Send(w.Bytes())
	case "hit":
		r.hit(t, st.Args)
	case "event":
		r.event(t, st.Args)
	case "hp", "mp", "want", "die", "top", "land", "stance", "swing":
		r.state(t, st.Verb, st.Args)
	case "cast":
		frames = r.cast(t, st.Args[0], st.Args[1])
	case "jump":
		r.jump(t, st.Args)
	case "away":
		r.away(t)
	case "kill":
		n := 1
		if len(st.Args) == 2 {
			n, _ = killCount(st.Args[1])
		}
		role := st.Args[0]
		if r.live[role] == nil && r.objs[role] != 0 {
			fail("NPC %s is not an attackable", role)
		}
		for range n {
			if h := r.live[role]; h == nil || h.AlikeDead() {
				r.spawn(t, r.npcs[role])
			}
			h := r.live[role]
			r.onPlayer(t, func(p attackable.Combatant) { h.TakeDamage(h.MaxHP()+1, p) })
			r.srv.Advance(t, killWait)
		}
	case "advance":
		d, _ := time.ParseDuration(st.Args[0])
		r.srv.AdvanceWithEffects(t, d)
		r.passStanceTime(t, d)
	case "approach":
		distance := 100
		if len(st.Args) == 2 {
			distance, _ = strconv.Atoi(st.Args[1])
		}
		r.approach(t, st.Args[0], distance)
	case "tick":
		n := 1
		if len(st.Args) == 1 {
			n, _ = killCount(st.Args[0])
		}
		for range n {
			if err := r.srv.AI.Tick(); err != nil {
				fail("AI tick: %v", err)
			}
			r.srv.Settle(t)
		}
	case "pickup":
		r.pickup(t, st.Args[0])
	case "look":
		r.look(t, st.Args)
	case "walk":
		r.walk(t, st.Args[0])
	case "mark":
		id, _ := strconv.ParseInt(st.Args[0], 10, 32)
		w := wire.NewPacketWriter(clientpackets.OpcodeRequestTutorialQuestionMark)
		w.WriteInt32(int32(id))
		c.Send(w.Bytes())
	case "relog":
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
		r.readThrough(t, serverpackets.OpcodeCharSelectInfo)
		r.enterWorld(t)
	case "restart":
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
		frames = r.readThrough(t, serverpackets.OpcodeCharSelectInfo)
	case "enter":
		if len(st.Args) == 1 {
			r.enterMate(t, r.mateIndex(t, st.Args[0]))
			break
		}
		frames = r.enterWorld(t)
	case "hallstatus":
		id, _ := strconv.Atoi(st.Args[0])
		r.setHallStatus(t, id, st.Args[1])
	case "create":
		c.Send(characterCreate(r.sc.Character))
		frames = r.readThrough(t, serverpackets.OpcodeCharCreateOk)
		frames = append(frames, r.readThrough(t, serverpackets.OpcodeCharSelectInfo)...)
		r.setPlayer(t)
	}
	frames = append(frames, r.srv.ReadQueued(t, c)...)
	r.drainMembers(t)
	others, err := r.othersSent(t, c)
	if err != nil {
		fail("%v", err)
	}
	drawn, bad := r.draws.take()
	if len(bad) > 0 {
		fail("script random: %s", strings.Join(bad, "; "))
	}
	if st.Random != nil {
		if n := r.draws.pop(); n > 0 {
			fail("%d draws of the step's random line were never drawn", n)
		}
	}
	if d := traceDrawDiff(r.traceDraws, i, drawn); d != "" {
		fail("%s (want: the trace's)", d)
	}
	if d := traceHeardDiff(r.traceHeard, i, r.heard.take(r.roles)); d != "" {
		fail("%s (want: the trace's)", d)
	}
	if st.Verb != "relog" {
		sentTo := map[string]bool{}
		for name := range others {
			sentTo[name] = true
		}
		for name := range st.To {
			sentTo[name] = true
		}
		for name := range sentTo {
			want, err := expandPackets(st.To[name], r.texts)
			if err != nil {
				fail("%v", err)
			}
			if d := FirstDiff("packet to "+name, others[name], want); d != "" {
				fail("%s", d)
			}
		}
		var got []string
		for _, f := range frames {
			if line, ok := r.groundDeleted(f); ok {
				got = appendRendered(got, line)
				continue
			}
			line, ok, err := r.rd.render(f)
			if err != nil {
				fail("frame %x: %v", f, err)
			}
			if ok {
				got = appendRendered(got, line)
			}
		}
		want, err := expandPackets(st.Packets, r.texts)
		if err != nil {
			fail("%v", err)
		}
		if d := FirstDiff("packet", matchNearTeleports(got, want), want); d != "" {
			fail("%s", d)
		}
	}
	r.srv.FlushPersistence(t)
	if d := FirstDiff("statement", statements(r.srv.TakeJournalWrites()), st.Statements); d != "" {
		fail("%s", d)
	}
	if d := FirstDiff("memo", memoLines(r.srv.TakeMemoWrites()), st.Memos); d != "" {
		fail("%s", d)
	}
	if d := FirstDiff("live", r.lives(st.Lives), st.Lives); d != "" {
		fail("%s", d)
	}
	if d := FirstDiff("spawned", r.spawns(st.Spawns), st.Spawns); d != "" {
		fail("%s", d)
	}
	for _, ck := range st.Checks {
		if d := r.check(t, ck); d != "" {
			fail("%s (line %d)", d, ck.Line)
		}
	}
}

// lives counts, for each live line of a step, the NPCs of that template
// standing alive in the world, as "<npc id>=<count>".
func (r *run) lives(want []string) []string {
	var got []string
	for _, w := range want {
		id, _, _ := strings.Cut(w, "=")
		n, _ := strconv.Atoi(id)
		count := 0
		for _, obj := range r.srv.State.Objects() {
			c, ok := obj.(interface{ Template() *npc.Template })
			if !ok || c.Template().ID != n {
				continue
			}
			if d, ok := obj.(interface{ AlikeDead() bool }); ok && d.AlikeDead() {
				continue
			}
			count++
		}
		got = append(got, fmt.Sprintf("%s=%d", id, count))
	}
	return got
}

// spawns renders, for each spawned line of a step, the places of the NPCs of
// that template standing alive that no npc line placed, as "<npc id>
// <x>,<y> ..." sorted, "<npc id> -" for none.
func (r *run) spawns(want []string) []string {
	var got []string
	for _, w := range want {
		id, _, _ := strings.Cut(w, " ")
		n, _ := strconv.Atoi(id)
		var places []string
		for _, obj := range r.srv.State.Objects() {
			c, ok := obj.(interface{ Template() *npc.Template })
			if !ok || c.Template().ID != n || r.rd.placed[obj.ObjectID()] {
				continue
			}
			if d, ok := obj.(interface{ AlikeDead() bool }); ok && d.AlikeDead() {
				continue
			}
			at, ok := obj.(interface{ Position() (int, int, int) })
			if !ok {
				continue
			}
			x, y, _ := at.Position()
			places = append(places, fmt.Sprintf("%d,%d", x, y))
		}
		slices.Sort(places)
		if len(places) == 0 {
			places = []string{"-"}
		}
		got = append(got, id+" "+strings.Join(places, " "))
	}
	return got
}

// pickup sends an Action on the one ground item of template id, from where
// the character stands, and lets time pass until it has left the ground.
func (r *run) pickup(t *testing.T, id string) {
	t.Helper()
	tmpl, _ := strconv.ParseInt(id, 10, 32)
	onGround := func(obj int32) (found []int32) {
		for _, g := range r.srv.GroundItems.Snapshots(nil) {
			if g.TemplateID == int32(tmpl) && (obj == 0 || g.ObjectID == obj) {
				found = append(found, g.ObjectID)
			}
		}
		return found
	}
	items := onGround(0)
	if len(items) != 1 {
		t.Fatalf("%s: pickup %s: %d ground items of that template, want 1", r.sc.Name, id, len(items))
	}
	x, y, z := r.srv.PlayerPosition(t, r.player)
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(items[0])
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteUint8(0)
	r.srv.Client.Send(w.Bytes())
	r.srv.AdvanceUntil(t, "item "+id+" picked up", func() bool { return len(onGround(items[0])) == 0 })
}

// approach puts the character distance in front of role's NPC, on the -x
// side, then runs one AI tick and lets it settle.
func (r *run) approach(t *testing.T, role string, distance int) {
	t.Helper()
	obj, ok := r.srv.State.Player(r.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	target, ok := r.srv.State.Object(r.objs[role])
	if !ok {
		t.Fatalf("NPC %s is not in the world", role)
	}
	n, ok := target.(attackable.Combatant)
	if !ok {
		t.Fatalf("NPC %s (%T) is not a combatant", role, target)
	}
	x, y, z := n.Position()
	if err := r.srv.State.Move(obj, x-distance, y, z); err != nil {
		t.Fatalf("approach %s: %v", role, err)
	}
	if err := r.srv.AI.Tick(); err != nil {
		t.Fatalf("AI tick: %v", err)
	}
	r.srv.Settle(t)
}

// onPlayer runs fn on the character's queue, where its hits land, and
// waits for it.
func (r *run) onPlayer(t *testing.T, fn func(p attackable.Combatant)) {
	t.Helper()
	obj, ok := r.srv.State.Player(r.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	p, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("the character %T is not a combatant", obj)
	}
	done := make(chan struct{})
	if !r.srv.PlayerQueue(t, r.player).Post(func() {
		defer close(done)
		fn(p)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
}

// check runs one items, rows or state line, describing the first
// difference.
func (r *run) check(t *testing.T, ck Check) string {
	t.Helper()
	switch ck.Kind {
	case "items":
		var got, want []string
		owner, args := r.player, ck.Args
		if !strings.Contains(args[0], "=") {
			owner, args = r.otherCharacter(t, args[0]), args[1:]
		}
		for _, a := range args {
			id, count, _ := strings.Cut(a, "=")
			n, _ := strconv.ParseInt(id, 10, 32)
			got = append(got, fmt.Sprintf("%s=%d", id, r.srv.PlayerItemCount(t, owner, int32(n))))
			want = append(want, id+"="+count)
		}
		return FirstDiff("item", got, want)
	case "rows":
		return rowsDiff(ck.Args[0], r.rows(t, ck.Args[0]), ck.Args[1:])
	case "state":
		quest, want := ck.Args[0], ck.Args[1]
		got := "-"
		r.srv.RunQuest(t, r.player, quest, func(_ *script.Quests, c *player.Character, _ *script.Script) {
			if st := c.Quests().State(quest); st != nil {
				got = st.Status().String()
			}
		})
		return FirstDiff(quest+" state", []string{got}, []string{want})
	case "open", "dungeon", "standing":
		return r.seedCheck(t, ck)
	case "hate", "desires", "at":
		return r.fightCheck(t, ck)
	case "mem", "hp", "privates", "master", "top", "effect", "dead", "inworld", "stance":
		return r.stateCheck(t, ck)
	}
	return ""
}

// rowsDiff compares the var=value rows of quest the database holds with
// those a rows check lists ("-" for none), both sorted the same way here: the
// database's collation orders talk before talk1, a byte sort of var=value
// the other way round.
func rowsDiff(quest string, got, want []string) string {
	got, want = slices.Clone(got), slices.Clone(want)
	if len(want) == 1 && want[0] == "-" {
		want = nil
	}
	slices.Sort(got)
	slices.Sort(want)
	return FirstDiff(quest+" row", got, want)
}

// rows returns the character's character_quests rows of quest as
// var=value.
func (r *run) rows(t *testing.T, quest string) []string {
	t.Helper()
	rows, err := r.srv.DB.QueryContext(context.Background(),
		"SELECT var,value FROM character_quests WHERE charId=? AND name=?", r.player, quest)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, k+"="+v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// enterWorld selects the character and enters the world, returning what
// it was sent; the first time, the NPC lines' NPCs are spawned after.
func (r *run) enterWorld(t *testing.T) [][]byte {
	t.Helper()
	r.srv.Client.Send(gameStart())
	frames := r.readThrough(t, serverpackets.OpcodeCharSelected)
	r.srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	frames = append(frames, r.srv.ReadQueued(t, r.srv.Client)...)
	if draw, ok := r.sc.Rolls["player"]; ok {
		r.pinPlayerRoll(t, draw)
	}
	r.spawnNPCs(t)
	return frames
}

// readThrough reads frames until one with opcode arrives and returns them
// all, that one last.
func (r *run) readThrough(t *testing.T, opcode byte) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		f := r.srv.Client.Read()
		frames = append(frames, f)
		if f[0] == opcode {
			return frames
		}
	}
	t.Fatalf("no frame with opcode %#x within 100 frames", opcode)
	return nil
}

// characterCreate is a RequestCharacterCreate of a human fighter named
// name: race, sex, class and the six stats 0, hair style 1, hair color and
// face 0.
func characterCreate(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	for range 9 {
		w.WriteInt32(0)
	}
	w.WriteInt32(1)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

// statements renders journal writes as Q lines.
func statements(writes []questlog.Write) []string {
	var out []string
	for _, w := range writes {
		switch w.Op {
		case questlog.OpSet:
			out = append(out, fmt.Sprintf("upsert %s | %s | %s", w.Quest, w.Var, w.Value))
		case questlog.OpUnset:
			out = append(out, fmt.Sprintf("delete-var %s | %s", w.Quest, w.Var))
		case questlog.OpDelete:
			out = append(out, "delete-quest "+w.Quest)
		case questlog.OpComplete:
			out = append(out, "delete-except-state "+w.Quest)
		}
	}
	return out
}

// memoLines renders player memo writes as M lines.
func memoLines(writes []gameservertest.MemoWrite) []string {
	var out []string
	for _, w := range writes {
		if w.Unset {
			out = append(out, "unset "+w.Key)
			continue
		}
		out = append(out, "set "+w.Key+" | "+w.Value)
	}
	return out
}

// classTemplates returns a template of every class, each with the
// fixture's stats: the class line's class loads with its parents, and a
// script's class change lands on whichever class it names.
func classTemplates() []*player.Template {
	var out []*player.Template
	for id := 1; id <= maxClassID; id++ {
		if _, ok := player.ClassParent(id); ok {
			tmpl := gameservertest.ClassTemplate()
			tmpl.ID = id
			out = append(out, tmpl)
		}
	}
	return out
}

// maxClassID bounds the class ids classTemplates looks up.
const maxClassID = 255

// loadPages reads the shipped pages the scenario serves, keyed by their
// path below data/html.
func loadPages(t *testing.T, paths []string) map[string]string {
	t.Helper()
	pages := map[string]string{}
	if len(paths) == 0 {
		return pages
	}
	root := datapack.Path(t, "data", "html")
	read := func(p string) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		pages[p] = string(b)
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, "/") {
			read(p)
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				read(path.Join(p, e.Name()))
			}
		}
	}
	return pages
}

// pageTexts returns each page as the server shows it: loaded through the
// production page cache, which normalizes line ends.
func pageTexts(t *testing.T, pages map[string]string) map[string]string {
	t.Helper()
	texts := map[string]string{}
	if len(pages) == 0 {
		return texts
	}
	cache := gameservertest.HTMLCache(t, pages)
	for p := range pages {
		texts[p], _ = cache.Get(p)
	}
	return texts
}

// shipped is the shipped item table, loaded once per test binary.
var shipped struct {
	once  sync.Once
	table *item.Table
	err   error
}

// shippedCursed is the shipped cursed weapon table, loaded once per test
// binary.
var shippedCursed struct {
	once  sync.Once
	table *entity.CursedWeaponTable
	err   error
}

// shippedCursedWeapons is the shipped cursed weapon table, its skills read
// from the shipped skill table.
func shippedCursedWeapons(t *testing.T) *entity.CursedWeaponTable {
	t.Helper()
	path := datapack.Path(t, "data", "xml", "cursedWeapons.xml")
	skills := shippedSkills(t)
	shippedCursed.once.Do(func() { shippedCursed.table, shippedCursed.err = gamexml.LoadCursedWeapons(path, skills) })
	if shippedCursed.err != nil {
		t.Fatalf("load shipped cursed weapons: %v", shippedCursed.err)
	}
	return shippedCursed.table
}

// shippedSkillTable is the shipped skill table, loaded once per test
// binary.
var shippedSkillTable struct {
	once  sync.Once
	table *modelskill.Table
	err   error
}

// shippedSkills is the shipped skill table the scenario civilians cast
// from.
func shippedSkills(t *testing.T) *modelskill.Table {
	t.Helper()
	dir := datapack.Path(t, "data", "xml", "skills")
	shippedSkillTable.once.Do(func() {
		shippedSkillTable.table, shippedSkillTable.err = gamexml.LoadSkillDefinitions(dir, zerolog.Nop())
	})
	if shippedSkillTable.err != nil {
		t.Fatalf("load shipped skills: %v", shippedSkillTable.err)
	}
	return shippedSkillTable.table
}

// itemTemplates is the fixture item catalog with the shipped templates of
// ids added.
func itemTemplates(t *testing.T, ids []int32) *item.Table {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	if len(ids) == 0 {
		return item.NewTable(templates)
	}
	dir := datapack.Path(t, "data", "xml", "items")
	shipped.once.Do(func() { shipped.table, shipped.err = gamexml.LoadItemTemplates(dir, zerolog.Nop()) })
	if shipped.err != nil {
		t.Fatalf("load shipped items: %v", shipped.err)
	}
	added := map[int32]bool{}
	for _, id := range ids {
		tmpl, ok := shipped.table.Get(id)
		if !ok {
			t.Fatalf("no shipped item %d", id)
		}
		if !added[id] {
			added[id] = true
			templates = append(templates, tmpl)
		}
	}
	return item.NewTable(templates)
}

// moduleRoot is the Go module's root directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}
