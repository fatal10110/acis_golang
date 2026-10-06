package clan

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The task clock starts at transferBoot, a Tuesday; the shipped
// ClanLeaderTransfer entry (WEEKLY, TUE 16:55:00) is next due at
// handoverDue.
var (
	transferBoot = time.Date(2026, 10, 6, 16, 50, 0, 0, time.UTC)
	handoverDue  = time.Date(2026, 10, 6, 16, 55, 0, 0, time.UTC)
)

// shippedTask returns the scripts.xml entry of path, as the datapack lists
// it, and a catalog holding only that task.
func shippedTask(t *testing.T, path string, ctor func() script.Script) gameservertest.Option {
	t.Helper()
	list, err := gamexml.LoadScriptList(datapack.Path(t, "data", "xml", "scripts.xml"), zerolog.Nop())
	if err != nil {
		t.Fatalf("load scripts.xml: %v", err)
	}
	i := slices.IndexFunc(list, func(l script.Listing) bool { return l.Path == path })
	if i < 0 {
		t.Fatalf("scripts.xml does not list %s", path)
	}
	return gameservertest.WithScripts(list[i:i+1], script.Catalog{path: ctor})
}

// bootLeaderTransfer boots the founder leading Knights, a level 5 clan
// holding Gludio Castle, wearing the Lord's Crown, with the recruit in the
// clan and both in the world beside a village master; the clan leader
// transfer task runs from transferBoot. It returns the world and the
// crown's object id.
func bootLeaderTransfer(t *testing.T, extra ...gameservertest.Option) (*clanWorld, int32) {
	t.Helper()
	return bootLeaderTransferWith(t, nil, extra...)
}

