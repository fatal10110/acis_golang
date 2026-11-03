package clan

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/fortressofresistance"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/fortressofthedead"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Reference: FortressOfResistance and FortressOfTheDead (spawnNpcs,
// unspawnNpcs, onAttacked, onMyDying, getWinner, onFirstTalk, startSiege,
// loadAttackers) on ClanHallSiege.
//
// The Fortress of Resistance (21) keeps no attacker list. Its siege raises
// Bloody Lord Nurka through the defence makers
// (agit_defend_warfare_start(21)) for a hall nobody owned, through the
// attack makers (agit_attack_warfare_start(21)) for one a clan held, and
// the clan that dealt him the most damage, registered or not, takes the
// hall once he falls. Brakel (35382) names the next siege's date.
//
// The Fortress of the Dead (64) is only fought at night: a start that comes
// by day waits for the next in-game midnight. Its siege raises Lidia von
// Hellmann (agit_defend_warfare_start(64)), and the attacking clan that
// dealt her the most damage takes the hall once she falls.

const (
	resistanceHall int32 = 21
	deadHall       int32 = 64
	nurka1ID             = 35368
	nurka2ID             = 35375
	brakelID             = 35382
	lidiaID              = 35629
)

// resistanceSpawnlist holds Nurka's two makers, each over the battlefield,
// and Brakel at his post.
const resistanceSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="partisan_field" minZ="-200" maxZ="200"><node x="-500" y="-500"/><node x="500" y="-500"/><node x="500" y="500"/><node x="-500" y="500"/></territory>
	<npcmaker name="partisan_agit_2121_01" territory="partisan_field" spawnTime="agit_defend_warfare_start(21)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="35368" pos="150;20;30;0" total="1" respawn="3hour"/>
	</npcmaker>
	<npcmaker name="partisan_agit_2121_02" territory="partisan_field" spawnTime="agit_attack_warfare_start(21)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="35375" pos="150;20;30;0" total="1" respawn="3hour"/>
	</npcmaker>
	<npcmaker name="partisan_agit_2121_03" territory="partisan_field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="35382" pos="30;0;0;0" total="1" respawn="60"/>
	</npcmaker>
</list>`

// deadSpawnlist holds Lidia's maker, over the battlefield.
const deadSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="restless_field" minZ="-200" maxZ="200"><node x="-500" y="-500"/><node x="500" y="-500"/><node x="500" y="500"/><node x="-500" y="500"/></territory>
	<npcmaker name="rune11_2117_13" territory="restless_field" spawnTime="agit_defend_warfare_start(64)" maximumNpcs="32">
		<ai type="default_maker"/>
		<npc id="35629" pos="150;20;30;32768" total="1" respawn="2hour"/>
	</npcmaker>
</list>`

// fortressSetup is one fortress siege's boot.
type fortressSetup struct {
	hall      int32
	path      string
	newScript func() script.Script
	spawnlist string
	// ids are the hall's monsters; Brakel is a folk.
	ids []int
	// owner holds the hall before the siege, 0 for none.
	owner int32
	// attackers are the hall's stored attackers.
	attackers []int32
	rivals    []castleRival
	extra     []gameservertest.Option
}

// fortressWorld is the Attacker, leading Besiegers, in the world on a
// fortress's battlefield.
type fortressWorld struct {
	srv    *gameservertest.Server
	c      *testsupport.ScriptedClient
	objID  int32
	player attackable.Combatant
	siege  *clanhall.HallSiege
	field  *zone.Siege
	setup  fortressSetup
}

func bootFortress(t *testing.T, s fortressSetup) *fortressWorld {
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
	if err := os.WriteFile(dir+"/fortress.xml", []byte(s.spawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	makers, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	field := zone.NewSiege(1, cuboid(t, -2000, 2000, -2000, 2000))
	field.ResidenceID = int(s.hall)
	grounds := zone.NewClanHall(2, cuboid(t, 5000, 6000, 5000, 6000))
	grounds.ResidenceID = int(s.hall)
	zones := zone.NewIndex()
	zones.Add(field)
	zones.Add(grounds)
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for _, id := range s.ids {
		tmpl := &npc.Template{
			ID: id, TemplateID: id, Type: "Monster", Name: "Defender " + strconv.Itoa(id), Level: 1, HPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: false, CollisionRadius: 10,
		}
		kinds[int32(id)] = script.KindHostile
		if id == brakelID {
			tmpl.Type = "Folk"
			kinds[int32(id)] = script.KindFolk
		}
		templates = append(templates, tmpl)
	}
	w := &fortressWorld{field: field, setup: s}
	w.srv = gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Attacker", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithHTMLPages(resistancePages(t)),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: s.path}}, script.Catalog{s.path: s.newScript}),
		gameservertest.WithScriptRand(func(int) int { return 4 }),
		gameservertest.WithLog(zerolog.New(zerolog.NewTestWriter(t)).Level(zerolog.WarnLevel)),
		seedCastleRivals(t, s.rivals),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedFortress(t, db, s) }),
	}, s.extra...)...)
	if !w.srv.DrivesClock() {
		t.Skip("the hall siege steps run on the driven clock")
	}
	w.c, w.objID = w.srv.Client, w.srv.SoleObjectID(t)
	hs, ok := w.srv.Halls.Siege(s.hall)
	if !ok {
		t.Fatalf("hall %d has no siege", s.hall)
	}
	w.siege = hs
	startInWorld(t, w.c)
	drainFrames(t, w.c)
	obj, ok := w.srv.State.Object(w.objID)
	if !ok {
		t.Fatal("the Attacker is not in the world")
	}
	w.player = obj.(attackable.Combatant)
	return w
}

