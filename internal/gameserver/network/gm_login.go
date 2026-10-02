package network

import "github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"

// applyGMLoginModes puts a game master entering the world in the modes the
// GM startup settings ask for, before the enter-world burst: invulnerable
// and invisible when its access level may use //invul and //hide, and
// blocking everything, which its client hears of at once through an
// EtcStatusUpdate.
func (l *GameClientLink) applyGMLoginModes(live *livePlayer) {
	level := live.accessLevel()
	if !level.IsGM {
		return
	}
	cfg := l.playerConfig
	if cfg.GMStartupInvulnerable && l.admin.HasAccess("admin_invul", level) {
		live.SetInvul(true)
	}
	if cfg.GMStartupInvisible && l.admin.HasAccess("admin_hide", level) {
		live.SetInvisible(true)
	}
	if cfg.GMStartupBlockAll {
		live.SetBlockingAll(true)
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
	}
}

// sendGMLoginModes tells a game master entering the world which of the
// invulnerable, invisible and block-everything modes it is in.
func sendGMLoginModes(live *livePlayer) {
	if !live.accessLevel().IsGM {
		return
	}
	if live.Invul() {
		sendText(live, "Entering world in Invulnerable mode.")
	}
	if live.Invisible() {
		sendText(live, "Entering world in Invisible mode.")
	}
	if live.BlockingAll() {
		sendText(live, "Entering world in Refusal mode.")
	}
}
