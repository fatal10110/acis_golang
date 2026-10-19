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
		"exclusive_despawn_normal":        maker.ExclusiveDespawnNormal,
		"exclusive_spawn_normal":          maker.ExclusiveSpawnNormal,
		"exclusive_spawn_use_db":          maker.ExclusiveSpawnNormal,
		"farm_maker":                      maker.Farm,
		"maker_instant_spawn":             maker.InstantSpawn,
		"maker_instant_spawn_random":      maker.InstantSpawnRandom,
		"maker_instant_spawn_serial":      maker.InstantSpawnSerial,
		"maker_instant_spawn_serial_last": maker.InstantSpawnSerialLast,
		"on_day_night_spawn":              maker.OnDayNightSpawn,
		"parent_spawn_all":                maker.ParentSpawnAll,
		"random_spawn":                    maker.RandomSpawn,
		"random_spawn_pawn":               maker.RandomSpawnPawn,
		"undead_band_archer_wazird_maker": maker.OpenDoor,
		"undead_band_lance_maker":         maker.OpenDoor,
		"undead_band_leader_maker":        maker.OpenDoor,
		"unique_npc_kill_event":           maker.UniqueNpcKillEvent,
	}
}