// resistancePages are the Fortress of Resistance's shipped pages.
func resistancePages(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "siegablehall", "FortressOfResistance", "partisan_ordery_brakel001.htm"))
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"script/siegablehall/FortressOfResistance/partisan_ordery_brakel001.htm": string(data)}
}

// seedFortress puts the Attacker in Besiegers, seeds Holders and Outsiders,
// the hall's row with its owner and next siege a week away, and its stored
// attackers.
func seedFortress(t *testing.T, db *sql.DB, s fortressSetup) {
	t.Helper()
	hall := strconv.Itoa(int(s.hall))
	stmts := []string{
		"UPDATE characters SET clanid = " + strconv.Itoa(int(besiegersClanID)) + " WHERE char_name = 'Attacker'",
		"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT " + strconv.Itoa(int(besiegersClanID)) + ", 'Besiegers', 5, obj_Id FROM characters WHERE char_name = 'Attacker'",
		"INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (" + strconv.Itoa(int(holdersClanID)) + ", 'Holders', 5)",
		"INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (" + strconv.Itoa(int(outsidersClanID)) + ", 'Outsiders', 5)",
		"INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (" + hall + ", " + strconv.Itoa(int(s.owner)) + ", " + strconv.Itoa(hallPaidUntil) + ", 1, " + strconv.FormatInt(time.Now().Add(7*24*time.Hour).UnixMilli(), 10) + ")",
	}
	for _, a := range s.attackers {
		stmts = append(stmts, "INSERT INTO clanhall_siege_attackers (clanhall_id, attacker_id) VALUES ("+hall+", "+strconv.Itoa(int(a))+")")
	}
	for _, r := range s.rivals {
		stmts = append(stmts, "UPDATE characters SET clanid = "+strconv.Itoa(int(r.clan))+" WHERE obj_Id = "+strconv.Itoa(int(r.id)))
	}
	for _, q := range stmts {
		if _, err := db.ExecContext(context.Background(), q); err != nil {
			t.Fatalf("seed: %s: %v", q, err)
		}
	}
}

// rival brings rival r into the world, on the battlefield.
func (w *fortressWorld) rival(t *testing.T, r castleRival) attackable.Combatant {
	t.Helper()
	c := w.srv.DialClient(t, r.account, 1)
	startInWorld(t, c)
	drainFrames(t, c)
	drainFrames(t, w.c)
	obj, ok := w.srv.State.Object(r.id)
	if !ok {
		t.Fatalf("%s is not in the world", r.name)
	}
	return obj.(attackable.Combatant)
}

// instant closes the registrations and lets the siege's start come.
func (w *fortressWorld) instant(t *testing.T) [][]byte {
	t.Helper()
	if !w.siege.Instant() {
		t.Fatal("no siege to start")
	}
	w.srv.Advance(t, 0)
	w.srv.Settle(t)
	return drainFrames(t, w.c)
}

