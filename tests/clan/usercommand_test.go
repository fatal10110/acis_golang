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

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// User command ids the client sends for the clan commands.
const (
	cmdAttackList      = 88
	cmdUnderAttackList = 89
	cmdWarList         = 90
	cmdSiegeStatus     = 99
	cmdClanPenalty     = 100
)

func encodeUserCommand(id int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUserCommand)
	w.WriteInt32(id)
	return w.Bytes()
}

// userCommand has c run user command id and returns c's answer.
func userCommand(t *testing.T, c *testsupport.ScriptedClient, id int32) [][]byte {
	t.Helper()
	c.Send(encodeUserCommand(id))
	return drainFrames(t, c)
}

// seedStatements runs stmts against db, failing the test on the first
// error.
func seedStatements(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
}

// bootUserCommandClan boots the founder leading the level-5 clan Knights,
// seeded with extra statements, in the world.
func bootUserCommandClan(t *testing.T, extra ...string) (*gameservertest.Server, *testsupport.ScriptedClient) {
	t.Helper()
	page, err := os.ReadFile(datapack.Path(t, "data", "html", "clan_penalty.htm"))
	if err != nil {
		t.Fatalf("read clan_penalty.htm: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(map[string]string{"clan_penalty.htm": string(page)}),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedStatements(t, db, append([]string{
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
					SELECT ` + itoa(subunitClanID) + `, 'Knights', 5, obj_Id FROM characters WHERE char_name = 'Founder'`,
				`UPDATE characters SET clanid = ` + itoa(subunitClanID) + `, power_grade = 0 WHERE char_name = 'Founder'`,
			}, extra...)...)
		}),
	)
	startInWorld(t, srv.Client)
	drainFrames(t, srv.Client)
	return srv, srv.Client
}

// wantSysMsgs fails unless frames are exactly the system messages want, each
// an id followed by its parameters.
func wantSysMsgs(t *testing.T, what string, frames [][]byte, want ...[]string) {
	t.Helper()
	var got [][]string
	for _, f := range frames {
		id, params := sysMsg(t, f)
		got = append(got, append([]string{strconv.Itoa(id)}, params...))
	}
	if !slices.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func msg(id int, params ...string) []string { return append([]string{strconv.Itoa(id)}, params...) }

// TestClanWarsListCommands pins /attacklist, /underattacklist and /warlist
// (ClanWarsList) over the stored war rows of the clan Knights:
//
//   - Knights declared war on Twenty (allied to Allies), Three and Hundred,
//     and keeps a re-declaration penalty row on Thousand from a war it
//     stopped; Hundred, Nine and Fifty declared war on Knights. Allies and
//     Pact are led by clans 77 and 88, seeded so the boot alliance check
//     (ClanTable.allianceCheck) keeps Twenty and Fifty in them.
//   - /attacklist lists the rows Knights holds that no row answers: Thousand,
//     Twenty and Three, in the rows' key order, which compares the clan ids
//     as text ("1000" < "20" < "3"). /underattacklist lists Fifty then Nine;
//     /warlist only Hundred.
//   - Each non-empty listing sends its header, then every clan but the
//     first, then the footer: the header's result set is positioned on the
//     first row and the loop advances before reading, so the first clan is
//     never named.
//
// The row order was checked against MariaDB with the shipped clan_data and
// clan_wars tables and the three listing queries, ids 3, 9, 20, 50, 100,
// 1000.
func TestClanWarsListCommands(t *testing.T) {
	t.Parallel()
	k := itoa(subunitClanID)
	penalty := strconv.FormatInt(time.Now().Add(72*time.Hour).UnixMilli(), 10)
	_, c := bootUserCommandClan(t,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, ally_id, ally_name) VALUES
			(20, 'Twenty', 3, 77, 'Allies'), (3, 'Three', 3, 0, NULL), (100, 'Hundred', 3, 0, NULL),
			(1000, 'Thousand', 3, 0, NULL), (9, 'Nine', 3, 0, NULL), (50, 'Fifty', 3, 88, 'Pact'),
			(77, 'Allies Lead', 5, 77, 'Allies'), (88, 'Pact Lead', 5, 88, 'Pact')`,
		`INSERT INTO clan_wars (clan1, clan2, expiry_time) VALUES
			('`+k+`', '20', 0), ('`+k+`', '3', 0), ('`+k+`', '100', 0), ('`+k+`', '1000', `+penalty+`),
			('100', '`+k+`', 0), ('9', '`+k+`', 0), ('50', '`+k+`', 0)`,
	)
	wantSysMsgs(t, "/attacklist", userCommand(t, c, cmdAttackList),
		msg(serverpackets.SystemMessageClansYouDeclaredWarOn),
		msg(serverpackets.SystemMessageS1S2Alliance, "Twenty", "Allies"),
		msg(serverpackets.SystemMessageS1NoAllianceExists, "Three"),
		msg(serverpackets.SystemMessageFriendListFooter))
	wantSysMsgs(t, "/underattacklist", userCommand(t, c, cmdUnderAttackList),
		msg(serverpackets.SystemMessageClansThatHaveDeclaredWarOnYou),
		msg(serverpackets.SystemMessageS1NoAllianceExists, "Nine"),
		msg(serverpackets.SystemMessageFriendListFooter))
	wantSysMsgs(t, "/warlist", userCommand(t, c, cmdWarList),
		msg(serverpackets.SystemMessageWarList),
		msg(serverpackets.SystemMessageFriendListFooter))
}

// TestClanWarsListEmptyAndClanless pins ClanWarsList's other answers: a
// clan with no war rows is told it is in no war of that kind, each listing
// with its own message, and a clanless player is told it is not
// authorized.
func TestClanWarsListEmptyAndClanless(t *testing.T) {
	t.Parallel()
	srv, c := bootUserCommandClan(t)
	wantSysMsgs(t, "empty /attacklist", userCommand(t, c, cmdAttackList), msg(serverpackets.SystemMessageYouArentInClanWars))
	wantSysMsgs(t, "empty /underattacklist", userCommand(t, c, cmdUnderAttackList), msg(serverpackets.SystemMessageNoClanWarsVsYou))
	wantSysMsgs(t, "empty /warlist", userCommand(t, c, cmdWarList), msg(serverpackets.SystemMessageNotInvolvedInWar))

	srv.SeedCharacterFor(t, "player2", "Drifter", 10, 0)
	drifter := srv.DialClient(t, "player2", 1)
	startInWorld(t, drifter)
	drainFrames(t, c)
	for _, id := range []int32{cmdAttackList, cmdUnderAttackList, cmdWarList} {
		wantSysMsgs(t, "clanless listing", userCommand(t, drifter, id), msg(serverpackets.SystemMessageNotAuthorizedToDoThat))
	}
}

// penaltyRows returns the %content% rows of a clan penalty window: the
// rows between the header table and the closing image.
func penaltyRows(t *testing.T, frames [][]byte) string {
	t.Helper()
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("/clanpenalty answer = %x, want one NpcHtmlMessage", opcodes(frames))
	}
	r := wire.NewReader(frames[0][1:])
	if id := r.ReadInt32(); id != 0 {
		t.Fatalf("window object id = %d, want 0", id)
	}
	html := r.ReadString()
	if item := r.ReadInt32(); item != 0 {
		t.Fatalf("window item id = %d, want 0", item)
	}
	const open, end = "<table width=270>", "</table>"
	i := strings.Index(html, open)
	if i < 0 {
		t.Fatalf("window = %q, want the penalty table", html)
	}
	rest := html[i+len(open):]
	return strings.TrimSpace(rest[:strings.Index(rest, end)])
}

