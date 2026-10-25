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
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q655"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/wildbeastreserve"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Reference: WildBeastReserve (onFirstTalk, onAdvEvent and its
// registration helpers) on the FlagWar base. The Wild Beast Reserve (63)
// is a siegable hall fought as a flag war; its messenger (35627) and its
// teleporters (35613-35617, then 35612) are the farm maker
// rune07_azit2115_11m1's, its flags 35606-35611, its warriors 35618-35622.

const (
	beastFarm         = 63
	beastScript       = "script.siegablehall.WildBeastReserve"
	beastScriptName   = "WildBeastReserve"
	beastMessengerID  = 35627
	beastRoyalFlagID  = 35606
	beastRedFlagID    = 35607
	beastYellowFlagID = 35608
	beastAlly1ID      = 35618
	beastAlly2ID      = 35619
	beastAlly3ID      = 35620
	beastTeleport1ID  = 35612
	q655Name          = "Q655_AGrandPlanForTamingWildBeasts"
	crystalOfPurityID = 8084
	trainerLicenseID  = 8293
)

// beastFlagPoint is the Wild Beast Reserve's flag point i.
var beastFlagPoint = []location.Location{
	{X: 56963, Y: -92211, Z: -1303},
	{X: 58090, Y: -91641, Z: -1303},
	{X: 58908, Y: -92556, Z: -1303},
	{X: 58336, Y: -93600, Z: -1303},
	{X: 57152, Y: -93360, Z: -1303},
	{X: 59116, Y: -93251, Z: -1302},
	{X: 56432, Y: -92864, Z: -1303},
}

// beastFlagWarIDs are every flag and warrior id of the hall.
var beastFlagWarIDs = []int{35606, 35607, 35608, 35609, 35610, 35611, 35618, 35619, 35620, 35621, 35622}

// beastTeleporterIDs are the hall's teleporters' ids.
var beastTeleporterIDs = []int{35612, 35613, 35614, 35615, 35616, 35617}

// beastSpawnlist is the teleporter maker, its messenger and teleporters
// beside the character.
const beastSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="farm_field" minZ="0" maxZ="100"><node x="-500" y="-500"/><node x="500" y="-500"/><node x="500" y="500"/><node x="-500" y="500"/></territory>
	<npcmaker name="rune07_azit2115_11m1" territory="farm_field" maximumNpcs="7">
		<ai type="farm_maker"><set name="CastleID" val="63"/></ai>
		<npc id="35627" pos="100;20;30;0" total="1" respawn="1min"/>
		<npc id="35613" pos="60;20;30;0" total="1" respawn="1min"/>
		<npc id="35614" pos="300;20;30;0" total="1" respawn="1min"/>
		<npc id="35615" pos="310;20;30;0" total="1" respawn="1min"/>
		<npc id="35616" pos="320;20;30;0" total="1" respawn="1min"/>
		<npc id="35617" pos="330;20;30;0" total="1" respawn="1min"/>
		<npc id="35612" pos="340;20;30;0" total="1" respawn="1min"/>
	</npcmaker>
</list>`

// beastTemplates are the hall's NPCs and their kinds: civilian messenger,
// flags and teleporters, monster warriors.
func beastTemplates() (*npc.Table, map[int32]script.NPCKind) {
	var out []*npc.Template
	kinds := map[int32]script.NPCKind{}
	folk := func(id int) {
		tp := gameservertest.FolkTemplate("Folk", id)
		tp.AIParams = npc.AIParams{
			"Pos_x1": strconv.Itoa(teleportLanding.X), "Pos_y1": strconv.Itoa(teleportLanding.Y), "Pos_z1": strconv.Itoa(teleportLanding.Z),
		}
		out = append(out, tp)
		kinds[int32(id)] = script.KindFolk
	}
	folk(beastMessengerID)
	for id := beastRoyalFlagID; id <= 35611; id++ {
		folk(id)
	}
	for _, id := range beastTeleporterIDs {
		folk(id)
	}
	for id := beastAlly1ID; id <= 35622; id++ {
		out = append(out, &npc.Template{
			ID: id, TemplateID: id, Type: "Monster", Name: "Warrior " + strconv.Itoa(id), Level: 1, HPMax: 100,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 10,
		})
		kinds[int32(id)] = script.KindHostile
	}
	return npc.NewTable(out), kinds
}

// bootBeastFarm boots the Attacker, leading Besiegers, beside the Wild
// Beast Reserve's messenger, on its battlefield, with A Grand Plan for
// Taming Wild Beasts started.
func bootBeastFarm(t *testing.T, s flagWarSetup) *flagWarWorld {
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
	if err := os.WriteFile(dir+"/farm.xml", []byte(beastSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	makers, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	field := zone.NewSiege(1, cuboid(t, -2000, 2000, -2000, 2000))
	field.ResidenceID = beastFarm
	grounds := zone.NewClanHall(2, cuboid(t, 5000, 6000, 5000, 6000))
	grounds.ResidenceID = beastFarm
	zones := zone.NewIndex()
	zones.Add(field)
	zones.Add(grounds)
	w := &flagWarWorld{field: field, date: time.Now().Add(time.Hour + time.Minute).UnixMilli()}
	s.seed = append(s.seed,
		"INSERT INTO character_quests (charId, name, var, value) SELECT obj_Id, '"+q655Name+"', '<state>', 'STARTED' FROM characters WHERE char_name = 'Attacker'",
		"INSERT INTO character_quests (charId, name, var, value) SELECT obj_Id, '"+q655Name+"', '<cond>', '1' FROM characters WHERE char_name = 'Attacker'")
	templates, kinds := beastTemplates()
	w.srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Attacker", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithHTMLPages(beastPages(t)),
		gameservertest.WithNPCs(templates),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithMakers(script.MakerCatalog{"farm_maker": maker.Farm}),
		gameservertest.WithNPCScripts(kinds,
			[]script.Listing{{Path: beastScript}, {Path: "quest." + q655Name}},
			script.Catalog{beastScript: wildbeastreserve.New, "quest." + q655Name: q655.New}),
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), questItem(crystalOfPurityID), questItem(trainerLicenseID)))),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithLog(zerolog.New(zerolog.NewTestWriter(t)).Level(zerolog.WarnLevel)),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedBeastFarm(t, db, s, w.date) }),
	)
	if !w.srv.DrivesClock() {
		t.Skip("the hall siege steps run on the driven clock")
	}
	w.c, w.objID = w.srv.Client, w.srv.SoleObjectID(t)
	hs, ok := w.srv.Halls.Siege(beastFarm)
	if !ok {
		t.Fatal("the Wild Beast Reserve has no siege")
	}
	w.siege = hs
	for _, id := range slices.Sorted(maps.Keys(s.items)) {
		w.srv.GiveItem(t, w.objID, id, s.items[id])
	}
	startInWorld(t, w.c)
	drainFrames(t, w.c)
	return w
}

// seedBeastFarm stores the clans of the flag war world, the Wild Beast
// Reserve's row and s's statements.
func seedBeastFarm(t *testing.T, db *sql.DB, s flagWarSetup, date int64) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed beast farm: %s: %v", q, err)
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
	exec("INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (?, ?, ?, 1, ?)", beastFarm, owner, hallPaidUntil, date)
	for _, q := range s.seed {
		exec(q)
	}
}

// beastPages are the Wild Beast Reserve's shipped pages.
func beastPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "script", "siegablehall", beastScriptName)
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
		pages["script/siegablehall/"+beastScriptName+"/"+e.Name()] = string(data)
	}
	return pages
}

// beastBypass sends the messenger link event and returns the frames it
// got back.
func (w *flagWarWorld) beastBypass(t *testing.T, event string) [][]byte {
	t.Helper()
	w.c.Send(encodeBypass("Quest " + beastScriptName + " " + event))
	w.srv.Settle(t)
	return drainFrames(t, w.c)
}

// wantBeastPage fails unless frames show the shipped page file,
// %objectId% filled with objectID, but for the white space around it.
func wantBeastPage(t *testing.T, what string, frames [][]byte, file string, objectID int32) {
	t.Helper()
	want := strings.ReplaceAll(beastPages(t)["script/siegablehall/"+beastScriptName+"/"+file], "%objectId%", strconv.Itoa(int(objectID)))
	if want == "" {
		t.Fatalf("%s: no shipped page %s", what, file)
	}
	if got := page(t, what, frames); strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("%s page = %q, want %s", what, got, file)
	}
}

// beastMessenger returns the hall's messenger.
func (w *flagWarWorld) beastMessenger(t *testing.T) flagWarNPC {
	t.Helper()
	list := w.npcsOf(beastMessengerID)
	if len(list) != 1 {
		t.Fatalf("messengers in the world = %d, want 1", len(list))
	}
	return list[0]
}

// beastOwnerID reads the Wild Beast Reserve's stored owner.
func (w *flagWarWorld) beastOwnerID(t *testing.T) int32 {
	t.Helper()
	got := w.rows(t, "SELECT ownerId FROM clanhall WHERE id = 63")
	if len(got) != 1 {
		t.Fatalf("hall rows = %v", got)
	}
	id, _ := strconv.Atoi(got[0])
	return int32(id)
}

// beastKill has the Attacker kill h, then lets its dying hooks run.
func (w *flagWarWorld) beastKill(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.srv.RunScript(t, w.objID, beastScriptName, func(_ *script.Script, p *script.Player) { script.NPCOf(h).DoDie(p) })
	w.srv.Advance(t, 3*time.Second)
}

// TestWildBeastReserveLicenseRegistration: Holders owns the hall, which
// stored the warrior 35620 as its own. Before the registrations close the
// messenger names the owner and a registration answers with the next siege
// date. After the close the Attacker, holding no Trainer License, is
// refused; holding two with ten Crystals of Purity and A Grand Plan for
// Taming Wild Beasts started, it registers Besiegers: one license pays the
// registration, then the other and every crystal are taken, the quest ends
// with its finish sound and its rows are deleted, and Besiegers fights
// under the yellow flag, the second registration after Holders'. A leader
// cannot register as a member; it picks the first warrior, sees it, and
// sees itself listed.
func TestWildBeastReserveLicenseRegistration(t *testing.T) {
	t.Parallel()
	w := bootBeastFarm(t, flagWarSetup{owned: true, seed: []string{
		"INSERT INTO clanhall_flagwar_owner_npcs VALUES (63, 35620, " + strconv.Itoa(int(holdersClanID)) + ")",
	}})
	messenger := w.beastMessenger(t)

	if html := page(t, "the messenger", w.talk(t, messenger)); !strings.Contains(html, "Holders") || strings.Contains(html, "%my_pledge_name%") || strings.Contains(html, "%my_owner_name%") {
		t.Fatalf("the messenger's page = %q, want it naming Holders", html)
	}
	date := time.UnixMilli(w.date).Format("2006-01-02 15:04:05")
	if html := page(t, "register before the close", w.beastBypass(t, "register")); !strings.Contains(html, date) || strings.Contains(html, "%next_siege%") {
		t.Fatalf("register before the close = %q, want the siege date %s", html, date)
	}

	w.srv.Advance(t, time.Duration(w.date-w.srv.Halls.Now()-3_600_000)*time.Millisecond)
	if !w.siege.WaitingBattle() {
		t.Fatal("the registrations did not close")
	}
	if got, want := w.flagWarClans(t), []string{row(beastFarm, beastRoyalFlagID, 0, int64(holdersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("flag war clans after the close = %v, want %v", got, want)
	}
	drainFrames(t, w.c)

	w.talk(t, messenger)
	wantBeastPage(t, "register after the close", w.beastBypass(t, "register"), "farm_kel_mahum_messenger_1.htm", messenger.ObjectID())
	wantBeastPage(t, "register with no license", w.beastBypass(t, "register_clan"), "farm_kel_mahum_messenger_27.htm", messenger.ObjectID())
	if got := w.flagWarClans(t); len(got) != 1 {
		t.Fatalf("flag war clans after the refusal = %v, want Holders alone", got)
	}

	w.srv.RunScript(t, w.objID, beastScriptName, func(s *script.Script, p *script.Player) {
		s.GiveItems(p, trainerLicenseID, 2)
		s.GiveItems(p, crystalOfPurityID, 10)
	})
	drainFrames(t, w.c)
	w.talk(t, messenger)
	w.beastBypass(t, "register")
	frames := w.beastBypass(t, "register_clan")
	wantBeastPage(t, "the license registration", frames, "farm_kel_mahum_messenger_4b.htm", messenger.ObjectID())
	sound, ok := firstOpcode(frames, serverpackets.OpcodePlaySound)
	if !ok {
		t.Fatalf("the registration played no sound among %x", opcodes(frames))
	}
	r := wire.NewReader(sound[1:])
	r.ReadInt32()
	if file := r.ReadString(); file != script.SoundFinish {
		t.Fatalf("the registration played %q, want %q", file, script.SoundFinish)
	}
	w.srv.RunScript(t, w.objID, beastScriptName, func(_ *script.Script, p *script.Player) {
		for id, want := range map[int32]int32{trainerLicenseID: 0, crystalOfPurityID: 0} {
			if n := p.ItemCount(id); n != want {
				t.Errorf("item %d after the registration = %d, want %d", id, n, want)
			}
		}
	})
	if got := w.rows(t, "SELECT COUNT(*) FROM character_quests WHERE name = '"+q655Name+"'"); !slices.Equal(got, []string{"0"}) {
		t.Fatalf("Q655 rows after the registration = %v, want none", got)
	}
	if got, want := w.flagWarClans(t), []string{row(beastFarm, beastRoyalFlagID, 0, int64(holdersClanID)), row(beastFarm, beastYellowFlagID, 0, int64(besiegersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("flag war clans after Besiegers registered = %v, want %v", got, want)
	}
	if got := w.flagWarMembers(t); !slices.Contains(got, row(beastFarm, int64(besiegersClanID), int64(w.objID))) {
		t.Fatalf("flag war members = %v, want the Attacker for Besiegers", got)
	}
	if got := w.siege.Attackers(); !slices.Equal(got, []int32{holdersClanID, besiegersClanID}) {
		t.Fatalf("attackers = %#x, want Holders then Besiegers", got)
	}

	// Registered, it sees its flag; as the leader it is no member to add.
	w.talk(t, messenger)
	w.beastBypass(t, "register")
	wantBeastPage(t, "a second registration", w.beastBypass(t, "register_clan"), "farm_kel_mahum_messenger_4b.htm", messenger.ObjectID())
	w.talk(t, messenger)
	w.beastBypass(t, "register")
	wantBeastPage(t, "the leader as a member", w.beastBypass(t, "register_clan_member"), "farm_kel_mahum_messenger_5.htm", messenger.ObjectID())

	// It picks the first warrior.
	w.talk(t, messenger)
	w.beastBypass(t, "register")
	wantBeastPage(t, "the warrior choice", w.beastBypass(t, "select_clan_npc"), "farm_kel_mahum_messenger_6.htm", messenger.ObjectID())
	wantBeastPage(t, "the first warrior", w.beastBypass(t, "info_npc_1"), "farm_kel_mahum_messenger_11.htm", messenger.ObjectID())
	wantBeastPage(t, "the warrior pick", w.beastBypass(t, "select_npc_1"), "farm_kel_mahum_messenger_9.htm", messenger.ObjectID())
	if got := w.flagWarClans(t); !slices.Contains(got, row(beastFarm, beastYellowFlagID, beastAlly1ID, int64(besiegersClanID))) {
		t.Fatalf("flag war clans after the pick = %v, want Besiegers' warrior %d", got, beastAlly1ID)
	}
	w.talk(t, messenger)
	w.beastBypass(t, "register")
	wantBeastPage(t, "the warrior shown", w.beastBypass(t, "view_clan_npc"), "farm_kel_mahum_messenger_17.htm", messenger.ObjectID())
	w.talk(t, messenger)
	if html := page(t, "the participants", w.beastBypass(t, "view_participants")); !strings.Contains(html, "Besiegers") || !strings.Contains(html, "**unregistered**") || strings.Contains(html, "Holders") {
		t.Fatalf("participants = %q, want Besiegers listed and the free slots", html)
	}
}

// TestWildBeastReserveFlagWarTakesAFreeHall: the messenger of the free
// hall says it has no owner. Its stored registrations (Besiegers under the
// red flag with the Attacker, Holders under the yellow with the warrior
// 35620) are loaded at boot. At the start the flags stand on the hall's
// points; in the tryouts the warriors stand and the maker's teleporters
// appear. Holders' warrior falls, then the warriors of no clan: Besiegers,
// the last flag, takes the hall, which stores its warrior, none picked, as
// the owner's; the war's NPCs and teleporters go and the flag war tables
// are cleared.
func TestWildBeastReserveFlagWarTakesAFreeHall(t *testing.T) {
	t.Parallel()
	besiegers, holders := strconv.Itoa(int(besiegersClanID)), strconv.Itoa(int(holdersClanID))
	w := bootBeastFarm(t, flagWarSetup{seed: []string{
		"INSERT INTO clanhall_flagwar_attackers VALUES (63, 35607, 0, " + besiegers + ")",
		"INSERT INTO clanhall_flagwar_attackers VALUES (63, 35608, 35620, " + holders + ")",
		"INSERT INTO clanhall_flagwar_members SELECT 63, " + besiegers + ", obj_Id FROM characters WHERE char_name = 'Attacker'",
	}})
	messenger := w.beastMessenger(t)
	wantBeastPage(t, "the free hall's messenger", w.talk(t, messenger), "farm_messenger002.htm", messenger.ObjectID())

	if !w.siege.Instant() {
		t.Fatal("no script runs the Wild Beast Reserve's siege")
	}
	w.srv.Advance(t, 0)
	w.wantStanding(t, "the start", map[int]int{35607: 1, 35608: 1, 35609: 1, 35610: 1, 35611: 1}, beastFlagWarIDs...)
	if red := w.npcsOf(beastRedFlagID)[0]; !near(red, beastFlagPoint[0]) {
		t.Fatal("the red flag does not stand on its point")
	}
	w.srv.Advance(t, 300*time.Second)
	drainFrames(t, w.c)
	if !w.siege.InProgress() || !w.field.Active() {
		t.Fatalf("after the tryouts began: in progress %v, battlefield %v", w.siege.InProgress(), w.field.Active())
	}
	w.wantStanding(t, "the tryouts", map[int]int{35607: 1, 35608: 1, 35609: 1, 35610: 1, 35611: 1, beastAlly2ID: 4, beastAlly3ID: 1}, beastFlagWarIDs...)
	w.wantStanding(t, "the tryouts' teleporters", map[int]int{35613: 1, 35614: 1, 35615: 1, 35616: 1, 35617: 1}, beastTeleporterIDs...)

	w.beastKill(t, w.warriors(beastAlly3ID)[0])
	if !w.siege.InProgress() {
		t.Fatal("the siege ended with warriors of no clan standing")
	}
	for _, h := range w.warriors(beastAlly2ID) {
		if h.Instance.ClanID == besiegersClanID {
			continue
		}
		w.beastKill(t, h)
	}
	if w.siege.InProgress() {
		t.Fatal("the siege goes on with one flag standing")
	}
	if owner := w.beastOwnerID(t); owner != besiegersClanID {
		t.Fatalf("hall owner = %#x, want Besiegers", owner)
	}
	if got, want := w.ownerNPCs(t), []string{row(beastFarm, 0, int64(besiegersClanID))}; !slices.Equal(got, want) {
		t.Fatalf("owner NPC rows = %v, want %v", got, want)
	}
	if c, m := w.flagWarClans(t), w.flagWarMembers(t); len(c) != 0 || len(m) != 0 {
		t.Fatalf("flag war rows after the end: clans %v, members %v", c, m)
	}
	if !w.siege.Registering() || w.field.Active() {
		t.Fatalf("after the end: registering %v, battlefield %v", w.siege.Registering(), w.field.Active())
	}
	w.wantStanding(t, "the end", map[int]int{}, append(beastFlagWarIDs, beastTeleporterIDs...)...)
	if len(w.npcsOf(beastMessengerID)) != 1 {
		t.Fatal("the messenger left with the siege")
	}
}
