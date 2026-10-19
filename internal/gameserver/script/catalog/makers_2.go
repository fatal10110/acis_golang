package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
)

// makers2 is the maker catalog of batch 2 of 3 (#213): the dungeon and instanced-hall makers, by npcmaker type.
func makers2() script.MakerCatalog {
	return script.MakerCatalog{
		"alarm_device1_maker":                maker.AlarmDevice1,
		"alarm_device2_maker":                maker.AlarmDevice2,
		"alarm_device3_maker":                maker.AlarmDevice3,
		"alarm_device4_maker":                maker.AlarmDevice4,
		"hall_keeper_captain_maker":          maker.HallKeeperDefault,
		"hall_keeper_guard_maker":            maker.HallKeeperDefault,
		"hall_keeper_patrol_maker":           maker.HallKeeperDefault,
		"hall_keeper_self_destruction_maker": maker.HallKeeperDefault,
		"hall_keeper_wizard_maker":           maker.HallKeeperDefault,
		"hall_timer_bomb_maker":              maker.HallKeeperDefault,
		"manage_teleport_dungeon":            nil,
		"rainbow_maker":                      maker.Rainbow,
		"royal_req_next_maker":               nil,
		"royal_rush_maker":                   nil,
		"royal_spawn_treasurebox":            nil,
		"undead_bandmaster_maker":            maker.UndeadBandmaster,
		"warrior_passive_weakness_maker":     maker.WarriorPassiveWeakness,
	}
}
