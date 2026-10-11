package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	escortFollowDistance = 150
	escortSlotCount      = 8
	escortSlotAngle      = 0.785
	escortStayRadius     = 100.0
	escortMasterMoveSkip = 5.0
	escortLooseRadius    = 150
	escortGeoFailLimit   = 10
	escortTeleportOffset = 10
)

// IdleFollowTarget returns this NPC's living master when it is a party
// private that should escort while idle. An NPC a behavior is bound to has
// none: its no-desire hooks decide whether it follows.
func (h *Hostile) IdleFollowTarget() attackable.Combatant {
	if h.behaves() || h.aiInt("Party_Type", 0) != 1 {
		return nil
	}
	master := h.Master()
	if master == nil || master.AlikeDead() {
		return nil
	}
	return master
}

// ThinkFollow advances one escort or loose-follow step toward target.
// clearDesire is true when the follow target is gone and the desire
// should be dropped.
func (h *Hostile) ThinkFollow(target attackable.Combatant, lastWasFollow bool) (clearDesire bool) {
	h.ForceRunStance()
	if h.DenyAIAction() || h.MovementDisabled() {
		return false
	}
	if target == nil || target.ObjectID() == h.ObjectID() || target.AlikeDead() {
		return true
	}
	if h.GeoPathFailCount() >= escortGeoFailLimit {
		h.move.Stop()
		h.teleportNear(target, escortTeleportOffset)
		h.ResetGeoPathFailCount()
		return false
	}
	master := h.Master()
	if master != nil && target.ObjectID() == master.ObjectID() {
		h.thinkMasterEscort(master, lastWasFollow)
		return false
	}
	h.thinkLooseFollow(target)
	return false
}

func (h *Hostile) thinkMasterEscort(master PartyMember, lastWasFollow bool) {
	h.partyMu.RLock()
	hasLast := h.hasLastFollow
	lastLoc := h.lastFollowingLoc
	h.partyMu.RUnlock()
	dest, masterLoc, ok := claimFollowSlot(master, h, lastWasFollow, hasLast, lastLoc)
	if !ok {
		return
	}
	h.moveTo(dest)
	h.partyMu.Lock()
	h.lastFollowingLoc = masterLoc
	h.hasLastFollow = true
	h.partyMu.Unlock()
}

// claimFollowSlot picks minion's escort point around master and records it
// in master.followSlots. The slots and the minion occupants are snapshotted
// under the master's partyMu; other occupants are resolved through the world and every
// position is read after unlock, so no world or actor lookup runs under the
// lock. The result is written back with a re-check (commitFollowSlots).
// masterLoc is the master position the escort points were laid out around.
func claimFollowSlot(master PartyMember, minion *Hostile, lastWasFollow, hasLast bool, lastLoc location.Location) (dest, masterLoc location.Location, ok bool) {
	var occupants [escortSlotCount]PartyMember
	p := master.links()
	p.partyMu.RLock()
	slots := p.followSlots
	filled := 0
	for i, id := range slots {
		if id != 0 {
			filled++
			occupants[i] = p.minions[id]
		}
	}
	allFilled := filled == len(p.minions)
	p.partyMu.RUnlock()

	if allFilled && hasLast && combatantLocation(master).Distance2D(lastLoc) < escortMasterMoveSkip {
		return location.Location{}, location.Location{}, false
	}
	if minion.roll(100) >= 70 {
		return location.Location{}, location.Location{}, false
	}

	masterLoc = combatantLocation(master)
	rndNum := minion.roll(1000000)
	slotHolder := -1
	distHolder := 10000.0
	finalLoc := minion.location()

	claimed := slots
	for i := 0; i < escortSlotCount; i++ {
		idx := (i + rndNum) % escortSlotCount
		if !lastWasFollow {
			claimed[idx] = 0
		}
		tmpX := math.Cos(escortSlotAngle*float64(idx)) * escortFollowDistance
		tmpY := math.Sin(escortSlotAngle*float64(idx)) * escortFollowDistance
		newPos := location.Location{
			X: masterLoc.X + int(tmpX),
			Y: masterLoc.Y + int(tmpY),
			Z: masterLoc.Z,
		}
		objectID := claimed[idx]
		if objectID != 0 {
			if objectID == minion.ObjectID() {
				claimed[idx] = 0
			} else if occupant := minion.slotOccupant(occupants[idx], objectID); occupant != nil && combatantLocation(occupant).Distance2D(newPos) <= escortStayRadius {
				continue
			}
		}
		distanceToNewPos := minion.location().Distance2D(newPos)
		if distHolder > distanceToNewPos {
			distHolder = distanceToNewPos
			slotHolder = idx
			finalLoc = newPos
		}
	}
	if slotHolder != -1 {
		claimed[slotHolder] = minion.ObjectID()
	}
	p.commitFollowSlots(slots, claimed)

	mx, my, _ := minion.Position()
	heading := int((math.Atan2(float64(my-masterLoc.Y), float64(mx-masterLoc.X))*360.0/(2*math.Pi) + 360.0)) % 360
	newSlot := (heading + 22) / 45
	distBetween := int(minion.location().Distance3D(masterLoc))
	if escortFollowDistance > distBetween && newSlot == slotHolder {
		finalLoc = minion.location()
	}
	return finalLoc, masterLoc, true
}

