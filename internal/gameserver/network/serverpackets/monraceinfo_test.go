package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// monRaceInfoBytes lays race out field by field as the reference's
// MonRaceInfo writes it: the two codes, 8, then per lane the runner's
// object id, npc id + 1000000, start x/y/z, end x/y/z, collision height and
// radius as doubles, 120, the 20 speed bytes (zeros unless the first code
// is 0) and a closing 0.
func monRaceInfoBytes(race event.DerbyRace) []byte {
	var b bytes.Buffer
	d := func(v int32) { _ = binary.Write(&b, binary.LittleEndian, v) }
	f := func(v float64) { _ = binary.Write(&b, binary.LittleEndian, math.Float64bits(v)) }
	b.WriteByte(0xdd)
	d(race.Code1)
	d(race.Code2)
	d(8)
	for i, r := range race.Runners {
		y := int32(181875 + 58*(7-i))
		d(r.ObjectID)
		d(int32(r.NpcID) + 1000000)
		d(14107)
		d(y)
		d(-3566)
		d(12080)
		d(y)
		d(-3566)
		f(r.CollisionHeight)
		f(r.CollisionRadius)
		d(120)
		for j := range 20 {
			if race.Code1 == 0 {
				b.WriteByte(race.Speeds[i][j])
			} else {
				b.WriteByte(0)
			}
		}
		d(0)
	}
	return b.Bytes()
}

func TestFrameMonRaceInfo(t *testing.T) {
	var race event.DerbyRace
	for i := range race.Runners {
		race.Runners[i] = event.DerbyRunner{ObjectID: int32(0x10000000 + i), NpcID: 31003 + i, CollisionHeight: 16 + float64(i)/2, CollisionRadius: 9.5 - float64(i)}
		for j := range race.Speeds[i] {
			race.Speeds[i][j] = uint8(65 + i + j)
		}
		race.Speeds[i][19] = 100
	}
	for _, code := range [][2]int32{{-1, 0}, {0, 15322}, {13765, -1}} {
		race.Code1, race.Code2 = code[0], code[1]
		got := framePayload(t, FrameMonRaceInfo(race))
		if want := monRaceInfoBytes(race); !bytes.Equal(got, want) {
			t.Fatalf("FrameMonRaceInfo(code %v) =\n%x\nwant\n%x", code, got, want)
		}
		if len(got) != 1+12+8*(4*8+16+4+20+4) {
			t.Fatalf("FrameMonRaceInfo(code %v) is %d bytes", code, len(got))
		}
		if code[0] != 0 && bytes.Contains(got, []byte{65, 66, 67}) {
			t.Fatalf("FrameMonRaceInfo(code %v) carries the speeds", code)
		}
	}
}
