package gameservertest

import (
	"testing"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// InstallNPCCastSeam wires the live NPC h's AI-cast seam over defs, as
// SpawnCastingHostileNPC does for a fixture monster: the WithNpcSpawns
// population loads no skill data, so its NPCs have nothing to cast until a
// suite gives one its skills. The seam is installed on h's own queue.
func (s *Server) InstallNPCCastSeam(t *testing.T, h *npc.Hostile, defs actorcast.Definitions) {
	t.Helper()
	done := make(chan struct{})
	if !h.Queue().Post(func() {
		defer close(done)
		s.installCastSeam(h, defs)
	}) {
		t.Fatalf("npc %d queue is closed", h.ObjectID())
	}
	<-done
}
