package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// npcResidence returns the residence inst serves: a castle, or else a clan
// hall. A maker whose spawn time names a castle or a siegable hall gives its
// NPCs that residence; otherwise the NPC serves the first castle whose npcs
// list holds its template id, else the first clan hall's.
func (l *GameClientLink) npcResidence(inst *npc.Instance) (*castle.Castle, *hallmodel.Hall) {
	if inst == nil || inst.Template == nil {
		return nil, nil
	}
	if id, ok := inst.Maker.ResidenceParam(); ok {
		if c, ok := l.castles.Get(id); ok {
			return c, nil
		}
		if l.clanHallData != nil {
			if h, ok := l.clanHallData.Get(id); ok && h.IsSiegable() {
				return nil, h
			}
		}
	}
	if c, ok := l.castles.ByNPC(inst.Template.ID); ok {
		return c, nil
	}
	if l.clanHallData == nil {
		return nil, nil
	}
	if h, ok := l.hallOfManager(inst.Template.ID); ok {
		return nil, h
	}
	return nil, nil
}

// residenceTalk is what live sees of the residence f serves, as its chat
// window reads it.
//
// ponytail: a siegable hall's siege is not run yet (#244), so such a hall
// never reads as under siege.
func (l *GameClientLink) residenceTalk(live *livePlayer, f *npc.Folk) npc.ResidenceTalk {
	var r npc.ResidenceTalk
	c, h := l.npcResidence(f.Instance)
	switch {
	case c != nil:
		r.CastleID, r.OwnerID = c.ID, c.OwnerID()
		if s, ok := l.castleSiege(int32(c.ID)); ok {
			r.SiegeInProgress = s.InProgress()
			if field := s.Battlefield(); field != nil {
				r.BattlefieldActive = field.Active()
			}
		}
	case h != nil:
		r.HallID, r.Siegable, r.HallName = h.ID, h.IsSiegable(), h.Name
		if v, ok := l.halls.View(int32(h.ID)); ok {
			r.OwnerID = v.OwnerID
		}
	}
	if r.OwnerID != 0 {
		if owner, ok := l.clanService().Table().Get(r.OwnerID); ok {
			info := owner.Info()
			r.OwnerName, r.OwnerLeader = info.Name, info.LeaderName
		}
	}
	if cl, ok := l.clanService().ClanOf(live.Character); ok {
		r.ClanID, r.ClanLeader = cl.ID(), cl.IsLeader(live.ObjectID())
		r.Privileges = clan.Privilege(cl.MemberPrivileges(live.ObjectID()))
	}
	if l.sevenSigns != nil {
		r.StrifeOwner = l.sevenSigns.SealOwners()[sevensigns.Strife-sevensigns.Avarice]
	}
	return r
}
