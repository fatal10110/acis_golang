package network

import (
	"sync"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// pvpChange is one PvP flag change owed to a player: the side effects of a
// PK karma gain (reset), or the start or refresh of its flag window.
type pvpChange struct {
	reset              bool
	useFlaggedDuration bool
}

// pendingPvPChanges is the FIFO of PvP flag changes a player's queue still
// has to run. Every change reaches the queue through it, so a flag its
// summon raises before a PK kill's reset stays ahead of it, and one raised
// after stays behind it, whichever task runs them.
//
// It also holds the frames that must reach the player only after those
// changes' own frames: a PK kill made on another actor's queue (the hit
// player's own proc killing its attacker inside the attacker's hit) leaves
// its side effects to the player's queue, while the rest of the killing
// blow goes on sending the player frames from the other queue. Once such a
// frame is held behind the changes, every later frame for the player is
// held too, except the frames the player's queue sends while it runs the
// changes, and all of them go out in order right after the changes ran.
type pendingPvPChanges struct {
	mu      sync.Mutex
	changes []pvpChange
	// settling counts the settlePvPChanges calls running on the player's
	// queue: frames sent meanwhile are theirs and are never held.
	settling int
	held     []heldFrame
	// holding is set, under mu, while held frames wait for the changes; an
	// atomic so an unheld send checks it without taking mu.
	holding atomic.Bool
}

// heldFrame is one frame held behind a player's PvP flag changes, and which
// of the player's client paths it goes out through.
type heldFrame struct {
	frame      wire.Frame
	visibility bool
}

func (p *pendingPvPChanges) push(c pvpChange) {
	p.mu.Lock()
	p.changes = append(p.changes, c)
	p.mu.Unlock()
}

// take starts a settle: it returns the pending changes, and every frame
// sent until the matching release is the settle's own.
func (p *pendingPvPChanges) take() []pvpChange {
	p.mu.Lock()
	defer p.mu.Unlock()
	changes := p.changes
	p.changes = nil
	p.settling++
	return changes
}

func (p *pendingPvPChanges) empty() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.changes) == 0
}

// holdBehind holds frame behind the changes when any are still pending or
// running, or frames are already held. It reports whether it held frame,
// and whether that started the hold.
func (p *pendingPvPChanges) holdBehind(frame wire.Frame) (held, started bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.changes) == 0 && p.settling == 0 && !p.holding.Load() {
		return false, false
	}
	p.held = append(p.held, heldFrame{frame: frame})
	started = !p.holding.Load()
	p.holding.Store(true)
	return true, started
}

