package clan

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: ClanHallSiege (prepareSiege, startSiege, endSiege), SiegableHall
// (registerClan, updateNextSiege, updateDb) and ClanHallManager
// (getActiveSiege). Devastated Castle (34) is a siegable hall: siegeLength
// 3600000, scheduleConfig 14;0;0;12;0, gates Devastated_castle_outter_001,
// _002 and Devastated_castle_001 to _004, messenger Loken (35420, SiegeNpc),
// banish point (178497, -13129, -2200).

const (
	devastatedCastle int32 = 34
	// besiegersClanID is the Attacker's clan, level 5; holdersClanID owns
	// Devastated Castle when the setup says so and has no member online.
	besiegersClanID int32 = 0x7f000401
	holdersClanID   int32 = 0x7f000402
	// devastatedGateID stands as Devastated_castle_001.
	devastatedGateID = 25_170_001
	lokenID          = 35420
	// hallPaidUntil is the lease term seeded on the hall's row, which a
	// siegable hall's row writes never touch.
	hallPaidUntil = 777
)

var devastatedBanish = location.Location{X: 178497, Y: -13129, Z: -2200}

// standIn stands in for the hall's siege script: it names winner and counts
// the NPC spawns and unspawns the siege asks for.
type standIn struct {
	winner             atomic.Int32
	spawned, unspawned atomic.Int32
}

func (s *standIn) script() clanhall.SiegeScript {
	return clanhall.SiegeScript{
		Winner:      func() int32 { return s.winner.Load() },
		SpawnNPCs:   func() { s.spawned.Add(1) },
		UnspawnNPCs: func() { s.unspawned.Add(1) },
	}
}

// hallSiegeSetup is the world a hall siege boots with.
type hallSiegeSetup struct {
	// owned has Holders own Devastated Castle.
	owned bool
	// endDate is the hall row's next siege date; 0 puts it a week ahead.
	endDate int64
	// groundsOverSpawn puts the hall's grounds over the spawn point,
	// otherwise they lie far from it; its battlefield covers the spawn
	// either way.
	groundsOverSpawn bool
	// seed are more statements, run after the clans and the hall row.
	seed []string
	// unbound leaves Devastated Castle with no script running its siege.
	unbound bool
	extra   []gameservertest.Option
}

// hallSiegeWorld is the Attacker, leading Besiegers, in the world on
// Devastated Castle's battlefield.
type hallSiegeWorld struct {
	srv    *gameservertest.Server
	c      *testsupport.ScriptedClient
	objID  int32
	at     location.Location
	script *standIn
	siege  *clanhall.HallSiege
	field  *zone.Siege
}

func cuboid(t *testing.T, minX, maxX, minY, maxY int) zone.Form {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, minY, maxY, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	return form
}

func bootHallSiege(t *testing.T, s hallSiegeSetup) *hallSiegeWorld {
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
	field := zone.NewSiege(1, cuboid(t, -2000, 2000, -2000, 2000))
	field.ResidenceID = int(devastatedCastle)
	groundsForm := cuboid(t, 5000, 6000, 5000, 6000)
	if s.groundsOverSpawn {
		groundsForm = cuboid(t, -200, 200, -200, 200)
	}
	grounds := zone.NewClanHall(2, groundsForm)
	grounds.ResidenceID = int(devastatedCastle)
	zones := zone.NewIndex()
	zones.Add(field)
	zones.Add(grounds)
	w := &hallSiegeWorld{script: &standIn{}, field: field}
	opts := []gameservertest.Option{
		gameservertest.WithCharacter("Attacker", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithHTMLPages(siegePages(t)),
		gameservertest.WithDoors(devastatedGate()),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedHallSiege(t, db, s) }),
	}
	if !s.unbound {
		opts = append(opts, gameservertest.WithHallSiegeScript(devastatedCastle, w.script.script()))
	}
	w.srv = gameservertest.Boot(t, append(opts, s.extra...)...)
	if !w.srv.DrivesClock() {
		t.Skip("the hall siege steps run on the driven clock")
	}
	w.c, w.objID = w.srv.Client, w.srv.SoleObjectID(t)
	hs, ok := w.srv.Halls.Siege(devastatedCastle)
	if !ok {
		t.Fatal("Devastated Castle has no siege")
	}
	w.siege = hs
	startInWorld(t, w.c)
	drainFrames(t, w.c)
	x, y, z := w.srv.PlayerPosition(t, w.objID)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w
}

