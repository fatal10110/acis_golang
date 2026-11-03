package clan

import (
	"context"
	"database/sql"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q504"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/banditstronghold"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Reference: FlagWar (prepareSiege, startSiege and its two delayed steps,
// onMyDying, the final's delayed step, endSiege, registerClan,
// loadAttackers, the teleporters' onFirstTalk) and BanditStronghold
// (onFirstTalk, onAdvEvent). The Bandit Stronghold (35) is a siegable hall
// fought as a flag war; its messenger (35437) and its teleporters
// (35561-35565, then 35560) are the farm maker oren15_azit_teleporter01's,
// its flags 35422-35427, its warriors 35428-35432.

const (
	bandit            = clanhall.BanditStronghold
	banditScript      = "script.siegablehall.BanditStronghold"
	banditScriptName  = "BanditStronghold"
	banditMessengerID = 35437
	royalFlagID       = 35422
	redFlagID         = 35423
	yellowFlagID      = 35424
	ally1ID           = 35428
	ally2ID           = 35429
	ally3ID           = 35430
	teleport1ID       = 35560
	banditMaker       = "oren15_azit_teleporter01"
	q504Name          = "Q504_CompetitionForTheBanditStronghold"
	// The items a trophy registration takes.
	tarlkAmuletID        = 4332
	contestCertificateID = 4333
	trophyID             = 5009
)

// questItem is a stackable quest item template.
func questItem(id int32) *item.Template {
	return &item.Template{
		ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
	}
}

// banditFlagPoint is the Bandit Stronghold's flag point i.
var banditFlagPoint = []location.Location{
	{X: 83699, Y: -17468, Z: -1774},
	{X: 82053, Y: -17060, Z: -1784},
	{X: 82142, Y: -15528, Z: -1799},
	{X: 83544, Y: -15266, Z: -1770},
	{X: 84609, Y: -16041, Z: -1769},
	{X: 81981, Y: -15708, Z: -1858},
	{X: 84375, Y: -17060, Z: -1860},
}

// The system messages of a flag war.
const (
	msgTryoutsBegun  = 789
	msgFinalsBegun   = 790
	msgFinalMatch    = 791
	msgTryoutsSoon   = 815
	msgSiegeBegun    = 844
	msgNoInterest    = 846
	msgWonMatch      = 857
	msgBeginsMinutes = 1826
	msgBeginsSeconds = 1828
	msgClanDefeated  = 855
	msgSiegeFinished = 843
	msgMatchDraw     = 858
)

// flagWarSpawnlist is the teleporter maker, its messenger and teleporters
// beside the character; the second teleporter stands 50 from it.
const flagWarSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="bandit_field" minZ="0" maxZ="100"><node x="-500" y="-500"/><node x="500" y="-500"/><node x="500" y="500"/><node x="-500" y="500"/></territory>
	<npcmaker name="oren15_azit_teleporter01" territory="bandit_field" maximumNpcs="7">
		<ai type="farm_maker"><set name="CastleID" val="35"/></ai>
		<npc id="35437" pos="100;20;30;0" total="1" respawn="1min"/>
		<npc id="35561" pos="60;20;30;0" total="1" respawn="1min"/>
		<npc id="35562" pos="300;20;30;0" total="1" respawn="1min"/>
		<npc id="35563" pos="310;20;30;0" total="1" respawn="1min"/>
		<npc id="35564" pos="320;20;30;0" total="1" respawn="1min"/>
		<npc id="35565" pos="330;20;30;0" total="1" respawn="1min"/>
		<npc id="35560" pos="340;20;30;0" total="1" respawn="1min"/>
	</npcmaker>
</list>`

// teleportLanding is where the teleporters send the players around them.
var teleportLanding = location.Location{X: 40, Y: 60, Z: gameservertest.SpawnZ}

// flagWarTemplates are the hall's NPCs: civilian messenger, flags and
// teleporters, monster warriors; the teleporters send the players around
// them to teleportLanding.
func flagWarTemplates() *npc.Table {
	var out []*npc.Template
	folk := func(id int) *npc.Template { return gameservertest.FolkTemplate("Folk", id) }
	out = append(out, folk(banditMessengerID))
	for id := royalFlagID; id <= 35427; id++ {
		out = append(out, folk(id))
	}
	for id := ally1ID; id <= 35432; id++ {
		out = append(out, &npc.Template{
			ID: id, TemplateID: id, Type: "Monster", Name: "Warrior " + strconv.Itoa(id), Level: 1, HPMax: 100,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 10,
		})
	}
	for id := teleport1ID; id <= 35565; id++ {
		tp := folk(id)
		tp.AIParams = npc.AIParams{
			"Pos_x1": strconv.Itoa(teleportLanding.X), "Pos_y1": strconv.Itoa(teleportLanding.Y), "Pos_z1": strconv.Itoa(teleportLanding.Z),
		}
		out = append(out, tp)
	}
	return npc.NewTable(out)
}

// flagWarKinds are the kinds of flagWarTemplates.
func flagWarKinds() map[int32]script.NPCKind {
	out := map[int32]script.NPCKind{banditMessengerID: script.KindFolk}
	for id := int32(royalFlagID); id <= 35427; id++ {
		out[id] = script.KindFolk
	}
	for id := int32(ally1ID); id <= 35432; id++ {
		out[id] = script.KindHostile
	}
	for id := int32(teleport1ID); id <= 35565; id++ {
		out[id] = script.KindFolk
	}
	return out
}

// flagWarSetup is the world a flag war boots with.
type flagWarSetup struct {
	// owned has Holders own the Bandit Stronghold.
	owned bool
	// seed are more statements, run after the clans and the hall row.
	seed []string
	// q504 lists Competition for the Bandit Stronghold too, started by the
	// Attacker.
	q504 bool
	// items are what the Attacker holds as it enters the world.
	items map[int32]int32
}

// flagWarWorld is the Attacker, leading Besiegers, in the world beside
// the Bandit Stronghold's messenger, on its battlefield.
type flagWarWorld struct {
	srv   *gameservertest.Server
	c     *testsupport.ScriptedClient
	objID int32
	date  int64
	siege *clanhall.HallSiege
	field *zone.Siege
}

func bootFlagWar(t *testing.T, s flagWarSetup) *flagWarWorld {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/bandit.xml", []byte(flagWarSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	makers, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	field := zone.NewSiege(1, cuboid(t, -2000, 2000, -2000, 2000))
	field.ResidenceID = int(bandit)
	grounds := zone.NewClanHall(2, cuboid(t, 5000, 6000, 5000, 6000))
	grounds.ResidenceID = int(bandit)
	zones := zone.NewIndex()
	zones.Add(field)
	zones.Add(grounds)
	w := &flagWarWorld{field: field, date: time.Now().Add(time.Hour + time.Minute).UnixMilli()}
	listing := []script.Listing{{Path: banditScript}}
	catalog := script.Catalog{banditScript: banditstronghold.New}
	if s.q504 {
		listing = append(listing, script.Listing{Path: "quest." + q504Name})
		catalog["quest."+q504Name] = q504.New
		s.seed = append(s.seed, "INSERT INTO character_quests (charId, name, var, value) SELECT obj_Id, '"+q504Name+"', '<state>', 'STARTED' FROM characters WHERE char_name = 'Attacker'",
			"INSERT INTO character_quests (charId, name, var, value) SELECT obj_Id, '"+q504Name+"', '<cond>', '1' FROM characters WHERE char_name = 'Attacker'")
	}
	w.srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Attacker", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithHTMLPages(banditPages(t)),
		gameservertest.WithNPCs(flagWarTemplates()),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithMakers(script.MakerCatalog{"farm_maker": maker.Farm}),
		gameservertest.WithNPCScripts(flagWarKinds(), listing, catalog),
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), questItem(tarlkAmuletID), questItem(contestCertificateID), questItem(trophyID)))),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithLog(zerolog.New(zerolog.NewTestWriter(t)).Level(zerolog.WarnLevel)),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedFlagWar(t, db, s, w.date) }),
	)
	if !w.srv.DrivesClock() {
		t.Skip("the hall siege steps run on the driven clock")
	}
	w.c, w.objID = w.srv.Client, w.srv.SoleObjectID(t)
	hs, ok := w.srv.Halls.Siege(bandit)
	if !ok {
		t.Fatal("the Bandit Stronghold has no siege")
	}
	w.siege = hs
	for _, id := range slices.Sorted(maps.Keys(s.items)) {
		w.srv.GiveItem(t, w.objID, id, s.items[id])
	}
	startInWorld(t, w.c)
	drainFrames(t, w.c)
	return w
}

// banditPages are the Bandit Stronghold's shipped pages.
func banditPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "script", "siegablehall", "BanditStronghold")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		pages["script/siegablehall/BanditStronghold/"+e.Name()] = string(data)
	}
	return pages
}

func seedFlagWar(t *testing.T, db *sql.DB, s flagWarSetup, date int64) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed flag war: %s: %v", q, err)
		}
	}
	exec("UPDATE characters SET clanid = ? WHERE char_name = 'Attacker'", besiegersClanID)
	exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
		SELECT ?, 'Besiegers', 5, obj_Id FROM characters WHERE char_name = 'Attacker'`, besiegersClanID)
	exec("INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (?, 'Holders', 5)", holdersClanID)
	owner := int32(0)
	if s.owned {
		owner = holdersClanID
	}
	exec("INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (?, ?, ?, 1, ?)", bandit, owner, hallPaidUntil, date)
	for _, q := range s.seed {
		exec(q)
	}
}

