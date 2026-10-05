package skills

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLearnBadPassiveCompletesTheLearn pins #2370. A passive whose stat
// functions fail to build used to answer NOTHING_HAPPENED with no SkillList,
// although the skill was already known and paid for. The reference cannot
// fail there (RequestAcquireSkill: removeExpAndSp, addSkill, LEARNED_SKILL,
// SkillList), so the learn completes with the full reply and the bad
// definition is only logged.
func TestLearnBadPassiveCompletesTheLearn(t *testing.T) {
	t.Parallel()
	t.Run("general", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t,
			gameservertest.WithCharacter("Newbie", 5, 50),
			gameservertest.WithWantChars(1),
			gameservertest.WithCapturedLog(),
			gameservertest.WithSkills(learnerTable(t, brokenPassive(3))))
		startInWorld(t, c)
		selectTrainer(t, srv, c, objID, 0)

		c.Send(encodeRequestAcquireSkill(3, 1, 0))
		assertSPStatus(t, c.Read(), objID, 0)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSystemMessage, "SP-decreased SystemMessage")
		assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 3, 1)
		assertSkillList(t, c.Read(), skillListEntry{passive: 1, level: 1, id: 3})
		assertEmptyListClose(t, c, serverpackets.SystemMessageNoMoreSkillsToLearn)
		drainUntilQuiet(t, c)
		assertKnownSkills(t, srv, objID, map[int]int{3: 1})
		assertBadPassiveLogged(t, srv)
	})
	t.Run("fishing", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t,
			gameservertest.WithCharacter("Newbie", 5, 50),
			gameservertest.WithWantChars(1),
			gameservertest.WithCapturedLog(),
			gameservertest.WithSkills(learnerTable(t, brokenPassive(1368))),
			gameservertest.WithSkillTrees(fishingTrees()))
		srv.GiveItem(t, objID, 57, 5)
		startInWorld(t, c)
		selectTrainer(t, srv, c, objID)

		c.Send(encodeRequestAcquireSkill(1368, 1, 1))
		assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 1368, 1)
		reply := c.Read()
		assertFrameOpcode(t, reply, serverpackets.OpcodeExtended, "ExStorageMaxCount extended")
		if sub := wireReader(reply[1:]).ReadUint16(); sub != serverpackets.OpcodeExStorageMaxCount {
			t.Fatalf("extended opcode = %#x, want ExStorageMaxCount (%#x)", sub, serverpackets.OpcodeExStorageMaxCount)
		}
		assertSkillList(t, c.Read(), skillListEntry{passive: 1, level: 1, id: 1368})
		assertEmptyListClose(t, c, serverpackets.SystemMessageNoMoreSkillsToLearn)
		drainUntilQuiet(t, c)
		assertKnownSkills(t, srv, objID, map[int]int{1368: 1})
		assertBadPassiveLogged(t, srv)
	})
}

func assertBadPassiveLogged(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	if text := srv.LogText(); !strings.Contains(text, "bad passive definition") {
		t.Fatalf("log = %q, want the bad passive definition logged", text)
	}
}