// standing returns, per template id, the living hall NPCs in the world.
func (w *fortressWorld) standing() map[int][]*npc.Hostile {
	out := map[int][]*npc.Hostile{}
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && !h.Dead() && slices.Contains(w.setup.ids, h.NpcID()) {
			out[h.NpcID()] = append(out[h.NpcID()], h)
		}
	}
	return out
}

// one returns the one living NPC of id in the world.
func (w *fortressWorld) one(t *testing.T, id int) *npc.Hostile {
	t.Helper()
	got := w.standing()[id]
	if len(got) != 1 {
		t.Fatalf("NPCs of %d in the world = %d, want 1", id, len(got))
	}
	return got[0]
}

// hit has attacker deal h damage, on the Attacker's queue.
func (w *fortressWorld) hit(t *testing.T, h *npc.Hostile, damage int, attacker attackable.Combatant) {
	t.Helper()
	onPlayerQueue(t, w.srv, w.objID, func() { h.TakeDamage(damage, attacker) })
	w.srv.Settle(t)
}

// kill has the Attacker kill h, then lets its dying hooks run.
func (w *fortressWorld) kill(t *testing.T, h *npc.Hostile) [][]byte {
	t.Helper()
	name := w.setup.path[strings.LastIndex(w.setup.path, ".")+1:]
	w.srv.RunScript(t, w.objID, name, func(_ *script.Script, p *script.Player) { script.NPCOf(h).DoDie(p) })
	w.srv.Advance(t, 3*time.Second)
	w.srv.Settle(t)
	return drainFrames(t, w.c)
}

// hallOwner reads the hall row's owner.
func (w *fortressWorld) hallOwner(t *testing.T) int64 {
	t.Helper()
	w.srv.FlushPersistence(t)
	var owner int64
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT ownerId FROM clanhall WHERE id = ?", w.setup.hall).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	return owner
}

// endNotices drops the LEFT_COMBAT_ZONE notices (284) the battlefield's
// turning off sends the players still on it.
func endNotices(t *testing.T, frames [][]byte) [][]string {
	t.Helper()
	return slices.DeleteFunc(sysMsgParams(t, frames), func(p []string) bool { return p[0] == "284" })
}

// hasCastDesire reports whether h wants to cast skill id at target.
func hasCastDesire(h *npc.Hostile, id int, target attackable.Combatant) bool {
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionCast && int(d.Skill.ID) == id && d.FinalTarget != nil && d.FinalTarget.ObjectID() == target.ObjectID() {
			return true
		}
	}
	return false
}

func resistanceSetup(owner int32, rivals ...castleRival) fortressSetup {
	return fortressSetup{
		hall: resistanceHall, path: "script.siegablehall.FortressOfResistance", newScript: fortressofresistance.New,
		spawnlist: resistanceSpawnlist, ids: []int{nurka1ID, nurka2ID, brakelID}, owner: owner,
		// The fortress loads no stored attackers: this row is never read.
		attackers: []int32{holdersClanID}, rivals: rivals,
		// Nurka's magic strike, which his attacked hook asks.
		extra: []gameservertest.Option{gameservertest.WithNpcSpawnSkills(modelskill.NewTable([]modelskill.Definition{{
			ID: 4042, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne, Offensive: true, Magic: true,
			SkillType: "MDAM", CastRange: 600, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1,
		}}))},
	}
}

