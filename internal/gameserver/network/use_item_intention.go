package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
	// replaced: a cast does not resume after the toggle, which stays
	// current (see heldIntention).
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
// Anyone else toggles the item now, then runs again the intention the
// toggle replaced, with the packets a fresh think of it sends: a walk to a
// point walks there afresh, a follow restarts, a pickup or summon interact
// approach releases the client and walks again, an attack is thought again.
// A cast approach (a walk into ground or target cast range) or another
// toggle is not run again: the toggle stays current, so the walk goes on but
// arriving casts nothing, a blocked walk reports no stopped cast, and the
// arrival toggles the item once more. The toggle itself stops nothing.
func (l *GameClientLink) tryToUseItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) {
	if live.DenyAIAction() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if itemAICastBusy(live) {
		live.deferUseItem(inst.ObjectID)
		return
	}
	rerun, keep := l.replacedIntention(live)
	// The toggle is the current intention now and empties the next-intention
	// slot, which also holds the skill request a walk into cast range casts
	// on arrival.
	live.clearNextIntention()
	if keep {
		live.holdUseItem(inst.ObjectID)
	}
	l.toggleEquipItem(live, inv, inst, tmpl, false)
	if rerun != nil {
		rerun()
	}
}

// replacedIntention reports the intention an immediate equip toggle
// replaces: rerun thinks it again once the item is toggled, and keep reports
// a cast approach or another toggle, which the toggle leaves unrun and stays
// current in place of. Both are empty for an idle player.
func (l *GameClientLink) replacedIntention(live *livePlayer) (rerun func(), keep bool) {
	held := live.heldIntention()
	if held.kind == heldUseItem || live.hasDeferredMagicSkill() || live.hasDeferredItemAICast() {
		return nil, true
	}
	if live.combat != nil && live.combat.Target() != nil {
		// ponytail: an approach already under way toward a target that has
		// not moved is not broadcast again (#2926).
		return live.thinkAttack, false
	}
	if live.hasPickup() {
		return func() { l.thinkLivePickup(live) }, false
	}
	if live.hasInteract() {
		return func() { l.finishInteract(live) }, false
	}
	if live.move != nil {
		if target := live.move.FriendlyFollowTarget(); target != nil {
			return func() { l.startLiveFollow(live, target, false) }, false
		}
	}
	if held.kind == heldMoveTo {
		return func() { l.thinkMoveTo(live, held.dest) }, false
	}
	return nil, false
}

// heldKind names the intention a heldIntention records.
type heldKind uint8

const (
	// heldMoveTo is a walk to a point, current until it arrives. The zero
	// value holds nothing.
	heldMoveTo heldKind = iota + 1
	// heldUseItem is an equip toggle left current because it replaced a
	// cast approach, a cast that ended, or another toggle.
	heldUseItem
)

// heldIntention is a player's current intention when it is one no other
// slot records. Every other intention taking over drops it: a new walk,
// attack, cast, follow, pickup or interact (clearParkedApproaches, the
// pickup and static-object interact paths), a sit or stand, going idle
// (tryToIdle), Stop and death. Guarded by pickupMu.
type heldIntention struct {
	kind heldKind
	// dest is a walk's destination.
	dest location.Location
	// itemID is a toggle's item object id.
	itemID int32
}

// holdMoveTo records a walk to dest as the current intention.
func (p *livePlayer) holdMoveTo(dest location.Location) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.held = heldIntention{kind: heldMoveTo, dest: dest}
}

// holdUseItem records toggling objectID as the current intention.
func (p *livePlayer) holdUseItem(objectID int32) {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.held = heldIntention{kind: heldUseItem, itemID: objectID}
}

// dropHeldIntention drops a held walk or toggle.
func (p *livePlayer) dropHeldIntention() {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	p.held = heldIntention{}
}

func (p *livePlayer) heldIntention() heldIntention {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	return p.held
}

// arriveHeldIntention answers an arrival for a held intention: a walk to a
// point ends idle, and a toggle left current toggles its item again.
func (l *GameClientLink) arriveHeldIntention(live *livePlayer) {
	switch held := live.heldIntention(); held.kind {
	case heldMoveTo:
		live.dropHeldIntention()
	case heldUseItem:
		l.toggleHeldItem(live, held.itemID)
	}
}

// thinkCurrentIntention thinks live's current intention again, as a THINK
// does: a toggle left current toggles its item again, a cast approach is
// thought as its arrival thinks it, and an attack, pickup, summon interact,
// follow or walk to a point is thought as an equip toggle re-runs it.
func (l *GameClientLink) thinkCurrentIntention(live *livePlayer) {
	if live.combat != nil && live.combat.Target() != nil {
		live.thinkAttack()
		return
	}
	if held := live.heldIntention(); held.kind == heldUseItem {
		l.toggleHeldItem(live, held.itemID)
		return
	}
	if live.hasDeferredMagicSkill() || live.hasDeferredItemAICast() {
		l.thinkParkedCast(live)
		return
	}
	if rerun, _ := l.replacedIntention(live); rerun != nil {
		rerun()
	}
}

// thinkParkedCast thinks a cast request held in the next-intention slot
// again. While a swing, a cast or a sit-down or stand-up is in flight the
// request is queued behind it, not current, and waits for that action's end
// to run it. Otherwise it is a cast approach, the current CAST intention: a
// player that cannot act or has every skill disabled goes idle, stopping any
// walk under way, and is answered ActionFailed; anyone else thinks it as the
// approach's arrival does, walking toward the target or signet afresh from
// where it stands while out of range and casting once in range.
func (l *GameClientLink) thinkParkedCast(live *livePlayer) {
	if itemAICastBusy(live) {
		// A THINK on the action in flight does not keep a cast queued
		// behind it in the reference AI: thinking an attack mid-swing
		// requeues the attack over the cast, thinking a cast mid-cast goes
		// idle and clears the queue, and both answer ActionFailed. Keeping
		// the cast queued and silent here cannot be told apart in play: a
		// player's only THINK is the ImmobileUntilAttacked exit, whose
		// start aborted every swing and cast and whose hold denies new
		// ones, so no swing or cast is in flight when it runs.
		return
	}
	if live.DenyAIAction() || live.Character.AllSkillsDisabled() {
		live.tryToIdle(false)
		if live.move != nil {
			live.move.Stop()
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !l.finishDeferredMagicSkill(live) {
		l.finishDeferredItemAICast(live)
	}
}

// toggleHeldItem toggles the item a toggle left current names, which stays
// current: what it replaced is not run again. An item no longer held
// toggles nothing.
func (l *GameClientLink) toggleHeldItem(live *livePlayer, objectID int32) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		return
	}
	l.toggleEquipItem(live, inv, inst, tmpl, false)
}

// thinkMoveTo thinks a walk to dest again. A player that cannot act or move
// goes idle and is answered ActionFailed; anyone else walks to dest afresh
// from where it stands.
func (l *GameClientLink) thinkMoveTo(live *livePlayer, dest location.Location) {
	if live.DenyAIAction() || live.MovementDisabled() || live.move == nil {
		live.tryToIdle(false)
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	accepted, err := live.move.MoveToLocation(dest)
	if err != nil {
		l.log.Warn().Err(err).Msg("move: broadcast")
	}
	if accepted {
		// Face dest from where the request left the player: a walk in
		// flight first advances by the time since its last update.
		live.Character.SetHeading(live.move.Position().HeadingTo(dest))
	}
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
	if resume == resumeNothing {
		// A cast does not run again after the toggle: the toggle stays
		// current, and every later think toggles the item again.
		live.holdUseItem(queued.objectID)
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
