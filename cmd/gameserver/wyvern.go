package main

import (
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// loadWyvernRules reads the npcs.properties RequiredStriderLevel (default
// 55) and RequiredCrystalsNumber (default 10) keys. A malformed value fails
// boot.
func loadWyvernRules(paths gameServerPaths) (npc.WyvernRules, error) {
	props, err := config.LoadFile(paths.NpcsConfigPath)
	if err != nil {
		return npc.WyvernRules{}, err
	}
	def := npc.DefaultWyvernRules()
	f := config.NewFields(props, "wyvern")
	rules := npc.WyvernRules{
		StriderLevel: f.Int("RequiredStriderLevel", def.StriderLevel),
		Crystals:     f.Int("RequiredCrystalsNumber", def.Crystals),
	}
	if err := f.Err(); err != nil {
		return npc.WyvernRules{}, err
	}
	return rules, nil
}