// flagWarNPC is an NPC of the war in the world.
type flagWarNPC interface {
	ObjectID() int32
	Position() (x, y, z int)
}

// npcsOf returns the NPCs of template id in the world, by object id.
func (w *flagWarWorld) npcsOf(id int) []flagWarNPC {
	var out []flagWarNPC
	for _, obj := range w.srv.State.Objects() {
		switch o := obj.(type) {
		case *npc.Hostile:
			if o.Instance.Template.ID == id && !o.Dead() {
				out = append(out, o)
			}
		case *npc.Folk:
			if o.Instance.Template.ID == id {
				out = append(out, o)
			}
		}
	}
	slices.SortFunc(out, func(a, b flagWarNPC) int { return int(a.ObjectID() - b.ObjectID()) })
	return out
}

// standing returns, per template id of ids, how many of its NPCs stand in
// the world, the ids none stands for left out.
func (w *flagWarWorld) standing(ids ...int) map[int]int {
	out := map[int]int{}
	for _, id := range ids {
		if n := len(w.npcsOf(id)); n > 0 {
			out[id] = n
		}
	}
	return out
}

// warriors returns the living warriors of template id with the clan each
// stands for, and where.
func (w *flagWarWorld) warriors(id int) []*npc.Hostile {
	var out []*npc.Hostile
	for _, n := range w.npcsOf(id) {
		if h, ok := n.(*npc.Hostile); ok {
			out = append(out, h)
		}
	}
	return out
}

