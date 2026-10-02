package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// hiddenFrom reports whether viewer must draw live invisible: live is
// invisible and viewer is no game master.
func hiddenFrom(live *livePlayer, viewer any) bool {
	if !live.Invisible() {
		return false
	}
	p, ok := viewer.(*livePlayer)
	return !ok || !p.accessLevel().IsGM
}

// broadcastCharInfo sends live's CharInfo, wearing items, to every client
// that knows live. An invisible live is drawn invisible to everyone but game
// masters, so a viewer gets one of two frames, each built on first use. Each
// player among them then gets live's relation to it, and its summon's
// (sendRelations).
func (l *GameClientLink) broadcastCharInfo(live *livePlayer, items []*item.Instance) {
	if l.world == nil {
		return
	}
	info := serverpackets.CharInfoSnapshot{Character: live.Character, Template: live.Template(), Items: items, Clan: l.clanFields(live.Character)}
	pet := l.summonOf(live)
	var frames [2]wire.Frame
	var built [2]bool
	defer func() {
		for i := range frames {
			if built[i] {
				frames[i].Release()
			}
		}
	}()
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(frameReceiver)
		if !ok {
			return
		}
		hidden := 0
		if hiddenFrom(live, o) {
			hidden = 1
		}
		if !built[hidden] {
			info.Hidden = hidden == 1
			frames[hidden] = serverpackets.FrameCharInfo(info)
			built[hidden] = true
		}
		if frame, ok := serverpackets.CopyFrame(frames[hidden]); ok {
			receiver.BroadcastFrame(frame)
		}
		if observer, ok := o.(*livePlayer); ok {
			l.sendRelations(live, pet, observer.Character, observer.BroadcastFrame)
		}
	})
}

// petAbnormalEffect is a's abnormal visual as its owner's pet window shows
// it: an invisible owner sees its summon drawn in stealth.
func petAbnormalEffect(a *summon.Actor, owner *livePlayer) int {
	abnormal := a.AbnormalEffect()
	if owner.Invisible() {
		abnormal |= modelskill.AbnormalStealth
	}
	return abnormal
}
