package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestFrameServerObjectInfo(t *testing.T) {
	got := framePayload(t, FrameServerObjectInfo(NPCInfoSnapshot{
		ObjectID: 0x01020304, TemplateID: 123, Name: "Goblin", Attackable: true,
		X: -1, Y: 2, Z: -3, Heading: 4, CollisionRadius: 5.5, CollisionHeight: 6.5,
		CurrentHP: 70, MaxHP: 100,
	}))
	want := []byte{OpcodeServerObjectInfo}
	for _, value := range []uint32{0x01020304, 1000123} {
		want = binary.LittleEndian.AppendUint32(want, value)
	}
	for _, char := range []uint16{'G', 'o', 'b', 'l', 'i', 'n', 0} {
		want = binary.LittleEndian.AppendUint16(want, char)
	}
	for _, value := range []uint32{1, 0xffffffff, 2, 0xfffffffd, 4} {
		want = binary.LittleEndian.AppendUint32(want, value)
	}
	for _, value := range []float64{1, 1, 5.5, 6.5} {
		want = binary.LittleEndian.AppendUint64(want, math.Float64bits(value))
	}
	for _, value := range []uint32{70, 100, 1, 0} {
		want = binary.LittleEndian.AppendUint32(want, value)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameServerObjectInfo() = %x, want %x", got, want)
	}
}

func TestFrameNPCInfoWritesPvpFlagAndKarma(t *testing.T) {
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{
		Name: "N", Title: "T", Summon: true, PvpFlag: 1, Karma: 500,
	}))
	fields := []byte{'N', 0, 0, 0, 'T', 0, 0, 0}
	offset := bytes.Index(payload, fields)
	if offset < 0 {
		t.Fatal("name/title fields missing")
	}
	got := payload[offset+len(fields):]
	if len(got) < 12 || binary.LittleEndian.Uint32(got[:4]) != 1 || binary.LittleEndian.Uint32(got[4:]) != 1 || binary.LittleEndian.Uint32(got[8:]) != 500 {
		t.Fatalf("summon/pvp/karma fields = %x, want 1/1/500", got)
	}
}

func TestFrameNPCInfoWritesAbnormalEffect(t *testing.T) {
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{Name: "N", Title: "T", AbnormalEffect: 0x010000}))
	fields := []byte{'N', 0, 0, 0, 'T', 0, 0, 0}
	offset := bytes.Index(payload, fields)
	if offset < 0 {
		t.Fatal("name/title fields missing")
	}
	got := payload[offset+len(fields):]
	if len(got) < 16 || binary.LittleEndian.Uint32(got[12:16]) != 0x010000 {
		t.Fatalf("abnormal effect = %x, want 01000000", got)
	}
}
