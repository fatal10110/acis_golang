package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// wireMotherTreeZones gives the Mother Tree zones of ix their entry and exit
// announcements.
func wireMotherTreeZones(ix *zone.Index) {
	for _, z := range zone.OfKind[*zone.MotherTree](ix) {
		z.Notify = sendZoneMessage
	}
}

// sendZoneMessage sends a player crossing a zone boundary system message id
// as the crossing happens: on a spawn, at the spawn's place in its frames,
// and on a walk, ahead of the compass update the revalidation sends next.
func sendZoneMessage(a zone.Actor, id int) {
	if za, ok := a.(*liveZoneActor); ok {
		za.live.SendFrame(serverpackets.FrameSystemMessage(id))
	}
}
