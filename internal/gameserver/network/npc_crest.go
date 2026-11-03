package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// npcInstance is the NPC instance behind v, nil for an object that is no
// NPC.
func npcInstance(v world.Tracked) *npc.Instance {
	switch o := v.(type) {
	case *npc.Folk:
		return o.Instance
	case *npc.Hostile:
		return o.Instance
	case *npc.Decoration:
		return o.Instance
	case *npc.EffectPoint:
		return o.Instance
	}
	return nil
}

// npcResidenceOwner is the clan owning the residence inst serves: its
// castle's owner, else its clan hall's; 0 when it serves none or the
// residence is free.
func (l *GameClientLink) npcResidenceOwner(inst *npc.Instance) int32 {
	c, h := l.npcResidence(inst)
	switch {
	case c != nil:
		return c.OwnerID()
	case h != nil:
		if v, ok := l.halls.View(int32(h.ID)); ok {
			return v.OwnerID
		}
	}
	return 0
}

// NPCResidenceOwner is the clan owning the residence inst serves, 0 for
// none: the spawned hostile NPCs read it as the clan they stand for, unless
// their spawn gave them one.
func (l *GameClientLink) NPCResidenceOwner(inst *npc.Instance) int32 {
	return l.npcResidenceOwner(inst)
}

// addNPCCrest puts into s, v's NpcInfo, the clan and alliance v stands for,
// when npcs.properties ShowNpcCrest is set: the owner of the castle v
// serves, else the clan v's spawn gave it, else the owner of the clan hall
// v serves. The alliance crest is the clan's as stored, whether or not it
// is still in an alliance. Without the switch, or for an NPC standing for
// no clan, s keeps no clan.
func (l *GameClientLink) addNPCCrest(v world.Tracked, s *npcinfo.Snapshot) {
	if l == nil || !l.showNPCCrest {
		return
	}
	inst := npcInstance(v)
	if inst == nil {
		return
	}
	if c, _ := l.npcResidence(inst); (c == nil || c.OwnerID() == 0) && inst.ClanID != 0 {
		l.putClanCrest(s, inst.ClanID)
		return
	}
	l.putClanCrest(s, l.npcResidenceOwner(inst))
}

// putClanCrest puts into s the clan clanID and its alliance, as stored:
// their ids and crests. A clanID of 0, or of no clan, leaves s alone.
func (l *GameClientLink) putClanCrest(s *npcinfo.Snapshot, clanID int32) {
	if clanID == 0 {
		return
	}
	cl, ok := l.clanService().Table().Get(clanID)
	if !ok {
		return
	}
	info := cl.Info()
	s.ClanID, s.ClanCrest = int(info.ID), int(info.CrestID)
	s.AllyID, s.AllyCrest = int(info.AllyID), int(info.AllyCrestID)
}
