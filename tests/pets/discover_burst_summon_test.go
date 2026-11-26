package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// TestEnteringPlayerSeesOwnerThenSummonRelation pins the head of the burst a
// player is sent when it comes to know a player with a summon, as the
// reference's Player.sendInfo orders it: the owner's CharInfo, the owner's
// RelationChanged, then the summon's RelationChanged carrying the same
// relation. The owner's own client is sent the viewer's relation to it at
// the same step.
func TestEnteringPlayerSeesOwnerThenSummonRelation(t *testing.T) {
	t.Parallel()
	w := bootSummonCrest(t)
	servitor, _ := w.call(t)
	drainUntilQuiet(t, w.observer)
	drainUntilQuiet(t, w.client)

	lateID := w.srv.SeedCharacterFor(t, "player3", "Late", 1, 0).ID
	late := w.srv.DialClient(t, "player3", 1)
	late.Send(clientwire.RequestGameStart(0))
	late.Read() // SSQInfo
	late.Read() // CharSelected
	late.Send(clientwire.EnterWorld())
	w.srv.Settle(t)
	lateFrames := late.ReadQueued()
	ownerFrames := w.client.ReadQueued()

	infoAt := -1
	for i, f := range lateFrames {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		if objectIDOfCharInfo(f) == w.id {
			infoAt = i
			break
		}
	}
	if infoAt < 0 {
		t.Fatalf("late viewer frames carry no CharInfo of the owner")
	}
	if infoAt+2 >= len(lateFrames) ||
		!isRelationOf(lateFrames[infoAt+1], w.id) || !isRelationOf(lateFrames[infoAt+2], servitor.ObjectID()) {
		t.Fatalf("late viewer frames from the owner's CharInfo = %x, want CharInfo, the owner's RelationChanged, the summon's RelationChanged",
			opcodes(lateFrames[infoAt:]))
	}
	if at, _ := framesAbout(ownerFrames, serverpackets.OpcodeRelationChanged, lateID); len(at) != 2 {
		t.Fatalf("owner frames %x carry %d RelationChanged of the viewer, want 2 (one per discovery direction)", opcodes(ownerFrames), len(at))
	}
}

// objectIDOfCharInfo reads the object id a CharInfo carries after its
// position and heading.
func objectIDOfCharInfo(frame []byte) int32 {
	r := wire.NewReader(frame[1:])
	for range 4 {
		r.ReadInt32()
	}
	return r.ReadInt32()
}

func isRelationOf(frame []byte, objectID int32) bool {
	at, _ := framesAbout([][]byte{frame}, serverpackets.OpcodeRelationChanged, objectID)
	return len(at) == 1
}