// TestFortressOfResistanceSiegeOfAFreeHall runs a siege of the free
// Fortress of Resistance through the real hall script. Brakel names the
// next siege's date. The hall keeps no attacker list: the stored attacker
// is never read, and the siege starts with no attacker, running the
// defence makers: Nurka's first form stands and sends the Attacker, in his
// territory, to 51952, 111060, -1970. His attacked hook counts the damage
// of every clan's members, registered or not, and wants his magic strike
// (4042) at a hitter on a draw under 10; his death ends the siege, won by
// the clan that dealt him the most: Outsiders owns the hall, its row names
// it, and Nurka leaves the world.
func TestFortressOfResistanceSiegeOfAFreeHall(t *testing.T) {
	t.Parallel()
	stranger := castleRival{0x7f000601, "player2", "Stranger", outsidersClanID}
	w := bootFortress(t, resistanceSetup(0, stranger))
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers after the binding = %#x, want none", got)
	}

	brakel := w.srv.State.Objects()
	var post flagWarNPC
	for _, obj := range brakel {
		if f, ok := obj.(*npc.Folk); ok && f.NpcID() == brakelID {
			post = f
		}
	}
	if post == nil {
		t.Fatal("Brakel is not in the world")
	}
	w.c.Send(encodeAction(post.ObjectID(), location.Location{}))
	w.srv.Settle(t)
	frames := drainFrames(t, w.c)
	if _, targeted := firstOpcode(frames, serverpackets.OpcodeMyTargetSelected); targeted {
		x, y, z := post.Position()
		w.c.Send(encodeAction(post.ObjectID(), location.Location{X: x, Y: y, Z: z}))
		w.srv.Settle(t)
		frames = drainFrames(t, w.c)
	}
	date := time.UnixMilli(w.siege.Date()).Format("2006-01-02 15:04:05")
	if got := page(t, "Brakel", frames); !strings.Contains(got, "around "+date+"...") {
		t.Fatalf("Brakel's page = %q, want the next siege date %s", got, date)
	}

	outsider := w.rival(t, stranger)
	w.instant(t)
	if !w.siege.InProgress() {
		t.Fatal("the siege did not start with no attacker")
	}
	nurka := w.one(t, nurka1ID)
	if got := w.standing()[nurka2ID]; len(got) != 0 {
		t.Fatalf("Nurka's second form stands for a free hall: %d", len(got))
	}
	w.srv.Settle(t)
	if x, y, _ := w.srv.PlayerPosition(t, w.objID); abs(x-51952) > 200 || abs(y-111060) > 200 {
		t.Fatalf("the Attacker stands at %d,%d after Nurka appeared, want within 200 of 51952,111060", x, y)
	}

	w.hit(t, nurka, 50, w.player)
	if !hasCastDesire(nurka, 4042, w.player) {
		t.Fatal("Nurka does not want to strike the Attacker on a draw of 4")
	}
	w.hit(t, nurka, 80, outsider)
	frames = w.kill(t, nurka)
	if got := endNotices(t, frames); !slices.EqualFunc(got, [][]string{{"855", "Outsiders", "21"}, {"843", "21"}}, slices.Equal) {
		t.Fatalf("end notices = %v, want S1_CLAN_DEFEATED_S2 for Outsiders, SIEGE_OF_S1_FINISHED", got)
	}
	if id, ok := w.srv.Halls.ByOwner(outsidersClanID); !ok || id != resistanceHall {
		t.Fatal("Outsiders does not own the Fortress of Resistance")
	}
	if owner := w.hallOwner(t); owner != int64(outsidersClanID) {
		t.Fatalf("hall row owner = %#x, want Outsiders", owner)
	}
	if got := w.standing(); len(got[nurka1ID])+len(got[nurka2ID]) != 0 {
		t.Fatalf("Nurka left after the siege: %v", got)
	}
}

// TestFortressOfResistanceSiegeOfAHeldHall runs a siege of the Fortress of
// Resistance that Holders held: the registrations' close frees the hall,
// and the start runs the attack makers, raising Nurka's second form and
// not his first. The siege running out its time is a draw that leaves the
// hall free and takes Nurka's second form out of the world.
func TestFortressOfResistanceSiegeOfAHeldHall(t *testing.T) {
	t.Parallel()
	w := bootFortress(t, resistanceSetup(holdersClanID))
	w.instant(t)
	if !w.siege.InProgress() {
		t.Fatal("the siege did not start")
	}
	w.one(t, nurka2ID)
	if got := w.standing()[nurka1ID]; len(got) != 0 {
		t.Fatalf("Nurka's first form stands for a held hall: %d", len(got))
	}
	if !w.siege.End() {
		t.Fatal("no siege to end")
	}
	w.srv.Settle(t)
	if got := endNotices(t, drainFrames(t, w.c)); !slices.EqualFunc(got, [][]string{{"856", "21"}, {"843", "21"}}, slices.Equal) {
		t.Fatalf("time-out notices = %v, want SIEGE_S1_DRAW, SIEGE_OF_S1_FINISHED", got)
	}
	if owner := w.hallOwner(t); owner != 0 {
		t.Fatalf("hall row owner after a draw = %#x, want none", owner)
	}
	if got := w.standing(); len(got[nurka1ID])+len(got[nurka2ID]) != 0 {
		t.Fatalf("Nurka left after the time-out: %v", got)
	}
}

