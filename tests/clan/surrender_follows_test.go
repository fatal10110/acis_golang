package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPersonalSurrenderFollowsLeaverIntoNextClan has Sentry surrender
// personally to Rivals, the war staying on with Knights' Drifter offline,
// then withdraw from Knights in the world and join
// Rivals, at mutual war with Knights (no join penalty is configured). Its
// flag stays with it (Clan.removeClanMember leaves it alone): its own
// surrender to Knights is refused
// (RequestSurrenderPersonally: FAILED_TO_PERSONALLY_SURRENDER), and it is
// counted when the Rivals leader surrenders, so two of two members want
// peace instead of exactly one less than the roster and the war stays on
// (ClanTable.checkSurrender). The flag is stored as it logs out.
func TestPersonalSurrenderFollowsLeaverIntoNextClan(t *testing.T) {
	t.Parallel()
	cfg := clan.DefaultConfig()
	cfg.JoinDays = 0
	w := bootAllianceCast(t, []castMember{{sentryID, "player4", "Sentry"}, {drifterID, "player5", "Drifter"}}, []string{
		`UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id IN (` + itoa(sentryID) + `, ` + itoa(drifterID) + `)`,
		warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID),
	}, gameservertest.WithClanConfig(cfg))
	sentry := w.srv.DialClient(t, "player4", 1)
	startInWorld(t, sentry)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)

	wantMessages(t, "Sentry's surrender", surrenderPersonally(t, sentry, "Rivals"),
		serverpackets.SystemMessageYouHavePersonallySurrenderedToS1Clan)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)

	sentry.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	drainFrames(t, sentry)
	drainFrames(t, w.leader)

	w.member.Send(encodeRequestJoinPledge(sentryID, 0))
	drainFrames(t, w.member)
	drainFrames(t, sentry)
	sentry.Send(encodeRequestAnswerJoinPledge(1))
	if joined := drainFrames(t, sentry); !hasOpcode(joined, serverpackets.OpcodeJoinPledge) {
		t.Fatalf("Sentry's join = %x, want JoinPledge", opcodes(joined))
	}
	drainFrames(t, w.member)
	drainFrames(t, w.leader)

	wantMessages(t, "Sentry's surrender from Rivals", surrenderPersonally(t, sentry, "Knights"),
		serverpackets.SystemMessageFailedToPersonallySurrender)

	rival := surrenderPersonally(t, w.member, "Knights")
	wantMessages(t, "Rivals leader's surrender", rival, serverpackets.SystemMessageYouHavePersonallySurrenderedToS1Clan)
	if hasOpcode(rival, serverpackets.OpcodePledgeShowInfoUpdate) {
		t.Fatal("war ended though Sentry's carried flag makes both Rivals members want peace")
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_wars WHERE expiry_time = 0`); got != 2 {
		t.Fatalf("stored wars = %d, want both still on", got)
	}
	w.leaveWorld(t, sentry)
	if got := queryInt(t, w, `SELECT wantspeace FROM characters WHERE obj_Id = ?`, sentryID); got != 1 {
		t.Fatalf("Sentry's stored wantspeace = %d, want 1", got)
	}
}

func hasOpcode(frames [][]byte, opcode byte) bool {
	_, ok := firstOpcode(frames, opcode)
	return ok
}
