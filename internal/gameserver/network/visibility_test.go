package network

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestLivePlayerVisibilitySendsCharInfoAndDeleteObject(t *testing.T) {
	state := world.New()
	firstFrames := &frameCapture{}
	secondFrames := &frameCapture{}
	first := newTestLivePlayer(t, 1, firstFrames)
	second := newTestLivePlayer(t, 2, secondFrames)

	state.Spawn(first, 0, 0, 0, 0)
	state.Spawn(second, 100, 0, 0, 0)

	if len(firstFrames.frames) != 1 || firstFrames.frames[0][0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("first player frames = %x, want one CharInfo", firstFrames.frames)
	}
	if len(secondFrames.frames) != 1 || secondFrames.frames[0][0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("second player frames = %x, want one CharInfo", secondFrames.frames)
	}

	state.Despawn(second)
	if got := firstFrames.frames[len(firstFrames.frames)-1][0]; got != serverpackets.OpcodeDeleteObject {
		t.Fatalf("last first-player frame opcode = %#x, want DeleteObject (%#x)", got, serverpackets.OpcodeDeleteObject)
	}
}

func TestLivePlayerVisibilityRendersSupportedWorldObjectsSymmetrically(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	viewer := newTestLivePlayer(t, 1, frames)
	state.Spawn(viewer, 0, 0, 0, 0)

	ground := &visibleGroundItem{id: 10, itemID: 57, count: 3, stackable: true}
	door := &visibleDoor{id: 11, doorID: 100}
	static := &visibleStaticObject{id: 12, staticID: 200}
	invisible := &invisibleTracked{id: 13}

	state.Spawn(ground, 100, 0, 0, 0)
	state.Spawn(door, 200, 0, 0, 0)
	state.Spawn(static, 300, 0, 0, 0)
	state.Spawn(invisible, 400, 0, 0, 0)

	want := []byte{
		serverpackets.OpcodeSpawnItem,
		serverpackets.OpcodeDoorInfo,
		serverpackets.OpcodeStaticObjectInfo,
	}
	if got := frameOpcodes(frames.frames); string(got) != string(want) {
		t.Fatalf("spawn opcodes = %x, want %x", got, want)
	}

	state.Despawn(invisible)
	if got := frameOpcodes(frames.frames); string(got) != string(want) {
		t.Fatalf("opcodes after despawning unsupported object = %x, want still %x", got, want)
	}

	state.Despawn(static)
	state.Despawn(door)
	state.Despawn(ground)
	want = append(want,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeDeleteObject,
	)
	if got := frameOpcodes(frames.frames); string(got) != string(want) {
		t.Fatalf("despawn opcodes = %x, want %x", got, want)
	}
}

func TestLivePlayerVisibilityRendersHostileNPC(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	viewer := newTestLivePlayer(t, 1, frames)
	state.Spawn(viewer, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 20)
	state.Spawn(hostile, 100, 0, -50, 123)

	const opcodeNPCInfo = 0x16
	if len(frames.frames) != 1 {
		t.Fatalf("frames = %x, want one NPCInfo frame", frames.frames)
	}
	got := frames.frames[0]
	appendInt32 := func(b []byte, v int32) []byte {
		return binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	wantPrefix := []byte{opcodeNPCInfo}
	wantPrefix = appendInt32(wantPrefix, 20)
	wantPrefix = appendInt32(wantPrefix, 1000100)
	wantPrefix = appendInt32(wantPrefix, 1)
	wantPrefix = appendInt32(wantPrefix, 100)
	wantPrefix = appendInt32(wantPrefix, 0)
	wantPrefix = appendInt32(wantPrefix, -50)
	wantPrefix = appendInt32(wantPrefix, 123)
	if len(got) < len(wantPrefix) || string(got[:len(wantPrefix)]) != string(wantPrefix) {
		t.Fatalf("NPCInfo prefix = % x, want % x", got[:min(len(got), len(wantPrefix))], wantPrefix)
	}

	state.Despawn(hostile)
	if len(frames.frames) != 2 || frames.frames[1][0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("frames after NPC despawn = %x, want DeleteObject after NPCInfo", frames.frames)
	}
}

func TestLivePlayerForgetSkipsObjectsItWouldNotDiscover(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	player := newTestLivePlayer(t, 1, frames)
	obj := &invisibleTracked{id: 2}

	state.Spawn(player, 0, 0, 0, 0)
	state.Spawn(obj, 100, 0, 0, 0)
	state.Despawn(obj)

	if len(frames.frames) != 0 {
		t.Fatalf("frames for non-live tracked object = %x, want none", frames.frames)
	}
}
