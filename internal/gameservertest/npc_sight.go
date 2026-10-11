package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// SpawnMovingHostileNPCTemplateAtGeo is SpawnMovingHostileNPCTemplate with
// an explicit geo: a geo that also answers npc.LineOfSight gives the NPC
// its line of sight, as production does.
func (s *Server) SpawnMovingHostileNPCTemplateAtGeo(t *testing.T, tmpl *npc.Template, home, at location.Location, geo move.Geo) *npc.Hostile {
	t.Helper()
	return s.spawnMovingHostile(t, tmpl, home, at, geo)
}
