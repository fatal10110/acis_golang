package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameCharSelected(t *testing.T) {
	c := &player.Character{
		ObjectID: 0x10000001,
		Name:     "Newbie",
		Title:    "Hero",
		ClanID:   5,
		Sex:      player.SexMale,
		Race:     player.RaceHuman,
		ClassID:  0,
		Position: location.Location{X: 10, Y: 20, Z: 30},
		CurHP:    75, CurMP: 30,
		SP: 7, Exp: 12345, Level: 3,
		Karma: 1, PKKills: 2,
	}
	tmpl := &player.Template{STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25}

	got := framePayload(t, FrameCharSelected(CharSelectedSnapshot{Character: c, Template: tmpl, SessionID: 999}))

	want := []byte{OpcodeCharSelected}
	want = append(want, encodeUTF16Z(c.Name)...)
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ObjectID))
	want = append(want, encodeUTF16Z(c.Title)...)
	want = binary.LittleEndian.AppendUint32(want, 999) // session id
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClanID))
	want = binary.LittleEndian.AppendUint32(want, 0) // unknown

	want = binary.LittleEndian.AppendUint32(want, uint32(c.Sex))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Race))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID))

	want = binary.LittleEndian.AppendUint32(want, 1)

	want = binary.LittleEndian.AppendUint32(want, uint32(c.Position.X))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Position.Y))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Position.Z))
	want = binary.LittleEndian.AppendUint64(want, math.Float64bits(c.CurHP))
	want = binary.LittleEndian.AppendUint64(want, math.Float64bits(c.CurMP))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.SP))
	want = binary.LittleEndian.AppendUint64(want, uint64(c.Exp))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Level))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Karma))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.PKKills))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.INT))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.STR))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.CON))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.MEN))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.DEX))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.WIT))

	for i := 0; i < 32; i++ { // 30 padding zeros + 2 reserved
		want = binary.LittleEndian.AppendUint32(want, 0)
	}
	want = binary.LittleEndian.AppendUint32(want, 0) // game time
	want = binary.LittleEndian.AppendUint32(want, 0) // reserved
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID))
	for i := 0; i < 4; i++ {
		want = binary.LittleEndian.AppendUint32(want, 0)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharSelected mismatch:\n got  % x\n want % x", got, want)
	}
}
