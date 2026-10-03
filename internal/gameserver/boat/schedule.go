package boat

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// announcement is one step of a departure schedule: the system messages
// announced together, then how many seconds pass before the next step.
type announcement struct {
	messages []int
	delay    int
}

// leg is one direction of an itinerary: the route a boat sails when it
// leaves dock, the departure schedule announced while it waits there, and
// the line announced while it waits for its destination dock to be free.
type leg struct {
	dock     route.Dock
	itemID   int
	path     []route.BoatLocation
	busy     int
	schedule []announcement
}

var errEmptyRoute = errors.New("boat: route has no nodes")

// newLeg builds the leg leaving r.Dock, marking that dock held when it is
// exclusive.
//
// The departure schedule gathers every node's scheduled messages in route
// order, one step per delay. A message whose delay an earlier step already
// has joins that step, and the schedule ends there: no later message of the
// route is announced. Two shipped schedules (Giran's and Innadril's) list a
// second message under an existing delay and lose their last steps to this.
func newLeg(r route.BoatRoute, d *docks) (leg, error) {
	if len(r.Nodes) == 0 {
		return leg{}, errEmptyRoute
	}
	l := leg{
		dock:   r.Dock,
		itemID: r.ItemID,
		path:   r.Nodes,
		busy:   r.Nodes[len(r.Nodes)-1].BusyMessage,
	}
	d.setBusy(r.Dock, true)
	l.schedule = buildSchedule(r.Nodes)
	return l, nil
}

func buildSchedule(nodes []route.BoatLocation) []announcement {
	var out []announcement
	for _, node := range nodes {
		for _, msg := range node.Scheduled {
			for i := range out {
				if out[i].delay == msg.Delay {
					out[i].messages = append(out[i].messages, msg.ID)
					return out
				}
			}
			out = append(out, announcement{messages: []int{msg.ID}, delay: msg.Delay})
		}
	}
	return out
}
