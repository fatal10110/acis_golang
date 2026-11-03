package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: EnterWorld (EnterWorld.java:124-136), after telling the other
// online members CLAN_MEMBER_S1_LOGGED_IN with the player's roster row and
// before the clan hall lease notice, sends the player's sponsor, when it
// has one in the world, YOUR_APPRENTICE_S1_HAS_LOGGED_IN (1756); else, when
// the player has an apprentice in the world, the apprentice
// YOUR_SPONSOR_S1_HAS_LOGGED_IN (1758), both naming the player.

// wantLoginNotices fails unless frames carry, among their system messages
// and roster rows, exactly CLAN_MEMBER_S1_LOGGED_IN, the roster row and
// then mentorNotice (none when 0), each message naming name.
func wantLoginNotices(t *testing.T, who string, frames [][]byte, name string, mentorNotice int) {
	t.Helper()
	want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListUpdate}
	wantIDs := []int{serverpackets.SystemMessageClanMemberS1LoggedIn}
	if mentorNotice != 0 {
		want = append(want, serverpackets.OpcodeSystemMessage)
		wantIDs = append(wantIDs, mentorNotice)
	}
	if got := only(frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListUpdate); !slices.Equal(got, want) {
		t.Fatalf("%s's view of the login = %x, want %x", who, opcodes(frames), want)
	}
	var ids []int
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		id, params := sysMsg(t, f)
		ids = append(ids, id)
		if !slices.Equal(params, []string{name}) {
			t.Fatalf("%s's message %d names %v, want %s", who, id, params, name)
		}
	}
	if !slices.Equal(ids, wantIDs) {
		t.Fatalf("%s's messages = %v, want %v", who, ids, wantIDs)
	}
}

// TestMentorLoginNotices links the academy recruit to the founder as its
// sponsor. The recruit logging in tells the founder its apprentice logged
// in, right after the clan's login notice and row; the founder logging in
// tells the recruit its sponsor logged in. With the founder offline the
// recruit's login tells nobody, its own burst included.
func TestMentorLoginNotices(t *testing.T) {
	t.Parallel()
	w, _ := bootSubunitWorld(t, 1, seedSubunitClan(t, academySeed))
	w.leader.Send(encodeRequestJoinPledge(w.memberID, -1))
	drainFrames(t, w.member)
	w.member.Send(encodeRequestAnswerJoinPledge(1))
	drainFrames(t, w.member)
	drainFrames(t, w.leader)
	w.leader.Send(encodeRequestPledgeSetAcademyMaster(1, "Recruit", "Founder"))
	drainFrames(t, w.leader)
	drainFrames(t, w.member)

	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)
	w.member = w.srv.DialClient(t, "player2", 1)
	burst := startInWorld(t, w.member)
	wantLoginNotices(t, "sponsor", drainFrames(t, w.leader), "Recruit", serverpackets.SystemMessageYourApprenticeS1HasLoggedIn)
	wantNoMentorNotice(t, "apprentice's own burst", burst)

	w.leaveWorld(t, w.leader)
	drainFrames(t, w.member)
	w.leader = w.srv.DialClient(t, "player1", 1)
	burst = startInWorld(t, w.leader)
	wantLoginNotices(t, "apprentice", drainFrames(t, w.member), "Founder", serverpackets.SystemMessageYourSponsorS1HasLoggedIn)
	wantNoMentorNotice(t, "sponsor's own burst", burst)

	w.leaveWorld(t, w.leader)
	w.leaveWorld(t, w.member)
	w.member = w.srv.DialClient(t, "player2", 1)
	burst = startInWorld(t, w.member)
	wantNoMentorNotice(t, "apprentice's burst with its sponsor offline", burst)
}

// wantNoMentorNotice fails if frames carry a sponsor or apprentice login
// notice.
func wantNoMentorNotice(t *testing.T, what string, frames [][]byte) {
	t.Helper()
	for _, id := range messages(t, frames) {
		if id == serverpackets.SystemMessageYourApprenticeS1HasLoggedIn || id == serverpackets.SystemMessageYourSponsorS1HasLoggedIn {
			t.Fatalf("%s carries mentor notice %d", what, id)
		}
	}
}
