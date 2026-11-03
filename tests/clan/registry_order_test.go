package clan

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// registryFillers seeds n clans outside the alliance, ids from first up,
// dissolving at expiry (0 for none).
func registryFillers(first int32, n int, expiry int64) []string {
	stmts := make([]string, 0, n)
	for i := range int32(n) {
		id := itoa(first + i)
		stmts = append(stmts, `INSERT INTO clan_data (clan_id, clan_name, clan_level, dissolving_expiry_time)
			VALUES (`+id+`, 'Filler`+id+`', 1, `+strconv.FormatInt(expiry, 10)+`)`)
	}
	return stmts
}

// allyInfoClans has c ask for its alliance's window and returns the clans
// the AllianceInfo packet lists, checking the clan-name system messages
// after it list them alike.
func allyInfoClans(t *testing.T, c *testsupport.ScriptedClient) []string {
	t.Helper()
	c.Send(encodeAllyBare(clientpackets.OpcodeRequestAllyInfo))
	info := drainFrames(t, c)
	if len(info) == 0 || info[0][0] != serverpackets.OpcodeAllianceInfo {
		t.Fatalf("alliance info = %x, want AllianceInfo first", opcodes(info))
	}
	r := wire.NewReader(info[0][1:])
	r.ReadString()
	r.ReadInt32()
	r.ReadInt32()
	r.ReadString()
	r.ReadString()
	var listed []string
	for range int(r.ReadInt32()) {
		listed = append(listed, r.ReadString())
		r.ReadInt32()
		r.ReadInt32()
		r.ReadString()
		r.ReadInt32()
		r.ReadInt32()
	}
	var named []string
	for _, f := range info[1:] {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id, params := sysMsg(t, f); id == allyMsgClanName {
			named = append(named, params...)
		}
	}
	if !slices.Equal(named, listed) {
		t.Fatalf("alliance info clan lines = %v, AllianceInfo lists %v", named, listed)
	}
	return listed
}

// TestAllianceInfoKeepsGrownRegistryOrder restores 13 clans, enough to
// double the registry's hash table to 32 buckets, then lets the 10 whose
// dissolution ran out while the server was down dissolve. The table keeps
// its 32 buckets: Ash (0x01020007, bucket 5 either way) still lists ahead
// of Knights and Rivals (buckets 17 and 18 of 32, but 1 and 2 of 16).
func TestAllianceInfoKeepsGrownRegistryOrder(t *testing.T) {
	ally := itoa(knightsClanID)
	past := time.Now().UnixMilli() - dayMs
	w := bootAllianceWorldSeeded(t, append([]string{
		`UPDATE clan_data SET ally_id = ` + ally + `, ally_name = 'Ally' WHERE clan_id IN (` + ally + `, ` + itoa(rivalsClanID) + `)`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, ally_id, ally_name) VALUES
			(` + itoa(int32(0x01020007)) + `, 'Ash', 3, ` + ally + `, 'Ally')`,
	}, registryFillers(0x01100000, 10, past)...))
	if !w.srv.DrivesClock() {
		t.Skip("the minute after boot is waited out on the test clock")
	}
	want := []string{"Ash", "Knights", "Rivals"}
	if got := allyInfoClans(t, w.member); !slices.Equal(got, want) {
		t.Fatalf("before the dissolutions AllianceInfo clans = %v, want %v", got, want)
	}

	w.srv.Advance(t, time.Minute)
	if n := w.srv.Clans.Table().Len(); n != 3 {
		t.Fatalf("clans left = %d, want 3", n)
	}
	drainFrames(t, w.member)
	if got := allyInfoClans(t, w.member); !slices.Equal(got, want) {
		t.Fatalf("after the dissolutions AllianceInfo clans = %v, want %v", got, want)
	}
}

// TestAllianceInfoFollowsRegistryResize restores 11 clans, three of them
// sharing bucket 3 of the registry's 16: Ash (0x01020001) and Birch
// (0x01020021), both in Knights' alliance, then Cedar (0x01020031). Loner
// then founds the twelfth clan, which doubles the table: Cedar moves to
// bucket 19 and the two left in bucket 3 swap places, Birch now listing
// ahead of the lower id Ash, both ahead of Knights and Rivals (buckets 17
// and 18).
func TestAllianceInfoFollowsRegistryResize(t *testing.T) {
	ally := itoa(knightsClanID)
	w := bootAllianceCast(t, []castMember{loner}, append([]string{
		`UPDATE clan_data SET ally_id = ` + ally + `, ally_name = 'Ally' WHERE clan_id IN (` + ally + `, ` + itoa(rivalsClanID) + `)`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, ally_id, ally_name) VALUES
			(` + itoa(int32(0x01020001)) + `, 'Ash', 3, ` + ally + `, 'Ally'),
			(` + itoa(int32(0x01020021)) + `, 'Birch', 3, ` + ally + `, 'Ally'),
			(` + itoa(int32(0x01020031)) + `, 'Cedar', 3, 0, '')`,
	}, registryFillers(0x01100004, 6, 0)...))
	if got, want := allyInfoClans(t, w.member), []string{"Knights", "Rivals", "Ash", "Birch"}; !slices.Equal(got, want) {
		t.Fatalf("with 11 clans AllianceInfo clans = %v, want %v", got, want)
	}

	lone := w.bringIn(t, loner)
	if _, ok := firstOpcode(w.masterCommandBy(t, lone, "create_clan Lone"), serverpackets.OpcodePledgeShowMemberListAll); !ok {
		t.Fatal("Loner founded no clan")
	}
	drainFrames(t, w.member)
	if got, want := allyInfoClans(t, w.member), []string{"Birch", "Ash", "Knights", "Rivals"}; !slices.Equal(got, want) {
		t.Fatalf("with 12 clans AllianceInfo clans = %v, want %v", got, want)
	}
}

// TestRosterListsJoinerAfterRestoredMember has Loner (0x7f200003) join
// Knights, whose restored member Page (0x7f200013) shares bucket 3 of the
// roster's 16: the roster lists Page, there first, ahead of the lower id
// Loner.
func TestRosterListsJoinerAfterRestoredMember(t *testing.T) {
	const pageID int32 = 0x7f200013
	w := bootAllianceCast(t, []castMember{loner}, []string{
		`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade)
			VALUES ('filler', ` + itoa(pageID) + `, 'Page', 40, ` + itoa(knightsClanID) + `, 6)`,
	})
	lone := w.bringIn(t, loner)
	w.leader.Send(encodeRequestJoinPledge(lonerID, 0))
	drainFrames(t, lone)
	lone.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, lone), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("Loner did not join")
	}
	drainFrames(t, w.leader)

	w.leader.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeMemberList).Bytes())
	list, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatal("roster request answered no PledgeShowMemberListAll")
	}
	_, _, _, members := unitList(t, list)
	if ip, il := slices.Index(members, "Page"), slices.Index(members, "Loner"); ip < 0 || il < 0 || il < ip {
		t.Fatalf("roster = %q, want Page before Loner", members)
	}
}
