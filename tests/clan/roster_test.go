package clan

import (
	"bytes"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// memberRow decodes a PledgeShowMemberListUpdate frame.
func memberRow(t *testing.T, frame []byte) (name string, level, online int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodePledgeShowMemberListUpdate {
		t.Fatalf("opcode = %#x, want PledgeShowMemberListUpdate", frame[0])
	}
	r := wire.NewReader(frame[1:])
	name, level = r.ReadString(), r.ReadInt32()
	r.ReadInt32() // class
	r.ReadInt32() // sex
	r.ReadInt32() // race
	return name, level, r.ReadInt32()
}

// TestMemberLoginAndLogout logs the recruit out and back in. Its fellow
// member sees its row go offline, then on login hears it logged in and
// sees its row online again. The login burst carries the clan's skill
// list, the member's own row and the roster right after EtcStatusUpdate,
// ahead of the world spawn's welcome.
func TestMemberLoginAndLogout(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)

	w.leaveWorld(t, w.member)
	frame, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodePledgeShowMemberListUpdate)
	if !ok {
		t.Fatal("leader saw no row update on the recruit's logout")
	}
	if name, _, online := memberRow(t, frame); name != "Recruit" || online != 0 {
		t.Fatalf("logout row = %q online %d, want Recruit offline", name, online)
	}

	c := w.srv.DialClient(t, "player2", 1)
	burst := startInWorld(t, c)
	order := opcodes(burst)
	etc := bytes.IndexByte(order, serverpackets.OpcodeEtcStatusUpdate)
	if etc < 0 || etc+3 >= len(order) || !isPledgeSkillList(burst[etc+1]) ||
		order[etc+2] != serverpackets.OpcodePledgeShowMemberListUpdate || order[etc+3] != serverpackets.OpcodePledgeShowMemberListAll {
		t.Fatalf("login burst = %x, want PledgeSkillList, PledgeShowMemberListUpdate, PledgeShowMemberListAll right after EtcStatusUpdate", order)
	}
	if name, _, online := memberRow(t, burst[etc+2]); name != "Recruit" || online != w.memberID {
		t.Fatalf("own login row = %q online %d, want Recruit online %d", name, online, w.memberID)
	}
	welcome := -1
	for i, f := range burst {
		if f[0] == serverpackets.OpcodeSystemMessage {
			if id, _ := sysMsg(t, f); id == serverpackets.SystemMessageWelcomeToLineage {
				welcome = i
				break
			}
		}
	}
	if welcome < etc+3 {
		t.Fatalf("WELCOME_TO_LINEAGE at %d, want after the clan block at %d", welcome, etc+3)
	}

	leader := drainFrames(t, w.leader)
	if got := only(leader, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListUpdate); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListUpdate}) {
		t.Fatalf("leader's view of the login = %x", opcodes(leader))
	}
	frame, _ = firstOpcode(leader, serverpackets.OpcodeSystemMessage)
	if id, params := sysMsg(t, frame); id != serverpackets.SystemMessageClanMemberS1LoggedIn || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("login message = %d %v, want CLAN_MEMBER_S1_LOGGED_IN Recruit", id, params)
	}
	frame, _ = firstOpcode(leader, serverpackets.OpcodePledgeShowMemberListUpdate)
	if _, _, online := memberRow(t, frame); online != w.memberID {
		t.Fatalf("login row online = %d, want %d", online, w.memberID)
	}
}

// TestLevelChangeReachesRoster raises the recruit a level: every member
// online, the recruit included, gets its row with the new level.
func TestLevelChangeReachesRoster(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	w.found(t, "Knights")
	w.recruit(t)

	w.srv.AddPlayerLevel(t, w.memberID, 4)
	for _, who := range []struct {
		name   string
		frames [][]byte
	}{{"leader", drainFrames(t, w.leader)}, {"recruit", drainFrames(t, w.member)}} {
		frame, ok := firstOpcode(who.frames, serverpackets.OpcodePledgeShowMemberListUpdate)
		if !ok {
			t.Fatalf("%s got no row update on the level change: %x", who.name, opcodes(who.frames))
		}
		if name, level, _ := memberRow(t, frame); name != "Recruit" || level != 5 {
			t.Fatalf("%s's row = %q level %d, want Recruit level 5", who.name, name, level)
		}
	}
}

