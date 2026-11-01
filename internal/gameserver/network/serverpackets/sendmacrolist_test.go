package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// recommendationDiff encodes c twice, without and then with recommendations
// (3 left, 200 held), and returns the second payload and where the two
// first differ. Both characters carry the same abnormal-effect mask, which
// anchors the recommendation fields' position.
func recommendationDiff(t *testing.T, frame func(*player.Character) []byte) ([]byte, []byte, int) {
	t.Helper()
	const mask = 0x55aa33cc
	plain := &player.Character{}
	plain.SetName("R")
	plain.StartAbnormalEffect(mask)
	recommended := &player.Character{}
	recommended.SetName("R")
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
