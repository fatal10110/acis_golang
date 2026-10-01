package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRaiseClanLevel raises a level 0 clan to level 1 for 30000 SP and
// 650000 adena: the adena and SP go with their notices, the clan learns
// its new level, and the leader shows the level-up visual. The next level
// costs more SP than the leader has and fails.
func TestRaiseClanLevel(t *testing.T) {
	w := bootClanWorld(t, 10, 30000, 650000)
	clanID := w.found(t, "Knights")
	w.recruit(t)

	frames := w.masterCommand(t, "increase_clan_level")
	if ids := messages(t, frames); !slices.Equal(ids, []int{
		serverpackets.SystemMessageS1DisappearedAdena, serverpackets.SystemMessageSPDecreasedS1, serverpackets.SystemMessageClanLevelIncreased,
	}) {
		t.Fatalf("level-up messages = %v (%x)", ids, opcodes(frames))
	}
	if got := only(frames, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeActionFailed); string(got) != string([]byte{
		serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeActionFailed,
	}) {
		t.Fatalf("level-up answer = %x", opcodes(frames))
	}
	member := drainFrames(t, w.member)
	if got := only(member, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage); string(got) != string([]byte{serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("member's view of the level-up = %x", opcodes(member))
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT clan_level FROM clan_data WHERE clan_id = ?`, clanID); got != 1 {
		t.Fatalf("stored clan level = %d, want 1", got)
	}

	frames = w.masterCommand(t, "increase_clan_level")
	if ids := messages(t, frames); !slices.Equal(ids, []int{serverpackets.SystemMessageFailedToIncreaseClanLevel}) {
		t.Fatalf("unaffordable level-up = %v, want FAILED_TO_INCREASE_CLAN_LEVEL", ids)
	}
}
