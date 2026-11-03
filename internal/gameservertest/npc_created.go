package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// RunCreated runs the created hooks of the scripts bound to the NPC n, as
// the spawn pass runs them once it has placed an NPC: for an NPC a fixture
// spawner placed (SpawnFolkNPCAt, SpawnCastingFolkNPCAt,
// SpawnHostileNPCTemplateAt), which runs none.
func (s *Server) RunCreated(tb testing.TB, n attackable.Combatant) {
	tb.Helper()
	switch o := n.(type) {
	case *npc.Hostile:
		s.quests.registry.HostileCreated(o)
	case *npc.Folk:
		s.quests.registry.FolkCreated(o)
	default:
		tb.Fatalf("RunCreated: %T is not an NPC", n)
	}
}

// ScriptEvent sends the NPC n the script event eventID with arg1 and arg2,
// as a script's SendScriptEvent does: the hooks of the scripts bound to its
// template run on the calling goroutine, so a caller that wants them on the
// NPC's queue posts the call there. WithScriptEventsHeard is not told of
// this send, as it is the suite's own.
func (s *Server) ScriptEvent(n attackable.Combatant, eventID, arg1, arg2 int32) {
	s.quests.registry.DeliverScriptEvent(n, eventID, arg1, arg2)
}