// kill has the Attacker kill h, then lets its dying hooks run.
func (w *flagWarWorld) kill(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.srv.RunScript(t, w.objID, banditScriptName, func(_ *script.Script, p *script.Player) { script.NPCOf(h).DoDie(p) })
	w.srv.Advance(t, 3*time.Second)
}

// talk clicks the NPC n, again when the click only targeted it, and
// returns the frames it got back.
func (w *flagWarWorld) talk(t *testing.T, n flagWarNPC) [][]byte {
	t.Helper()
	x, y, z := n.Position()
	at := location.Location{X: x, Y: y, Z: z}
	w.c.Send(encodeAction(n.ObjectID(), at))
	w.srv.Settle(t)
	frames := drainFrames(t, w.c)
	if _, targeted := firstOpcode(frames, serverpackets.OpcodeMyTargetSelected); !targeted {
		return frames
	}
	w.c.Send(encodeAction(n.ObjectID(), at))
	w.srv.Settle(t)
	return drainFrames(t, w.c)
}

// bypass sends the messenger link event and returns the frames it got
// back.
func (w *flagWarWorld) bypass(t *testing.T, event string) [][]byte {
	t.Helper()
	w.c.Send(encodeBypass("Quest " + banditScriptName + " " + event))
	w.srv.Settle(t)
	return drainFrames(t, w.c)
}

// page returns the html of the first NpcHtmlMessage among frames.
func page(t *testing.T, what string, frames [][]byte) string {
	t.Helper()
	f, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("%s: no page among %x", what, opcodes(frames))
	}
	r := wire.NewReader(f[1:])
	r.ReadInt32()
	return r.ReadString()
}

// wantPage fails unless frames show the shipped page file, %objectId%
// filled with objectID, but for the white space around it.
func wantPage(t *testing.T, what string, frames [][]byte, file string, objectID int32) {
	t.Helper()
	want := strings.ReplaceAll(banditPages(t)["script/siegablehall/BanditStronghold/"+file], "%objectId%", strconv.Itoa(int(objectID)))
	if got := page(t, what, frames); strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("%s page = %q, want %s", what, got, file)
	}
}

// messenger returns the hall's messenger.
func (w *flagWarWorld) messenger(t *testing.T) flagWarNPC {
	t.Helper()
	list := w.npcsOf(banditMessengerID)
	if len(list) != 1 {
		t.Fatalf("messengers in the world = %d, want 1", len(list))
	}
	return list[0]
}