// bootLeaderTransferWith is bootLeaderTransfer with seed run on the
// founder before it enters the world.
func bootLeaderTransferWith(t *testing.T, seed func(srv *gameservertest.Server, leaderID int32), extra ...gameservertest.Option) (*clanWorld, int32) {
	t.Helper()
	opts := append(castleClanOptions(t),
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(clanPages(t)),
		knightsSeed(t, 5, 1),
		shippedTask(t, "task.ClanLeaderTransfer", scripttask.ClanLeaderTransfer),
		gameservertest.WithScheduledTasks(transferBoot),
	)
	srv := gameservertest.Boot(t, append(opts, extra...)...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	crown := srv.GiveItem(t, w.leaderID, lordsCrown, 1)
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE items SET loc = 'PAPERDOLL', loc_data = ? WHERE object_id = ?`, faceSlot, crown); err != nil {
		t.Fatalf("wear the crown: %v", err)
	}
	if seed != nil {
		seed(srv, w.leaderID)
	}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Recruit", 40, 0).ID
	w.member = srv.DialClient(t, "player2", 1)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.recruit(t)
	x, y, z := srv.PlayerPosition(t, w.leaderID)
	w.at = location.Location{X: x, Y: y, Z: z}
	w.master = srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("VillageMaster", masterID), location.Location{X: x + 30, Y: y, Z: z})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	if loc := itemLoc(t, w, crown); loc != "PAPERDOLL" {
		t.Fatalf("crown before the transfer = %s, want PAPERDOLL", loc)
	}
	return w, crown
}

// nominateRecruit has the founder name the recruit the next leader.
func nominateRecruit(t *testing.T, w *clanWorld) {
	t.Helper()
	if page := pageText(t, w.command(t, "change_clan_leader Recruit")); !strings.Contains(page, "is a success") {
		t.Fatalf("nomination page = %q, want 9000-07-success", page)
	}
	drainFrames(t, w.member)
}

// runTransfer moves the task clock to just before the transfer is due,
// checks nothing ran, then past it.
func runTransfer(t *testing.T, w *clanWorld) {
	t.Helper()
	clock := w.srv.ScheduleClock
	clock.Advance(handoverDue.Sub(clock.Now()) - time.Second)
	w.srv.Settle(t)
	if got := queryInt(t, w, `SELECT leader_id FROM clan_data WHERE clan_id = ?`, titleClanID); got != int64(w.leaderID) {
		t.Fatalf("leader before the transfer is due = %d, want the founder %d", got, w.leaderID)
	}
	clock.Advance(time.Second)
	w.srv.Settle(t)
}

// clanStatusTail is what every member in the world receives last: its clan
// window refreshed (cleared, the main clan's list, UserInfo), then
// CLAN_LEADER_PRIVILEGES_HAVE_BEEN_TRANSFERRED_TO_S1 naming the new leader.
var clanStatusTail = []byte{
	serverpackets.OpcodePledgeShowMemberListDelAll, serverpackets.OpcodePledgeShowMemberListAll,
	serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage,
}

// checkStatusTail checks frames end with clanStatusTail naming Recruit,
// and returns the frames before it.
func checkStatusTail(t *testing.T, who string, frames [][]byte) [][]byte {
	t.Helper()
	n := len(frames) - len(clanStatusTail)
	if n < 0 || string(opcodes(frames[n:])) != string(clanStatusTail) {
		t.Fatalf("%s frames = %x, want them to end with %x", who, opcodes(frames), clanStatusTail)
	}
	if id, params := sysMsg(t, frames[len(frames)-1]); id != serverpackets.SystemMessageClanLeaderPrivilegesTransferredToS1 || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("%s last message = %d %v, want CLAN_LEADER_PRIVILEGES_HAVE_BEEN_TRANSFERRED_TO_S1 Recruit", who, id, params)
	}
	return frames[:n]
}

// formerLeaderPart is what the former leader receives ahead of the status
// tail: its new rank (UserInfo), the crown it may no longer wear coming off
// (S1_DISARMED, UserInfo), then the new leader showing its rank (CharInfo,
// RelationChanged).
var formerLeaderPart = []byte{
	serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo,
	serverpackets.OpcodeCharInfo, serverpackets.OpcodeRelationChanged,
}

// newLeaderPart is what the new leader receives ahead of the status tail:
// the former leader's rank and then its unequip shown first (CharInfo,
// RelationChanged each), then its own new rank (UserInfo).
var newLeaderPart = []byte{
	serverpackets.OpcodeCharInfo, serverpackets.OpcodeRelationChanged,
	serverpackets.OpcodeCharInfo, serverpackets.OpcodeRelationChanged,
	serverpackets.OpcodeUserInfo,
}

// checkOpcodes checks frames carry exactly the opcodes want, in order.
func checkOpcodes(t *testing.T, who string, frames [][]byte, want []byte) {
	t.Helper()
	if got := opcodes(frames); !slices.Equal(got, want) {
		t.Fatalf("%s frames before the clan window = %x, want %x", who, got, want)
	}
}

// checkCrownDisarmed checks f is S1_DISARMED naming the Lord's Crown.
func checkCrownDisarmed(t *testing.T, f []byte) {
	t.Helper()
	if id, params := sysMsg(t, f); id != serverpackets.SystemMessageS1Disarmed || !slices.Equal(params, []string{itoa(lordsCrown)}) {
		t.Fatalf("former leader message = %d %v, want S1_DISARMED naming the Lord's Crown", id, params)
	}
}

// TestClanLeaderTransfer runs the weekly leader transfer on a clan whose
// leader nominated the recruit, on both executors. At the task's start,
// not before, the recruit leads: the clan row names it and clears the
// nomination, the power grades swap (0 for the new leader, 6 for the
// former), and each takes its new clan rank. The former leader first shows
// its new rank, then takes off the Lord's Crown it may no longer wear; the
// new leader then shows its rank; then both get their clan window
// refreshed and are told who leads.
func TestClanLeaderTransfer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, crown := bootLeaderTransfer(t, tc.opts...)
			nominateRecruit(t, w)
			runTransfer(t, w)

			founder := checkStatusTail(t, "former leader", drainFrames(t, w.leader))
			checkOpcodes(t, "former leader", founder, formerLeaderPart)
			checkCrownDisarmed(t, founder[1])
			recruit := checkStatusTail(t, "new leader", drainFrames(t, w.member))
			checkOpcodes(t, "new leader", recruit, newLeaderPart)

			if got := w.srv.PlayerPledgeClass(t, w.leaderID); got != 2 {
				t.Errorf("former leader's rank = %d, want 2 (a level 5 member)", got)
			}
			if got := w.srv.PlayerPledgeClass(t, w.memberID); got != 4 {
				t.Errorf("new leader's rank = %d, want 4 (a level 5 leader)", got)
			}
			if loc := itemLoc(t, w, crown); loc != "INVENTORY" {
				t.Errorf("crown after the transfer = %s, want INVENTORY", loc)
			}
			w.srv.FlushPersistence(t)
			if got := queryInt(t, w, `SELECT leader_id FROM clan_data WHERE clan_id = ?`, titleClanID); got != int64(w.memberID) {
				t.Errorf("stored leader = %d, want the recruit %d", got, w.memberID)
			}
			if got := queryInt(t, w, `SELECT new_leader_id FROM clan_data WHERE clan_id = ?`, titleClanID); got != 0 {
				t.Errorf("stored nominee = %d, want 0", got)
			}
			if got := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
				t.Errorf("new leader's stored grade = %d, want 0", got)
			}
			if got := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.leaderID); got != 6 {
				t.Errorf("former leader's stored grade = %d, want 6", got)
			}
		})
	}
}

// TestClanLeaderTransferDropsLeaverNomination: a nominee that left the clan
// before the transfer takes nothing; the nomination is cleared and stored,
// and nobody is told anything.
func TestClanLeaderTransferDropsLeaverNomination(t *testing.T) {
	t.Parallel()
	w, crown := bootLeaderTransfer(t)
	nominateRecruit(t, w)
	withdrawRecruit(t, w)
	drainFrames(t, w.leader)
	runTransfer(t, w)

	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("leader frames = %x, want none", opcodes(frames))
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT leader_id FROM clan_data WHERE clan_id = ?`, titleClanID); got != int64(w.leaderID) {
		t.Errorf("stored leader = %d, want the founder %d", got, w.leaderID)
	}
	if got := queryInt(t, w, `SELECT new_leader_id FROM clan_data WHERE clan_id = ?`, titleClanID); got != 0 {
		t.Errorf("stored nominee = %d, want 0", got)
	}
	if loc := itemLoc(t, w, crown); loc != "PAPERDOLL" {
		t.Errorf("crown = %s, want PAPERDOLL: the founder still leads", loc)
	}
}

