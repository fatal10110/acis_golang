package network

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
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
type pendingPvPChanges struct {
	mu      sync.Mutex
	changes []pvpChange
}

func (p *pendingPvPChanges) push(c pvpChange) {
	p.mu.Lock()
	p.changes = append(p.changes, c)
	p.mu.Unlock()
}

func (p *pendingPvPChanges) take() []pvpChange {
	p.mu.Lock()
	defer p.mu.Unlock()
	changes := p.changes
	p.changes = nil
	return changes
}

func (p *pendingPvPChanges) empty() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.changes) == 0
}

// queuePvPChange hands c to live's queue behind the changes already
// pending. The posted task runs whatever is still pending by then; a task
// of live's own may already have run it (settlePvPChanges).
func (l *GameClientLink) queuePvPChange(live *livePlayer, c pvpChange) {
	live.pvpChanges.push(c)
	postLive(live, func() { l.settlePvPChanges(live) })
}

// settlePvPChanges runs, in order, every PvP flag change pending for live.
// It runs on live's queue only.
func (l *GameClientLink) settlePvPChanges(live *livePlayer) {
	for _, c := range live.pvpChanges.take() {
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

// applyPKKarmaSideEffects schedules what a PK karma gain costs the killer:
// every equipped item whose conditions it no longer meets comes off, then
// its PvP flag task stops and the flag resets. The kill can run on another
// actor's queue, so this queues the work; a task of live's own runs it
// inside the killing blow: right after its physical hit's damage
// (HitDamageApplied), and before each message of its cast's hit, its own
// procs and its cubics' skills.
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
// flagged. A request from live's summon comes from the summon's queue and
// always queues, so the flag and a PK kill's reset keep the order the
// summon produced them in: a lethal hit flags first and ends unflagged, a
// lethal skill flags after and ends flagged.
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
