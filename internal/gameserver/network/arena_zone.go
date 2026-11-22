package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// arenaCombatNotice tells a player it entered or left an arena, as the
// crossing happens.
func arenaCombatNotice(a zone.Actor, entering bool) {
	id := serverpackets.SystemMessageLeftCombatZone
	if entering {
		id = serverpackets.SystemMessageEnteredCombatZone
	}
	sendZoneMessage(a, id)
}
