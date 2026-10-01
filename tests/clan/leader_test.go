package clan

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// pageText returns the NpcHtmlMessage among frames.
func pageText(t *testing.T, frames [][]byte) string {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("answer = %x, want a page", opcodes(frames))
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	return r.ReadString()
}

// command talks to the master again, so its page is the last one sent,
// and sends command.
func (w *clanWorld) command(t *testing.T, command string) [][]byte {
	t.Helper()
	w.talkToMaster(t)
	return w.masterCommand(t, command)
}

// TestLeaderNomination names the recruit the clan's next leader and takes
// the nomination back, each answered by its datapack page and stored. An
// unknown name, oneself and an offline member are refused.
func TestLeaderNomination(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	clanID := w.found(t, "Knights")
	w.recruit(t)

	frames := w.masterCommand(t, "change_clan_leader Nobody")
	if len(frames) == 0 {
		t.Fatal("unknown nominee answered nothing")
	}
	if id, params := sysMsg(t, frames[0]); id != serverpackets.SystemMessageS1DoesNotExist || !slices.Equal(params, []string{"Nobody"}) {
		t.Fatalf("unknown nominee = %d %v, want S1_DOES_NOT_EXIST Nobody", id, params)
	}
	if got := opcodes(w.masterCommand(t, "change_clan_leader founder")); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("self nomination = %x, want ActionFailed only", got)
	}

	if page := pageText(t, w.command(t, "change_clan_leader Recruit")); !strings.Contains(page, "has been submitted") || !strings.Contains(page, "is a success") {
		t.Fatalf("nomination page = %q, want 9000-07-success", page)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT new_leader_id FROM clan_data WHERE clan_id = ?`, clanID); got != int64(w.memberID) {
		t.Fatalf("stored nominee = %d, want %d", got, w.memberID)
	}
	if page := pageText(t, w.command(t, "change_clan_leader Recruit")); !strings.Contains(page, "on progress") {
		t.Fatalf("second nomination page = %q, want 9000-07-in-progress", page)
	}
	if page := pageText(t, w.command(t, "cancel_clan_leader_change")); !strings.Contains(page, "cancellation is a success") {
		t.Fatalf("cancel page = %q, want 9000-08-success", page)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT new_leader_id FROM clan_data WHERE clan_id = ?`, clanID); got != 0 {
		t.Fatalf("stored nominee after cancel = %d, want 0", got)
	}
	if page := pageText(t, w.command(t, "cancel_clan_leader_change")); !strings.Contains(page, "is a failure") {
		t.Fatalf("second cancel page = %q, want 9000-08-no", page)
	}

	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)
	if ids := messages(t, w.command(t, "change_clan_leader Recruit")); !slices.Equal(ids, []int{serverpackets.SystemMessageInvitedUserNotOnline}) {
		t.Fatalf("offline nominee = %v, want INVITED_USER_NOT_ONLINE", ids)
	}
}

// TestLoginUnderJoinPenalty reminds a character serving a clan join
// penalty, at the end of its login burst, that its membership ended.
func TestLoginUnderJoinPenalty(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)
	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	drainFrames(t, w.member)
	w.leaveWorld(t, w.member)

	burst := startInWorld(t, w.srv.DialClient(t, "player2", 1))
	n := len(burst)
	if n < 2 || burst[n-1][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("login burst = %x, want it to end with ActionFailed", opcodes(burst))
	}
	if id, _ := sysMsg(t, burst[n-2]); id != serverpackets.SystemMessageClanMembershipTerminated {
		t.Fatalf("login burst ends %x, want CLAN_MEMBERSHIP_TERMINATED before ActionFailed", opcodes(burst[n-2:]))
	}
}