// TestClanPenaltyCommand pins /clanpenalty (ClanPenalty): every penalty in
// force, in order and dated yyyy-MM-dd, then the dissolution refusal of a
// clan in an alliance; a clanless player without penalties sees the
// no-penalty row. Expired penalties are not listed. Knights' alliance is
// led by a seeded clan 77, which keeps it through the boot alliance check.
func TestClanPenaltyCommand(t *testing.T) {
	t.Parallel()
	k := itoa(subunitClanID)
	future := time.Date(2031, 3, 4, 12, 0, 0, 0, time.Local)
	ms := strconv.FormatInt(future.UnixMilli(), 10)
	past := strconv.FormatInt(time.Now().Add(-time.Hour).UnixMilli(), 10)
	srv, c := bootUserCommandClan(t,
		`UPDATE characters SET clan_join_expiry_time = `+ms+`, clan_create_expiry_time = `+past+` WHERE char_name = 'Founder'`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, ally_id, ally_name) VALUES (77, 'Allies Lead', 5, 77, 'Allies')`,
		`UPDATE clan_data SET ally_id = 77, ally_name = 'Allies', char_penalty_expiry_time = `+ms+`,
			ally_penalty_type = 3, ally_penalty_expiry_time = `+ms+`, dissolving_expiry_time = `+ms+` WHERE clan_id = `+k,
	)
	date := "2031-03-04"
	want := "<tr><td width=170>Unable to join a clan.</td><td width=100 align=center>" + date + "</td></tr>" +
		"<tr><td width=170>Unable to invite a clan member.</td><td width=100 align=center>" + date + "</td></tr>" +
		"<tr><td width=170>Unable to invite a new alliance member.</td><td width=100 align=center>" + date + "</td></tr>" +
		"<tr><td width=170>The request to dissolve the clan is currently being processed.  (Restrictions are now going to be imposed on the use of clan functions.)</td><td width=100 align=center>" + date + "</td></tr>" +
		"<tr><td width=170>Unable to dissolve a clan.</td><td></td></tr>"
	if got := penaltyRows(t, userCommand(t, c, cmdClanPenalty)); got != want {
		t.Fatalf("penalty rows =\n%s\nwant\n%s", got, want)
	}

	srv.SeedCharacterFor(t, "player2", "Drifter", 10, 0)
	drifter := srv.DialClient(t, "player2", 1)
	startInWorld(t, drifter)
	drainFrames(t, c)
	if got, want := penaltyRows(t, userCommand(t, drifter, cmdClanPenalty)), "<tr><td width=170>No penalty is imposed.</td><td width=100 align=center></td></tr>"; got != want {
		t.Fatalf("clanless penalty rows = %s, want %s", got, want)
	}
}

// TestClanPenaltyAllianceKinds pins the alliance penalty lines by type: a
// clan that left (1) or was dismissed from (2) an alliance may not join one,
// and one that dissolved its own (4) may not found one. A clan at war may
// not be dissolved.
func TestClanPenaltyAllianceKinds(t *testing.T) {
	t.Parallel()
	future := time.Date(2031, 3, 4, 12, 0, 0, 0, time.Local)
	ms := strconv.FormatInt(future.UnixMilli(), 10)
	for _, tc := range []struct {
		typ  string
		line string
	}{
		{"1", "Unable to join an alliance."},
		{"2", "Unable to join an alliance."},
		{"4", "Unable to create an alliance."},
	} {
		t.Run("type "+tc.typ, func(t *testing.T) {
			t.Parallel()
			alliancePenaltyRows(t, tc.typ, ms, tc.line)
		})
	}
}

// alliancePenaltyRows boots Knights under alliance penalty typ until ms and
// at war, and checks its report shows line, then the dissolution refusal.
func alliancePenaltyRows(t *testing.T, typ, ms, line string) {
	t.Helper()
	k := itoa(subunitClanID)
	_, c := bootUserCommandClan(t,
		`UPDATE clan_data SET ally_penalty_type = `+typ+`, ally_penalty_expiry_time = `+ms+` WHERE clan_id = `+k,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (20, 'Twenty', 3)`,
		`INSERT INTO clan_wars (clan1, clan2) VALUES ('`+k+`', '20')`,
	)
	want := "<tr><td width=170>" + line + "</td><td width=100 align=center>2031-03-04</td></tr>" +
		"<tr><td width=170>Unable to dissolve a clan.</td><td></td></tr>"
	if got := penaltyRows(t, userCommand(t, c, cmdClanPenalty)); got != want {
		t.Fatalf("type %s rows = %s, want %s", typ, got, want)
	}
}

// TestSiegeStatusCommand pins /siegestatus up to the noble check: a player
// leading no clan is told only a clan leader may ask. A clan leader's
// noble status is not modeled yet, so the leader is released with
// ActionFailed.
func TestSiegeStatusCommand(t *testing.T) {
	t.Parallel()
	srv, c := bootUserCommandClan(t)
	srv.SeedCharacterFor(t, "player2", "Drifter", 10, 0)
	drifter := srv.DialClient(t, "player2", 1)
	startInWorld(t, drifter)
	drainFrames(t, c)
	wantSysMsgs(t, "clanless /siegestatus", userCommand(t, drifter, cmdSiegeStatus), msg(serverpackets.SystemMessageOnlyClanLeaderCanIssueCommands))
	if got := opcodes(userCommand(t, c, cmdSiegeStatus)); !slices.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("clan leader /siegestatus = %x, want ActionFailed", got)
	}
}
