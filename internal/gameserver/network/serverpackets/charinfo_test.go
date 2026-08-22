package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

func TestFrameCharInfoCoreFields(t *testing.T) {
	c := &player.Character{
		ID: 0x10000001, Name: "Observer", ClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		Location:    location.Location{X: 10, Y: 20, Z: 30},
		LastHeading: 123,
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

func TestFrameCharInfoMirrorsPvPFlag(t *testing.T) {
	c := &player.Character{Name: "Observer"}
	c.UpdatePvPFlag(task.PvPFlagOn)

	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: &player.Template{}}))
	offset := 1 + 5*4 + (len(c.Name)+1)*2 + 3*4 + len(charInfoPaperdollOrder)*4 + 4*2 + 4 + 12*2 + 4 + 4*2
	for _, fieldOffset := range []int{offset, offset + 4*4} {
		if v := binary.LittleEndian.Uint32(got[fieldOffset:]); v != uint32(task.PvPFlagOn) {
			t.Fatalf("PvP flag at offset %d = %d, want %d", fieldOffset, v, task.PvPFlagOn)
		}
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

func TestFrameCharInfo_CubicsSerializeCountAndIDs(t *testing.T) {
	tmpl := &player.Template{}
	empty := &player.Character{Name: "C"}
	withCubics := &player.Character{Name: "C"}
	withCubics.SetSkillLevel(143, 5) // Cubic Mastery: room for more than one cubic
	withCubics.AddOrRefreshCubic(1, false)
	withCubics.AddOrRefreshCubic(3, false)

	base := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: empty, Template: tmpl}))
	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: withCubics, Template: tmpl}))

	// The cubic field is the only difference between the two encodings, so
	// its exact position is the point the two payloads diverge, and the
	// bytes after it must resync with base once the field is skipped.
	prefixLen := 0
	for prefixLen < len(base) && prefixLen < len(got) && base[prefixLen] == got[prefixLen] {
		prefixLen++
	}
	want := binary.LittleEndian.AppendUint16(nil, 2)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 3)
	if prefixLen+len(want) > len(got) || !bytes.Equal(got[prefixLen:prefixLen+len(want)], want) {
		t.Fatalf("cubic field at offset %d = % x, want count 2 followed by ids [1 3] (% x)", prefixLen, got[prefixLen:], want)
	}
	if suffix := got[prefixLen+len(want):]; !bytes.Equal(suffix, base[prefixLen+2:]) {
		t.Fatalf("bytes after cubic field don't resync with the no-cubic encoding: got %x, want %x", suffix, base[prefixLen+2:])
	}
}

func TestFrameCharInfoCarriesAbnormalEffectMask(t *testing.T) {
	tmpl := &player.Template{}
	plain := &player.Character{Name: "C"}
	bigHead := &player.Character{Name: "C"}
	bigHead.StartAbnormalEffect(0x002000)

	base := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: plain, Template: tmpl}))
	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: bigHead, Template: tmpl}))

	// Same-length encodings that must differ in exactly the abnormal-effect
	// int32 field, and resync everywhere else.
	if len(base) != len(got) {
		t.Fatalf("payload length changed: base %d, got %d", len(base), len(got))
	}
	prefixLen := 0
	for prefixLen < len(base) && base[prefixLen] == got[prefixLen] {
		prefixLen++
	}
	// 0x002000's only non-zero byte is its little-endian byte 1, so the diff
	// starts one byte into the field.
	fieldStart := prefixLen - 1
	if v := binary.LittleEndian.Uint32(got[fieldStart:]); v != 0x002000 {
		t.Fatalf("abnormal effect field at offset %d = %#x, want %#x", fieldStart, v, 0x002000)
	}
	if suffix := got[fieldStart+4:]; !bytes.Equal(suffix, base[fieldStart+4:]) {
		t.Fatalf("bytes after the abnormal effect field don't resync: got %x, want %x", suffix, base[fieldStart+4:])
	}
}
