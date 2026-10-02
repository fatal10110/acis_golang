package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestPartyRoomFlag pins the party-matching byte of UserInfo and CharInfo:
// written right after the cubic list (writeH count, then the ids), 1 while
// the character is in a room, 0 otherwise. It is the only byte a room
// changes.
func TestPartyRoomFlag(t *testing.T) {
	encoders := map[string]func(*player.Character) []byte{
		"UserInfo": func(c *player.Character) []byte {
			return framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: &player.Template{}}))
		},
		"CharInfo": func(c *player.Character) []byte {
			return framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: &player.Template{}}))
		},
	}
	for name, encode := range encoders {
		// The cubic count's offset is where an encoding with one cubic
		// first differs from one with none.
		withCubic := &player.Character{Name: "C"}
		withCubic.AddOrRefreshCubic(1, false)
		base := encode(&player.Character{Name: "C"})
		cubic := encode(withCubic)
		countAt := 0
		for base[countAt] == cubic[countAt] {
			countAt++
		}
		flagAt := countAt + 2

		inRoom := &player.Character{Name: "C"}
		inRoom.SetPartyRoom(3)
		got := encode(inRoom)
		if len(got) != len(base) {
			t.Fatalf("%s: length %d in a room, %d without", name, len(got), len(base))
		}
		for i := range got {
			want := base[i]
			if i == flagAt {
				want = 1
			}
			if got[i] != want {
				t.Fatalf("%s: byte %d = %d in a room, want %d (room flag at %d)", name, i, got[i], want, flagAt)
			}
		}
		if base[flagAt] != 0 {
			t.Fatalf("%s: room flag = %d outside a room, want 0", name, base[flagAt])
		}
	}
}
