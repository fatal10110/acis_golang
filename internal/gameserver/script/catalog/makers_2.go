package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// makers2 is the maker catalog of batch 2 of 3 (#213): the dungeon and instanced-hall makers, by npcmaker type.
func makers2() script.MakerCatalog {
	return script.MakerCatalog{
		"alarm_device1_maker":                nil,
		"alarm_device2_maker":                nil,
		"alarm_device3_maker":                nil,
		"alarm_device4_maker":                nil,
		"hall_keeper_captain_maker":          nil,
		"hall_keeper_guard_maker":            nil,
		"hall_keeper_patrol_maker":           nil,
		"hall_keeper_self_destruction_maker": nil,
		"hall_keeper_wizard_maker":           nil,
		"hall_timer_bomb_maker":              nil,
		"manage_teleport_dungeon":            nil,
		"rainbow_maker":                      nil,
		"royal_req_next_maker":               nil,
		"royal_rush_maker":                   nil,
		"royal_spawn_treasurebox":            nil,
		"undead_bandmaster_maker":            nil,
		"warrior_passive_weakness_maker":     nil,
	}
}
