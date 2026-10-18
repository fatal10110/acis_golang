package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// rideWyvernAtNpc carries out a wyvern ride f granted live: the B-grade
// crystals are taken, naming what disappeared, then live leaves its
// strider, storing a pet strider's meal, and mounts a wyvern no collar
// called, and the ride's page opens. A talker short of the crystals gets
// the shortage page and keeps its strider; a mount refused once the
// crystals are gone, by a weapon that cannot be taken off, opens no page.
// Each page releases the client.
func (l *GameClientLink) rideWyvernAtNpc(live *livePlayer, f *npc.Folk, reply npc.BypassReply) {
	page := reply.ShortHTML
	if destroyItemByItemID(live, item.CrystalB.ItemID(), reply.Count) {
		live.Character.Dismount()
		if !l.mountWyvern(live, player.WyvernNPCID, 0) {
			return
		}
		page = reply.HTML
	}
	sendFilledHTML(live, f.ObjectID(), page, 0)
	live.SendFrame(serverpackets.FrameActionFailed())
}
