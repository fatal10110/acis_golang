package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
)

// makers1 is the maker catalog of batch 1 of 3 (#212): the shared and generic makers, the parents the other two batches extend among them, by npcmaker type.
func makers1() script.MakerCatalog {
	return script.MakerCatalog{
		"Close_Door_maker":                maker.CloseDoor,
		"default_use_db_maker":            maker.DefaultUseDB,
		"exclusive_despawn_normal":        nil,
		"exclusive_spawn_normal":          nil,
		"exclusive_spawn_use_db":          nil,
		"farm_maker":                      maker.Farm,
		"maker_instant_spawn":             maker.InstantSpawn,
		"maker_instant_spawn_random":      nil,
		"maker_instant_spawn_serial":      nil,
		"maker_instant_spawn_serial_last": nil,
		"on_day_night_spawn":              nil,
		"parent_spawn_all":                nil,
		"random_spawn":                    nil,
		"random_spawn_pawn":               nil,
		"undead_band_archer_wazird_maker": nil,
		"undead_band_lance_maker":         nil,
		"undead_band_leader_maker":        nil,
		"unique_npc_kill_event":           nil,
	}
}
