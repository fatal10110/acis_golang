package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// useItemIntention is a weapon or shield equip toggle queued as the next
// intention behind a swing, a cast or a sit/stand transition. It names the
// item by object id: the item is looked up again when the toggle runs.
type useItemIntention struct {
	objectID int32
}

// useItemResume is what follows a queued equip toggle: the intention that
// was current when it was queued resumes, unless it was a cast.
type useItemResume uint8

const (
	// resumeAttack re-thinks the attack a finished swing belonged to.
	resumeAttack useItemResume = iota
	// resumeNothing ends the cast that finished and the attack it
	// replaced: a cast does not resume after the toggle.
	resumeNothing
	// resumePosture re-thinks the sit-down or stand-up that finished. The
	// posture is already reached, so the think goes idle and answers
	// ActionFailed.
	resumePosture
)

// tryToUseItem runs UseItem on a weapon or shield as a USE_ITEM intention.
// A player that cannot take AI actions is answered ActionFailed. One still
// swinging, casting, sitting down or standing up keeps the toggle as its
// next intention, replacing whatever was queued, so the swing or cast in
// flight resolves with the weapon it started with; the reference answers
// nothing for that, and UseItem registers no pending client action.
// Anyone else toggles the item now and carries on with the intention it
// had, except a walk into ground-cast range: the toggle takes the CAST
// intention's place, so the walk goes on but arriving casts nothing and a
// blocked walk reports no stopped cast. Nothing is stopped.
// ponytail: the intention carried on is not re-issued, so a walk, follow,
// pickup, interact or attack approach under way sends no fresh movement
// packet, and a toggle that replaced a cast approach is not run a second
// time when the walk arrives (#2877); the upgrade is a player
// current-intention slot that can re-run what was current.
func (l *GameClientLink) tryToUseItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) {
	if live.DenyAIAction() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if itemAICastBusy(live) {
		live.deferUseItem(inst.ObjectID)
		return
	}
	// The toggle is the current intention now and empties the next-intention
	// slot, which also holds the skill request a walk into ground-cast range
	// casts on arrival.
	live.clearNextIntention()
	l.toggleEquipItem(live, inv, inst, tmpl, false)
}

// finishDeferredUseItem runs the equip toggle queued as the next intention,
// if any, then resumes as resume says, and reports whether one was waiting.
// An item no longer held is dropped without toggling or resuming anything.
// During a sit-down or stand-up the toggle stays queued for PostureSettled.
func (l *GameClientLink) finishDeferredUseItem(live *livePlayer, resume useItemResume) bool {
	if live == nil || live.detached() {
		return false
	}
	if inPostureTransition(live) {
		return live.hasDeferredUseItem()
	}
	queued := live.takeDeferredUseItem()
	if queued == nil {
		return false
	}
	if resume != resumeAttack && live.combat != nil {
		live.combat.Replace()
	}
	inv := live.Inventory()
	if inv == nil {
		return true
	}
	inst := inv.ItemByObjectID(queued.objectID)
	if inst == nil {
		if live.combat != nil {
			live.combat.Replace()
		}
		return true
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		return true
	}
	l.toggleEquipItem(live, inv, inst, tmpl, false)
	switch resume {
	case resumeAttack:
		live.thinkAttack()
	case resumePosture:
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	return true
}