// hold holds any frame sent to the player while earlier frames wait for
// the changes, so it cannot overtake them. It does not while a settle runs:
// that settle's own frames go out first. A frame another queue sends the
// player through SendFrame in that window, rather than through
// sendBehindPvPChanges, goes out at once too; sends carry no sender, so
// the two cannot be told apart there.
func (p *pendingPvPChanges) hold(frame wire.Frame, visibility bool) bool {
	if !p.holding.Load() {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.holding.Load() || p.settling > 0 {
		return false
	}
	p.held = append(p.held, heldFrame{frame: frame, visibility: visibility})
	return true
}

// release ends a settle started by take. Once no settle runs and no change
// is pending, it sends the held frames through send, in order, then ends
// the hold. Frames held while it sends go out after them.
func (p *pendingPvPChanges) release(send func(heldFrame)) {
	p.mu.Lock()
	if p.settling > 0 {
		p.settling--
	}
	p.mu.Unlock()
	p.flush(send, false)
}

// flush sends the held frames once no settle runs and no change is
// pending; a change still pending keeps them for the settle it posted.
// force sends them regardless, for a player whose queue takes no more
// tasks.
func (p *pendingPvPChanges) flush(send func(heldFrame), force bool) {
	for {
		p.mu.Lock()
		if !force && (p.settling > 0 || len(p.changes) > 0) {
			p.mu.Unlock()
			return
		}
		frames := p.held
		p.held = nil
		if len(frames) == 0 {
			p.holding.Store(false)
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
		for _, f := range frames {
			send(f)
		}
	}
}

// queuePvPChange hands c to live's queue behind the changes already
// pending. The posted task runs whatever is still pending by then; a task
// of live's own may already have run it (settlePvPChanges).
func (l *GameClientLink) queuePvPChange(live *livePlayer, c pvpChange) {
	live.pvpChanges.push(c)
	postLive(live, func() { l.settlePvPChanges(live) })
}

// settlePvPChanges runs, in order, every PvP flag change pending for live,
// then sends live the frames held behind them. It runs on live's queue
// only.
func (l *GameClientLink) settlePvPChanges(live *livePlayer) {
	changes := live.pvpChanges.take()
	defer live.pvpChanges.release(live.sendHeldFrame)
	for _, c := range changes {
		if live.detached() {
			return
		}
		if c.reset {
			l.runPKKarmaSideEffects(live)
			continue
		}
		l.startPvPFlag(live, c.useFlaggedDuration)
	}
}

// sendBehindPvPChanges sends frame to live once live's queue has run the
// PvP flag changes pending for it, or at once when none is pending or
// running. A task of another actor's queue uses it for the frames of a
// killing blow that must follow the kill's side effects on live.
func (l *GameClientLink) sendBehindPvPChanges(live *livePlayer, frame wire.Frame) {
	if live.session == nil || live.SessionDetached() {
		frame.Release()
		return
	}
	held, started := live.pvpChanges.holdBehind(frame)
	if !held {
		live.sendFrameNow(frame, false)
		return
	}
	// The changes' own posted settle sends the frames, and so does the one
	// running now; this one only finds out whether live's queue still
	// takes tasks. One that does not belongs to a player gone offline,
	// whose frames are dropped anyway.
	if started && !postLive(live, func() { l.settlePvPChanges(live) }) {
		live.pvpChanges.flush(live.sendHeldFrame, true)
	}
}

// settleSummonOwnerPvPChanges runs, from a task of actor's own, the PvP flag
// changes pending for its owner. A summon's work is scheduled on its owner's
// queue (wireSummonAI), so a kill the summon's hit, skill or own proc just
// made settles inside that killing blow, as a player's own does. A corpse
// its owner left behind, and one revived since, has its work on a queue of
// its own (AdoptCorpseQueue) and leaves them to the owner's queue.
func (l *GameClientLink) settleSummonOwnerPvPChanges(actor *summon.Actor) {
	owner, ok := liveSummonOwner(actor)
	if !ok || actor.OwnerLeft() || actor.Queue() != owner.Queue() {
		return
	}
	l.settlePvPChanges(owner)
}

// applyPKKarmaSideEffects schedules what a PK karma gain costs the killer:
// every equipped item whose conditions it no longer meets comes off, then
// its PvP flag task stops and the flag resets. The kill can run on another
// actor's queue, so this queues the work; a task of live's queue runs it
// inside the killing blow: right after its own or its summon's physical
// hit's damage (HitDamageApplied), and before each message of its own or
// its summon's cast hit, their own procs and its cubics' skills.
func (l *GameClientLink) applyPKKarmaSideEffects(live *livePlayer) {
	l.queuePvPChange(live, pvpChange{reset: true})
}

func (l *GameClientLink) runPKKarmaSideEffects(live *livePlayer) {
	l.unequipRestrictedItems(live)
	if l.pvpFlags != nil {
		l.pvpFlags.Remove(live.Character, true)
	}
}

// applyPvPFlag starts or refreshes live's PvP flag window. A request from
// live's own action applies at once unless a change is still pending, in
// which case it queues behind it: an offensive skill that kills an innocent
// player flags its caster again after the kill's reset, and the caster ends
// flagged. A request from live's summon always queues, so the flag and a
// PK kill's reset keep the order the summon produced them in: a lethal hit
// flags first and ends unflagged, a lethal skill flags after and ends
// flagged.
func (l *GameClientLink) applyPvPFlag(live *livePlayer, e event.PvPFlagged) {
	if l.pvpFlags == nil {
		return
	}
	if e.ByServitor || !live.pvpChanges.empty() {
		l.queuePvPChange(live, pvpChange{useFlaggedDuration: e.UseFlaggedDuration})
		return
	}
	l.startPvPFlag(live, e.UseFlaggedDuration)
}

func (l *GameClientLink) startPvPFlag(live *livePlayer, useFlaggedDuration bool) {
	if l.pvpFlags == nil {
		return
	}
	if useFlaggedDuration {
		l.pvpFlags.AddFlagged(live.Character)
		return
	}
	l.pvpFlags.AddNormal(live.Character)
}

// unequipRestrictedItems takes off, through the UseItem equip toggle, every
// paperdoll item whose use conditions live fails. Taking off a weapon also
// aborts the attack in progress. An item an earlier removal in the same
// pass already took off is not toggled back on.
func (l *GameClientLink) unequipRestrictedItems(live *livePlayer) {
	inv := live.Inventory()
	if inv == nil || l.inventory == nil {
		return
	}
	for _, inst := range inv.PaperdollItems() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok || !inst.Equipped() || useConditionsHold(live, tmpl) {
			continue
		}
		l.toggleEquipItem(live, inv, inst, tmpl, tmpl.Kind == item.KindWeapon)
	}
}
