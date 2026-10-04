package admin

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/fence"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// fenceExchange sends payload and returns every frame c drew before the
// reply to a manor-list barrier sent after it. ExColosseumFenceInfo shares
// the extended opcode with that reply, so the barrier is told apart by its
// sub-opcode.
func fenceExchange(t *testing.T, c *testsupport.ScriptedClient, payload []byte) [][]byte {
	t.Helper()
	if payload != nil {
		c.Send(payload)
	}
	c.Send(encodeManorBarrier())
	var frames [][]byte
	for range 100 {
		frame := c.Read()
		if len(frame) >= 3 && frame[0] == serverpackets.OpcodeExtended && binary.LittleEndian.Uint16(frame[1:3]) == serverpackets.OpcodeExSendManorList {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("no manor-list barrier reply within 100 frames")
	return nil
}

// fenceInfo is ExColosseumFenceInfo as ExColosseumFenceInfo.java writes it:
// 0xfe, sub-opcode 0x0009 (H), then object id, type, x, y, z, width and
// length (D each).
func fenceInfo(id, typ, x, y, z, sizeX, sizeY int32) []byte {
	b := []byte{0xfe, 0x09, 0x00}
	for _, v := range []int32{id, typ, x, y, z, sizeX, sizeY} {
		b = binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	return b
}

// deleteObject is DeleteObject.java of an object that is not seated: 0x12,
// object id, then 1 (D each).
func deleteObject(id int32) []byte {
	b := binary.LittleEndian.AppendUint32([]byte{0x12}, uint32(id))
	return binary.LittleEndian.AppendUint32(b, 1)
}

// fenceListPage is AdminSpawn.listFences' page for fences given as
// {id, x, y, z}.
func fenceListPage(fences ...[4]int32) string {
	page := fmt.Sprintf("<html><body>Total Fences: %d<br><br>", len(fences))
	for _, f := range fences {
		page += fmt.Sprintf(`<a action="bypass -h admin_deletefence %d 1">Fence: %d [%d %d %d]</a><br>`, f[0], f[0], f[1], f[2], f[3])
	}
	return page + "</body></html>"
}

// requireFrames requires frames to be exactly want.
func requireFrames(t *testing.T, who string, frames, want [][]byte) {
	t.Helper()
	if len(frames) != len(want) {
		t.Fatalf("%s: frames %x, want %d frame(s) %x", who, testsupport.FrameOpcodes(frames), len(want), testsupport.FrameOpcodes(want))
	}
	for i := range want {
		if !bytes.Equal(frames[i], want[i]) {
			t.Fatalf("%s: frame %d = % x, want % x", who, i, frames[i], want[i])
		}
	}
}

// TestAdminFences pins AdminSpawn.java's fence commands and the fence
// object FenceManager.addFence places.
//
// //spawnfence <type> <width> <length> [height] places a fence of that type
// at the GM, its width and length cut to whole hundreds, its position
// aligned to the geodata grid (a 100-unit side masks the low three bits, a
// 300-unit side the low four), one extra layer object per height layer
// above the first: the GM and every player around are shown the fence,
// then each layer, each with ExColosseumFenceInfo of the fence under its
// own id, and the GM gets the fence list. A fence too long to place still
// lists; a missing or unreadable argument answers the usage.
//
// Clicking a fence or a layer answers ActionFailed. //deletefence of a
// layer, of an unknown id or of a removed fence is an invalid target; of a
// fence, it deletes the layers then the fence, and lists again only when a
// second argument follows, as the list's own links do.
func TestAdminFences(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	watcher, _ := addPlayer(t, srv, "player2", "Watcher", userLevel)
	drain(t, gm)

	frames := fenceExchange(t, gm, encodeBuildCmd("spawnfence 2 150 399 2"))
	if len(frames) != 3 || frames[0][0] != serverpackets.OpcodeExtended {
		t.Fatalf("//spawnfence frames %x, want two fence infos and a page", testsupport.FrameOpcodes(frames))
	}
	fenceID := int32(binary.LittleEndian.Uint32(frames[0][3:7]))
	layerID := int32(binary.LittleEndian.Uint32(frames[1][3:7]))
	if obj, ok := srv.State.Object(fenceID); !ok {
		t.Fatalf("fence %d not in the world", fenceID)
	} else if _, ok := obj.(*fence.Fence); !ok {
		t.Fatalf("object %d is %T, want a fence", fenceID, obj)
	}
	// (10, 20) aligns to (8, 16): 100 wide masks x with 0xFFFFFFF8, 300 long
	// masks y with 0xFFFFFFF0.
	shown := [][]byte{fenceInfo(fenceID, 2, 8, 16, spawnZ, 100, 300), fenceInfo(layerID, 2, 8, 16, spawnZ, 100, 300)}
	requireFrames(t, "GM", frames[:2], shown)
	page := fenceListPage([4]int32{fenceID, 8, 16, spawnZ})
	if got := htmlBody(t, frames[2]); got != page {
		t.Fatalf("//spawnfence page = %q, want %q", got, page)
	}
	requireFrames(t, "watcher", fenceExchange(t, watcher, nil), shown)

	for _, id := range []int32{fenceID, layerID} {
		requireFrames(t, "click", fenceExchange(t, watcher, encodeAction(id)), [][]byte{{serverpackets.OpcodeActionFailed}})
	}

	for _, cmd := range []string{"listfence", "spawnfence 1 1100 100"} {
		if frames := fenceExchange(t, gm, encodeBuildCmd(cmd)); len(frames) != 1 || htmlBody(t, frames[0]) != page {
			t.Fatalf("//%s frames %x, want the one-fence list", cmd, testsupport.FrameOpcodes(frames))
		}
	}
	for _, cmd := range []string{"spawnfence", "spawnfence 2 100", "spawnfence 2 100 x", "spawnfence 2 100 100 x"} {
		assertTexts(t, fenceExchange(t, gm, encodeBuildCmd(cmd)), "Usage: //spawnfence <type> <width> <length> [height]")
	}
	for _, cmd := range []string{"deletefence", "deletefence x"} {
		assertTexts(t, fenceExchange(t, gm, encodeBuildCmd(cmd)), "Usage: //deletefence <objectId>")
	}
	for _, id := range []int32{layerID, 999999} {
		frames := fenceExchange(t, gm, encodeBuildCmd(fmt.Sprintf("deletefence %d", id)))
		if len(frames) != 1 {
			t.Fatalf("//deletefence %d frames %x, want INVALID_TARGET", id, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	}

	// The list's link removes the fence and lists again.
	frames = fenceExchange(t, gm, encodeBypass(fmt.Sprintf("admin_deletefence %d 1", fenceID)))
	gone := [][]byte{deleteObject(layerID), deleteObject(fenceID)}
	if len(frames) != 3 {
		t.Fatalf("admin_deletefence frames %x, want two DeleteObject and a page", testsupport.FrameOpcodes(frames))
	}
	requireFrames(t, "GM", frames[:2], gone)
	if got := htmlBody(t, frames[2]); got != fenceListPage() {
		t.Fatalf("admin_deletefence page = %q, want the empty list", got)
	}
	requireFrames(t, "watcher", fenceExchange(t, watcher, nil), gone)
	for _, id := range []int32{fenceID, layerID} {
		if _, ok := srv.State.Object(id); ok {
			t.Fatalf("object %d still in the world", id)
		}
	}
	frames = fenceExchange(t, gm, encodeBuildCmd(fmt.Sprintf("deletefence %d 1", fenceID)))
	if len(frames) != 1 {
		t.Fatalf("//deletefence of a removed fence frames %x, want INVALID_TARGET", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)

	// A one-layer fence of any type has no layer; deleted without a second
	// argument, no list follows.
	frames = fenceExchange(t, gm, encodeBuildCmd("spawnfence 7 -50 200"))
	if len(frames) != 2 {
		t.Fatalf("//spawnfence frames %x, want a fence info and a page", testsupport.FrameOpcodes(frames))
	}
	fenceID = int32(binary.LittleEndian.Uint32(frames[0][3:7]))
	// -50 cuts to 0, sized as 100 (offset 8); 200 has offset 0.
	requireFrames(t, "GM", frames[:1], [][]byte{fenceInfo(fenceID, 7, 8, 16, spawnZ, 0, 200)})
	requireFrames(t, "watcher", fenceExchange(t, watcher, nil), [][]byte{fenceInfo(fenceID, 7, 8, 16, spawnZ, 0, 200)})
	requireFrames(t, "GM", fenceExchange(t, gm, encodeBuildCmd(fmt.Sprintf("deletefence %d", fenceID))), [][]byte{deleteObject(fenceID)})
}