// devastatedGate is an open door named Devastated_castle_001 beside the
// spawn point.
func devastatedGate() *door.Template {
	return &door.Template{
		ID: devastatedGateID, Name: "Devastated_castle_001", Kind: door.KindDoor, Level: 1,
		Position: location.Location{X: 60, Y: 100, Z: gameservertest.SpawnZ},
		Coordinates: []location.Point{
			{X: 52, Y: 92}, {X: 68, Y: 92}, {X: 68, Y: 108}, {X: 52, Y: 108},
		},
		HP: 100, PDef: 10, MDef: 10, Height: 32,
		Opened:   true,
		OpenKind: door.OpenNPC,
	}
}

func seedHallSiege(t *testing.T, db *sql.DB, s hallSiegeSetup) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed hall siege: %s: %v", q, err)
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
	endDate := s.endDate
	if endDate == 0 {
		endDate = time.Now().Add(7 * 24 * time.Hour).UnixMilli()
	}
	exec("INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (?, ?, ?, 1, ?)", devastatedCastle, owner, hallPaidUntil, endDate)
	for _, q := range s.seed {
		exec(q)
	}
}

// hallRow reads Devastated Castle's owner, lease term and next siege date.
func (w *hallSiegeWorld) hallRow(t *testing.T) (owner, paidUntil, endDate int64) {
	t.Helper()
	w.srv.FlushPersistence(t)
	if err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT ownerId, paidUntil, endDate FROM clanhall WHERE id = ?", devastatedCastle).Scan(&owner, &paidUntil, &endDate); err != nil {
		t.Fatal(err)
	}
	return owner, paidUntil, endDate
}

// storedAttackers reads hall hallID's clanhall_siege_attackers rows.
func (w *hallSiegeWorld) storedAttackers(t *testing.T, hallID int32) []int32 {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.DB.QueryContext(context.Background(),
		"SELECT attacker_id FROM clanhall_siege_attackers WHERE clanhall_id = ? ORDER BY attacker_id", hallID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int32
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// sysMsgParams returns the id and parameters of each system message among
// frames, in order.
func sysMsgParams(t *testing.T, frames [][]byte) [][]string {
	t.Helper()
	var out [][]string
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			id, params := sysMsg(t, f)
			out = append(out, append([]string{strconv.Itoa(id)}, params...))
		}
	}
	return out
}

// register sends the Attacker's registration on Devastated Castle's siege
// and returns the frames it got back.
func (w *hallSiegeWorld) register(t *testing.T, joining int32) [][]byte {
	t.Helper()
	w.c.Send(encodeSiegeRequest(clientpackets.OpcodeRequestJoinSiege, devastatedCastle, 1, joining))
	return drainFrames(t, w.c)
}

