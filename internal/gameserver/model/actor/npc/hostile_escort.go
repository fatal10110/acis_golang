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
// private that should escort while idle.
func (h *Hostile) IdleFollowTarget() attackable.Combatant {
	if h.aiInt("Party_Type", 0) != 1 {
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
		_ = h.move.Stop()
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

func (h *Hostile) thinkMasterEscort(master *Hostile, lastWasFollow bool) {
	h.minionsMu.RLock()
	hasLast := h.hasLastFollow
	lastLoc := h.lastFollowingLoc
	h.minionsMu.RUnlock()
	dest, ok := master.claimFollowSlot(h, lastWasFollow, hasLast, lastLoc)
	if !ok {
		return
	}
	h.moveTo(dest)
	h.minionsMu.Lock()
	h.lastFollowingLoc = master.location()
	h.hasLastFollow = true
	h.minionsMu.Unlock()
}

func (master *Hostile) claimFollowSlot(minion *Hostile, lastWasFollow, hasLast bool, lastLoc location.Location) (location.Location, bool) {
	master.minionsMu.Lock()
	defer master.minionsMu.Unlock()

	filled := 0
	for _, id := range master.followSlots {
		if id != 0 {
			filled++
		}
	}
	if filled == len(master.minions) && hasLast && master.location().Distance2D(lastLoc) < escortMasterMoveSkip {
		return location.Location{}, false
	}
	if minion.roll(100) >= 70 {
		return location.Location{}, false
	}

	masterLoc := master.location()
	rndNum := minion.roll(1000000)
	slotHolder := -1
	distHolder := 10000.0
	finalLoc := minion.location()

	for i := 0; i < escortSlotCount; i++ {
		idx := (i + rndNum) % escortSlotCount
		if !lastWasFollow {
			master.followSlots[idx] = 0
		}
		tmpX := math.Cos(escortSlotAngle*float64(idx)) * escortFollowDistance
		tmpY := math.Sin(escortSlotAngle*float64(idx)) * escortFollowDistance
		newPos := location.Location{
			X: masterLoc.X + int(tmpX),
			Y: masterLoc.Y + int(tmpY),
			Z: masterLoc.Z,
		}
		objectID := master.followSlots[idx]
		if objectID != 0 {
			if objectID == minion.ObjectID() {
				master.followSlots[idx] = 0
			} else if occupant := master.slotOccupant(objectID); occupant != nil && occupant.location().Distance2D(newPos) <= escortStayRadius {
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
		master.followSlots[slotHolder] = minion.ObjectID()
	}

	mx, my, _ := minion.Position()
	heading := int((math.Atan2(float64(my-masterLoc.Y), float64(mx-masterLoc.X))*360.0/(2*math.Pi) + 360.0)) % 360
	newSlot := (heading + 22) / 45
	distBetween := int(minion.location().Distance3D(masterLoc))
	if escortFollowDistance > distBetween && newSlot == slotHolder {
		finalLoc = minion.location()
	}
	return finalLoc, true
}

func (master *Hostile) slotOccupant(id int32) *Hostile {
	if minion := master.minions[id]; minion != nil {
		return minion
	}
	if master.world == nil {
		return nil
	}
	obj, ok := master.world.Object(id)
	if !ok {
		return nil
	}
	occupant, _ := obj.(*Hostile)
	return occupant
}

func (h *Hostile) thinkLooseFollow(target attackable.Combatant) {
	pos, ok := combatantLocation(target)
	if !ok || h.IsMoving() {
		return
	}
	if h.location().Distance2D(pos) <= float64(escortLooseRadius) {
		return
	}
	if rnd.Get(100) > 50 {
		distance := math.Sqrt(rnd.GetFloat(1)) * 300
		angle := rnd.GetFloat(1) * math.Pi * 2
		h.moveTo(location.Location{
			X: int(distance*math.Cos(angle)) + pos.X,
			Y: int(distance*math.Sin(angle)) + pos.Y,
			Z: pos.Z,
		})
	}
}

func (h *Hostile) teleportNear(target attackable.Combatant, offset int) {
	pos, ok := combatantLocation(target)
	if !ok {
		return
	}
	if offset > 0 {
		pos.X += rnd.GetRange(-offset, offset)
		pos.Y += rnd.GetRange(-offset, offset)
	}
	h.TeleportTo(pos)
}

func (h *Hostile) moveTo(dest location.Location) {
	mover, ok := h.move.(interface {
		MoveToLocation(location.Location) (bool, error)
	})
	if !ok {
		return
	}
	_, _ = mover.MoveToLocation(dest)
}

func combatantLocation(target attackable.Combatant) (location.Location, bool) {
	pos, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return location.Location{}, false
	}
	x, y, z := pos.Position()
	return location.Location{X: x, Y: y, Z: z}, true
}
