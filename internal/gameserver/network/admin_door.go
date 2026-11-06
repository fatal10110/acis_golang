package network

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// adminOpen answers //open; see adminDoor.
func (l *GameClientLink) adminOpen(gm *livePlayer, line string) {
	l.adminDoor(gm, line, "admin_open", true)
}

// adminClose answers //close; see adminDoor.
func (l *GameClientLink) adminClose(gm *livePlayer, line string) {
	l.adminDoor(gm, line, "admin_close", false)
}

// adminDoor opens or closes, as open says, the door whose id follows word
// and one separator in line, or, when no number follows, the GM's selected
// door. The change is any door's change: a broken door, or one already so,
// stays as it is. A number naming no door, or a selection that is no door,
// is an invalid target.
func (l *GameClientLink) adminDoor(gm *livePlayer, line, word string, open bool) {
	var obj world.Tracked
	id, err := strconv.ParseInt(line[min(len(word)+1, len(line)):], 10, 32)
	switch {
	case err != nil:
		obj = gm.Target()
	case l.doors != nil:
		if d, ok := l.doors.Door(int(id)); ok {
			obj = d
		}
	}
	d, ok := obj.(*door.Object)
	if !ok || l.doors == nil {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	l.doors.SetDoorOpen(d.DoorID(), open)
}