// TestHallSiegeRegistersStartsAndEnds runs a siege of Devastated Castle
// from the Attacker's registration through a win. The registration is
// stored and answered with the hall's siege window, and the attacker list
// names Besiegers. Closing the registrations adds the owner Holders to the
// attackers, frees the hall (its row now owned by no one) and tells
// everyone (REGISTRATION_TERM_FOR_S1_ENDED naming the hall). The start
// stands the gate again closed at full HP, turns the battlefield on, gives
// the Attacker the attacker state and has the script spawn its NPCs; the
// gate then takes the Attacker's hits. The end, the script naming
// Besiegers after it won, gives Besiegers the hall, tells everyone
// (S1_CLAN_DEFEATED_S2 with the clan and the hall, then
// SIEGE_OF_S1_FINISHED), turns the battlefield off, clears the attacker
// state and the stored attackers, flags the players left on the
// battlefield, stores the next date on the row (owner and endDate only),
// opens the registrations and has the script take its NPCs away.
func TestHallSiegeRegistersStartsAndEnds(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{owned: true})
	if w.srv.PlayerPvPFlagged(t, w.objID) {
		t.Fatal("the Attacker is flagged before the siege")
	}
	frame, ok := firstOpcode(w.register(t, 1), serverpackets.OpcodeSiegeInfo)
	if !ok {
		t.Fatal("the registration got no siege window")
	}
	if hall, lord, owner, name := siegeInfo(t, frame); hall != devastatedCastle || lord != 0 || owner != holdersClanID || name != "Holders" {
		t.Fatalf("siege window = hall %d, lord %d, owner %#x %q", hall, lord, owner, name)
	}
	if got := w.storedAttackers(t, devastatedCastle); !slices.Equal(got, []int32{besiegersClanID}) {
		t.Fatalf("stored attackers = %#x, want Besiegers", got)
	}
	w.c.Send(encodeSiegeRequest(clientpackets.OpcodeRequestSiegeAttackerList, devastatedCastle))
	if frame, ok := firstOpcode(drainFrames(t, w.c), serverpackets.OpcodeSiegeAttackerList); !ok {
		t.Fatal("no attacker list")
	} else if id, clans := siegeListClans(t, frame, false); id != devastatedCastle || !slices.Equal(clans, []int32{besiegersClanID}) {
		t.Fatalf("attacker list = hall %d, clans %#x", id, clans)
	}
	if w.srv.DoorAttackableBy(t, devastatedGateID, w.objID) {
		t.Fatal("the gate is attackable before the siege")
	}
	w.srv.HitDoor(t, devastatedGateID, w.objID, 30)
	if hp, _ := w.srv.DoorState(t, devastatedGateID); hp != 100 {
		t.Fatalf("gate HP before the siege = %v, want 100", hp)
	}

	// The start follows the close at once, on the halls' queue.
	if !w.siege.Instant() {
		t.Fatal("no siege to start")
	}
	if !w.siege.WaitingBattle() || !slices.Equal(w.siege.Attackers(), []int32{besiegersClanID, holdersClanID}) {
		t.Fatalf("after the close: waiting %v, attackers %#x", w.siege.WaitingBattle(), w.siege.Attackers())
	}
	w.srv.Advance(t, 0)
	frames := drainFrames(t, w.c)
	// ENTERED_COMBAT_ZONE: the battlefield turns on under the Attacker.
	if got := sysMsgParams(t, frames); !slices.EqualFunc(got, [][]string{{"293", "34"}, {"283"}}, slices.Equal) {
		t.Fatalf("close and start notices = %v", got)
	}
	if owner, _, _ := w.hallRow(t); owner != 0 {
		t.Fatalf("hall owner after the close = %#x, want none", owner)
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeUserInfo); !ok {
		t.Fatalf("the start sent the Attacker no UserInfo: %x", opcodes(frames))
	}
	if got := w.srv.PlayerSiegeState(t, w.objID); got != 1 {
		t.Fatalf("siege state = %d, want 1", got)
	}
	if !w.siege.InProgress() || !w.field.Active() || w.script.spawned.Load() != 1 {
		t.Fatalf("started: in progress %v, battlefield %v, spawns %d", w.siege.InProgress(), w.field.Active(), w.script.spawned.Load())
	}
	if hp, open := w.srv.DoorState(t, devastatedGateID); open || hp != 100 {
		t.Fatalf("gate after the start: HP %v, open %v; want 100, closed", hp, open)
	}
	if s, ok := w.srv.Halls.ActiveSiegeAt(w.at.X, w.at.Y, w.at.Z); !ok || s.ID() != devastatedCastle {
		t.Fatal("no active hall siege where the Attacker stands")
	}
	if !w.siege.Attacking(besiegersClanID) || !w.siege.Attacking(holdersClanID) || w.siege.Attacking(0x7f0004ff) {
		t.Fatal("attacker sides wrong during the siege")
	}
	if !w.srv.DoorAttackableBy(t, devastatedGateID, w.objID) {
		t.Fatal("the gate is not attackable by an attacker during the siege")
	}
	w.srv.HitDoor(t, devastatedGateID, w.objID, 30)
	if hp, _ := w.srv.DoorState(t, devastatedGateID); hp != 70 {
		t.Fatalf("gate HP after a hit = %v, want 70", hp)
	}

	w.script.winner.Store(besiegersClanID)
	w.siege.SetMissionAccomplished(true)
	if !w.siege.End() {
		t.Fatal("no siege to end")
	}
	frames = drainFrames(t, w.c)
	// LEFT_COMBAT_ZONE: the battlefield turns off under the Attacker.
	if got := sysMsgParams(t, frames); !slices.EqualFunc(got, [][]string{{"855", "Besiegers", "34"}, {"843", "34"}, {"284"}}, slices.Equal) {
		t.Fatalf("end notices = %v", got)
	}
	if got := w.srv.PlayerSiegeState(t, w.objID); got != 0 {
		t.Fatalf("siege state after the end = %d, want 0", got)
	}
	if !w.srv.PlayerPvPFlagged(t, w.objID) {
		t.Fatal("the Attacker, left on the battlefield, is not flagged")
	}
	if w.siege.Status() != siege.StatusRegistrationOpened || w.field.Active() || w.script.unspawned.Load() != 1 {
		t.Fatalf("ended: status %v, battlefield %v, unspawns %d", w.siege.Status(), w.field.Active(), w.script.unspawned.Load())
	}
	if len(w.siege.Attackers()) != 0 || len(w.storedAttackers(t, devastatedCastle)) != 0 {
		t.Fatal("attackers kept after the end")
	}
	if id, ok := w.srv.Halls.ByOwner(besiegersClanID); !ok || id != devastatedCastle {
		t.Fatal("Besiegers does not own Devastated Castle")
	}
	want := clanhall.NextSiegeDate(time.UnixMilli(w.srv.Halls.Now()), []int{14, 0, 0, 12, 0}).UnixMilli()
	if owner, paidUntil, endDate := w.hallRow(t); owner != int64(besiegersClanID) || paidUntil != hallPaidUntil || endDate != want {
		t.Fatalf("hall row = owner %#x, paidUntil %d, endDate %d; want %#x, %d, %d", owner, paidUntil, endDate, besiegersClanID, hallPaidUntil, want)
	}
	if w.siege.Date() != want {
		t.Fatalf("next siege date = %d, want %d", w.siege.Date(), want)
	}
	if hp, open := w.srv.DoorState(t, devastatedGateID); open || hp != 100 {
		t.Fatalf("gate after the end: HP %v, open %v; want 100, closed", hp, open)
	}
	if w.srv.DoorAttackableBy(t, devastatedGateID, w.objID) {
		t.Fatal("the gate is attackable after the siege")
	}
}

