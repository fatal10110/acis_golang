package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ownerSystemMessages is every system message id among frames.
func ownerSystemMessages(frames [][]byte) []int32 {
	var ids []int32
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, wire.NewReader(f[1:]).ReadInt32())
		}
	}
	return ids
}

// TestPetAttackRefusalTellsOwner commands a pet to attack a target its
// attack think's gate refuses: a monster the geodata hides answers
// CANT_SEE_TARGET (181), and a karma player while a peace zone holds the pet
// answers CANT_ATK_PEACEZONE (84). The pet relays the message to its owner
// once, and the pet never swings: the think goes idle rather than retrying.
func TestPetAttackRefusalTellsOwner(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		player  bool
		wantMsg int32
	}{
		{name: "hidden monster", wantMsg: serverpackets.SystemMessageCantSeeTarget},
		{name: "player from a peace zone", player: true, wantMsg: serverpackets.SystemMessageCantAtkPeacezone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var opts []gameservertest.Option
			if tc.player {
				zones := zone.NewIndex()
				zones.Add(zone.NewPeace(1, wholeWorldForm(t)))
				opts = append(opts, gameservertest.WithZones(zones))
			} else {
				geo := &sightGeo{}
				geo.blind.Store(true)
				opts = append(opts, gameservertest.WithGeo(geo))
			}
			h := bootOwnerWithCollarOpts(t, opts)
			petActor, _ := h.spawnWolf(t)

			var targetID int32
			var x, y, z int32
			if tc.player {
				victim := h.joinKarmaPlayer(t, "Pk", 500)
				targetID = victim.id
				vx, vy, vz := victim.actor.Position()
				x, y, z = int32(vx), int32(vy), int32(vz)
			} else {
				hostile := h.srv.SpawnHostileNPC(t)
				targetID = hostile.ObjectID()
				x, y, z = hostileX, hostileY, hostileZ
			}
			drainUntilQuiet(t, h.client)

			h.client.Send(encodeAction(targetID, x, y, z, false))
			drainFrames(t, h.client)
			h.client.Send(encodeRequestActionUse(petAttackAction, false))

			frames := advanceCollecting(t, h, 3*time.Second)
			if n := petAttacks(frames, petActor); n != 0 {
				t.Fatalf("pet swings = %d, want none", n)
			}
			var told int
			for _, id := range ownerSystemMessages(frames) {
				if id == tc.wantMsg {
					told++
				}
			}
			if told != 1 {
				t.Fatalf("owner system messages = %v, want %d exactly once", ownerSystemMessages(frames), tc.wantMsg)
			}
		})
	}
}