// TestClanLeaderTransferWithFormerLeaderOffline: a former leader out of the
// world still loses the lead and its grade; only the new leader shows its
// rank, then its clan window is refreshed and it is told who leads. The
// offline former leader's equipment is not checked.
func TestClanLeaderTransferWithFormerLeaderOffline(t *testing.T) {
	t.Parallel()
	w, crown := bootLeaderTransfer(t)
	nominateRecruit(t, w)
	w.leaveWorld(t, w.leader)
	drainFrames(t, w.member)
	runTransfer(t, w)

	recruit := checkStatusTail(t, "new leader", drainFrames(t, w.member))
	if string(opcodes(recruit)) != string([]byte{serverpackets.OpcodeUserInfo}) {
		t.Fatalf("new leader frames before the clan window = %x, want its UserInfo alone", opcodes(recruit))
	}
	if got := w.srv.PlayerPledgeClass(t, w.memberID); got != 4 {
		t.Errorf("new leader's rank = %d, want 4 (a level 5 leader)", got)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT leader_id FROM clan_data WHERE clan_id = ?`, titleClanID); got != int64(w.memberID) {
		t.Errorf("stored leader = %d, want the recruit %d", got, w.memberID)
	}
	if got := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.leaderID); got != 6 {
		t.Errorf("former leader's stored grade = %d, want 6", got)
	}
	if loc := itemLoc(t, w, crown); loc != "PAPERDOLL" {
		t.Errorf("offline former leader's crown = %s, want PAPERDOLL", loc)
	}
}
