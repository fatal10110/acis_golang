package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// AssertSpawnTeleport checks that frames, an enter-world burst, carries
// the TeleportToLocation of a zone rule moving objectID out as it spawns:
// after EtcStatusUpdate, ahead of WELCOME_TO_LINEAGE, with the burst's
// UserInfo already at the destination. It returns the destination.
func AssertSpawnTeleport(tb testing.TB, frames [][]byte, objectID int32) location.Location {
	tb.Helper()
	etc, tele, welcome, info := -1, -1, -1, -1
	var to location.Location
	for i, f := range frames {
		if len(f) == 0 {
			continue
		}
		r := wire.NewReader(f[1:])
		switch f[0] {
		case serverpackets.OpcodeEtcStatusUpdate:
			if etc < 0 {
				etc = i
			}
		case serverpackets.OpcodeTeleportToLocation:
			if tele < 0 && r.ReadInt32() == objectID {
				tele = i
				to = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
			}
		case serverpackets.OpcodeSystemMessage:
			if welcome < 0 && r.ReadInt32() == serverpackets.SystemMessageWelcomeToLineage {
				welcome = i
			}
		case serverpackets.OpcodeUserInfo:
			if info < 0 {
				info = i
				at := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
				if tele >= 0 && at != to {
					tb.Fatalf("enter-burst UserInfo at %+v, want the teleport destination %+v", at, to)
				}
			}
		}
	}
	if tele < 0 {
		tb.Fatalf("no TeleportToLocation for %d in the enter burst", objectID)
	}
	if etc < 0 || welcome < 0 || info < 0 || !(etc < tele && tele < welcome && welcome < info) {
		tb.Fatalf("enter burst order: EtcStatusUpdate %d, TeleportToLocation %d, WELCOME_TO_LINEAGE %d, UserInfo %d; want ascending", etc, tele, welcome, info)
	}
	return to
}
