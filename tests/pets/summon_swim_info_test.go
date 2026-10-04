package pets

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// petInfoMoveTypeFromEnd is how far from the end of a PetInfo payload its
// move type byte sits: it is followed by a uint16, the team byte and the
// soulshot and spiritshot counts.
const petInfoMoveTypeFromEnd = 1 + 2 + 1 + 4 + 4

// npcInfoMoveTypeFromEnd is how far from the end of an NpcInfo payload its
// move type byte sits: it is followed by the team byte, the two collision
// sizes, the enchant effect and the flying flag.
const npcInfoMoveTypeFromEnd = 1 + 1 + 8 + 8 + 4 + 4

// lastFrame returns the last frame in frames with opcode op, nil if none.
func lastFrame(frames [][]byte, op byte, keep func([]byte) bool) []byte {
	var out []byte
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == op && keep(frame) {
			out = frame
		}
	}
	return out
}

// TestSummonInWaterShowsSwimming pins the summon's move type in its client
// views (PetInfo.java:105 for its owner, AbstractNpcInfo.java:296 for anyone
// else, both getMove().getMoveType()): a pet standing in water is shown
// swimming, and one on dry land on the ground. The owner's spawn PetInfo is
// not judged: the reference sends it from the known-list update that comes
// before the spawn's zone revalidation (Creature.setRegion), so the owner
// reads the water from the next refresh, here a rename's.
func TestSummonInWaterShowsSwimming(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		water bool
		want  byte
	}{
		{name: "in water", water: true, want: byte(move.MoveSwim)},
		{name: "on land", water: false, want: byte(move.MoveGround)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			zones := zone.NewIndex()
			if tc.water {
				// The water covers the wolf's spawn point, not its owner.
				zones.Add(zone.NewWater(1, summonZoneBox(t, 30, 1_000)))
			}
			h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithZones(zones)})
			wolf, _ := h.spawnWolf(t)
			if got := wolf.InsideZone(zone.FlagWater); got != tc.water {
				t.Fatalf("wolf InsideZone(water) = %v, want %v", got, tc.water)
			}

			info := lastFrame(renameTo(t, h, "Fenrir"), serverpackets.OpcodePetInfo, func([]byte) bool { return true })
			if info == nil {
				t.Fatal("the rename sent no PetInfo")
			}
			if got := info[len(info)-petInfoMoveTypeFromEnd]; got != tc.want {
				t.Fatalf("PetInfo move type = %d, want %d", got, tc.want)
			}

			seen := lastFrame(enterWorldSeeing(t, h.srv, "viewer", "Viewer"), serverpackets.OpcodeNPCInfo, func(frame []byte) bool {
				return len(frame) > 4 && int32(binary.LittleEndian.Uint32(frame[1:5])) == wolf.ObjectID()
			})
			if seen == nil {
				t.Fatal("a viewer entering the world saw no NpcInfo of the wolf")
			}
			if got := seen[len(seen)-npcInfoMoveTypeFromEnd]; got != tc.want {
				t.Fatalf("viewer's NpcInfo move type = %d, want %d", got, tc.want)
			}
		})
	}
}