// TestHallSiegeCalledOffWithoutAttackers closes the registrations of a
// free hall nobody registered on: the start calls the siege off
// (SIEGE_OF_S1_HAS_BEEN_CANCELED_DUE_TO_LACK_OF_INTEREST naming the hall),
// stores the next date and opens the registrations again; the script
// spawns nothing. The Fortress of Resistance starts with no attacker.
func TestHallSiegeCalledOffWithoutAttackers(t *testing.T) {
	t.Parallel()
	fortress := &standIn{}
	w := bootHallSiege(t, hallSiegeSetup{
		seed: []string{"INSERT INTO clanhall (id, endDate) VALUES (21, " + strconv.FormatInt(time.Now().Add(7*24*time.Hour).UnixMilli(), 10) + ")"},
		extra: []gameservertest.Option{
			gameservertest.WithHallSiegeScript(clanhall.FortressOfResistance, fortress.script()),
		},
	})
	w.siege.Instant()
	w.srv.Advance(t, 0)
	if got := sysMsgParams(t, drainFrames(t, w.c)); !slices.EqualFunc(got, [][]string{{"293", "34"}, {"846", "34"}}, slices.Equal) {
		t.Fatalf("notices = %v", got)
	}
	if w.siege.Status() != siege.StatusRegistrationOpened || w.script.spawned.Load() != 0 || w.field.Active() {
		t.Fatal("a siege with no attacker went ahead")
	}
	want := clanhall.NextSiegeDate(time.UnixMilli(w.srv.Halls.Now()), []int{14, 0, 0, 12, 0}).UnixMilli()
	if _, _, endDate := w.hallRow(t); endDate != want {
		t.Fatalf("stored next siege date = %d, want %d", endDate, want)
	}

	fr, ok := w.srv.Halls.Siege(clanhall.FortressOfResistance)
	if !ok || !fr.Instant() {
		t.Fatal("no Fortress of Resistance siege")
	}
	w.srv.Advance(t, 0)
	if !fr.InProgress() || fortress.spawned.Load() != 1 {
		t.Fatal("the Fortress of Resistance siege did not start without attackers")
	}
}

