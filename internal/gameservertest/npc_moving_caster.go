package gameservertest

import (
	"testing"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// SpawnMovingCastingHostileNPC is SpawnMovingHostileNPCTemplate with the
// production AI-cast seam of SpawnCastingHostileNPC installed over defs, so
// a suite can watch whether a cast intention walks toward its target.
func (s *Server) SpawnMovingCastingHostileNPC(t *testing.T, tmpl *npc.Template, home, at location.Location, defs actorcast.Definitions) (*npc.Hostile, *actorcast.AIController) {
	t.Helper()
	hostile := s.spawnMovingHostile(t, tmpl, home, at, Geo{})
	return hostile, s.installCastSeam(hostile, defs)
}
