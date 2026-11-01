package serverpackets

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// oracleCases maps each golden line's name to the encoder call that must
// produce the same bytes. The golden (testdata/packets.golden) is the output
// of testdata/oracle/PacketProbe.java running the unmodified reference
// writeImpl of each packet into a byte buffer; the probe's call arguments
// and these ones are the same values. To pin another encoder, add a case to
// PacketProbe.java, regenerate with testdata/oracle/run.sh, and add its
// encoder call here under the same name.
func oracleCases() map[string]func() wire.Frame {
	tell := int32(2) // the reference chat channel enum's TELL ordinal
	cases := map[string]func() wire.Frame{
		"GetOnVehicle": func() wire.Frame {
			return FrameGetOnVehicle(0x0a0b0c0d, 7, location.Location{X: 230, Y: -260, Z: -40})
		},
		"GetOffVehicle": func() wire.Frame {
			return FrameGetOffVehicle(0x0a0b0c0d, 7, location.Location{X: -96622, Y: 261660, Z: -3610})
		},
		"MoveToLocationInVehicle": func() wire.Frame {
			return FrameMoveToLocationInVehicle(0x0a0b0c0d, 7,
				location.Location{X: 230, Y: -260, Z: -48}, location.Location{X: 10, Y: -100, Z: -40})
		},
		"StopMoveInVehicle": func() wire.Frame {
			return FrameStopMoveInVehicle(0x0a0b0c0d, 7, location.Location{X: 10, Y: -100, Z: -40}, 32768)
		},
		"ValidateLocationInVehicle": func() wire.Frame {
			return FrameValidateLocationInVehicle(0x0a0b0c0d, 7, location.Location{X: 230, Y: -260, Z: -40}, 61440)
		},
		"VehicleInfo": func() wire.Frame {
			return FrameVehicleInfo(0x01020304, location.Location{X: -96622, Y: 261660, Z: -3610}, 32768)
		},
		"OnVehicleCheckLocation": func() wire.Frame {
			return FrameOnVehicleCheckLocation(7, location.Location{X: 48950, Y: 190613, Z: -3610}, 60800)
		},
		"VehicleDeparture": func() wire.Frame {
			return FrameVehicleDeparture(7, 150, 800, location.Location{X: 51914, Y: 189023, Z: -3610})
		},
		"VehicleStarted/moving":  func() wire.Frame { return FrameVehicleStarted(0x0a0b0c0d, true) },
		"VehicleStarted/stopped": func() wire.Frame { return FrameVehicleStarted(0x0a0b0c0d, false) },
		"BoatSay":                func() wire.Frame { return FrameBoatSay(1223) },
		"CreatureSay": func() wire.Frame {
			return FrameCreatureSay(0x01020304, tell, "->Bo", "hi")
		},
		"FriendAddRequestResult/accepted": func() wire.Frame { return FrameFriendAddRequestResult(true) },
		"FriendAddRequestResult/failed":   func() wire.Frame { return FrameFriendAddRequestResult(false) },
		"FriendAddRequest":                func() wire.Frame { return FrameFriendAddRequest("Bo") },
		"L2Friend/add-online": func() wire.Frame {
			return FrameL2Friend(L2FriendAdd, "Bo", true, 0x01020304)
		},
		"L2Friend/remove-by-name": func() wire.Frame { return FrameL2Friend(L2FriendRemove, "Bo", false, 0) },
		"L2FriendStatus":          func() wire.Frame { return FrameL2FriendStatus(true, "Bo", 7) },
		"L2FriendSay":             func() wire.Frame { return FrameL2FriendSay(620, "A", "B", "hi") },
		"FriendList": func() wire.Frame {
			return FrameFriendList([]FriendListEntry{
				{ObjectID: 9, Name: "On", Online: true},
				{ObjectID: 5, Name: "", Online: false},
			})
		},
		"EtcStatusUpdate/blocked": func() wire.Frame { return FrameEtcStatusUpdate(EtcStatus{Blocked: true}) },
		"EtcStatusUpdate/all": func() wire.Frame {
			return FrameEtcStatusUpdate(EtcStatus{
				Charges: 3, WeightPenalty: 2, Blocked: true, DangerArea: true,
				GradePenalty: true, CharmOfCourage: true, DeathPenaltyLevel: 4,
			})
		},
	}
	// The families probed from plain numbers, strings and creature stand-ins
	// are in packets_oracle_cases_test.go.
	for _, family := range []map[string]func() wire.Frame{oracleSimpleCases(), oracleWorldCases(), oracleEngineCases()} {
		for name, build := range family {
			cases[name] = build
		}
	}
	return cases
}

// TestPacketsMatchReferenceProbe replays testdata/packets.golden: every line
// is "name hex" for one packet the reference serialized, and the Go encoder
// must emit exactly those payload bytes (opcode first).
func TestPacketsMatchReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/packets.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	cases := oracleCases()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for line := 1; sc.Scan(); line++ {
		name, hexBytes, ok := strings.Cut(sc.Text(), " ")
		if !ok {
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
		build, ok := cases[name]
		if !ok {
			t.Fatalf("golden line %d: no Go case named %q", line, name)
		}
		seen[name] = true
		want, err := hex.DecodeString(hexBytes)
		if err != nil {
			t.Fatalf("golden line %d: %v", line, err)
		}
		if got := framePayload(t, build()); !bytes.Equal(got, want) {
			t.Errorf("%s = %x, want %x", name, got, want)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for name := range cases {
		if !seen[name] {
			t.Errorf("Go case %q has no golden line", name)
		}
	}
}
