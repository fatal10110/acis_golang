package manager

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// newStartedNpcs builds the population of cfg with the shipped tuning, no
// civilian sink and no script hooks, and spawns it at once as SpawnOnStart
// does.
func newStartedNpcs(cfg NpcsConfig) (*Npcs, error) {
	cfg.NewFolkSink = nil
	cfg.Limits = npc.DefaultLimits()
	cfg.RandomWalkRate = 30
	cfg.MaxGeoPathFailCount = 0
	cfg.RaidMultipliers = npc.DefaultRaidMultipliers()
	cfg.AIConfig = npc.DefaultAIConfig()
	cfg.SpawnEvents = DefaultSpawnEvents()
	cfg.Scripts = nil
	n, err := NewNpcs(cfg)
	if err != nil {
		return nil, err
	}
	n.SpawnOnStart()
	return n, nil
}
