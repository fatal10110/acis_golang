package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// addSummonCrest puts into s, a's NpcInfo, the clan and alliance of a's
// owner, when npcs.properties ShowSummonCrest is set and the owner is in a
// clan. The alliance crest is the clan's as stored, whether or not it is
// still in an alliance. Without the switch s keeps no clan.
func (l *GameClientLink) addSummonCrest(a *summon.Actor, s *serverpackets.NPCInfoSnapshot) {
	if l == nil || !l.showSummonCrest {
		return
	}
	if owner, ok := liveSummonOwner(a); ok {
		l.putClanCrest(s, owner.ClanID())
	}
}

// showSummonCrestToOwner sends a's owner the NpcInfo others see of a, as
// it sees it, when ShowSummonCrest is set: the owner's own pet window
// carries no crest, so this is how it is shown its clan's crest over a. A
// summon sends it as it spawns, ahead of its RelationChanged, and as it
// lands from a teleport.
func (l *GameClientLink) showSummonCrestToOwner(owner *livePlayer, a *summon.Actor) {
	if l == nil || !l.showSummonCrest || owner == nil || a == nil {
		return
	}
	if snap, ok := l.summonInfoSnapshot(a, owner, owner.npcs, l.summonInCombat(a)); ok {
		owner.sendVisibilityFrame(serverpackets.FrameNPCInfo(snap))
	}
}
