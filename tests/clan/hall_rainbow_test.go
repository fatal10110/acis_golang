package clan

import (
	"context"
	"database/sql"
	"fmt"
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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/rainbowspringschateau"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Reference: RainbowSpringsChateau on ClanHallSiege (onFirstTalk,
// onAdvEvent, setFinalAttackers, siegeStart, siegeEnd, setMiniGameWinner,
// spawnNpcs, endSiege).
//
// Rainbow Springs Chateau (62): clanHalls.xml siegeLength 3600000. Clan
// leaders hand their war decrees (8034) to the messenger (35604); at the
// siege date the owner and the clans with the most decrees, four in all,
// are chosen, and an hour later the game starts: the arenas' NPCs
// (agit_defend_warfare_start(62)) stand and the caretaker (35603, the first
// NPC of the hall's maker) tells the players around it. The game's end is
// always a draw; the hall's maker then places its cleaners (35600) and the
// players left on the battlefield go to town two minutes later.

const (
	rainbowHall      int32 = 62
	rainbowScript          = "script.siegablehall.RainbowSpringsChateau"
	rainbowMessenger       = 35604
	rainbowCaretaker       = 35603
	rainbowCleaner         = 35600
	rainbowGourd           = 35588
	// rainbowGourd2 is the second arena's gourd.
	rainbowGourd2 = 35589
	// rainbowServitor is the servitor a party member calls before the
	// entry.
	rainbowServitor = 14001
	rainbowDecree   = 8034

	// The clans of the Rainbow Springs scenarios: the Attacker leads
	// Springers.
	springersClanID int32 = 0x7f001101
	bathersClanID   int32 = 0x7f001102
	soakersClanID   int32 = 0x7f001103
	waders4ClanID   int32 = 0x7f001104
	fifthClanID     int32 = 0x7f001105
	leaversClanID   int32 = 0x7f001106
	// rainbowDecreeObject is the Attacker's stack of war decrees.
	rainbowDecreeObject int32 = 0x7f001201
)

// The arena entry: the caretaker's post, inside the battlefield, where
// every character of the scenarios starts, and the first arena.
var (
	rainbowPost   = location.Location{X: 152924, Y: -126604, Z: -2293}
	rainbowArena1 = location.Location{X: 151562, Y: -127080, Z: -2214}
	// rainbowGoddard is where a restart to town from the arenas leads.
	rainbowGoddard = location.Location{X: 147450, Y: -55400, Z: -2728}
)

// rainbowSpawnlist holds the messenger by the caretaker's post, the hall's
// maker (the caretaker, then four cleaners) and one arena maker holding the
// first arena's gourd.
const rainbowSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rainbow_post" minZ="-2400" maxZ="-2100"><node x="152000" y="-127000"/><node x="153500" y="-127000"/><node x="153500" y="-126000"/><node x="152000" y="-126000"/></territory>
	<npcmaker name="godard06_npc2414_01m1" territory="rainbow_post" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="35604" pos="152970;-126604;-2293;0" total="1" respawn="1min"/>
	</npcmaker>
	<npcmaker name="godard06_npc2414_02m1" territory="rainbow_post" maximumNpcs="5">
		<ai type="rainbow_maker"><set name="CastleID" val="62"/></ai>
		<npc id="35603" pos="152924;-126604;-2293;33000" total="1" respawn="0sec"/>
		<npc id="35600" pos="151556;-127202;-2208;16384" total="1" respawn="0sec"/>
		<npc id="35600" pos="153136;-125219;-2208;16384" total="1" respawn="0sec"/>
		<npc id="35600" pos="155658;-125638;-2208;16384" total="1" respawn="0sec"/>
		<npc id="35600" pos="153887;-127654;-2208;16384" total="1" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="godard06_2414_01" territory="rainbow_post" spawnTime="agit_defend_warfare_start(62)" maximumNpcs="4">
		<ai type="default_maker"/>
		<npc id="35588" pos="151557;-126987;-2224;0" total="1" respawn="2hour"/>
		<npc id="35589" pos="153135;-125425;-2224;0" total="1" respawn="2hour"/>
	</npcmaker>
</list>`

// rainbowMember is a character of the scenarios other than the Attacker.
type rainbowMember struct {
	castleRival
	leads bool
}

// rainbowSetup is one Rainbow Springs boot.
type rainbowSetup struct {
	// owner holds the hall, 0 for none.
	owner int32
	// date is how far the siege date lies after the boot.
	date time.Duration
	// decrees are the stored registrations, per clan.
	decrees map[int32]int
	// dissolving clans have asked to be dissolved.
	dissolving []int32
	// heldDecrees is how many war decrees the Attacker holds.
	heldDecrees int
	members     []rainbowMember
}

type rainbowWorld struct {
	srv   *gameservertest.Server
	c     *testsupport.ScriptedClient
	objID int32
	siege *clanhall.HallSiege
	date  time.Time
}

// springersMembers are the four other members of Springers, none leading.
func springersMembers() []rainbowMember {
	var out []rainbowMember
	for i := range int32(4) {
		out = append(out, rainbowMember{castleRival: castleRival{0x7f001001 + i, "spring" + strconv.Itoa(int(i)), "Springer" + strconv.Itoa(int(i)), springersClanID}})
	}
	return out
}

func bootRainbow(t *testing.T, s rainbowSetup) *rainbowWorld {
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
	if err := os.WriteFile(dir+"/rainbow.xml", []byte(rainbowSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	makers, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	field := zone.NewSiege(1, cuboid(t, 150000, 158000, -129000, -123000))
	field.ResidenceID = int(rainbowHall)
	grounds := zone.NewClanHall(2, cuboid(t, 140500, 141500, -125100, -124100))
	grounds.ResidenceID = int(rainbowHall)
	zones := zone.NewIndex()
	zones.Add(field)
	zones.Add(grounds)
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	templates = append(templates, &npc.Template{
		ID: rainbowServitor, TemplateID: rainbowServitor, Type: "Servitor", Name: "Bath Servant", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 10, CollisionHeight: 20,
	})
	for _, id := range []int{rainbowMessenger, rainbowCaretaker, rainbowCleaner, rainbowGourd, rainbowGourd2} {
		templates = append(templates, &npc.Template{
			ID: id, TemplateID: id, Type: "Folk", Name: "Rainbow " + strconv.Itoa(id), Level: 1, HPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 10,
		})
		kinds[int32(id)] = script.KindFolk
	}
	itemTemplates := append([]*item.Template(nil), gameservertest.ItemTemplates().All()...)
	itemTemplates = append(itemTemplates, &item.Template{
		ID: rainbowDecree, Name: "Rainbow Springs Clan Hall War Decree", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
	})
	rivals := make([]castleRival, 0, len(s.members))
	for _, m := range s.members {
		rivals = append(rivals, m.castleRival)
	}
	date := time.Now().Add(s.date).Truncate(time.Minute)
	w := &rainbowWorld{date: date}
	w.srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Attacker", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithHTMLPages(rainbowPages(t)),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithMakers(script.MakerCatalog{"rainbow_maker": maker.Rainbow}),
		gameservertest.WithItemTemplates(item.NewTable(itemTemplates)),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: rainbowScript}}, script.Catalog{rainbowScript: rainbowspringschateau.New}),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithRestartPoints(rainbowTown(t)),
		gameservertest.WithLog(zerolog.New(zerolog.NewTestWriter(t)).Level(zerolog.WarnLevel)),
		seedCastleRivals(t, rivals),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedRainbow(t, db, s, date) }),
	)
	if !w.srv.DrivesClock() {
		t.Skip("the hall siege steps run on the driven clock")
	}
	w.c, w.objID = w.srv.Client, w.srv.SoleObjectID(t)
	hs, ok := w.srv.Halls.Siege(rainbowHall)
	if !ok {
		t.Fatal("Rainbow Springs has no siege")
	}
	w.siege = hs
	startInWorld(t, w.c)
	drainFrames(t, w.c)
	return w
}

// rainbowTown sends a human restarting to town from around the arenas to
// Goddard.
func rainbowTown(t *testing.T) *restart.Table {
	t.Helper()
	area, err := restart.NewArea([]location.Point{{X: 140000, Y: -130000}, {X: 160000, Y: -130000}, {X: 160000, Y: -120000}, {X: 140000, Y: -120000}},
		-10_000, 10_000, map[player.Race]string{player.RaceHuman: "goddard"})
	if err != nil {
		t.Fatal(err)
	}
	return &restart.Table{Areas: []restart.Area{area}, Points: []restart.Point{{Name: "goddard", Points: []location.Location{rainbowGoddard}}}}
}

// rainbowPages are the hall's shipped pages.
func rainbowPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "script", "siegablehall", "RainbowSpringsChateau")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		pages["script/siegablehall/RainbowSpringsChateau/"+e.Name()] = string(data)
	}
	return pages
}

// seedRainbow puts the Attacker at the head of Springers, level 5, with its
// other members; seeds the other clans, the hall's row with its owner and
// siege date, the stored decrees and the Attacker's war decrees; and stands
// everyone at the caretaker's post.
func seedRainbow(t *testing.T, db *sql.DB, s rainbowSetup, date time.Time) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed: %s: %v", q, err)
		}
	}
	exec("UPDATE characters SET clanid = ?, x = ?, y = ?, z = ?", springersClanID, rainbowPost.X, rainbowPost.Y, rainbowPost.Z)
	exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT ?, 'Springers', 5, obj_Id FROM characters WHERE char_name = 'Attacker'", springersClanID)
	leaders := map[int32]int32{}
	for _, m := range s.members {
		exec("UPDATE characters SET clanid = ? WHERE obj_Id = ?", m.clan, m.id)
		if m.leads {
			leaders[m.clan] = m.id
		}
	}
	for i, id := range []int32{bathersClanID, soakersClanID, waders4ClanID, fifthClanID, leaversClanID} {
		var expiry int64
		if slices.Contains(s.dissolving, id) {
			expiry = time.Now().Add(24 * time.Hour).UnixMilli()
		}
		exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, dissolving_expiry_time) VALUES (?, ?, 5, ?, ?)",
			id, "Rival"+strconv.Itoa(i), leaders[id], expiry)
	}
	exec("INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (?, ?, ?, 1, ?)", rainbowHall, s.owner, hallPaidUntil, date.UnixMilli())
	for clanID, n := range s.decrees {
		exec("INSERT INTO rainbowsprings_attacker_list (clanId, war_decrees_count) VALUES (?, ?)", clanID, n)
	}
	if s.heldDecrees > 0 {
		exec("INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) SELECT obj_Id, ?, ?, ?, 'INVENTORY', 0 FROM characters WHERE char_name = 'Attacker'",
			rainbowDecreeObject, rainbowDecree, s.heldDecrees)
	}
}

// npcOf returns the one NPC of template id in the world.
func (w *rainbowWorld) npcOf(t *testing.T, id int) *npc.Folk {
	t.Helper()
	got := w.npcsOf(id)
	if len(got) != 1 {
		t.Fatalf("NPCs of %d in the world = %d, want 1", id, len(got))
	}
	return got[0]
}

func (w *rainbowWorld) npcsOf(id int) []*npc.Folk {
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.Instance.Template.ID == id {
			out = append(out, f)
		}
	}
	return out
}

// talk has c click n, again when the click only targeted it, and returns
// the frames c got back.
func (w *rainbowWorld) talk(t *testing.T, c *testsupport.ScriptedClient, n *npc.Folk) [][]byte {
	t.Helper()
	x, y, z := n.Position()
	at := location.Location{X: x, Y: y, Z: z}
	c.Send(encodeAction(n.ObjectID(), at))
	w.srv.Settle(t)
	frames := drainFrames(t, c)
	if _, targeted := firstOpcode(frames, serverpackets.OpcodeMyTargetSelected); !targeted {
		return frames
	}
	c.Send(encodeAction(n.ObjectID(), at))
	w.srv.Settle(t)
	return drainFrames(t, c)
}

// link has c follow the script's link event and returns the frames c got
// back.
func (w *rainbowWorld) link(t *testing.T, c *testsupport.ScriptedClient, event string) [][]byte {
	t.Helper()
	c.Send(encodeBypass("Quest RainbowSpringsChateau " + event))
	w.srv.Settle(t)
	return drainFrames(t, c)
}

// wantRainbowPage fails unless frames show the shipped page file, but for
// %objectId% and the white space around it.
func wantRainbowPage(t *testing.T, what string, frames [][]byte, file string) {
	t.Helper()
	want := rainbowPages(t)["script/siegablehall/RainbowSpringsChateau/"+file]
	got := page(t, what, frames)
	if strings.Contains(want, "%objectId%") {
		t.Fatalf("%s: page %s names the NPC; compare it by hand", what, file)
	}
	norm := func(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }
	if norm(got) != norm(want) {
		t.Fatalf("%s page = %q, want %s", what, got, file)
	}
}

// storedDecrees reads rainbowsprings_attacker_list.
func (w *rainbowWorld) storedDecrees(t *testing.T) map[int32]int {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.DB.QueryContext(context.Background(), "SELECT clanId, war_decrees_count FROM rainbowsprings_attacker_list")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int32]int{}
	for rows.Next() {
		var id int32
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatal(err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// heldDecrees reads how many war decrees the Attacker's stored inventory
// holds.
func (w *rainbowWorld) heldDecrees(t *testing.T) int {
	t.Helper()
	w.srv.FlushItems(t)
	var n sql.NullInt64
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT SUM(count) FROM items WHERE owner_id = ? AND item_id = ? AND loc = 'INVENTORY'", w.objID, rainbowDecree).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return int(n.Int64)
}

// hallOwner reads the hall row's owner.
func (w *rainbowWorld) hallOwner(t *testing.T) int32 {
	t.Helper()
	w.srv.FlushPersistence(t)
	var owner int32
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT ownerId FROM clanhall WHERE id = ?", rainbowHall).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	return owner
}

// member brings m into the world.
func (w *rainbowWorld) member(t *testing.T, m rainbowMember) *testsupport.ScriptedClient {
	t.Helper()
	c := w.srv.DialClient(t, m.account, 1)
	startInWorld(t, c)
	drainFrames(t, c)
	drainFrames(t, w.c)
	return c
}

// invite has the Attacker invite the character name into its party, which
// c, its client, accepts.
func (w *rainbowWorld) invite(t *testing.T, c *testsupport.ScriptedClient, name string) {
	t.Helper()
	req := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	req.WriteString(name)
	req.WriteInt32(0)
	w.c.Send(req.Bytes())
	w.srv.Settle(t)
	answer := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	answer.WriteInt32(1)
	c.Send(answer.Bytes())
	w.srv.Settle(t)
	drainFrames(t, c)
	drainFrames(t, w.c)
}

// buffAndSummon lands a buff on the member id and has it call a servitor.
func (w *rainbowWorld) buffAndSummon(t *testing.T, id int32) *player.Character {
	t.Helper()
	obj, ok := w.srv.State.Player(id)
	if !ok {
		t.Fatalf("player %#x is not in the world", id)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %#x = %T, want an online character", id, obj)
	}
	onPlayerQueue(t, w.srv, id, func() {
		e, err := effect.New(effect.Skill{ID: 1040, Level: 1, SkillType: "BUFF"}, skill.EffectTemplate{Name: "Buff", Time: 1200, Icon: true})
		if err != nil {
			t.Errorf("effect.New: %v", err)
			return
		}
		e.Effector, e.Effected = c, c
		c.EffectList().Add(e)
		c.SummonServitor(skill.Definition{ID: 1111, Level: 1, SkillType: "SUMMON", NpcID: rainbowServitor, SummonTotalLifeTime: 1_200_000})
	})
	w.srv.AdvanceUntil(t, "the servitor is in the world", func() bool {
		_, ok := w.srv.State.Summon(id)
		return ok
	})
	var effects int
	onPlayerQueue(t, w.srv, id, func() { effects = len(c.EffectList().All()) })
	if effects != 1 {
		t.Fatalf("the member holds %d effects after the buff, want 1", effects)
	}
	return c
}

// advanceTo lets the driven clock run to at, in Unix milliseconds.
func (w *rainbowWorld) advanceTo(t *testing.T, at int64) {
	t.Helper()
	w.srv.Advance(t, time.Duration(at-w.srv.Halls.Now())*time.Millisecond)
}

// registrationEnd writes date as the messenger names the registrations'
// deadline: year-month-day hour:minutes, on a 12-hour clock with no zero
// padding but the minutes'.
func registrationEnd(date time.Time) string {
	return fmt.Sprintf("%d-%d-%d %d:%02d", date.Year(), int(date.Month()), date.Day(), date.Hour()%12, date.Minute())
}

// sysMsgIDs returns the ids of the system messages among frames, with
// each one's parameters after a space.
func sysMsgIDs(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, p := range sysMsgParams(t, frames) {
		out = append(out, strings.Join(p, " "))
	}
	return out
}

// TestRainbowSpringsMessengerTakesRegistrations runs the messenger's
// registration through the real hall script. Its page names the
// registrations' deadline, the siege date on a 12-hour clock, and no owner.
// A clan member who does not lead is refused (messenger_yetti010). The
// leader of Springers, unregistered, cannot cancel (016); its registration
// takes all 7 of its war decrees and stores them (009); a second one is
// refused (013); its cancellation gives back 3, half of them, and drops
// the stored row (019).
func TestRainbowSpringsMessengerTakesRegistrations(t *testing.T) {
	t.Parallel()
	members := springersMembers()
	w := bootRainbow(t, rainbowSetup{date: 5 * time.Hour, heldDecrees: 7, members: members})
	messenger := w.npcOf(t, rainbowMessenger)

	got := page(t, "messenger", w.talk(t, w.c, messenger))
	if !strings.Contains(got, "Messenger Yetti:") || !strings.Contains(got, registrationEnd(w.date)+" </font>") {
		t.Fatalf("messenger page = %q, want messenger_yetti001a.htm naming the deadline %s", got, registrationEnd(w.date))
	}

	mc := w.member(t, members[0])
	w.talk(t, mc, messenger)
	wantRainbowPage(t, "member's registration", w.link(t, mc, "register"), "messenger_yetti010.htm")

	// The messenger's main page links the registration and the
	// cancellation's page, which links the cancellation.
	register := func(what string) [][]byte {
		t.Helper()
		w.talk(t, w.c, messenger)
		return w.link(t, w.c, "register")
	}
	cancel := func(what string) [][]byte {
		t.Helper()
		w.talk(t, w.c, messenger)
		wantRainbowPage(t, what+" page", w.link(t, w.c, "messenger_yetti015.htm"), "messenger_yetti015.htm")
		return w.link(t, w.c, "cancel")
	}
	wantRainbowPage(t, "cancel unregistered", cancel("cancel unregistered"), "messenger_yetti016.htm")
	wantRainbowPage(t, "registration", register("registration"), "messenger_yetti009.htm")
	if got := w.storedDecrees(t); !sameDecrees(got, map[int32]int{springersClanID: 7}) {
		t.Fatalf("stored decrees after the registration = %v, want Springers 7", got)
	}
	if n := w.heldDecrees(t); n != 0 {
		t.Fatalf("decrees held after the registration = %d, want none", n)
	}
	wantRainbowPage(t, "second registration", register("second registration"), "messenger_yetti013.htm")

	wantRainbowPage(t, "cancel", cancel("cancel"), "messenger_yetti019.htm")
	if got := w.storedDecrees(t); len(got) != 0 {
		t.Fatalf("stored decrees after the cancellation = %v, want none", got)
	}
	if n := w.heldDecrees(t); n != 3 {
		t.Fatalf("decrees held after the cancellation = %d, want 3", n)
	}
}

// sameDecrees reports whether a and b hold the same decrees.
func sameDecrees(a, b map[int32]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// TestRainbowSpringsSiegeRunsTheGame runs a Rainbow Springs siege of the
// free hall through the real hall script. Springers (9 decrees), Bathers
// (5), Leavers (4, dissolving), Soakers (3), Waders (2) and Fifth (1) are
// registered. The registrations close an hour before the date. At the date
// Springers, Bathers, Soakers and Waders are chosen, in that order, and
// attack; Leavers is dropped and Fifth's leader, in the world, is told its
// registration failed; Springers' members are told the registrations
// ended and they are registered; Leavers' stored row stays until the
// clear. The caretaker opens; it refuses the Attacker leading no party
// (game_manager005) and a member who does not lead (004), and lets the
// Attacker's party of five Springers into the first arena, answering with
// the link's own name: the member who held a buff and a servitor loses the
// buff, is shown its emptied effects, and its servitor leaves the world. The countdown runs to
// the start an hour later: Springers is told the game began, and the
// caretaker tells the players around it; the arena's gourd stands. The
// Attacker breaking its arena's gourd ends the game as a draw: the gourd
// leaves, the hall's maker places its four cleaners, every registration is
// dropped, the registrations open for the next date and the hall stays
// free. Two minutes later the players on the battlefield go to town.
func TestRainbowSpringsSiegeRunsTheGame(t *testing.T) {
	t.Parallel()
	members := springersMembers()
	fifthLeader := rainbowMember{castleRival: castleRival{0x7f001011, "fifth", "FifthLeader", fifthClanID}, leads: true}
	w := bootRainbow(t, rainbowSetup{
		date: 2 * time.Hour,
		decrees: map[int32]int{
			springersClanID: 9, bathersClanID: 5, leaversClanID: 4, soakersClanID: 3, waders4ClanID: 2, fifthClanID: 1,
		},
		dissolving: []int32{leaversClanID},
		members:    append(slices.Clone(members), fifthLeader),
	})
	var clients []*testsupport.ScriptedClient
	for _, m := range members {
		clients = append(clients, w.member(t, m))
	}
	fc := w.member(t, fifthLeader)
	caretaker := w.npcOf(t, rainbowCaretaker)
	wantRainbowPage(t, "caretaker before the choice", w.talk(t, w.c, caretaker), "game_manager003.htm")

	w.srv.Advance(t, time.Hour)
	if st := w.siege.Status(); st != siege.StatusRegistrationOver {
		t.Fatalf("status after the registrations closed = %v, want registration over", st)
	}
	drainFrames(t, w.c)
	drainFrames(t, fc)

	w.srv.Advance(t, time.Hour)
	w.srv.Advance(t, 0)
	if got := w.siege.Attackers(); !slices.Equal(got, []int32{springersClanID, bathersClanID, soakersClanID, waders4ClanID}) {
		t.Fatalf("attackers after the choice = %#x, want Springers, Bathers, Soakers, Waders", got)
	}
	if got := sysMsgIDs(t, drainFrames(t, fc)); !slices.Contains(got, "1825") {
		t.Fatalf("Fifth's leader got %v, want CLANHALL_WAR_REGISTRATION_FAILED", got)
	}
	if got := sysMsgIDs(t, drainFrames(t, w.c)); !slices.Equal(got, []string{"1823", "1824"}) {
		t.Fatalf("the Attacker got %v, want CLANHALL_WAR_REGISTRATION_PERIOD_ENDED, REGISTERED_FOR_CLANHALL_WAR", got)
	}
	if got := w.storedDecrees(t); got[springersClanID] != 9 || got[fifthClanID] != 1 || got[leaversClanID] != 4 {
		t.Fatalf("stored decrees after the choice = %v, want every row kept, Leavers' too, until the clear", got)
	}

	wantRainbowPage(t, "caretaker after the choice", w.talk(t, w.c, caretaker), "game_manager001.htm")
	wantRainbowPage(t, "entry with no party", w.link(t, w.c, "portToArena"), "game_manager005.htm")
	w.talk(t, clients[0], caretaker)
	wantRainbowPage(t, "member's entry", w.link(t, clients[0], "portToArena"), "game_manager004.htm")
	for i, m := range members {
		w.invite(t, clients[i], m.name)
	}
	buffed := w.buffAndSummon(t, members[0].id)
	drainFrames(t, clients[0])
	drainFrames(t, w.c)
	frames := w.link(t, w.c, "portToArena")
	if got := sysMsgParams(t, frames); !slices.ContainsFunc(got, func(p []string) bool { return slices.Contains(p, "portToArena") }) {
		t.Fatalf("arena entry answered %v, want the link's name as a message", got)
	}
	w.srv.Settle(t)
	for _, c := range append([]*testsupport.ScriptedClient{w.c}, clients...) {
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	}
	w.srv.Settle(t)
	ids := []int32{w.objID}
	for _, m := range members {
		ids = append(ids, m.id)
	}
	for _, id := range ids {
		if x, y, _ := w.srv.PlayerPosition(t, id); abs(x-rainbowArena1.X) > 50 || abs(y-rainbowArena1.Y) > 50 {
			t.Fatalf("party member %#x at %d,%d after the entry, want the first arena %v", id, x, y, rainbowArena1)
		}
	}
	var effects int
	onPlayerQueue(t, w.srv, members[0].id, func() { effects = len(buffed.EffectList().All()) })
	if effects != 0 {
		t.Fatalf("the buffed member holds %d effects after the entry, want none", effects)
	}
	if _, ok := firstOpcode(drainFrames(t, clients[0]), serverpackets.OpcodeAbnormalStatusUpdate); !ok {
		t.Fatal("the buffed member was not shown its emptied effects")
	}
	if _, ok := w.srv.State.Summon(members[0].id); ok {
		t.Fatal("the member's servitor is still in the world after the entry")
	}
	for _, c := range clients {
		drainFrames(t, c)
	}
	drainFrames(t, w.c)

	w.srv.Advance(t, 10*time.Minute)
	if got := sysMsgIDs(t, drainFrames(t, w.c)); !slices.Equal(got, []string{"1826 50"}) {
		t.Fatalf("ten minutes into the wait the Attacker got %v, want the game in 50 minutes", got)
	}
	w.srv.Advance(t, 50*time.Minute)
	w.srv.Advance(t, 0)
	want := []string{"1826 40", "1826 30", "1826 20", "1826 10", "1826 5", "1826 3", "1826 2", "1826 1"}
	for secs := 10; secs >= 1; secs-- {
		want = append(want, "1828 "+strconv.Itoa(secs))
	}
	want = append(want, "844 62", "844 62")
	if got := sysMsgIDs(t, drainFrames(t, w.c)); !slices.Equal(got, want) {
		t.Fatalf("to the start the Attacker got %v, want %v", got, want)
	}
	if !w.siege.InProgress() {
		t.Fatal("the game did not start")
	}
	gourd := w.npcOf(t, rainbowGourd)
	if got := len(w.npcsOf(rainbowCleaner)); got != 0 {
		t.Fatalf("cleaners during the game = %d, want none", got)
	}

	w.srv.RunScript(t, w.objID, "RainbowSpringsChateau", func(s *script.Script, p *script.Player) {
		s.SiegableHall(rainbowHall).SetMiniGameWinner(script.NPCOf(gourd), p)
	})
	w.srv.Advance(t, 0)
	if got := sysMsgIDs(t, drainFrames(t, w.c)); !slices.Contains(got, "856 62") {
		t.Fatalf("the game's end sent the Attacker %v, want SIEGE_S1_DRAW", got)
	}
	if w.siege.Status() != siege.StatusRegistrationOpened {
		t.Fatalf("status after the game = %v, want registration opened", w.siege.Status())
	}
	if got := len(w.npcsOf(rainbowGourd)); got != 0 {
		t.Fatalf("gourds after the game = %d, want none", got)
	}
	if got := len(w.npcsOf(rainbowCleaner)); got != 4 {
		t.Fatalf("cleaners after the game = %d, want 4", got)
	}
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers after the game = %#x, want none", got)
	}
	if got := w.storedDecrees(t); len(got) != 0 {
		t.Fatalf("stored decrees after the game = %v, want none", got)
	}
	if owner := w.hallOwner(t); owner != 0 {
		t.Fatalf("hall owner after the game = %#x, want none: the game is a draw", owner)
	}
	if next := time.UnixMilli(w.siege.Date()); !next.After(w.date) {
		t.Fatalf("next siege date %v, want after %v", next, w.date)
	}

	w.srv.Advance(t, 2*time.Minute-time.Millisecond)
	if x, y, _ := w.srv.PlayerPosition(t, w.objID); abs(x-rainbowArena1.X) > 50 || abs(y-rainbowArena1.Y) > 50 {
		t.Fatalf("the Attacker left the arena before two minutes: at %d,%d", x, y)
	}
	w.srv.Advance(t, time.Millisecond)
	w.srv.Settle(t)
	if x, y, _ := w.srv.PlayerPosition(t, w.objID); abs(x-rainbowGoddard.X) > 50 || abs(y-rainbowGoddard.Y) > 50 {
		t.Fatalf("the Attacker stands at %d,%d two minutes after the game, want in Goddard %v", x, y, rainbowGoddard)
	}
}

// TestRainbowSpringsSiegeCalledOffWithOneClan runs a Rainbow Springs siege
// that Bathers held, with Springers alone registered. The registrations'
// close an hour before the date frees the hall, as every hall siege's does,
// so at the date no owner is chosen: Springers alone is too few. Its
// members are told the registrations ended, the siege was called off and
// it is a draw; every registration is dropped, no clan attacks, the
// registrations open for the next date and the hall stays free.
func TestRainbowSpringsSiegeCalledOffWithOneClan(t *testing.T) {
	t.Parallel()
	w := bootRainbow(t, rainbowSetup{owner: bathersClanID, date: 2 * time.Hour, decrees: map[int32]int{springersClanID: 4}})
	w.srv.Advance(t, time.Hour)
	if owner := w.hallOwner(t); owner != 0 {
		t.Fatalf("hall owner after the registrations closed = %#x, want none", owner)
	}
	drainFrames(t, w.c)
	w.srv.Advance(t, time.Hour)
	if got := sysMsgIDs(t, drainFrames(t, w.c)); !slices.Equal(got, []string{"1823", "1841", "856 62"}) {
		t.Fatalf("the Attacker got %v, want REGISTRATION_PERIOD_ENDED, CLANHALL_WAR_CANCELLED, SIEGE_S1_DRAW", got)
	}
	if got := w.storedDecrees(t); len(got) != 0 {
		t.Fatalf("stored decrees after the call-off = %v, want none", got)
	}
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers after the call-off = %#x, want none", got)
	}
	if w.siege.Status() != siege.StatusRegistrationOpened {
		t.Fatalf("status after the call-off = %v, want registration opened", w.siege.Status())
	}
	if next := time.UnixMilli(w.siege.Date()); !next.After(w.date) {
		t.Fatalf("next siege date %v, want after %v", next, w.date)
	}
	if owner := w.hallOwner(t); owner != 0 {
		t.Fatalf("hall owner after the call-off = %#x, want none", owner)
	}
}

// TestRainbowSpringsGameRunsOutOfTime runs a Rainbow Springs game between
// Springers and Bathers that no one wins. A player of Springers, the first
// clan chosen, breaking the second arena's gourd ends nothing. The game
// ends as a draw two minutes before the siege's length runs out, 58
// minutes after its start and not a millisecond sooner: Springers is told,
// the gourds leave, the hall's maker places its four cleaners, the
// registrations open for the next date and every registration is dropped.
func TestRainbowSpringsGameRunsOutOfTime(t *testing.T) {
	t.Parallel()
	w := bootRainbow(t, rainbowSetup{date: 2 * time.Hour, decrees: map[int32]int{springersClanID: 9, bathersClanID: 5}})
	date := w.siege.Date()
	w.advanceTo(t, date)
	w.srv.Advance(t, 0)
	start := date + time.Hour.Milliseconds()
	w.advanceTo(t, start)
	w.srv.Advance(t, 0)
	if !w.siege.InProgress() {
		t.Fatal("the game did not start")
	}
	drainFrames(t, w.c)

	second := w.npcOf(t, rainbowGourd2)
	w.srv.RunScript(t, w.objID, "RainbowSpringsChateau", func(s *script.Script, p *script.Player) {
		s.SiegableHall(rainbowHall).SetMiniGameWinner(script.NPCOf(second), p)
	})
	w.srv.Advance(t, 0)
	if !w.siege.InProgress() {
		t.Fatal("breaking another clan's gourd ended the game")
	}
	if got := sysMsgIDs(t, drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("breaking another clan's gourd sent %v, want nothing", got)
	}

	end := start + (58 * time.Minute).Milliseconds()
	w.advanceTo(t, end-1)
	if !w.siege.InProgress() || len(w.npcsOf(rainbowCleaner)) != 0 {
		t.Fatal("the game ended before its time ran out")
	}
	w.advanceTo(t, end)
	if got := sysMsgIDs(t, drainFrames(t, w.c)); !slices.Equal(got, []string{"856 62"}) {
		t.Fatalf("the time-out sent the Attacker %v, want SIEGE_S1_DRAW", got)
	}
	if w.siege.Status() != siege.StatusRegistrationOpened {
		t.Fatalf("status after the time-out = %v, want registration opened", w.siege.Status())
	}
	if got := len(w.npcsOf(rainbowGourd)) + len(w.npcsOf(rainbowGourd2)); got != 0 {
		t.Fatalf("gourds after the time-out = %d, want none", got)
	}
	if got := len(w.npcsOf(rainbowCleaner)); got != 4 {
		t.Fatalf("cleaners after the time-out = %d, want 4", got)
	}
	if got := w.storedDecrees(t); len(got) != 0 {
		t.Fatalf("stored decrees after the time-out = %v, want none", got)
	}
	if next := w.siege.Date(); next <= date {
		t.Fatalf("next siege date %d, want after %d", next, date)
	}
}