// rows reads query's int32 rows, each joined by commas.
func (w *flagWarWorld) rows(t *testing.T, query string) []string {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.DB.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for rows.Next() {
		vals := make([]int64, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, v := range vals {
			parts = append(parts, strconv.FormatInt(v, 10))
		}
		out = append(out, strings.Join(parts, ","))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (w *flagWarWorld) flagWarClans(t *testing.T) []string {
	return w.rows(t, "SELECT clanhall_id, flag, npc, clan_id FROM clanhall_flagwar_attackers ORDER BY flag")
}

func (w *flagWarWorld) flagWarMembers(t *testing.T) []string {
	return w.rows(t, "SELECT clanhall_id, clan_id, object_id FROM clanhall_flagwar_members ORDER BY clan_id, object_id")
}

func (w *flagWarWorld) ownerNPCs(t *testing.T) []string {
	return w.rows(t, "SELECT clanhall_id, npc_id, clan_id FROM clanhall_flagwar_owner_npcs")
}

func (w *flagWarWorld) hallOwner(t *testing.T) int32 {
	t.Helper()
	got := w.rows(t, "SELECT ownerId FROM clanhall WHERE id = 35")
	if len(got) != 1 {
		t.Fatalf("hall rows = %v", got)
	}
	id, _ := strconv.Atoi(got[0])
	return int32(id)
}

// restartPoint returns the restart point the Attacker's position holds
// for clanID.
func (w *flagWarWorld) restartPoint(t *testing.T, clanID int32) (location.Location, bool) {
	t.Helper()
	x, y, z := w.srv.PlayerPosition(t, w.objID)
	return w.srv.Halls.ClanRestartPointAt(x, y, z, clanID)
}

func row(vals ...int64) string {
	var parts []string
	for _, v := range vals {
		parts = append(parts, strconv.FormatInt(v, 10))
	}
	return strings.Join(parts, ",")
}

// wantStanding fails unless the NPCs of ids standing are want.
func (w *flagWarWorld) wantStanding(t *testing.T, step string, want map[int]int, ids ...int) {
	t.Helper()
	if got := w.standing(ids...); !mapsEqual(got, want) {
		t.Fatalf("%s: NPCs standing = %v, want %v", step, got, want)
	}
}

func mapsEqual(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// flagWarIDs are every flag and warrior id.
var flagWarIDs = []int{35422, 35423, 35424, 35425, 35426, 35427, 35428, 35429, 35430, 35431, 35432}

// teleporterIDs are the teleporters' ids.
var teleporterIDs = []int{35560, 35561, 35562, 35563, 35564, 35565}

// TestBanditStrongholdFlagWarToTheFinal plays a flag war over the Bandit
// Stronghold, owned by Holders, which stored the warrior 35430 as its own.
// Until the registrations close, the messenger names the owner and answers
// a registration with the next siege date. At the close Holders registers
// under the royal flag. The Attacker then pays 200,000 adena for Besiegers,
// which fights under the yellow flag, picks the first warrior and sees
// itself listed, each through the links of the page before. At the siege date the flags of Besiegers and of the four
// free points stand, the players around the messenger hear the siege begin
// and the countdown, and 5 minutes later the tryouts begin: the battlefield
// turns on, Besiegers' warrior and four warriors of no clan stand, the
// maker's teleporters appear and Besiegers' players restart at its own.
// The teleporter beside the Attacker sends it on once, then is not ready.
// Beating the four warriors of no clan leaves Besiegers and the owner: the
// players hear Besiegers won its match and the final begins, the
// battlefield turns off and the two flags stand on the final points; 5
// minutes later the final begins with Holders' warrior 35430 and
// Besiegers' 35428, and the royal flag's teleporter appears. Beating
// Holders' warrior gives Besiegers the hall: everyone is told, the maker's
// teleporters and the war's NPCs go, the flag war tables are cleared, and
// the owner NPC row, which a hall keeps once, stays Holders'.
func TestBanditStrongholdFlagWarToTheFinal(t *testing.T) {
	t.Parallel()
	w := bootFlagWar(t, flagWarSetup{owned: true, seed: []string{
		"INSERT INTO clanhall_flagwar_owner_npcs VALUES (35, 35430, " + strconv.Itoa(int(holdersClanID)) + ")",
	}})
	messenger := w.messenger(t)

	if html := page(t, "the messenger", w.talk(t, messenger)); !strings.Contains(html, "Holders") || strings.Contains(html, "%my_pledge_name%") || strings.Contains(html, "%my_owner_name%") {
		t.Fatalf("the messenger's page = %q, want it naming Holders", html)
	}
	date := time.UnixMilli(w.date).Format("2006-01-02 15:04:05")
	if html := page(t, "register before the close", w.bypass(t, "register")); !strings.Contains(html, date) {
		t.Fatalf("register before the close = %q, want the siege date %s", html, date)
	}

	// The registrations close an hour before the date: the owner registers.
	w.srv.Advance(t, time.Duration(w.date-w.srv.Halls.Now()-3_600_000)*time.Millisecond)
	if !w.siege.WaitingBattle() {
		t.Fatal("the registrations did not close")
	}
	if got, want := w.flagWarClans(t), []string{row(35, royalFlagID, 0, int64(holdersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("flag war clans after the close = %v, want %v", got, want)
	}

	// The Attacker registers Besiegers: holding no trophy, it pays 200,000
	// adena.
	w.srv.RunScript(t, w.objID, banditScriptName, func(s *script.Script, p *script.Player) { s.GiveItems(p, 57, 200000) })
	drainFrames(t, w.c)
	w.talk(t, messenger)
	wantPage(t, "register after the close", w.bypass(t, "register"), "agit_oel_mahum_messeger_1.htm", messenger.ObjectID())
	wantPage(t, "register with no trophy", w.bypass(t, "register_clan"), "agit_oel_mahum_messeger_24.htm", messenger.ObjectID())
	wantPage(t, "register with adena", w.bypass(t, "register_with_adena"), "agit_oel_mahum_messeger_4b.htm", messenger.ObjectID())
	w.srv.RunScript(t, w.objID, banditScriptName, func(_ *script.Script, p *script.Player) {
		if n := p.Adena(); n != 0 {
			t.Errorf("adena after the registration = %d, want 0", n)
		}
	})
	if got, want := w.flagWarClans(t), []string{row(35, royalFlagID, 0, int64(holdersClanID)), row(35, yellowFlagID, 0, int64(besiegersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("flag war clans after Besiegers registered = %v, want %v", got, want)
	}
	if got := w.flagWarMembers(t); !slices.Contains(got, row(35, int64(besiegersClanID), int64(w.objID))) {
		t.Fatalf("flag war members = %v, want the Attacker for Besiegers", got)
	}
	if got := w.siege.Attackers(); !slices.Equal(got, []int32{holdersClanID, besiegersClanID}) {
		t.Fatalf("attackers = %#x, want Holders then Besiegers", got)
	}

	// Registered, it sees its flag; it picks the first warrior.
	w.talk(t, messenger)
	w.bypass(t, "register")
	wantPage(t, "a second registration", w.bypass(t, "register_clan"), "agit_oel_mahum_messeger_4b.htm", messenger.ObjectID())
	w.talk(t, messenger)
	w.bypass(t, "register")
	wantPage(t, "the warrior choice", w.bypass(t, "select_clan_npc"), "agit_oel_mahum_messeger_6.htm", messenger.ObjectID())
	wantPage(t, "the first warrior", w.bypass(t, "info_npc_1"), "agit_oel_mahum_messeger_11.htm", messenger.ObjectID())
	wantPage(t, "the warrior pick", w.bypass(t, "select_npc_1"), "agit_oel_mahum_messeger_9.htm", messenger.ObjectID())
	if got := w.flagWarClans(t); !slices.Contains(got, row(35, yellowFlagID, ally1ID, int64(besiegersClanID))) {
		t.Fatalf("flag war clans after the pick = %v, want Besiegers' warrior %d", got, ally1ID)
	}
	w.talk(t, messenger)
	w.bypass(t, "register")
	wantPage(t, "the warrior shown", w.bypass(t, "view_clan_npc"), "agit_oel_mahum_messeger_17.htm", messenger.ObjectID())
	w.talk(t, messenger)
	if html := page(t, "the participants", w.bypass(t, "view_participants")); !strings.Contains(html, "Besiegers") || !strings.Contains(html, "**unregistered**") || strings.Contains(html, "Holders") {
		t.Fatalf("participants = %q, want Besiegers listed and the free slots", html)
	}

	// The siege starts at its date: the flags stand.
	w.srv.Advance(t, time.Hour)
	if got := sysMsgParams(t, drainFrames(t, w.c)); !slices.Equal(flat(got), flat([][]string{{strconv.Itoa(msgSiegeBegun), "35"}, {strconv.Itoa(msgTryoutsSoon)}})) {
		t.Fatalf("start notices = %v", got)
	}
	w.wantStanding(t, "the start", map[int]int{35423: 1, 35424: 1, 35425: 1, 35426: 1, 35427: 1}, flagWarIDs...)
	if w.field.Active() {
		t.Fatal("the battlefield is on before the tryouts")
	}

	// The countdown, then the tryouts.
	w.srv.Advance(t, 240*time.Second)
	if got := sysMsgParams(t, drainFrames(t, w.c)); !slices.Equal(flat(got), flat([][]string{{strconv.Itoa(msgBeginsMinutes), "1"}})) {
		t.Fatalf("a minute before the tryouts = %v", got)
	}
	w.srv.Advance(t, 60*time.Second)
	want := [][]string{}
	for _, secs := range []int{20, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1} {
		want = append(want, []string{strconv.Itoa(msgBeginsSeconds), strconv.Itoa(secs)})
	}
	// The battlefield turning on tells the Attacker standing on it.
	want = append(want, []string{strconv.Itoa(msgTryoutsBegun)}, []string{"283"})
	if got := sysMsgParams(t, drainFrames(t, w.c)); !slices.Equal(flat(got), flat(want)) {
		t.Fatalf("countdown = %v, want %v", got, want)
	}
	if !w.siege.InProgress() || !w.field.Active() {
		t.Fatalf("after the tryouts began: in progress %v, battlefield %v", w.siege.InProgress(), w.field.Active())
	}
	w.wantStanding(t, "the tryouts", map[int]int{35423: 1, 35424: 1, 35425: 1, 35426: 1, 35427: 1, ally1ID: 1, ally2ID: 4}, flagWarIDs...)
	if mine := w.warriors(ally1ID); mine[0].Instance.ClanID != besiegersClanID {
		t.Fatalf("Besiegers' warrior stands for %#x", mine[0].Instance.ClanID)
	}
	w.wantStanding(t, "the tryouts' teleporters", map[int]int{35561: 1, 35562: 1, 35563: 1, 35564: 1, 35565: 1}, teleporterIDs...)
	if at, ok := w.restartPoint(t, besiegersClanID); !ok || at.X != 300 || at.Y != 20 {
		t.Fatalf("Besiegers' restart point = %v %v, want its teleporter's 300,20", at, ok)
	}
	if _, ok := w.restartPoint(t, holdersClanID); ok {
		t.Fatal("Holders restarts at a teleporter that is not in the world")
	}

	// The teleporter beside the Attacker sends it on, then is not ready.
	tp := w.npcsOf(35561)[0]
	w.talk(t, tp)
	if x, y, _ := w.srv.PlayerPosition(t, w.objID); x != teleportLanding.X || y != teleportLanding.Y {
		t.Fatalf("after the teleporter: the Attacker stands at %d,%d, want %v", x, y, teleportLanding)
	}
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	w.srv.Settle(t)
	drainFrames(t, w.c)
	wantPage(t, "the teleporter again", w.talk(t, tp), "teleporter_notyet.htm", tp.ObjectID())

	// The warriors of no clan fall: the final comes, and the battlefield
	// turning off tells the Attacker it left the combat zone.
	for _, h := range w.warriors(ally2ID) {
		w.kill(t, h)
	}
	if got := sysMsgParams(t, drainFrames(t, w.c)); !slices.Equal(flat(got), flat([][]string{{strconv.Itoa(msgWonMatch), "Besiegers", "35"}, {strconv.Itoa(msgFinalMatch)}, {"284"}})) {
		t.Fatalf("the tryouts' end = %v", got)
	}
	if w.field.Active() {
		t.Fatal("the battlefield stays on between the tryouts and the final")
	}
	w.wantStanding(t, "between the tryouts and the final", map[int]int{royalFlagID: 1, yellowFlagID: 1}, flagWarIDs...)
	if royal := w.npcsOf(royalFlagID)[0]; !near(royal, banditFlagPoint[5]) {
		t.Fatal("the royal flag does not stand on the owner's final point")
	}
	w.srv.Advance(t, 300*time.Second)
	if got := sysMsgParams(t, drainFrames(t, w.c)); !slices.Equal(flat(got), flat([][]string{{strconv.Itoa(msgFinalsBegun)}, {"283"}})) {
		t.Fatalf("the final's start = %v", got)
	}
	w.wantStanding(t, "the final", map[int]int{royalFlagID: 1, yellowFlagID: 1, ally1ID: 1, ally3ID: 1}, flagWarIDs...)
	theirs := w.warriors(ally3ID)[0]
	if theirs.Instance.ClanID != holdersClanID || !near(theirs, banditFlagPoint[5]) {
		t.Fatalf("Holders' warrior stands for %#x", theirs.Instance.ClanID)
	}
	if !w.field.Active() || len(w.npcsOf(teleport1ID)) != 1 {
		t.Fatal("the final began with the battlefield off or no royal teleporter")
	}
	if at, ok := w.restartPoint(t, holdersClanID); !ok || at.X != 340 {
		t.Fatalf("Holders' restart point in the final = %v %v, want the royal teleporter's", at, ok)
	}

	// Holders' warrior falls: Besiegers takes the hall.
	w.kill(t, theirs)
	got := sysMsgParams(t, drainFrames(t, w.c))
	if !slices.ContainsFunc(got, func(m []string) bool {
		return slices.Equal(m, []string{strconv.Itoa(msgClanDefeated), "Besiegers", "35"})
	}) ||
		!slices.ContainsFunc(got, func(m []string) bool { return slices.Equal(m, []string{strconv.Itoa(msgSiegeFinished), "35"}) }) {
		t.Fatalf("the end's notices = %v", got)
	}
	if owner := w.hallOwner(t); owner != besiegersClanID {
		t.Fatalf("hall owner = %#x, want Besiegers", owner)
	}
	if !w.siege.Registering() || w.field.Active() {
		t.Fatalf("after the end: registering %v, battlefield %v", w.siege.Registering(), w.field.Active())
	}
	w.wantStanding(t, "the end", map[int]int{}, append(flagWarIDs, teleporterIDs...)...)
	if len(w.npcsOf(banditMessengerID)) != 1 {
		t.Fatal("the messenger left with the siege")
	}
	if c, m := w.flagWarClans(t), w.flagWarMembers(t); len(c) != 0 || len(m) != 0 {
		t.Fatalf("flag war rows after the end: clans %v, members %v", c, m)
	}
	if got, want := w.ownerNPCs(t), []string{row(35, ally3ID, int64(holdersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("owner NPC rows = %v, want %v", got, want)
	}
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers after the end = %#x", got)
	}
	if _, ok := w.restartPoint(t, besiegersClanID); ok {
		t.Fatal("Besiegers keeps a restart point after the end")
	}
}

// TestBanditStrongholdCalledOffWithoutChallengers: an owner alone calls
// the siege off at its start: the owner leaves the war, the flag war
// tables are cleared, the players around the messenger hear it was called
// off for lack of interest, and the registrations open for the next date.
func TestBanditStrongholdCalledOffWithoutChallengers(t *testing.T) {
	t.Parallel()
	w := bootFlagWar(t, flagWarSetup{owned: true})
	if !w.siege.Instant() {
		t.Fatal("no script runs the Bandit Stronghold's siege")
	}
	w.srv.Advance(t, 0)
	if got := sysMsgParams(t, drainFrames(t, w.c)); !slices.ContainsFunc(got, func(m []string) bool { return slices.Equal(m, []string{strconv.Itoa(msgNoInterest), "35"}) }) {
		t.Fatalf("the start's notices = %v, want the lack of interest", got)
	}
	if !w.siege.Registering() || w.siege.InProgress() {
		t.Fatal("the called off siege does not take registrations")
	}
	if c, m := w.flagWarClans(t), w.flagWarMembers(t); len(c) != 0 || len(m) != 0 {
		t.Fatalf("flag war rows after the call off: clans %v, members %v", c, m)
	}
	if w.siege.Date() <= w.date {
		t.Fatalf("next siege %d, want after %d", w.siege.Date(), w.date)
	}
	w.wantStanding(t, "the call off", map[int]int{}, flagWarIDs...)
}

// TestBanditStrongholdLoneChallengerTakesAFreeHall: the free hall's stored
// registrations (Besiegers under the red flag with the Attacker, Holders
// under the yellow with the warrior 35430) are loaded at boot. In the
// tryouts Holders' warrior falls and Holders leaves; the warriors of no
// clan still stand, so the war goes on until they fall too. Then
// Besiegers, the last flag, takes the free hall, which stores its warrior,
// none picked, as the owner's.
func TestBanditStrongholdLoneChallengerTakesAFreeHall(t *testing.T) {
	t.Parallel()
	besiegers, holders := strconv.Itoa(int(besiegersClanID)), strconv.Itoa(int(holdersClanID))
	w := bootFlagWar(t, flagWarSetup{seed: []string{
		"INSERT INTO clanhall_flagwar_attackers VALUES (35, 35423, 0, " + besiegers + ")",
		"INSERT INTO clanhall_flagwar_attackers VALUES (35, 35424, 35430, " + holders + ")",
		"INSERT INTO clanhall_flagwar_members SELECT 35, " + besiegers + ", obj_Id FROM characters WHERE char_name = 'Attacker'",
	}})
	if !w.siege.Instant() {
		t.Fatal("no script runs the Bandit Stronghold's siege")
	}
	w.srv.Advance(t, 0)
	w.wantStanding(t, "the start", map[int]int{35423: 1, 35424: 1, 35425: 1, 35426: 1, 35427: 1}, flagWarIDs...)
	if red := w.npcsOf(redFlagID)[0]; !near(red, banditFlagPoint[0]) {
		t.Fatal("the red flag does not stand on its point")
	}
	w.srv.Advance(t, 300*time.Second)
	drainFrames(t, w.c)
	w.wantStanding(t, "the tryouts", map[int]int{35423: 1, 35424: 1, 35425: 1, 35426: 1, 35427: 1, ally2ID: 4, ally3ID: 1}, flagWarIDs...)

	w.kill(t, w.warriors(ally3ID)[0])
	if !w.siege.InProgress() {
		t.Fatal("the siege ended with warriors of no clan standing")
	}
	if _, ok := w.restartPoint(t, holdersClanID); ok {
		t.Fatal("Holders keeps its restart point once out of the war")
	}
	for _, h := range w.warriors(ally2ID) {
		if h.Instance.ClanID == besiegersClanID {
			continue
		}
		w.kill(t, h)
	}
	if w.siege.InProgress() {
		t.Fatal("the siege goes on with one flag standing")
	}
	if owner := w.hallOwner(t); owner != besiegersClanID {
		t.Fatalf("hall owner = %#x, want Besiegers", owner)
	}
	if got, want := w.ownerNPCs(t), []string{row(35, 0, int64(besiegersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("owner NPC rows = %v, want %v", got, want)
	}
	if x, y, _ := w.srv.PlayerPosition(t, w.objID); x != 10 || y != 20 {
		t.Fatalf("the owner's Attacker was moved to %d,%d", x, y)
	}
}

// TestBanditStrongholdTrophyRegistration: the Attacker, holding two
// Trophies of Alliance, 31 Tarlk Amulets and a Contest Certificate with
// Competition for the Bandit Stronghold started, registers Besiegers after
// the close by its trophy. One trophy pays the registration; then 30
// amulets, every certificate and the trophies left are taken, the quest
// ends with its finish sound and its rows are deleted, and Besiegers
// fights under the red flag, the first free.
func TestBanditStrongholdTrophyRegistration(t *testing.T) {
	t.Parallel()
	w := bootFlagWar(t, flagWarSetup{q504: true, items: map[int32]int32{trophyID: 2, tarlkAmuletID: 31, contestCertificateID: 1}})
	messenger := w.messenger(t)
	w.srv.Advance(t, time.Duration(w.date-w.srv.Halls.Now()-3_600_000)*time.Millisecond)
	if !w.siege.WaitingBattle() {
		t.Fatal("the registrations did not close")
	}
	drainFrames(t, w.c)

	w.talk(t, messenger)
	wantPage(t, "register after the close", w.bypass(t, "register"), "agit_oel_mahum_messeger_1.htm", messenger.ObjectID())
	frames := w.bypass(t, "register_clan")
	wantPage(t, "the trophy registration", frames, "agit_oel_mahum_messeger_4a.htm", messenger.ObjectID())
	sound, ok := firstOpcode(frames, serverpackets.OpcodePlaySound)
	if !ok {
		t.Fatalf("the registration played no sound among %x", opcodes(frames))
	}
	r := wire.NewReader(sound[1:])
	r.ReadInt32()
	if file := r.ReadString(); file != script.SoundFinish {
		t.Fatalf("the registration played %q, want %q", file, script.SoundFinish)
	}
	w.srv.RunScript(t, w.objID, banditScriptName, func(_ *script.Script, p *script.Player) {
		for id, want := range map[int32]int32{trophyID: 0, tarlkAmuletID: 1, contestCertificateID: 0} {
			if n := p.ItemCount(id); n != want {
				t.Errorf("item %d after the registration = %d, want %d", id, n, want)
			}
		}
	})
	if got := w.rows(t, "SELECT COUNT(*) FROM character_quests WHERE name = '"+q504Name+"'"); !slices.Equal(got, []string{"0"}) {
		t.Fatalf("Q504 rows after the registration = %v, want none", got)
	}
	if got, want := w.flagWarClans(t), []string{row(35, redFlagID, 0, int64(besiegersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("flag war clans = %v, want %v", got, want)
	}
	if got, want := w.flagWarMembers(t), []string{row(35, int64(besiegersClanID), int64(w.objID))}; !slices.Equal(got, want) {
		t.Fatalf("flag war members = %v, want %v", got, want)
	}
}

// TestBanditStrongholdTryoutsTimeOutKeepTheOwner: the tryouts of the hall
// Holders owns run their 20 minutes with Besiegers' warrior standing. The
// first phase ends drawn: Holders keeps the hall, everyone is told the
// match was drawn and the siege is over, the flag war rows are cleared,
// the owner NPC row stays and the registrations open again.
func TestBanditStrongholdTryoutsTimeOutKeepTheOwner(t *testing.T) {
	t.Parallel()
	besiegers, holders := strconv.Itoa(int(besiegersClanID)), strconv.Itoa(int(holdersClanID))
	w := bootFlagWar(t, flagWarSetup{owned: true, seed: []string{
		"INSERT INTO clanhall_flagwar_attackers VALUES (35, 35423, 0, " + besiegers + ")",
		"INSERT INTO clanhall_flagwar_members SELECT 35, " + besiegers + ", obj_Id FROM characters WHERE char_name = 'Attacker'",
		"INSERT INTO clanhall_flagwar_owner_npcs VALUES (35, 35430, " + holders + ")",
	}})
	if !w.siege.Instant() {
		t.Fatal("no script runs the Bandit Stronghold's siege")
	}
	w.srv.Advance(t, 0)
	w.srv.Advance(t, 300*time.Second)
	if !w.siege.InProgress() {
		t.Fatal("the tryouts did not begin")
	}
	w.wantStanding(t, "the tryouts", map[int]int{35423: 1, 35424: 1, 35425: 1, 35426: 1, 35427: 1, ally2ID: 5}, flagWarIDs...)
	drainFrames(t, w.c)

	w.srv.Advance(t, 20*time.Minute)
	got := flat(sysMsgParams(t, drainFrames(t, w.c)))
	draw, over := slices.Index(got, strconv.Itoa(msgMatchDraw)+" 35"), slices.Index(got, strconv.Itoa(msgSiegeFinished)+" 35")
	if draw < 0 || over < draw {
		t.Fatalf("the time-out's notices = %v, want %d then %d", got, msgMatchDraw, msgSiegeFinished)
	}
	if owner := w.hallOwner(t); owner != holdersClanID {
		t.Fatalf("hall owner = %#x, want Holders", owner)
	}
	if !w.siege.Registering() || w.field.Active() {
		t.Fatalf("after the time-out: registering %v, battlefield %v", w.siege.Registering(), w.field.Active())
	}
	if c, m := w.flagWarClans(t), w.flagWarMembers(t); len(c) != 0 || len(m) != 0 {
		t.Fatalf("flag war rows after the time-out: clans %v, members %v", c, m)
	}
	if got, want := w.ownerNPCs(t), []string{row(35, ally3ID, int64(holdersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("owner NPC rows = %v, want %v", got, want)
	}
	w.wantStanding(t, "the time-out", map[int]int{}, append(flagWarIDs, teleporterIDs...)...)
}

// near reports whether n stands within 50 of at on the plane.
func near(n flagWarNPC, at location.Location) bool {
	x, y, _ := n.Position()
	dx, dy := x-at.X, y-at.Y
	return dx*dx+dy*dy <= 50*50
}

// flat joins each message's fields, for comparing message lists.
func flat(msgs [][]string) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, strings.Join(m, " "))
	}
	return out
}