// commitFollowSlots writes back each slot a claim changed from its snapshot,
// unless another minion changed that slot since: the other minion's write
// stands, as it would under per-slot writes with no lock across the scan.
func (p *partyLinks) commitFollowSlots(snapshot, claimed [escortSlotCount]int32) {
	p.partyMu.Lock()
	defer p.partyMu.Unlock()
	for i, id := range claimed {
		if id != snapshot[i] && p.followSlots[i] == snapshot[i] {
			p.followSlots[i] = id
		}
	}
}

// slotOccupant resolves the NPC holding slot id of h's master: occupant
// when the snapshot found it among the master's privates, otherwise
// whatever the world holds.
func (h *Hostile) slotOccupant(occupant PartyMember, id int32) PartyMember {
	if occupant != nil {
		return occupant
	}
	if h.world == nil {
		return nil
	}
	obj, ok := h.world.Object(id)
	if !ok {
		return nil
	}
	found, _ := obj.(PartyMember)
	return found
}

func (h *Hostile) thinkLooseFollow(target attackable.Combatant) {
	pos := combatantLocation(target)
	if h.IsMoving() {
		return
	}
	if h.location().Distance2D(pos) <= float64(escortLooseRadius) {
		return
	}
	if h.roll(100) > 50 {
		distance := math.Sqrt(float64(h.roll(1000000))/1000000) * 300
		angle := float64(h.roll(1000000)) / 1000000 * math.Pi * 2
		h.moveTo(location.Location{
			X: int(distance*math.Cos(angle)) + pos.X,
			Y: int(distance*math.Sin(angle)) + pos.Y,
			Z: pos.Z,
		})
	}
}

func (h *Hostile) teleportNear(target attackable.Combatant, offset int) {
	pos := combatantLocation(target)
	if offset > 0 {
		nx := pos.X + rnd.GetRange(-offset, offset)
		ny := pos.Y + rnd.GetRange(-offset, offset)
		valid := h.ValidLocation(pos.X, pos.Y, pos.Z, nx, ny, pos.Z)
		pos.X, pos.Y = valid.X, valid.Y
	}
	h.TeleportTo(pos)
}

func (h *Hostile) moveTo(dest location.Location) {
	_, _ = h.move.MoveToLocation(dest)
}

func combatantLocation(target attackable.Combatant) location.Location {
	x, y, z := target.Position()
	return location.Location{X: x, Y: y, Z: z}
}
