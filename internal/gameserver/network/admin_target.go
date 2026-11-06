package network

import "github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"

// adminTargetPrefix is what precedes the player name in "admin_target name".
const adminTargetPrefix = "admin_target "

// adminTarget answers //target name as a click on the online player named
// by the rest of the line, unaltered: the first selects the player, a
// second on the selected player attacks, interacts with or follows it as a
// click would. A name nobody online carries is answered as offline; a
// command with no name at all asks for a correct name.
func (l *GameClientLink) adminTarget(gm *livePlayer, line string) {
	if len(line) < len(adminTargetPrefix) {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageIncorrectCharacterNameTryAgain))
		return
	}
	target, ok := l.livePlayerByName(line[len(adminTargetPrefix):])
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageContactCurrentlyOffline))
		return
	}
	if cur := gm.Target(); cur == nil || cur.ObjectID() != target.ObjectID() {
		l.selectLiveTarget(gm, target)
		return
	}
	l.actOnTargetAgain(gm, target, false, false)
}
