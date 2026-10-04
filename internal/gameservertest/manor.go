package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// WithManor sets the manor the seed and harvester items and the harvest
// read (default: the zero value, manor off).
func WithManor(cfg network.ManorConfig) Option {
	return func(o *options) { o.manor = cfg }
}

// SpawnHostileNPCTemplateHomedAt seeds a parked hostile NPC built from tmpl
// standing at at but spawned at home, as a live spawn records it: a monster
// that has walked away from its spawn point still belongs to the manor area
// of home.
func (s *Server) SpawnHostileNPCTemplateHomedAt(t *testing.T, tmpl *npc.Template, at, home location.Location) *npc.Hostile {
	t.Helper()
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	inst.Home, inst.HasHome = home, true
	return s.spawnHostileInstance(t, inst, at, parkedAttack{})
}