// deadSetup boots the Fortress of the Dead with Besiegers and Holders
// registered, the in-game clock at hour 10.
func deadSetup(rivals ...castleRival) fortressSetup {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	// An in-game hour passes in ten real minutes: 100 minutes after
	// midnight is 10:00 in game.
	at := midnight.Add(100 * time.Minute)
	return fortressSetup{
		hall: deadHall, path: "script.siegablehall.FortressOfTheDead", newScript: fortressofthedead.New,
		spawnlist: deadSpawnlist, ids: []int{lidiaID, 35630, 35631, 35647}, attackers: []int32{besiegersClanID, holdersClanID},
		rivals: rivals, extra: []gameservertest.Option{gameservertest.WithGameClockNow(func() time.Time { return at })},
	}
}

// TestFortressOfTheDeadSiegeWaitsForTheNight runs a siege of the Fortress
// of the Dead through the real hall script. Its start comes at in-game
// 10:00 and waits (24 - 10) * 10 minutes: the siege is not under way and
// Lidia does not stand. Night has fallen when the wait is over: the siege
// starts, running the defence makers (agit_defend_warfare_start(64)), and
// Lidia stands with Alfred, Giselle and her 15 vampire leaders. Lidia,
// carried out of her territory, goes back home as she is hit. Besiegers'
// hits count toward the hall, those of Outsiders, registered on no siege,
// do not; Lidia's death ends the siege at once, won by Besiegers.
func TestFortressOfTheDeadSiegeWaitsForTheNight(t *testing.T) {
	t.Parallel()
	stranger := castleRival{0x7f000602, "player2", "Stranger", outsidersClanID}
	w := bootFortress(t, deadSetup(stranger))
	outsider := w.rival(t, stranger)
	w.instant(t)
	if w.siege.InProgress() || w.siege.Status() != siege.StatusRegistrationOver {
		t.Fatalf("the siege by day: in progress %v, status %v; want waiting", w.siege.InProgress(), w.siege.Status())
	}
	if got := w.standing(); len(got) != 0 {
		t.Fatalf("hall NPCs by day: %v", got)
	}

	if !w.srv.CrossDayNight(t) {
		t.Fatal("the day did not turn to night")
	}
	wait := 14 * 600000 * time.Millisecond
	w.srv.Advance(t, wait-time.Millisecond)
	w.srv.Settle(t)
	if w.siege.InProgress() {
		t.Fatal("the siege started before its wait was over")
	}
	w.srv.Advance(t, time.Millisecond)
	w.srv.Settle(t)
	if !w.siege.InProgress() {
		t.Fatal("the siege did not start at night")
	}
	drainFrames(t, w.c)
	lidia := w.one(t, lidiaID)
	for id, n := range map[int]int{35630: 1, 35631: 1, 35647: 15} {
		if got := len(w.standing()[id]); got != n {
			t.Errorf("NPCs of %d after the start = %d, want %d", id, got, n)
		}
	}

	home := script.NPCOf(lidia).SpawnLocation()
	w.srv.RunScript(t, w.objID, "FortressOfTheDead", func(*script.Script, *script.Player) {
		script.NPCOf(lidia).TeleportTo(script.Loc{X: home.X + 3000, Y: home.Y, Z: home.Z})
	})
	w.srv.Settle(t)
	w.hit(t, lidia, 50, w.player)
	if x, y, _ := lidia.Position(); x != home.X || y != home.Y {
		t.Fatalf("Lidia after a hit out of her territory at %d,%d, want home at %d,%d", x, y, home.X, home.Y)
	}
	w.hit(t, lidia, 500, outsider)
	frames := w.kill(t, lidia)
	if got := endNotices(t, frames); !slices.EqualFunc(got, [][]string{{"855", "Besiegers", "64"}, {"843", "64"}}, slices.Equal) {
		t.Fatalf("end notices = %v, want S1_CLAN_DEFEATED_S2 for Besiegers, SIEGE_OF_S1_FINISHED", got)
	}
	if owner := w.hallOwner(t); owner != int64(besiegersClanID) {
		t.Fatalf("hall row owner = %#x, want Besiegers", owner)
	}
	if got := w.standing(); len(got) != 0 {
		t.Fatalf("hall NPCs left after the siege: %v", got)
	}
}
