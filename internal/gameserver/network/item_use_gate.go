package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// olympiadBarred reports whether tmpl may not be used or worn by live
// because live competes in an Olympiad match: hero items and items the
// datapack restricts for the Olympiad are barred there, ahead of any use
// condition.
func olympiadBarred(live *livePlayer, tmpl *item.Template) bool {
	return (tmpl.OlyRestricted || tmpl.HeroItem()) && live.OlympiadMode()
}

// sendOlympiadBar tells live it may not use tmpl in its Olympiad match: as
// an item that cannot be worn there when it can be worn, as one unavailable
// there otherwise.
func sendOlympiadBar(live *livePlayer, tmpl *item.Template) {
	if tmpl.Equipable() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemCantBeEquippedForOlympiad))
	} else {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemUnavailableForOlympiad))
	}
}

// itemRestrictionHolds reports whether live may keep wearing tmpl: it is
// not barred in live's Olympiad match and live meets every use condition.
// The Olympiad bar still tells the client why, as it does on every path;
// a failed use condition stays silent. aborted reports a use condition test
// that reached a <not> around no condition: the check fails hard there,
// which ends the whole worn-item sweep.
func itemRestrictionHolds(live *livePlayer, tmpl *item.Template) (holds, aborted bool) {
	if olympiadBarred(live, tmpl) {
		sendOlympiadBar(live, tmpl)
		return false, false
	}
	_, held, aborted := evaluateItemUse(live, tmpl)
	return held, aborted
}

// rejectUseItemConditions tells live why it may not use tmpl and reports
// true, or reports false when nothing bars it: an item barred in the
// Olympiad is refused as one that cannot be worn there when it can be
// worn, as one unavailable there otherwise; then the first use condition
// live fails answers with its own message. A use condition test that
// aborts carries no message and answers ActionFailed.
func rejectUseItemConditions(live *livePlayer, tmpl *item.Template) bool {
	if live == nil || tmpl == nil {
		return false
	}
	if olympiadBarred(live, tmpl) {
		sendOlympiadBar(live, tmpl)
		return true
	}
	failed, held, _ := evaluateItemUse(live, tmpl)
	if held {
		return false
	}
	sendUseConditionFailure(live, tmpl, failed)
	return true
}

// evaluateItemUse tests tmpl's use conditions with live using the item on
// itself.
func evaluateItemUse(live *livePlayer, tmpl *item.Template) (failed item.UseCondition, held, aborted bool) {
	actor := live.ConditionActor()
	return conditions.EvaluateItem(tmpl.UseConditions, actor, actor)
}

func sendUseConditionFailure(live *livePlayer, tmpl *item.Template, uc item.UseCondition) {
	switch {
	case uc.Message != "":
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, uc.Message))
	case uc.MessageID > 0 && uc.AddName:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(int(uc.MessageID), tmpl.ID))
	case uc.MessageID > 0:
		live.SendFrame(serverpackets.FrameSystemMessage(int(uc.MessageID)))
	default:
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}
