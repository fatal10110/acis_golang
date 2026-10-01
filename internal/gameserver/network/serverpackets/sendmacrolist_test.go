package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
)

// TestFrameSendMacroList pins both SendMacroList forms against the
// reference layout (SendMacroList.writeImpl): opcode 0xE7, revision int32,
// a zero byte, the list's count byte, then 0 for the empty list or 1 and
// one macro: id int32, name, description and acronym strings, icon byte,
// line count byte, and per line its 1-based number, type byte, d1 int32,
// d2 byte and text string.
func TestFrameSendMacroList(t *testing.T) {
	empty := framePayload(t, FrameSendMacroList(2, 0, nil))
	if want := []byte{0xe7, 2, 0, 0, 0, 0, 0, 0}; !bytes.Equal(empty, want) {
		t.Fatalf("empty SendMacroList = % x, want % x", empty, want)
	}

	m := macro.Macro{
		ID: 1000, Icon: 3, Name: "Go", Description: "d", Acronym: "G",
		Commands: []macro.Command{
			{Type: macro.CommandSkill, D1: 1177, D2: 0, Text: ""},
			{Type: macro.CommandShortcut, D1: 2, D2: 5, Text: "/a"},
		},
	}
	got := framePayload(t, FrameSendMacroList(5, 2, &m))
	want := []byte{0xe7}
	want = binary.LittleEndian.AppendUint32(want, 5)
	want = append(want, 0, 2, 1)
	want = binary.LittleEndian.AppendUint32(want, 1000)
	want = append(want, encodeUTF16Z("Go")...)
	want = append(want, encodeUTF16Z("d")...)
	want = append(want, encodeUTF16Z("G")...)
	want = append(want, 3, 2)
	want = append(want, 1, 1)
	want = binary.LittleEndian.AppendUint32(want, 1177)
	want = append(want, 0)
	want = append(want, encodeUTF16Z("")...)
	want = append(want, 2, 4)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = append(want, 5)
	want = append(want, encodeUTF16Z("/a")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("SendMacroList =\n% x\nwant\n% x", got, want)
	}
}

// recommendationDiff encodes c twice, without and then with recommendations
// (3 left, 200 held), and returns the second payload and where the two
// first differ. Both characters carry the same abnormal-effect mask, which
// anchors the recommendation fields' position.
func recommendationDiff(t *testing.T, frame func(*player.Character) []byte) ([]byte, []byte, int) {
	t.Helper()
	const mask = 0x55aa33cc
	plain := &player.Character{Name: "R"}
	plain.StartAbnormalEffect(mask)
	recommended := &player.Character{Name: "R"}
	recommended.StartAbnormalEffect(mask)
	recommended.SetRecommendationCounts(200, 3)
	base, got := frame(plain), frame(recommended)
	if len(base) != len(got) {
		t.Fatalf("payload length changed: base %d, got %d", len(base), len(got))
	}
	i := 0
	for i < len(base) && base[i] == got[i] {
		i++
	}
	return base, got, i
}

// TestFrameUserInfoCarriesRecommendations pins UserInfo's recommendation
// fields (UserInfo.writeImpl): after the abnormal-effect int32, a byte and
// the clan-privileges int32, recommendations left then held, each an int16.
func TestFrameUserInfoCarriesRecommendations(t *testing.T) {
	tmpl := &player.Template{}
	base, got, i := recommendationDiff(t, func(c *player.Character) []byte {
		return framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl}))
	})
	if mask := binary.LittleEndian.Uint32(got[i-9:]); mask != 0x55aa33cc {
		t.Fatalf("abnormal effect 9 bytes before the first difference = %#x, want the anchor mask", mask)
	}
	if left, have := binary.LittleEndian.Uint16(got[i:]), binary.LittleEndian.Uint16(got[i+2:]); left != 3 || have != 200 {
		t.Fatalf("recommendations = left %d have %d, want 3 and 200", left, have)
	}
	if !bytes.Equal(got[i+4:], base[i+4:]) {
		t.Fatal("bytes after the recommendation fields differ")
	}
}

// TestFrameCharInfoCarriesRecommendations pins CharInfo's recommendation
// fields (CharInfo.writeImpl): right after the abnormal-effect int32,
// recommendations left as a byte, then held as an int16.
func TestFrameCharInfoCarriesRecommendations(t *testing.T) {
	tmpl := &player.Template{}
	base, got, i := recommendationDiff(t, func(c *player.Character) []byte {
		return framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl}))
	})
	if mask := binary.LittleEndian.Uint32(got[i-4:]); mask != 0x55aa33cc {
		t.Fatalf("abnormal effect right before the first difference = %#x, want the anchor mask", mask)
	}
	if left, have := got[i], binary.LittleEndian.Uint16(got[i+1:]); left != 3 || have != 200 {
		t.Fatalf("recommendations = left %d have %d, want 3 and 200", left, have)
	}
	if !bytes.Equal(got[i+3:], base[i+3:]) {
		t.Fatal("bytes after the recommendation fields differ")
	}
}
