package scriptcontract

import (
	"encoding/hex"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The camera shot, social animation, appearing and leaving of the world's
// objects render with their roles, and the Go encoders write the packets the
// reference traces carry.
func TestPacketRendersCameraSocialNpcInfoAndDelete(t *testing.T) {
	roles := map[int32]string{0x10000484: "camera", 0x10000485: "player"}
	frame := func(f interface{ Bytes() []byte }) []byte { return append([]byte(nil), f.Bytes()[2:]...) }
	cases := []struct {
		name    string
		payload []byte
		want    string
	}{
		{"SpecialCamera", frame(serverpackets.FrameSpecialCamera(0x10000484, event.CameraShot{
			Distance: 1500, Yaw: 88, Pitch: 89, Time: 0, Duration: 10000, Turn: 0, Rise: 110, Widescreen: 1,
		})), "S SpecialCamera obj=camera distance=1500 yaw=88 pitch=89 time=0 duration=10000 turn=0 rise=110 widescreen=1 unknown=0"},
		{"SocialAction", frame(serverpackets.FrameSocialAction(0x10000485, 3)), "S SocialAction obj=player action=3"},
		// The reference trace's DeleteObject of the Fallen Unicorn.
		{"DeleteObject", mustHex(t, "128d04001001000000"), "S DeleteObject obj=268436621"},
		// The reference trace's NpcInfo of the Pure White Unicorn, cut after its template id: the rest is the world's.
		// The reference trace's stance change of the Pure White Unicorn, and a Die of a monster.
		{"ChangeMoveType", mustHex(t, "2e910400100100000000000000"), "S ChangeMoveType obj=268436625 running=1 swimming=0"},
		{"Die", mustHex(t, "068f040010010000000000000000000000000000000000000000000000"), "S Die obj=268436623 village=1 clanhall=0 castle=0 siegehq=0 sweep=0 fixed=0"},
		{"ExPlayScene", mustHex(t, "fe5b00"), "S ExPlayScene"},
		{"Earthquake", mustHex(t, "c40100000002000000030000000400000005000000"+"01000000"), "S Earthquake intensity=4 duration=5 npc=1"},
		{"NpcInfo", mustHex(t, "169204001043be0f0000000000280000001400000032000000"), "S NpcInfo obj=268436626 npc=31747"},
	}
	for _, c := range cases {
		got, err := Packet(c.payload, roles)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s = %s, want %s", c.name, got, c.want)
		}
		cut := c.payload[:3]
		if len(c.payload) == 3 {
			cut = c.payload[:2]
		}
		if _, err := Packet(cut, roles); err == nil {
			t.Errorf("%s: Packet rendered a cut payload", c.name)
		}
	}
	for _, name := range []string{"SpecialCamera", "SocialAction", "DeleteObject", "ChangeMoveType", "Die", "ExPlayScene", "Earthquake"} {
		for _, c := range cases {
			if c.name == name {
				if _, err := Packet(append(append([]byte(nil), c.payload...), 0), roles); err == nil {
					t.Errorf("%s: Packet rendered a payload with a trailing byte", name)
				}
			}
		}
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
