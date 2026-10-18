package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/follower"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/randomteleport"
)

// aiGroup is the catalog of the group behaviors (#208), except the walkers the engine slices ported.
func aiGroup() script.Catalog {
	return script.Catalog{
		"script.ai.group.FollowerMovingAroundMaster": follower.New,
		"script.ai.group.RandomTeleport":             randomteleport.New,
	}
}
