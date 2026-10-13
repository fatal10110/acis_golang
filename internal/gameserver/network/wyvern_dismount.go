package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// wyvernGraceDelay is how long a wyvern rider may stay over ground closed
// to wyverns before it is taken off its mount.
const wyvernGraceDelay = 5 * time.Second

// wireNoLandingZones gives the no-landing zones of ix their forced
// dismount of wyvern riders.
func (l *GameClientLink) wireNoLandingZones(ix *zone.Index) {
	for _, z := range zone.OfKind[*zone.NoLanding](ix) {
		z.DismountTimer = l.wyvernDismountTimer
	}
}

// siegeDismountTimer is wyvernDismountTimer for a battlefield under
// siege. A player thrown off the battlefield as its siege starts is out
// before the battlefield turns on, so it is neither warned nor counted
// down.
func (l *GameClientLink) siegeDismountTimer(a zone.Actor, entering bool) {
	if za, ok := a.(*liveZoneActor); ok && l.siegeBanished(za.live) {
		return
	}
	l.wyvernDismountTimer(a, entering)
}

// wyvernDismountTimer warns a wyvern rider entering ground closed to
// wyverns and starts the countdown that takes it off its mount, or calls
// the countdown off as the rider leaves. Players on foot or on a strider
// are left alone. A rider has one countdown however many such zones it
// stands in: entering another only warns again, and leaving any one calls
// the countdown off. Zone rules run under the zone lock, so the countdown
// is armed and stopped on the rider's queue, in the order the zones ran
// them.
func (l *GameClientLink) wyvernDismountTimer(a zone.Actor, entering bool) {
	za, ok := a.(*liveZoneActor)
	if !ok {
		return
	}
	live := za.live
	if live.MountType() != player.MountTypeWyvern {
		return
	}
	if entering {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoWyvernArea))
	}
	postLive(live, func() { armWyvernDismount(live, entering) })
}

// armWyvernDismount starts (arm true) the forced-dismount countdown of
// live unless one is already running, or stops it (arm false). Once the
// countdown has run it is spent, so the next entry starts a new one. The
// rider is taken off only if it still rides a wyvern then. It runs on
// live's queue.
func armWyvernDismount(live *livePlayer, arm bool) {
	if !arm {
		if live.wyvernDismount != nil {
			live.wyvernDismount.Stop()
			live.wyvernDismount = nil
		}
		return
	}
	if live.wyvernDismount != nil {
		return
	}
	live.wyvernDismount = live.after(wyvernGraceDelay, func() {
		live.wyvernDismount = nil
		if live.detached() || live.MountType() != player.MountTypeWyvern {
			return
		}
		live.Character.Dismount()
	})
}
