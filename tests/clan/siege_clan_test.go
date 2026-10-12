package clan

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: a clan registered on a castle siege may not be dissolved
// (VillageMaster.java:187-194: the first castle whose siege lists the clan
// answers CANNOT_DISSOLVE_WHILE_IN_SIEGE (265) while that siege runs, else
// CANNOT_DISSOLVE_CAUSE_CLAN_WILL_PARTICIPATE_IN_CASTLE_SIEGE (13)), and
// /clanpenalty lists it as unable to dissolve (ClanPenalty.java:68-80). A
// clan on a side of a siege in progress may not found an alliance
// (Clan.createAlly, NO_ALLY_CREATION_WHILE_SIEGE 721) nor dissolve its
// alliance (Clan.dissolveAlly, CANNOT_DISSOLVE_ALLY_WHILE_IN_SIEGE 722);
// a clan registered against the leading clan may not be invited
// (Clan.checkAllyJoinCondition, OPPOSING_CLAN_IS_PARTICIPATING_IN_SIEGE
// 723). A destroyed clan leaves every siege's registrations
// (ClanTable.destroyClan); each member loses its siege state, the leader
// its siege skills (Clan.removeClanMember). A member entering the world or
// joining its clan during a siege takes the siege state of its clan's
// side (EnterWorld, Clan.addClanMember); an attacker or a spectator
// entering the world on a battlefield under siege is sent to town
// (EnterWorld.java:266-269). /siegestatus opens siege_status.htm for a
// noble clan leader whose clan fights a siege in progress
// (SiegeStatus.useUserCommand).

// The siege refusals, as the reference numbers them.
const (
	msgCannotDissolveRegistered  = 13
	msgCannotDissolveInSiege     = 265
	msgNoAllyCreationInSiege     = 721
	msgCannotDissolveAllyInSiege = 722
	msgOpposingClanInSiege       = 723
	msgOnlyDuringSiege           = 1695
)

// Seal of Ruler and Build Headquarters, the siege skills of a clan leader.
const (
	sealOfRuler = 246
	buildHQ     = 247
)

// siegeField is the middle of the Gludio Castle battlefield the siege clan
// tests stand on, and siegeTown the town its restart point sends players
// to; both lie in the map region of the world's origin.
var (
	siegeField = location.Location{X: 20000, Y: 20000, Z: 0}
	siegeTown  = location.Location{X: -5000, Y: 2000, Z: 30}
)

// siegeClanZones is the Gludio Castle battlefield, 3000 around siegeField.
func siegeClanZones(t *testing.T) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(siegeField.X-3000, siegeField.X+3000, siegeField.Y-3000, siegeField.Y+3000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(11, form)
	field.ResidenceID = 1
	zones.Add(field)
	return zones
}

// siegeClanTown is a restart point table sending every player in the
// origin's map region to siegeTown.
func siegeClanTown() *restart.Table {
	region := location.Point{
		X: (0-world.MinX)/world.TileSize + world.TileXMin,
		Y: (0-world.MinY)/world.TileSize + world.TileYMin,
	}
	return &restart.Table{Points: []restart.Point{{
		Name: "TestTown", LocName: 910, MapRegions: []location.Point{region},
		Points: []location.Location{siegeTown},
	}}}
}

// siegeClanPages are the alliance dialog pages with the clan dissolution
// pages, the /clanpenalty window and the /siegestatus window.
func siegeClanPages(t *testing.T) gameservertest.Option {
	t.Helper()
	read := func(parts ...string) string {
		data, err := os.ReadFile(datapack.Path(t, parts...))
		if err != nil {
			t.Fatalf("read %v: %v", parts, err)
		}
		return string(data)
	}
	pages := alliancePages(t)
	key := "villagemaster/" + strconv.Itoa(masterID) + ".htm"
	for _, name := range []string{"9000-04.htm", "9000-05.htm"} {
		pages[key] += read("data", "html", "script", "feature", "Clan", name)
	}
	pages["clan_penalty.htm"] = read("data", "html", "clan_penalty.htm")
	pages["siege_status.htm"] = read("data", "html", "siege_status.htm")
	return gameservertest.WithHTMLPages(pages)
}

// bootSiegeClanWorld is bootAllianceCast with the shipped castles and
// their sieges, the Gludio battlefield around siegeField, the shipped
// skills and the siege clan pages.
func bootSiegeClanWorld(t *testing.T, cast []castMember, stmts []string, extra ...gameservertest.Option) *clanWorld {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	opts := append(clanSkillOptions(t),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(siegeClanZones(t)),
		gameservertest.WithRestartPoints(siegeClanTown()),
		siegeClanPages(t),
	)
	return bootAllianceCast(t, cast, stmts, append(opts, extra...)...)
}

// registerOnGludio registers clanID on Gludio Castle's siege on side.
func registerOnGludio(clanID int32, side string) string {
	return `INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ` + itoa(clanID) + `, '` + side + `')`
}

// standOnField places character id on the battlefield.
func standOnField(id int32) string {
	return `UPDATE characters SET x = ` + itoa(int32(siegeField.X)) + `, y = ` + itoa(int32(siegeField.Y)) + `, z = ` + itoa(int32(siegeField.Z)) + ` WHERE obj_Id = ` + itoa(id)
}

