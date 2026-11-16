package combat

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func encodeRequestCursedWeaponList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestCursedWeaponList)
	return w.Bytes()
}

// TestCursedWeaponListInHashTableOrder pins RequestCursedWeaponList: the
// weapons are listed in the order of a 16-bucket hash table, a bucket
// being the id's low bits folded with its high half, so Akamanah (8689,
// bucket 1) comes before Zariche (8190, bucket 14). With cursed weapons
// off the list is empty.
func TestCursedWeaponListInHashTableOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		boot func(t *testing.T) *gameservertest.Server
		want []int32
	}{
		{"on", func(t *testing.T) *gameservertest.Server {
			return cursedBoot(t, 10, gameservertest.WithCharacter("Newbie", 5, 0))
		}, []int32{akamanahID, zaricheID}},
		{"off", func(t *testing.T) *gameservertest.Server {
			return gameservertest.Boot(t, gameservertest.WithWantChars(1), gameservertest.WithCharacter("Newbie", 5, 0))
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := tc.boot(t)
			c := srv.Client
			startInWorld(t, c)
			drainUntilQuiet(t, c)

			c.Send(encodeRequestCursedWeaponList())
			frame := mustRead(t, c, "ExCursedWeaponList")
			r := wire.NewReader(frame[1:])
			if op := r.ReadUint16(); frame[0] != serverpackets.OpcodeExtended || op != serverpackets.OpcodeExCursedWeaponList {
				t.Fatalf("list answer %#x/%#x, want ExCursedWeaponList", frame[0], op)
			}
			var got []int32
			for n := r.ReadInt32(); n > 0; n-- {
				got = append(got, r.ReadInt32())
			}
			if err := r.Err(); err != nil || r.Remaining() != 0 {
				t.Fatalf("ExCursedWeaponList: err %v, %d bytes left", err, r.Remaining())
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("cursed weapon list = %v, want %v", got, tc.want)
			}
		})
	}
}