// TestPledgeInfoAndMemberList answers any player's name-card request for a
// clan, and a member's roster request.
func TestPledgeInfoAndMemberList(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	clanID := w.found(t, "Knights")

	w.member.Send(func() []byte {
		p := wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeInfo)
		p.WriteInt32(clanID)
		return p.Bytes()
	}())
	frames := drainFrames(t, w.member)
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodePledgeInfo, serverpackets.OpcodePledgeStatusChanged}) {
		t.Fatalf("name card = %x, want PledgeInfo, PledgeStatusChanged", got)
	}
	r := wire.NewReader(frames[0][1:])
	if id, name := r.ReadInt32(), r.ReadString(); id != clanID || name != "Knights" {
		t.Fatalf("PledgeInfo = %d %q, want %d Knights", id, name, clanID)
	}
	r = wire.NewReader(frames[1][1:])
	if leader, id := r.ReadInt32(), r.ReadInt32(); leader != w.leaderID || id != clanID {
		t.Fatalf("PledgeStatusChanged = leader %d clan %d, want %d %d", leader, id, w.leaderID, clanID)
	}

	w.leader.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeMemberList).Bytes())
	if got := opcodes(drainFrames(t, w.leader)); string(got) != string([]byte{serverpackets.OpcodePledgeShowMemberListAll}) {
		t.Fatalf("roster request = %x, want PledgeShowMemberListAll", got)
	}
	// A clanless player's roster request and an unknown clan's card answer
	// nothing.
	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeMemberList).Bytes())
	w.member.Send(func() []byte {
		p := wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeInfo)
		p.WriteInt32(clanID + 1000)
		return p.Bytes()
	}())
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("clanless roster and unknown card = %x, want nothing", opcodes(frames))
	}
}

// TestClanSurvivesRestart stores a clan and its two members, restarts the
// server on the same database, and logs the leader in: the roster carries
// both members, the recruit offline at its stored level. Both characters
// are refused deletion while in the clan, the leader as its leader.
func TestClanSurvivesRestart(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	clanID := w.found(t, "Knights")
	w.recruit(t)
	w.leaveWorld(t, w.member)
	w.leaveWorld(t, w.leader)
	w.srv.Shutdown(t)

	srv := gameservertest.Boot(t, gameservertest.WithWantChars(1), gameservertest.WithReuseDelays(0, 0))
	assertDeleteRefused(t, srv.Client, serverpackets.CharDeleteFailReasonClanLeaderMayNotDelete)
	assertDeleteRefused(t, srv.DialClient(t, "player2", 1), serverpackets.CharDeleteFailReasonClanMemberMayNotDelete)

	burst := startInWorld(t, srv.Client)
	list, ok := firstOpcode(burst, serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatalf("leader's login after restart = %x, want its roster", opcodes(burst))
	}
	r := wire.NewReader(list[1:])
	r.ReadInt32()
	if id := r.ReadInt32(); id != clanID {
		t.Fatalf("restored clan = %d, want %d", id, clanID)
	}
	r.ReadInt32()
	if name, leader := r.ReadString(), r.ReadString(); name != "Knights" || leader != "Founder" {
		t.Fatalf("restored header = %q led by %q", name, leader)
	}
	for range 9 {
		r.ReadInt32()
	}
	r.ReadString()
	r.ReadInt32()
	r.ReadInt32()
	if count := r.ReadInt32(); count != 2 {
		t.Fatalf("restored roster = %d members, want 2", count)
	}
	online := map[string]int32{}
	for range 2 {
		name := r.ReadString()
		r.ReadInt32()
		r.ReadInt32()
		r.ReadInt32()
		r.ReadInt32()
		online[name] = r.ReadInt32()
		r.ReadInt32()
	}
	if online["Founder"] != w.leaderID || online["Recruit"] != 0 {
		t.Fatalf("restored rows online = %v, want Founder online, Recruit offline", online)
	}
}

// assertDeleteRefused asks to delete c's first character and checks the
// refusal reason, then the refreshed character list.
func assertDeleteRefused(t *testing.T, c *testsupport.ScriptedClient, want serverpackets.CharDeleteFailReason) {
	t.Helper()
	p := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterDelete)
	p.WriteInt32(0)
	c.Send(p.Bytes())
	frame := c.Read()
	if reason := wire.NewReader(frame[1:]).ReadInt32(); frame[0] != serverpackets.OpcodeCharDeleteFail || reason != int32(want) {
		t.Fatalf("deletion = %#x reason %d, want CharDeleteFail %d", frame[0], reason, want)
	}
	if frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("after the refusal = %#x, want CharSelectInfo", frame[0])
	}
}