// startGludio starts Gludio Castle's siege and drains what the founder and
// the rival hear of it.
func startGludio(t *testing.T, w *clanWorld) *siege.Siege {
	t.Helper()
	gludio, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("Gludio's siege did not start")
	}
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return gludio
}

// TestSiegeRegisteredClanRefusals: Knights, registered to attack Gludio,
// may not be dissolved (13) and /clanpenalty says so; once the siege runs,
// the refusal is 265 and founding an alliance is refused with 721. Nothing
// is stored either way.
func TestSiegeRegisteredClanRefusals(t *testing.T) {
	t.Parallel()
	w := bootSiegeClanWorld(t, nil, []string{registerOnGludio(knightsClanID, "ATTACKER")})
	knights := ` WHERE clan_id = ` + itoa(knightsClanID)

	expectMessages(t, "dissolution before the siege", w.masterCommandBy(t, w.leader, "dissolve_clan"), msgCannotDissolveRegistered)
	if got, want := penaltyRows(t, userCommand(t, w.leader, cmdClanPenalty)), "<tr><td width=170>Unable to dissolve a clan.</td><td></td></tr>"; got != want {
		t.Fatalf("/clanpenalty rows = %q, want %q", got, want)
	}

	startGludio(t, w)
	expectMessages(t, "dissolution during the siege", w.masterCommandBy(t, w.leader, "dissolve_clan"), msgCannotDissolveInSiege)
	expectMessages(t, "alliance founding during the siege", w.masterCommandBy(t, w.leader, "create_ally Ally"), msgNoAllyCreationInSiege)

	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT dissolving_expiry_time FROM clan_data`+knights); got != 0 {
		t.Fatalf("dissolving_expiry_time = %d after the refusals, want 0", got)
	}
	if got := storedAlly(t, w, knightsClanID); got.allyID != 0 {
		t.Fatalf("stored alliance = %+v after the refusal, want none", got)
	}
}

// TestAllianceSiegeRefusals: Knights leads the alliance Ally and attacks
// Gludio, which Rivals defends. Inviting Rivals is refused with 723 before
// the siege; once it runs, dissolving the alliance is refused with 722.
func TestAllianceSiegeRefusals(t *testing.T) {
	t.Parallel()
	w := bootSiegeClanWorld(t, nil, []string{
		`UPDATE clan_data SET ally_id = ` + itoa(knightsClanID) + `, ally_name = 'Ally' WHERE clan_id = ` + itoa(knightsClanID),
		registerOnGludio(knightsClanID, "ATTACKER"),
		registerOnGludio(rivalsClanID, "DEFENDER"),
	})

	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	expectMessages(t, "invitation of the defending clan", drainFrames(t, w.leader), msgOpposingClanInSiege)
	if got := drainFrames(t, w.member); len(got) != 0 {
		t.Fatalf("the refused invitation reached the rival: %x", opcodes(got))
	}

	startGludio(t, w)
	w.leader.Send(encodeAllyBare(clientpackets.OpcodeRequestDismissAlly))
	expectMessages(t, "alliance dissolution during the siege", drainFrames(t, w.leader), msgCannotDissolveAllyInSiege)
	if got := storedAlly(t, w, knightsClanID); got.allyID != int64(knightsClanID) {
		t.Fatalf("stored alliance = %+v after the refusal, want Ally kept", got)
	}
}

// TestDestroyedClanLeavesTheSiege restores Knights, registered to attack
// Gludio, with its dissolution due: its leader holds the siege skills of a
// level 5 clan's leader and, once the siege runs, the attacker state. A
// minute after boot the clan is destroyed: it leaves the siege, and its
// leader loses the siege state and the siege skills.
func TestDestroyedClanLeavesTheSiege(t *testing.T) {
	t.Parallel()
	past := strconv.FormatInt(time.Now().UnixMilli()-dayMs, 10)
	w := bootSiegeClanWorld(t, nil, []string{
		registerOnGludio(knightsClanID, "ATTACKER"),
		`UPDATE clan_data SET dissolving_expiry_time = ` + past + ` WHERE clan_id = ` + itoa(knightsClanID),
	})
	if !w.srv.DrivesClock() {
		t.Skip("the minute after boot is waited out on the test clock")
	}
	for _, id := range []int{sealOfRuler, buildHQ} {
		if got := w.srv.PlayerSkillLevel(t, w.leaderID, id); got != 1 {
			t.Fatalf("leader's skill %d at login = %d, want 1", id, got)
		}
	}
	gludio := startGludio(t, w)
	if got := w.srv.PlayerSiegeState(t, w.leaderID); got != 1 {
		t.Fatalf("leader's siege state = %d, want 1", got)
	}

	w.srv.Advance(t, time.Minute)
	wantDispersed(t, "leader", drainFrames(t, w.leader))
	if side := gludio.Side(knightsClanID); side != siege.SideNone {
		t.Fatalf("the destroyed clan stays on the siege as %v", side)
	}
	if got := w.srv.PlayerSiegeState(t, w.leaderID); got != 0 {
		t.Fatalf("leader's siege state after the dissolution = %d, want 0", got)
	}
	for _, id := range []int{sealOfRuler, buildHQ} {
		if got := w.srv.PlayerSkillLevel(t, w.leaderID, id); got != 0 {
			t.Fatalf("leader's skill %d after the dissolution = %d, want 0", id, got)
		}
	}
}

// TestSiegeLoginOnTheBattlefield runs Gludio's siege with Knights attacking
// and Rivals defending, then brings three players in on the battlefield:
// Squire, defending with Rivals, takes the defender state and stays; Third,
// attacking with Knights, takes the attacker state and is sent to town
// after the login's reuse timers and clan reminder, ahead of its closing
// ActionFailed; Loner, in no clan, is sent to town too, then joins Knights
// and takes the attacker state. The rival, a noble leading Rivals, then
// sees the siege status window: Rivals' kills and deaths, and its members
// in the world, the rival off the battlefield and Squire on it.
func TestSiegeLoginOnTheBattlefield(t *testing.T) {
	t.Parallel()
	w := bootSiegeClanWorld(t, []castMember{allySquire, loner, third}, []string{
		registerOnGludio(knightsClanID, "ATTACKER"),
		registerOnGludio(rivalsClanID, "DEFENDER"),
		squireInRivals(),
		`UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(thirdID),
		`UPDATE characters SET nobless = 1 WHERE obj_Id = ` + itoa(rivalLeaderID),
		standOnField(allySquireID), standOnField(lonerID), standOnField(thirdID),
	})
	gludio := startGludio(t, w)
	w.srv.Sieges.RecordKill(rivalsClanID, knightsClanID)
	w.srv.Sieges.RecordKill(rivalsClanID, knightsClanID)
	w.srv.Sieges.RecordKill(knightsClanID, rivalsClanID)

	login := func(m castMember) (*testsupport.ScriptedClient, [][]byte) {
		t.Helper()
		c := w.srv.DialClient(t, m.account, 1)
		return c, startInWorld(t, c)
	}
	sentToTown := func(who string, frames [][]byte) bool {
		t.Helper()
		i := firstIndexOf(frames, serverpackets.OpcodeTeleportToLocation)
		if i < 0 {
			return false
		}
		cool := firstIndexOf(frames, serverpackets.OpcodeSkillCoolTime)
		if cool < 0 || i < cool || frames[len(frames)-1][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("%s login = %x, want the teleport after SkillCoolTime and ActionFailed last", who, opcodes(frames))
		}
		return true
	}

	squire, frames := login(allySquire)
	if sentToTown("defender", frames) {
		t.Fatal("the defender was sent to town")
	}
	if got := w.srv.PlayerSiegeState(t, allySquireID); got != 2 {
		t.Fatalf("defender's siege state = %d, want 2", got)
	}
	_, frames = login(third)
	if !sentToTown("attacker", frames) {
		t.Fatalf("the attacker logging in on the battlefield stayed: %x", opcodes(frames))
	}
	if got := w.srv.PlayerSiegeState(t, thirdID); got != 1 {
		t.Fatalf("attacker's siege state = %d, want 1", got)
	}
	lonerClient, frames := login(loner)
	if !sentToTown("spectator", frames) {
		t.Fatalf("the spectator logging in on the battlefield stayed: %x", opcodes(frames))
	}
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	drainFrames(t, squire)

	w.leader.Send(encodeRequestJoinPledge(lonerID, 0))
	drainFrames(t, lonerClient)
	lonerClient.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, lonerClient), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("Loner did not join Knights")
	}
	if got := w.srv.PlayerSiegeState(t, lonerID); got != 1 {
		t.Fatalf("siege state of the member joining an attacking clan = %d, want 1", got)
	}

	page := siegeStatusPage(t, userCommand(t, w.member, cmdSiegeStatus))
	for _, want := range []string{
		"Kills: 2<br1>", "Deaths: 1<br>",
		"<tr><td width=170>Rival</td><td width=100>Outside Castle Siege Zone</td></tr><tr><td width=170>Squire</td><td width=100>Castle Siege in Progress</td></tr>",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("siege status window = %q, want it to hold %q", page, want)
		}
	}

	gludio.End()
	drainFrames(t, w.member)
	wantSysMsgs(t, "/siegestatus after the siege", userCommand(t, w.member, cmdSiegeStatus), msg(msgOnlyDuringSiege))
}

// firstIndexOf is the index of the first frame with opcode, -1 for none.
func firstIndexOf(frames [][]byte, opcode byte) int {
	return slices.IndexFunc(frames, func(f []byte) bool { return len(f) > 0 && f[0] == opcode })
}

// siegeStatusPage returns the html of the one window frames hold.
func siegeStatusPage(t *testing.T, frames [][]byte) string {
	t.Helper()
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("/siegestatus answer = %x, want one NpcHtmlMessage", opcodes(frames))
	}
	r := wire.NewReader(frames[0][1:])
	if id := r.ReadInt32(); id != 0 {
		t.Fatalf("window object id = %d, want 0", id)
	}
	return r.ReadString()
}
