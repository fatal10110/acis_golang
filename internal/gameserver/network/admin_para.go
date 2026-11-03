package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// paralyzable is a creature //para holds in place or lets go.
type paralyzable interface {
	Paralyzed() bool
	SetParalyzed(v bool) bool
	StartAbnormalEffect(mask int)
	StopAbnormalEffect(mask int)
	UpdateAbnormalEffect()
	AbortAll(resetTarget bool)
}

// adminPara answers //para on the selected creature, on that creature's
// own queue: one not paralyzed is shown held and paralyzed, and stops its
// walk, swing and cast; one paralyzed, by //para or by an effect, is shown
// no longer held and loses the lock, and an NPC or summon then takes up its
// current intention again. The GM is told nothing either way. With nothing
// selected, or no creature, the target is invalid.
//
// A door, decoration or effect point is a creature too, but the command
// changes nothing on one and tells the GM nothing: a door shows no
// abnormal state and never moves, swings or casts, so its lock would be
// unseen, and a decoration or effect point refuses selection.
func (l *GameClientLink) adminPara(gm *livePlayer, _ string) {
	switch target := gm.Target().(type) {
	case *livePlayer:
		onPlayer(gm, target, func() { toggleParalysis(target) })
	case *npc.Hostile:
		target.Queue().Post(func() {
			if !toggleParalysis(target) {
				_ = target.Think()
			}
		})
	case *npc.Folk:
		target.Queue().Post(func() {
			if !toggleParalysis(target) {
				target.Think()
			}
		})
	case *summon.Actor:
		target.Post(func() {
			if !toggleParalysis(target) {
				_ = target.Think()
			}
		})
	case *door.Object, *npc.Decoration, *npc.EffectPoint:
	default:
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	}
}

// toggleParalysis paralyzes c when it is not paralyzed, and reports true;
// otherwise it lifts c's paralysis lock, which leaves any paralyzing
// effect in place, and reports false. Run it on c's queue.
func toggleParalysis(c paralyzable) bool {
	if !c.Paralyzed() {
		c.StartAbnormalEffect(modelskill.AbnormalHold2)
		c.UpdateAbnormalEffect()
		c.SetParalyzed(true)
		c.AbortAll(false)
		return true
	}
	c.StopAbnormalEffect(modelskill.AbnormalHold2)
	c.UpdateAbnormalEffect()
	c.SetParalyzed(false)
	return false
}
