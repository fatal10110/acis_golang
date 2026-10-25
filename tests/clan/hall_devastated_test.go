package clan

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/devastatedcastle"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Reference: DevastatedCastle (spawnNpcs, unspawnNpcs, onAttacked,
// onMyDying, getWinner) on ClanHallSiege. The Devastated Castle (34) is
// defended by an NPC clan whose makers start with the siege
// (agit_defend_warfare_start(34)); Gustav (35410) raises his lieutenants
// Dietrich (35408) and Mikhail (35409), 15 doom knights (35416) and 15
// doom archers (35413), and the attacking clan that dealt him the most
// damage takes the castle once he falls.

const (
	devastatedScript     = "script.siegablehall.DevastatedCastle"
	devastatedScriptName = "DevastatedCastle"
	castleGustavID       = 35410
)

// devastatedSpawnlist is a defenders' maker of the hall's siege holding
// Gustav, beside the Attacker on the battlefield.
const devastatedSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="devastated_field" minZ="0" maxZ="100"><node x="-500" y="-500"/><node x="500" y="-500"/><node x="500" y="500"/><node x="-500" y="500"/></territory>
	<npcmaker name="aden05_agit2517_05" territory="devastated_field" spawnTime="agit_defend_warfare_start(34)" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="35410" pos="150;20;30;0" total="1" respawn="2hour"/>
	</npcmaker>
