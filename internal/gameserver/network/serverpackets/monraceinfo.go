package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// OpcodeMonRaceInfo is the wire opcode for MonRaceInfo, the monster race
// track's race state.
const OpcodeMonRaceInfo = 0xdd

// Monster race track geometry: every lane starts at monRaceStartX and ends
// at monRaceEndX, on the lane's own y, all at monRaceZ.
const (
	monRaceStartX = 14107
	monRaceEndX   = 12080
	monRaceLaneY  = 181875
	monRaceLaneDY = 58
	monRaceZ      = -3566
)

// FrameMonRaceInfo builds the MonRaceInfo packet of race: its phase codes,
// then each lane's runner, path, body and segment speeds. The speeds are
// written only for a race whose first code is 0, zeros otherwise.
func FrameMonRaceInfo(race event.DerbyRace) wire.Frame {
	w := newFrameWriter(OpcodeMonRaceInfo)
	w.WriteInt32(race.Code1)
	w.WriteInt32(race.Code2)
	w.WriteInt32(event.DerbyLanes)
	for i, r := range race.Runners {
		y := int32(monRaceLaneY + monRaceLaneDY*(event.DerbyLanes-1-i))
		w.WriteInt32(r.ObjectID)
		w.WriteInt32(int32(r.NpcID + 1000000))
		w.WriteInt32(monRaceStartX)
		w.WriteInt32(y)
		w.WriteInt32(monRaceZ)
		w.WriteInt32(monRaceEndX)
		w.WriteInt32(y)
		w.WriteInt32(monRaceZ)
		w.WriteFloat64(r.CollisionHeight)
		w.WriteFloat64(r.CollisionRadius)
		w.WriteInt32(120)
		for _, speed := range race.Speeds[i] {
			if race.Code1 == 0 {
				w.WriteUint8(speed)
			} else {
				w.WriteUint8(0)
			}
		}
		w.WriteInt32(0)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
