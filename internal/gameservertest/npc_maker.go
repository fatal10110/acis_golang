package gameservertest

import (
	"testing"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// SpawnCastingFolkNPCOfMakerAt is SpawnCastingFolkNPCAt for a civilian NPC
// an npcmaker placed: it reads maker's territories as its spawn territory
// (a script's InSpawnTerritory, its own InMyTerritory), as an NPC the
// population's maker spawns does. The maker spawns nothing itself.
func (s *Server) SpawnCastingFolkNPCOfMakerAt(t *testing.T, tmpl *npc.Template, at location.Location, defs actorcast.Definitions, maker *spawn.Maker) *npc.Folk {
	t.Helper()
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	inst.Home, inst.HasHome, inst.Maker = at, true, maker
	spawner := s.folkSpawner(nil, Geo{})
	spawner.Skills = defs
	f, err := spawner.Spawn(inst, at, 0, nil)
	if err != nil {
		t.Fatalf("spawn folk npc: %v", err)
	}
	return f
}

// SpawnHostileNPCOfMakerAt is SpawnHostileNPCTemplateAt for a hostile NPC
// an npcmaker placed, reading maker's territories as its spawn territory as
// SpawnCastingFolkNPCOfMakerAt does.
func (s *Server) SpawnHostileNPCOfMakerAt(t *testing.T, tmpl *npc.Template, at location.Location, maker *spawn.Maker) *npc.Hostile {
	t.Helper()
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	inst.Maker = maker
	return s.spawnHostileInstance(t, inst, at, parkedAttack{})
}
