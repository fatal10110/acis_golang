package scriptcontract

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The tutorial window, radar, drop and cast packets, written by hand from
// the client's field layout (little-endian, opcode first), render as the
// scenario S lines write them, and the Go encoders' bytes are those bytes.
func TestPacketRendersTutorialRadarDropAndCast(t *testing.T) {
	roles := map[int32]string{0x10000001: "player", 0x10000002: "gremlin"}
	for _, tc := range []struct {
		name, hex, want string
		encoded         []byte
	}{
		{
			"tutorial page", "a0" + "3c0062003e00" + "320036003800340033003500340035003700" + "0000",
			`S TutorialShowHtml html="<b>{player}"`,
			nil,
		},
		{"question mark", "a1" + "05000000", "S TutorialShowQuestionMark id=5", nil},
		{"client event", "a2" + "00008000", "S TutorialEnableClientEvent id=8388608", nil},
		{"close", "a3", "S TutorialCloseHtml", nil},
		{
			"radar", "eb" + "02000000" + "02000000" + "62b6feff" + "7fb60300" + "6ef1ffff",
			"S RadarControl show=2 type=2 loc=-84382,243327,-3730", nil,
		},
		{
			"drop", "0c" + "02000010" + "99000010" + "d1180000" + "0a000000" + "14000000" + "1e000000" + "01000000" + "01000000" + "01000000",
			"S DropItem by=gremlin item=6353 count=1 stackable=1 animate=1", nil,
		},
		{
			"cast", "48" + "01000010" + "01000010" + "e2100000" + "01000000" + "00000000" + "00000000" + "0a000000" + "14000000" + "1e000000" + "01000000" + "0000" + "0a000000" + "14000000" + "1e000000",
			"S MagicSkillUse caster=player target=player skill=4322-1 hit=0 reuse=0 success=1", nil,
		},
		{
			"failed cast", "48" + "02000010" + "01000010" + "e2100000" + "01000000" + "f4010000" + "e8030000" + "0a000000" + "14000000" + "1e000000" + "00000000" + "0b000000" + "15000000" + "1f000000",
			"S MagicSkillUse caster=gremlin target=player skill=4322-1 hit=500 reuse=1000 success=0", nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Packet(b, roles)
			if err != nil {
				t.Fatalf("Packet: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Packet = %s, want %s", got, tc.want)
			}
			if _, err := Packet(append(b, 0), roles); err == nil {
				t.Error("Packet rendered a payload with a trailing byte")
			}
			if len(b) > 1 {
				if _, err := Packet(b[:len(b)-1], roles); err == nil {
					t.Error("Packet rendered a truncated payload")
				}
			}
		})
	}
}

// The Go encoders write the bytes the hand-written fixtures above decode.
func TestTutorialRadarDropAndCastEncodersRender(t *testing.T) {
	at := location.Location{X: 10, Y: 20, Z: 30}
	roles := map[int32]string{0x10000001: "player"}
	for _, tc := range []struct {
		name, want string
		payload    []byte
	}{
		{"page", `S TutorialShowHtml html="<b>{player}"`, payload(t, serverpackets.FrameTutorialShowHTML("<b>268435457"))},
		{"question mark", "S TutorialShowQuestionMark id=5", payload(t, serverpackets.FrameTutorialShowQuestionMark(5))},
		{"client event", "S TutorialEnableClientEvent id=8388608", payload(t, serverpackets.FrameTutorialEnableClientEvent(8388608))},
		{"close", "S TutorialCloseHtml", payload(t, serverpackets.FrameTutorialCloseHTML())},
		{"radar", "S RadarControl show=0 type=1 loc=-84382,243327,-3730", payload(t, serverpackets.FrameRadarControl(serverpackets.Radar{Show: 0, Type: 1, X: -84382, Y: 243327, Z: -3730}))},
		{
			"cast", "S MagicSkillUse caster=player target=player skill=4322-1 hit=0 reuse=0 success=1",
			payload(t, serverpackets.FrameMagicSkillUse(serverpackets.SkillCastObject{ObjectID: 0x10000001, Location: at}, serverpackets.SkillCastObject{ObjectID: 0x10000001, Location: at}, 4322, 1, 0, 0, true)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Packet(tc.payload, roles)
			if err != nil {
				t.Fatalf("Packet: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Packet = %s, want %s", got, tc.want)
			}
		})
	}
	if _, err := Packet([]byte{0xa4}, nil); err == nil || !strings.Contains(err.Error(), "not rendered") {
		t.Fatalf("Packet of an undecoded tutorial opcode = %v", err)
	}
}
