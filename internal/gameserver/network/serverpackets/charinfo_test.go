package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameCharInfoCoreFields(t *testing.T) {
	c := &player.Character{
		ID: 0x10000001, Name: "Observer", ClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		Location: location.Location{X: 10, Y: 20, Z: 30},
		Heading:  123,
	}
	tmpl := &player.Template{CollisionRadius: 9, CollisionHeight: 23, RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50}
	items := []*item.Instance{{ObjectID: 100, TemplateID: 2369, Location: item.LocationPaperdoll, LocationData: rhandPaperdollIndex}}

	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl, Items: items}))
	if got[0] != OpcodeCharInfo {
		t.Fatalf("opcode = %#x, want %#x", got[0], OpcodeCharInfo)
	}

	offset := 1
	for _, want := range []uint32{10, 20, 30, 0, uint32(c.ObjectID())} {
		if v := binary.LittleEndian.Uint32(got[offset:]); v != want {
			t.Fatalf("field at offset %d = %d, want %d", offset, v, want)
		}
		offset += 4
	}

	// Skip UTF-16 name, race, sex, class id; the first 12 equipment template
	// ids follow. RHAND is the third entry in CharInfo's paperdoll order.
	for got[offset] != 0 || got[offset+1] != 0 {
		offset += 2
	}
	offset += 2 + 4 + 4 + 4
	if v := binary.LittleEndian.Uint32(got[offset+2*4:]); v != 2369 {
		t.Fatalf("right-hand template id = %d, want 2369", v)
	}
}

func TestFrameCharInfoUsesDoublePrecisionFloatFields(t *testing.T) {
	c := &player.Character{
		ID: 0x10000001, Name: "Observer", ClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		Location: location.Location{X: 10, Y: 20, Z: 30},
	}
	tmpl := &player.Template{
		CollisionRadius: 9, CollisionHeight: 23,
		RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
	}

	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl}))
	want := appendF64(nil, 1)
	want = appendF64(want, 1)
	want = appendF64(want, tmpl.CollisionRadius)
	want = appendF64(want, tmpl.CollisionHeight)
	if !bytes.Contains(got, want) {
		t.Fatalf("CharInfo missing double-width movement/collision block %x", want)
	}
}
