package network

import "github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"

// registerGM puts live on the online game-master list when its access level
// makes it a game master. It is listed when GMs log in listed and its access
// level may use //gmlist; otherwise it is on the list hidden.
func (l *GameClientLink) registerGM(live *livePlayer) {
	if !live.accessLevel().IsGM {
		return
	}
	listed := !l.playerConfig.GMStartupUnlisted && l.admin.HasAccess("admin_gmlist", live.accessLevel())
	l.gms.Add(live, !listed)
}

// requestGmList answers /gmlist: the GMs online, the hidden ones too when
// live is a GM itself, or the notice that none is, with its sound.
func (l *GameClientLink) requestGmList(live *livePlayer) {
	if live == nil {
		return
	}
	includeHidden := live.accessLevel().IsGM
	if !l.gms.Online(includeHidden) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoGMProvidingServiceNow))
		live.SendFrame(serverpackets.FramePlaySound("systemmsg_e.702"))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageGMList))
	for _, gm := range l.gms.Entries(includeHidden) {
		name := gm.Player.Name
		if gm.Hidden {
			name += " (invis)"
		}
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageGMS1, name))
	}
}

// adminToggleGMList answers //gmlist: gm leaves the listed GMs, or rejoins
// them. A player off the GM list altogether is answered nothing.
func (l *GameClientLink) adminToggleGMList(gm *livePlayer, _ string) {
	hidden, ok := l.gms.Toggle(gm)
	if !ok {
		l.log.Warn().Int32("object_id", gm.ObjectID()).Msg("admin: //gmlist from a player off the GM list")
		return
	}
	if hidden {
		sendText(gm, "Removed from GMList.")
	} else {
		sendText(gm, "Registered into GMList.")
	}
}
