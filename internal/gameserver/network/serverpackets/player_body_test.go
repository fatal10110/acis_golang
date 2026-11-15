package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

type fakeMountBodies map[int32][2]float64

func (m fakeMountBodies) CollisionBody(npcID int32) (float64, float64, bool) {
	b, ok := m[npcID]
	return b[0], b[1], ok
}

// bodyBlock is the speed-multiplier and collision run CharInfo, UserInfo and
// GMViewCharacterInfo all write: movement and attack multipliers, then the
// collision radius and height.
func bodyBlock(c *player.Character, radius, height float64) []byte {
	b := appendF64(nil, float64(c.MovementSpeedMultiplier()))
	b = appendF64(b, float64(c.AttackSpeedMultiplier()))
	b = appendF64(b, radius)
	return appendF64(b, height)
}

func bodyFrames(t *testing.T, c *player.Character) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		"CharInfo":            framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c})),
		"UserInfo":            framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c})),
		"GMViewCharacterInfo": framePayload(t, FrameGMViewCharacterInfo(UserInfoSnapshot{Character: c})),
	}
}

// TestPlayerFramesWriteBaseClassAndMountCollision pins the collision floats
// of CharInfo, UserInfo and GMViewCharacterInfo to the character's own body:
// a subclass of another race keeps its base class's body (here a human
// fighter playing an elven mystic class), and a mounted character shows its
// mount's body.
func TestPlayerFramesWriteBaseClassAndMountCollision(t *testing.T) {
	active := &player.Template{
		RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
		CollisionRadius: 7.5, CollisionHeight: 24, CollisionRadiusFemale: 7.5, CollisionHeightFemale: 23,
	}
	base := &player.Template{
		CollisionRadius: 9, CollisionHeight: 23, CollisionRadiusFemale: 8, CollisionHeightFemale: 23.5,
	}
	for _, tc := range []struct {
		name                   string
		sex                    player.Sex
		radius, height         float64
		activeRadius, activeHt float64
	}{
		{"male", player.SexMale, 9, 23, 7.5, 24},
		{"female", player.SexFemale, 8, 23.5, 7.5, 23},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &player.Character{ID: 0x10000001, Race: player.RaceHuman}
			c.SetName("Rider")
			c.SetSex(tc.sex)
			c.AttachRuntime(active, nil)
			c.SetBaseTemplate(base)
			c.Configure(player.Runtime{Mounts: fakeMountBodies{12526: {23, 31}}})

			for name, got := range bodyFrames(t, c) {
				if !bytes.Contains(got, bodyBlock(c, tc.radius, tc.height)) {
					t.Errorf("%s: unmounted collision is not the base class body (%v, %v)", name, tc.radius, tc.height)
				}
				if bytes.Contains(got, bodyBlock(c, tc.activeRadius, tc.activeHt)) {
					t.Errorf("%s: unmounted collision is the active class body", name)
				}
			}

			if !c.Mount(12526, 77) {
				t.Fatal("Mount() = false, want true")
			}
			for name, got := range bodyFrames(t, c) {
				if !bytes.Contains(got, bodyBlock(c, 23, 31)) {
					t.Errorf("%s: mounted collision is not the mount body (23, 31)", name)
				}
			}
		})
	}
}

// TestFrameUserInfoBonusSlotsIgnoreTheRightHand pins the bonus-slot field
// after the weight limit at 40 for an unarmed character: an empty right hand
// still holds the class's fists.
func TestFrameUserInfoBonusSlotsIgnoreTheRightHand(t *testing.T) {
	c := &player.Character{ID: 0x10000001, Race: player.RaceHuman}
	c.SetName("U")
	got := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c}))

	// opcode, x/y/z/heading/object id, "U\x00", race/sex/class/level, exp,
	// six attributes, HP/MP/SP/weight/weight-limit words.
	off := 1 + 5*4 + 2*2 + 4*4 + 8 + 6*4 + 7*4
	if len(got) < off+4 {
		t.Fatalf("UserInfo payload length = %d, want at least %d", len(got), off+4)
	}
	if v := int32(binary.LittleEndian.Uint32(got[off:])); v != 40 {
		t.Fatalf("unarmed bonus slots = %d, want 40", v)
	}
}