// TestHallSiegeBanishesAtTheStart has the Attacker, of a clan registered
// on the siege, and the Recruit, of none, in the free hall's grounds, its
// siege 70 minutes away: the registrations close an hour before it,
// throwing out every clan's players; the start, at the siege date, throws
// out everyone, each to the hall's banish point.
func TestHallSiegeBanishesAtTheStart(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{groundsOverSpawn: true, endDate: time.Now().Add(70 * time.Minute).UnixMilli()})
	recruitID := w.srv.SeedCharacterFor(t, "player2", "Recruit", 1, 0).ID
	recruit := w.srv.DialClient(t, "player2", 1)
	startInWorld(t, recruit)
	w.register(t, 1)
	near := func(id int32) bool {
		x, y, _ := w.srv.PlayerPosition(t, id)
		return max(x-devastatedBanish.X, devastatedBanish.X-x) <= 20 && max(y-devastatedBanish.Y, devastatedBanish.Y-y) <= 20
	}
	w.srv.Advance(t, 11*time.Minute)
	if !w.siege.WaitingBattle() {
		t.Fatalf("status 11 minutes on = %v, want the registrations closed", w.siege.Status())
	}
	if !near(w.objID) || near(recruitID) {
		t.Fatal("the close did not banish the clan's player alone")
	}
	w.srv.Advance(t, time.Hour)
	if !w.siege.InProgress() {
		t.Fatalf("status at the siege date = %v, want in progress", w.siege.Status())
	}
	if !near(recruitID) {
		t.Fatal("the start did not banish the clanless player")
	}
}

// TestHallSiegeMessenger has Loken answer the siege window outside the
// siege, its owner's leader siege/01.htm, and anyone siege/02.htm while the
// siege is under way.
func TestHallSiegeMessenger(t *testing.T) {
	t.Parallel()
	talk := func(t *testing.T, w *hallSiegeWorld) [][]byte {
		t.Helper()
		loken := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("SiegeNpc", lokenID), location.Location{X: w.at.X + 30, Y: w.at.Y, Z: w.at.Z})
		drainFrames(t, w.c)
		w.c.Send(encodeAction(loken.ObjectID(), w.at))
		drainFrames(t, w.c)
		w.c.Send(encodeAction(loken.ObjectID(), w.at))
		return drainFrames(t, w.c)
	}
	page := func(t *testing.T, frames [][]byte) string {
		t.Helper()
		f, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
		if !ok {
			t.Fatalf("no page: %x", opcodes(frames))
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32()
		return r.ReadString()
	}
	t.Run("outside the siege", func(t *testing.T) {
		t.Parallel()
		w := bootHallSiege(t, hallSiegeSetup{owned: true})
		if f, ok := firstOpcode(talk(t, w), serverpackets.OpcodeSiegeInfo); !ok {
			t.Fatal("no siege window")
		} else if hall, _, _, _ := siegeInfo(t, f); hall != devastatedCastle {
			t.Fatalf("siege window of hall %d", hall)
		}
	})
	t.Run("owner's leader", func(t *testing.T) {
		t.Parallel()
		w := bootHallSiege(t, hallSiegeSetup{seed: []string{"UPDATE clanhall SET ownerId = " + itoa(besiegersClanID) + " WHERE id = 34"}})
		if got := page(t, talk(t, w)); !strings.Contains(got, "This area is secure") {
			t.Fatalf("owner's leader page = %q", got)
		}
	})
	t.Run("during the siege", func(t *testing.T) {
		t.Parallel()
		w := bootHallSiege(t, hallSiegeSetup{})
		w.register(t, 1)
		w.siege.Instant()
		w.srv.Advance(t, 0)
		if got := page(t, talk(t, w)); !strings.Contains(got, "How dare our foolish enemies") {
			t.Fatalf("page during the siege = %q", got)
		}
	})
}
