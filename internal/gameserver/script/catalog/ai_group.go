package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// aiGroup is the catalog of the group behaviors (#208), except the walkers the engine slices ported.
func aiGroup() script.Catalog {
	return script.Catalog{
		"script.ai.group.FollowerMovingAroundMaster": nil,
		"script.ai.group.RandomTeleport":             nil,
	}
}
