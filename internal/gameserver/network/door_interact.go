package network

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// actOnDoor answers a click on an already-selected door and reports
// whether target was one. A door live may attack is attacked, forced or
// not; any other door is interacted with.
func (l *GameClientLink) actOnDoor(live *livePlayer, target world.Tracked, shift bool) bool {
	d, ok := target.(*door.Object)
	if !ok || live == nil {
		return false
	}
	if d.AttackableBy(live.Character) {
		l.attackLiveTarget(live, d, shift)
		return true
	}
	l.tryToInteract(live, d, shift)
	return true
}

// interactDoor is a door's interact: a member of the clan owning the hall
// the door belongs to, holding the hall's entry rights, is asked whether to
// open the door, or to close it when it stands open, and the door is kept
// as the one the answer acts on. Anyone else gets nothing.
func (l *GameClientLink) interactDoor(live *livePlayer, d *door.Object) {
	if !l.opensDoorByHand(live, d) {
		return
	}
	live.requestedGate.Store(d)
	live.SendFrame(serverpackets.FrameConfirmDlgGate(!d.Opened()))
}

// answerGate is the answer to the open (open set) or close question of the
// requested door. A yes opens or closes it when the player still has it
// selected and may still open it by hand; any answer drops the request.
func (l *GameClientLink) answerGate(live *livePlayer, answer int32, open bool) {
	d := live.requestedGate.Swap(nil)
	if d == nil {
		return
	}
	if answer != 1 || live.Target() != world.Tracked(d) || !l.opensDoorByHand(live, d) || l.doors == nil {
		return
	}
	l.doors.SetDoorOpen(d.DoorID(), open)
}

// opensDoorByHand reports whether live may open or close door d by hand:
// d is a clan hall's door, live's clan owns that hall, and live holds the
// hall's entry rights.
func (l *GameClientLink) opensDoorByHand(live *livePlayer, d *door.Object) bool {
	hall, ok := l.hallOfDoor(d.Template.Name)
	if !ok {
		return false
	}
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return false
	}
	v, ok := l.halls.View(int32(hall.ID))
	return ok && v.OwnerID == cl.ID() && cl.HasPrivilege(live.ObjectID(), clan.PrivHallEntryExit)
}

// hallOfDoor returns the clan hall whose gates list the door called name.
func (l *GameClientLink) hallOfDoor(name string) (*hallmodel.Hall, bool) {
	for _, h := range l.clanHallData.All() {
		if slices.Contains(h.Gates, name) {
			return h, true
		}
	}
	return nil, false
}