</list>`

// devastatedTemplates are the castle's defenders, monsters of 1000 HP.
func devastatedTemplates() (*npc.Table, map[int32]script.NPCKind) {
	var out []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for id := 35408; id <= 35416; id++ {
		out = append(out, &npc.Template{
			ID: id, TemplateID: id, Type: "Monster", Name: "Defender " + strconv.Itoa(id), Level: 1, HPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: false, CollisionRadius: 10,
		})
		kinds[int32(id)] = script.KindHostile
	}
	return npc.NewTable(out), kinds
}

// devastatedWorld is the Attacker, leading Besiegers, registered on the
// Devastated Castle's siege and in the world on its battlefield.
type devastatedWorld struct {
	srv    *gameservertest.Server
	c      *testsupport.ScriptedClient
	objID  int32
	player attackable.Combatant
	siege  *clanhall.HallSiege
	field  *zone.Siege
}

func bootDevastated(t *testing.T) *devastatedWorld {
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
	if err := os.WriteFile(dir+"/devastated.xml", []byte(devastatedSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	makers, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	field := zone.NewSiege(1, cuboid(t, -2000, 2000, -2000, 2000))
	field.ResidenceID = int(devastatedCastle)
	grounds := zone.NewClanHall(2, cuboid(t, 5000, 6000, 5000, 6000))
	grounds.ResidenceID = int(devastatedCastle)
	zones := zone.NewIndex()
	zones.Add(field)
	zones.Add(grounds)
	templates, kinds := devastatedTemplates()
	w := &devastatedWorld{field: field}
	w.srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Attacker", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithNPCs(templates),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: devastatedScript}}, script.Catalog{devastatedScript: devastatedcastle.New}),
		gameservertest.WithScriptRand(func(int) int { return 4 }),
		gameservertest.WithLog(zerolog.New(zerolog.NewTestWriter(t)).Level(zerolog.WarnLevel)),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedDevastated(t, db) }),
	)
	if !w.srv.DrivesClock() {
		t.Skip("the hall siege steps run on the driven clock")
	}
	w.c, w.objID = w.srv.Client, w.srv.SoleObjectID(t)
	hs, ok := w.srv.Halls.Siege(devastatedCastle)
	if !ok {
		t.Fatal("the Devastated Castle has no siege")
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

// seedDevastated puts the Attacker in Besiegers, which is registered on
// the free hall's siege, and the clan Holders beside it.
func seedDevastated(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		"UPDATE characters SET clanid = " + strconv.Itoa(int(besiegersClanID)) + " WHERE char_name = 'Attacker'",
		"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT " + strconv.Itoa(int(besiegersClanID)) + ", 'Besiegers', 5, obj_Id FROM characters WHERE char_name = 'Attacker'",
		"INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (" + strconv.Itoa(int(holdersClanID)) + ", 'Holders', 5)",
		"INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (34, 0, " + strconv.Itoa(hallPaidUntil) + ", 1, " + strconv.FormatInt(time.Now().Add(7*24*time.Hour).UnixMilli(), 10) + ")",
		"INSERT INTO clanhall_siege_attackers (clanhall_id, attacker_id) VALUES (34, " + strconv.Itoa(int(besiegersClanID)) + ")",
	} {
		if _, err := db.ExecContext(context.Background(), q); err != nil {
			t.Fatalf("seed: %s: %v", q, err)
		}
	}
}

// start closes the registrations and starts the siege at once.
func (w *devastatedWorld) start(t *testing.T) [][]byte {
	t.Helper()
	if !w.siege.Instant() {
		t.Fatal("no siege to start")
	}
	w.srv.Advance(t, 0)
	w.srv.Settle(t)
	if !w.siege.InProgress() {
		t.Fatal("the siege did not start")
	}
	return drainFrames(t, w.c)
}

// defenders returns, per template id, the living defenders in the world.
func (w *devastatedWorld) defenders() map[int][]*npc.Hostile {
	out := map[int][]*npc.Hostile{}
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && !h.Dead() && h.NpcID() >= 35408 && h.NpcID() <= 35416 {
			out[h.NpcID()] = append(out[h.NpcID()], h)
		}
	}
	return out
}

// gustav returns the one living Gustav in the world.
func (w *devastatedWorld) gustav(t *testing.T) *npc.Hostile {
	t.Helper()
	got := w.defenders()[castleGustavID]
	if len(got) != 1 {
		t.Fatalf("Gustavs in the world = %d, want 1", len(got))
	}
	return got[0]
}

// hit has attacker deal Gustav damage, on the Attacker's queue.
func (w *devastatedWorld) hit(t *testing.T, gustav *npc.Hostile, damage int, attacker attackable.Combatant) {
	t.Helper()
	onPlayerQueue(t, w.srv, w.objID, func() { gustav.TakeDamage(damage, attacker) })
	w.srv.Settle(t)
}

// kill has killer kill gustav, then lets his dying hooks run.
func (w *devastatedWorld) kill(t *testing.T, gustav *npc.Hostile, byAttacker bool) [][]byte {
	t.Helper()
	w.srv.RunScript(t, w.objID, devastatedScriptName, func(_ *script.Script, p *script.Player) {
		n := script.NPCOf(gustav)
		if byAttacker {
			n.DoDie(p)
		} else {
			n.DoDie(n)
		}
	})
	w.srv.Advance(t, 3*time.Second)
	w.srv.Settle(t)
	return drainFrames(t, w.c)
}

// hallOwner reads the hall row's owner.
func (w *devastatedWorld) hallOwner(t *testing.T) int64 {
	t.Helper()
	w.srv.FlushPersistence(t)
	var owner int64
	if err := w.srv.DB.QueryRowContext(context.Background(), "SELECT ownerId FROM clanhall WHERE id = 34").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	return owner
}

// TestDevastatedCastleSiegeWonByGustavsBane runs a siege of the Devastated
// Castle through the real hall script. Before the siege its defenders'
// maker holds. The start runs the maker (agit_defend_warfare_start(34)):
// Gustav stands, raising Mikhail, Dietrich, 15 knights and 15 archers.
// Besiegers' hits on Gustav count toward it; Gustav's death ends the siege
// at once, won: Besiegers owns the hall (S1_CLAN_DEFEATED_S2 naming it and
// the hall, then SIEGE_OF_S1_FINISHED), its row names it, the attackers
// are dropped, the registrations open again and the maker's NPCs leave the
// world.
func TestDevastatedCastleSiegeWonByGustavsBane(t *testing.T) {
	t.Parallel()
	w := bootDevastated(t)
	if got := w.defenders(); len(got) != 0 {
		t.Fatalf("defenders before the siege = %d kinds, want none", len(got))
	}

	w.start(t)
	gustav := w.gustav(t)
	want := map[int]int{35408: 1, 35409: 1, castleGustavID: 1, 35413: 15, 35416: 15}
	for id, n := range want {
		if got := len(w.defenders()[id]); got != n {
			t.Errorf("defenders of %d after the start = %d, want %d", id, got, n)
		}
	}

	w.hit(t, gustav, 50, w.player)
	frames := w.kill(t, gustav, true)
	// LEFT_COMBAT_ZONE: the battlefield turns off under the Attacker.
	if got := sysMsgParams(t, frames); !slices.EqualFunc(got, [][]string{{"855", "Besiegers", "34"}, {"843", "34"}, {"284"}}, slices.Equal) {
		t.Fatalf("end notices = %v, want S1_CLAN_DEFEATED_S2 for Besiegers, SIEGE_OF_S1_FINISHED", got)
	}
	if w.siege.Status() != siege.StatusRegistrationOpened || w.field.Active() || len(w.siege.Attackers()) != 0 {
		t.Fatalf("after Gustav's death: status %v, battlefield %v, attackers %#x", w.siege.Status(), w.field.Active(), w.siege.Attackers())
	}
	if id, ok := w.srv.Halls.ByOwner(besiegersClanID); !ok || id != devastatedCastle {
		t.Fatal("Besiegers does not own the Devastated Castle")
	}
	if owner := w.hallOwner(t); owner != int64(besiegersClanID) {
		t.Fatalf("hall row owner = %#x, want Besiegers", owner)
	}
	if got := w.defenders()[castleGustavID]; len(got) != 0 {
		t.Fatalf("Gustavs left after the siege = %d", len(got))
	}
}

// TestDevastatedCastleSiegeTimeOutForgetsTheDamage ends a siege on its
// time with Besiegers' damage on Gustav: a draw (SIEGE_S1_DRAW), the hall
// stays free and the maker's NPCs leave the world. The end read the
// damage and forgot it: in the next siege a Gustav dying of no clan's
// hands decides nothing, a draw again.
func TestDevastatedCastleSiegeTimeOutForgetsTheDamage(t *testing.T) {
	t.Parallel()
	w := bootDevastated(t)
	w.start(t)
	w.hit(t, w.gustav(t), 50, w.player)
	if !w.siege.End() {
		t.Fatal("no siege to end")
	}
	w.srv.Settle(t)
	frames := drainFrames(t, w.c)
	if got := sysMsgParams(t, frames); !slices.EqualFunc(got, [][]string{{"856", "34"}, {"843", "34"}, {"284"}}, slices.Equal) {
		t.Fatalf("time-out notices = %v, want SIEGE_S1_DRAW, SIEGE_OF_S1_FINISHED", got)
	}
	if owner := w.hallOwner(t); owner != 0 {
		t.Fatalf("hall row owner after a draw = %#x, want none", owner)
	}
	if got := w.defenders(); len(got[castleGustavID]) != 0 {
		t.Fatalf("Gustavs left after the time-out = %d", len(got[castleGustavID]))
	}

	if !w.siege.AddAttacker(besiegersClanID) {
		t.Fatal("Besiegers not added to the next siege")
	}
	w.start(t)
	frames = w.kill(t, w.gustav(t), false)
	if got := sysMsgParams(t, frames); !slices.EqualFunc(got, [][]string{{"856", "34"}, {"843", "34"}, {"284"}}, slices.Equal) {
		t.Fatalf("notices of a death no clan dealt = %v, want SIEGE_S1_DRAW, SIEGE_OF_S1_FINISHED", got)
	}
	if _, ok := w.srv.Halls.ByOwner(besiegersClanID); ok {
		t.Fatal("Besiegers owns the hall on the last siege's damage")
	}
}
