package clan

import (
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: VillageMaster.onBypassFeedback dissolve_clan
// (VillageMaster.java:160-213): not the clan leader -> 794; in an alliance
// -> CANNOT_DISPERSE_THE_CLANS_IN_ALLY (554); at war -> 264; owning a
// castle or clan hall -> 266; a dissolution still pending ->
// DISSOLUTION_IN_PROGRESS (263). DaysToPassToDissolveAClan (7) > 0 stores
// now + days in dissolving_expiry_time and sends PledgeShowInfoUpdate (its
// dissolution field 3) to the members, else ClanTable.destroyClan runs at
// once; either way the leader then takes a full death's experience loss.
// recover_clan (VillageMaster.java:282-299): not the leader -> 794; nothing
// pending -> NO_REQUESTS_TO_DISPERSE (267); else the expiry is cleared and
// the members get PledgeShowInfoUpdate. ClanTable.destroyClan
// (ClanTable.java:200-279): CLAN_HAS_DISPERSED (193) to the members, every
// member removed (each online one gets SkillList, UserInfo and
// PledgeShowMemberListDeleteAll), then the clan_data, clan_privs,
// clan_skills, clan_subpledges, clan_wars (either side) and siege_clans
// rows deleted; the clan warehouse's items are destroyed. A leader leaving
// takes the DaysBeforeCreateAClan creation penalty (Clan.java:716-720, 747).
// The boot reschedules a pending dissolution at least a minute out
// (ClanTable.java:95-96, 286-297).

// Dissolution system messages, as the reference numbers them.
const (
	msgClanHasDispersed       = 193
	msgDissolutionInProgress  = 263
	msgCannotDissolveAtWar    = 264
	msgCannotDissolveOwning   = 266
	msgNoRequestsToDisperse   = 267
	msgCannotDisperseInAlly   = 554
	msgNotAuthorizedToDoThat  = 794
	dissolveDays              = 7
	createDays                = 10
	dissolvedWarehouseItemID  = 57
	dissolvedWarehouseObject  = 0x7f300001
	dissolvedWarehouseObject2 = 0x7f300002
)

// dissolutionPages extends the alliance dialog pages with the clan pages
// linking dissolve_clan and recover_clan.
func dissolutionPages(t *testing.T) gameservertest.Option {
	t.Helper()
	pages := alliancePages(t)
	key := "villagemaster/" + strconv.Itoa(masterID) + ".htm"
	for _, name := range []string{"9000-04.htm", "9000-05.htm"} {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "feature", "Clan", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		pages[key] += string(data)
	}
	return gameservertest.WithHTMLPages(pages)
}

// squireInKnights and lonerInKnights make Squire and Loner ordinary members
// of Knights.
func squireInKnights() string {
	return `UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(allySquireID)
}

func lonerInKnights() string {
	return `UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(lonerID)
}

// headerDissolving reads the dissolution field of a PledgeShowInfoUpdate
// frame: 3 while a dissolution is pending, 0 otherwise.
func headerDissolving(t *testing.T, frame []byte) int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodePledgeShowInfoUpdate {
		t.Fatalf("opcode = %#x, want PledgeShowInfoUpdate", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 7 {
		r.ReadInt32()
	}
	return r.ReadInt32()
}

// wantHeader fails unless frames carry a PledgeShowInfoUpdate of Knights
// whose dissolution field is dissolving, and no system message.
func wantHeader(t *testing.T, who string, frames [][]byte, dissolving int32) {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate)
	if !ok {
		t.Fatalf("%s frames = %x, want PledgeShowInfoUpdate", who, opcodes(frames))
	}
	if got := headerDissolving(t, frame); got != dissolving {
		t.Fatalf("%s header dissolution field = %d, want %d", who, got, dissolving)
	}
	if ids := messages(t, frames); len(ids) != 0 {
		t.Fatalf("%s messages = %v, want none", who, ids)
	}
}

// TestDissolveClanRefusals refuses a dissolution in the reference's order
// of checks: each refusal answers its message alone and stores nothing.
func TestDissolveClanRefusals(t *testing.T) {
	future := strconv.FormatInt(time.Now().UnixMilli()+dayMs, 10)
	knights := ` WHERE clan_id = ` + itoa(knightsClanID)
	warOnRivals := `INSERT INTO clan_wars (clan1, clan2, expiry_time) VALUES (` + itoa(knightsClanID) + `, ` + itoa(rivalsClanID) + `, 0)`
	cases := []struct {
		name string
		seed []string
		want int
	}{
		{"alliance before war", []string{`UPDATE clan_data SET ally_id = ` + itoa(knightsClanID) + `, ally_name = 'Ally'` + knights, warOnRivals}, msgCannotDisperseInAlly},
		{"war before castle", []string{warOnRivals, `UPDATE clan_data SET hasCastle = 1` + knights}, msgCannotDissolveAtWar},
		{"castle", []string{`UPDATE clan_data SET hasCastle = 1` + knights}, msgCannotDissolveOwning},
		{"clan hall", []string{`INSERT INTO clanhall (id, ownerId) VALUES (22, ` + itoa(knightsClanID) + `)`}, msgCannotDissolveOwning},
		{"pending", []string{`UPDATE clan_data SET dissolving_expiry_time = ` + future + knights}, msgDissolutionInProgress},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := bootAllianceWorldSeeded(t, tc.seed, dissolutionPages(t))
			before := queryInt(t, w, `SELECT dissolving_expiry_time FROM clan_data`+knights)
			frames := w.masterCommandBy(t, w.leader, "dissolve_clan")
			if ids := messages(t, frames); !slices.Equal(ids, []int{tc.want}) {
				t.Fatalf("refusal messages = %v, want [%d]", ids, tc.want)
			}
			if _, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate); ok {
				t.Fatal("a refused dissolution refreshed the clan header")
			}
			w.srv.FlushPersistence(t)
			if got := queryInt(t, w, `SELECT dissolving_expiry_time FROM clan_data`+knights); got != before {
				t.Fatalf("dissolving_expiry_time = %d after a refusal, want %d", got, before)
			}
			w.leaveWorld(t, w.leader)
			if exp := queryInt(t, w, `SELECT exp FROM characters WHERE obj_Id = ?`, w.leaderID); exp != 60000 {
				t.Fatalf("refused leader's experience = %d, want 60000", exp)
			}
		})
	}
}

