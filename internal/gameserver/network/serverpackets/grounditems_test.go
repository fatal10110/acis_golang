package serverpackets

import (
	"encoding/binary"
	"testing"
)

func TestFrameSpawnItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 57, count: 500, stackable: true, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameSpawnItem(ground))

	want := []byte{OpcodeSpawnItem}
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 57)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 500)
	want = binary.LittleEndian.AppendUint32(want, 0)
	if string(got) != string(want) {
		t.Fatalf("FrameSpawnItem() = % x, want % x", got, want)
	}
}

func TestFrameDropItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 10, count: 1, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameDropItem(ground, 200))

	want := []byte{OpcodeDropItem}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	if string(got) != string(want) {
		t.Fatalf("FrameDropItem() = % x, want % x", got, want)
	}
}

func TestFrameGetItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 57, count: 500, stackable: true, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameGetItem(ground, 200))

	want := []byte{OpcodeGetItem}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	if string(got) != string(want) {
		t.Fatalf("FrameGetItem() = % x, want % x", got, want)
	}
}

func appendInt32(b []byte, v int32) []byte {
	return binary.LittleEndian.AppendUint32(b, uint32(v))
}

type packetGroundItem struct {
	id, itemID int32
	count      int
	stackable  bool
	x, y, z    int
}

func (p packetGroundItem) ObjectID() int32 { return p.id }
func (p packetGroundItem) ItemID() int32   { return p.itemID }
func (p packetGroundItem) Count() int      { return p.count }
func (p packetGroundItem) Stackable() bool { return p.stackable }
func (p packetGroundItem) Position() (int, int, int) {
	return p.x, p.y, p.z
}
