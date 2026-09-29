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
// had: nothing it was doing is stopped.
// ponytail: the intention carried on is not re-issued, and a cast approach
// is kept rather than dropped (#2815); the upgrade is a player intention
// slot that can re-run what was current.
func (l *GameClientLink) tryToUseItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) {
	if live.DenyAIAction() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if itemAICastBusy(live) {
		live.deferUseItem(inst.ObjectID)
		return
	}
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