// TestDissolveClanSchedulesAndRecovers has the leader ask for the clan's
// dissolution: the expiry is stored seven days out, both members see the
// header marked, a second request and a member's request are refused, and
// the leader loses one death's experience. Recovery clears it and shows the
// header unmarked; a second recovery finds nothing to recover.
func TestDissolveClanSchedulesAndRecovers(t *testing.T) {
	w := bootAllianceCast(t, []castMember{allySquire}, []string{squireInKnights()}, dissolutionPages(t))
	squire := w.bringIn(t, allySquire)

	before := time.Now().UnixMilli()
	wantHeader(t, "leader", w.masterCommandBy(t, w.leader, "dissolve_clan"), 3)
	after := time.Now().UnixMilli()
	wantHeader(t, "member", drainFrames(t, squire), 3)
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodePledgeShowInfoUpdate); ok {
		t.Fatal("another clan's leader saw the dissolving clan's header")
	}
	w.srv.FlushPersistence(t)
	const window = dissolveDays * dayMs
	if got := queryInt(t, w, `SELECT dissolving_expiry_time FROM clan_data WHERE clan_id = ?`, knightsClanID); got < before+window || got > after+window {
		t.Fatalf("dissolving_expiry_time = %d, want 7 days after the request (%d..%d)", got, before+window, after+window)
	}

	if ids := messages(t, w.masterCommandBy(t, w.leader, "dissolve_clan")); !slices.Equal(ids, []int{msgDissolutionInProgress}) {
		t.Fatalf("second request messages = %v, want [%d]", ids, msgDissolutionInProgress)
	}
	if ids := messages(t, w.masterCommandBy(t, squire, "dissolve_clan")); !slices.Equal(ids, []int{msgNotAuthorizedToDoThat}) {
		t.Fatalf("member's request messages = %v, want [%d]", ids, msgNotAuthorizedToDoThat)
	}
	if ids := messages(t, w.masterCommandBy(t, squire, "recover_clan")); !slices.Equal(ids, []int{msgNotAuthorizedToDoThat}) {
		t.Fatalf("member's recovery messages = %v, want [%d]", ids, msgNotAuthorizedToDoThat)
	}

	wantHeader(t, "leader", w.masterCommandBy(t, w.leader, "recover_clan"), 0)
	wantHeader(t, "member", drainFrames(t, squire), 0)
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT dissolving_expiry_time FROM clan_data WHERE clan_id = ?`, knightsClanID); got != 0 {
		t.Fatalf("dissolving_expiry_time after recovery = %d, want 0", got)
	}
	if ids := messages(t, w.masterCommandBy(t, w.leader, "recover_clan")); !slices.Equal(ids, []int{msgNoRequestsToDisperse}) {
		t.Fatalf("second recovery messages = %v, want [%d]", ids, msgNoRequestsToDisperse)
	}
	if _, ok := w.srv.Clans.Table().Get(knightsClanID); !ok {
		t.Fatal("a recovered clan is gone")
	}

	// Level 10 spans 22972 experience; 8.875% of it is 2039, taken once.
	w.leaveWorld(t, w.leader)
	if exp := queryInt(t, w, `SELECT exp FROM characters WHERE obj_Id = ?`, w.leaderID); exp != 60000-2039 {
		t.Fatalf("leader's experience = %d, want %d", exp, 60000-2039)
	}
}

// seedDissolvedRows gives Knights a row in each table its destruction
// clears, two clan warehouse items, and a war Rivals declared on it.
func seedDissolvedRows() []string {
	id := itoa(knightsClanID)
	return []string{
		`INSERT INTO clan_privs (clan_id, ranking, privs) VALUES (` + id + `, 6, 8)`,
		`INSERT INTO clan_skills (clan_id, skill_id, skill_level) VALUES (` + id + `, 370, 1)`,
		`INSERT INTO clan_subpledges (clan_id, sub_pledge_id, name, leader_id) VALUES (` + id + `, -1, 'Pupils', 0)`,
		`INSERT INTO clan_wars (clan1, clan2, expiry_time) VALUES (` + itoa(rivalsClanID) + `, ` + id + `, 0)`,
		`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, ` + id + `, 'PENDING')`,
		`INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES
			(` + id + `, ` + itoa(dissolvedWarehouseObject) + `, ` + itoa(dissolvedWarehouseItemID) + `, 5000, 'CLANWH', 0),
			(` + id + `, ` + itoa(dissolvedWarehouseObject2) + `, ` + itoa(soulshotID) + `, 100, 'CLANWH', 0)`,
	}
}

// wantDissolvedRows fails unless every row of Knights is gone and none of
// its members belongs to a clan.
func wantDissolvedRows(t *testing.T, w *clanWorld, memberIDs ...int32) {
	t.Helper()
	w.srv.FlushPersistence(t)
	id := knightsClanID
	for query, args := range map[string][]any{
		`SELECT COUNT(*) FROM clan_data WHERE clan_id = ?`:            {id},
		`SELECT COUNT(*) FROM clan_privs WHERE clan_id = ?`:           {id},
		`SELECT COUNT(*) FROM clan_skills WHERE clan_id = ?`:          {id},
		`SELECT COUNT(*) FROM clan_subpledges WHERE clan_id = ?`:      {id},
		`SELECT COUNT(*) FROM clan_wars WHERE clan1 = ? OR clan2 = ?`: {id, id},
		`SELECT COUNT(*) FROM siege_clans WHERE clan_id = ?`:          {id},
		`SELECT COUNT(*) FROM items WHERE owner_id = ?`:               {id},
		`SELECT COUNT(*) FROM characters WHERE clanid = ?`:            {id},
		`SELECT COUNT(*) FROM items WHERE object_id IN (?, ?)`:        {dissolvedWarehouseObject, dissolvedWarehouseObject2},
	} {
		if got := queryInt(t, w, query, args...); got != 0 {
			t.Fatalf("%s = %d after the dissolution, want 0", query, got)
		}
	}
	for _, m := range memberIDs {
		if got := queryInt(t, w, `SELECT clanid FROM characters WHERE obj_Id = ?`, m); got != 0 {
			t.Fatalf("member %d's clanid = %d, want 0", m, got)
		}
	}
	if _, ok := w.srv.Clans.Table().Get(knightsClanID); ok {
		t.Fatal("the dissolved clan is still registered")
	}
	rivals, ok := w.srv.Clans.Table().Get(rivalsClanID)
	if !ok {
		t.Fatal("Rivals is gone")
	}
	if info := rivals.Info(); info.AtWar {
		t.Fatal("Rivals still counts its war on the dissolved clan")
	}
}

// wantDispersed fails unless frames tell a member the clan dispersed, then
// send its skill list and clear its clan tab, in that order.
func wantDispersed(t *testing.T, who string, frames [][]byte) {
	t.Helper()
	if ids := messages(t, frames); len(ids) == 0 || ids[0] != msgClanHasDispersed {
		t.Fatalf("%s messages = %v, want CLAN_HAS_DISPERSED first", who, ids)
	}
	ops := only(frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSkillList, serverpackets.OpcodePledgeShowMemberListDelAll)
	want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSkillList, serverpackets.OpcodePledgeShowMemberListDelAll}
	if len(ops) < len(want) || !slices.Equal(ops[:len(want)], want) {
		t.Fatalf("%s frames = %x, want CLAN_HAS_DISPERSED, SkillList, PledgeShowMemberListDeleteAll first", who, ops)
	}
}

// TestDissolveClanImmediately dissolves a clan with no delay configured:
// both members online learn the clan dispersed and lose their clan tab, the
// clan's rows, warehouse items and war go, every member is clanless, and
// the leader may not found a clan for ten days and loses one death's
// experience.
func TestDissolveClanImmediately(t *testing.T) {
	cfg := clan.DefaultConfig()
	cfg.DissolveDays = 0
	w := bootAllianceCast(t, []castMember{allySquire, loner},
		append([]string{squireInKnights(), lonerInKnights()}, seedDissolvedRows()...),
		dissolutionPages(t), gameservertest.WithClanConfig(cfg))
	squire := w.bringIn(t, allySquire)

	before := time.Now().UnixMilli()
	wantDispersed(t, "leader", w.masterCommandBy(t, w.leader, "dissolve_clan"))
	after := time.Now().UnixMilli()
	wantDispersed(t, "member", drainFrames(t, squire))

	wantDissolvedRows(t, w, w.leaderID, allySquireID, lonerID)
	w.leaveWorld(t, w.leader)
	const window = createDays * dayMs
	if got := queryInt(t, w, `SELECT clan_create_expiry_time FROM characters WHERE obj_Id = ?`, w.leaderID); got < before+window || got > after+window {
		t.Fatalf("leader's clan_create_expiry_time = %d, want 10 days after the dissolution (%d..%d)", got, before+window, after+window)
	}
	if got := queryInt(t, w, `SELECT clan_join_expiry_time FROM characters WHERE obj_Id = ?`, lonerID); got != 0 {
		t.Fatalf("offline member's clan_join_expiry_time = %d, want 0", got)
	}
	if exp := queryInt(t, w, `SELECT exp FROM characters WHERE obj_Id = ?`, w.leaderID); exp != 60000-2039 {
		t.Fatalf("leader's experience = %d, want %d", exp, 60000-2039)
	}
}

// TestDissolutionComesDueAfterBoot restores a clan whose dissolution ran
// out while the server was down: it is destroyed a minute after boot, not
// before, its member online told the clan dispersed.
func TestDissolutionComesDueAfterBoot(t *testing.T) {
	past := strconv.FormatInt(time.Now().UnixMilli()-dayMs, 10)
	w := bootAllianceCast(t, []castMember{allySquire},
		append([]string{squireInKnights(), `UPDATE clan_data SET dissolving_expiry_time = ` + past + ` WHERE clan_id = ` + itoa(knightsClanID)},
			seedDissolvedRows()...),
		dissolutionPages(t))
	if !w.srv.DrivesClock() {
		t.Skip("the minute after boot is waited out on the test clock")
	}
	squire := w.bringIn(t, allySquire)

	w.srv.Advance(t, 50*time.Second)
	if _, ok := w.srv.Clans.Table().Get(knightsClanID); !ok {
		t.Fatal("the clan was dissolved within a minute of boot")
	}
	w.srv.Advance(t, 10*time.Second)
	wantDispersed(t, "leader", drainFrames(t, w.leader))
	wantDispersed(t, "member", drainFrames(t, squire))
	wantDissolvedRows(t, w, w.leaderID, allySquireID)
}
