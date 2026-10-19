package quest

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/testsupport/scenario"
)

// preconditionProbeID is the NPC id the precondition probe answers the
// first talk of.
const preconditionProbeID = 30001

// preconditionProbe answers its NPC's first talk with a page naming what
// the scenario header set up: the talker's class, race and clan
// leadership, its Q001 state, and three script draws of [0, 100), so a
// scenario sees the header's effect through packets. It stands in for the
// proof quests that read these until they are ported.
func preconditionProbe() script.Script {
	return script.Script{
		Bind: script.Bindings{script.EventFirstTalk: {preconditionProbeID}},
		Hooks: script.Hooks{OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			q001 := "-"
			if st := s.QuestState(e.Player, "Q001_LettersOfLove"); st != nil {
				q001 = fmt.Sprintf("%s/%d", st.Status(), st.Cond())
			}
			return fmt.Sprintf("<html><body>class=%d race=%s leader=%t q001=%s draws=%d,%d,%d</body></html>",
				e.Player.ClassID(), e.Player.Race(), e.Player.IsClanLeader(), q001, s.Rnd(100), s.Rnd(100), s.Rnd(100))
		}},
	}
}

// TestScenarioPreconditions plays the scenarios of testdata/preconditions,
// whose headers set the class, the played subclass, the led clan, seeded
// quest rows, the script random draws, and what the character holds and
// stands for (format:
// package scenario).
func TestScenarioPreconditions(t *testing.T) {
	t.Parallel()
	scenario.RunDir(t, "testdata/preconditions", scenario.Config{Catalog: script.Catalog{
		"quest.Q001_LettersOfLove": quest001.New,
		"test.PreconditionProbe":   preconditionProbe,
		"test.HoldingsProbe":       holdingsProbe,
		"test.SubclassProbe":       subclassProbe,
	}})
}
