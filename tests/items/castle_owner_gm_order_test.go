package items

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAdminCastleSetGameMasterInClanOrder has a game master who leads the
// clan give its own clan Gludio Castle with //castle set: it sees the clan
// header carry the castle and hears the siege victory music before the
// castle page the command opens.
func TestAdminCastleSetGameMasterInClanOrder(t *testing.T) {
	t.Parallel()
	w := bootCastleWorld(t, 0, func(db *sql.DB) {
		if _, err := db.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE char_name = 'Newbie'"); err != nil {
			t.Fatalf("set access level: %v", err)
		}
	})
	enterAsClanMember(w.leader)
	drainUntilQuiet(t, w.leader)
	w.leader.Send(encodeAction(w.leaderID, 0, 0, 0, false))
	drainUntilQuiet(t, w.leader)

	cmd := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	cmd.WriteString("castle set gludio_castle")
	w.leader.Send(cmd.Bytes())
	frames := collectFrames(w.leader)

	castle, header := pledgeCastle(t, frames)
	if castle != 1 {
		t.Fatalf("PledgeShowInfoUpdate castle = %d, want 1", castle)
	}
	want := []byte{serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodePlaySound, serverpackets.OpcodeNpcHtmlMessage}
	if len(frames) != header+len(want) {
		t.Fatalf("opcodes %x; want them to end with %x", opcodesOf(frames), want)
	}
	for i, op := range want {
		if frames[header+i][0] != op {
			t.Fatalf("opcodes %x; want them to end with %x", opcodesOf(frames), want)
		}
	}
}
