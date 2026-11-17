package link

import (
	"bytes"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
)

// referencePacket is a golden payload with the reference's transport tail
// (the reserved checksum and the padding to a multiple of 8) cut off after
// checking it.
func referencePacket(t *testing.T, line int, payload string, encoded []byte) {
	t.Helper()
	want, err := hex.DecodeString(payload)
	if err != nil {
		t.Fatalf("line %d: %v", line, err)
	}
	padded := (len(encoded) + 4 + 7) &^ 7
	if len(want) != padded || !bytes.HasPrefix(want, encoded) || !bytes.Equal(want[len(encoded):], make([]byte, padded-len(encoded))) {
		t.Errorf("line %d: payload = %x, want %x without its %d-byte zero tail", line, encoded, want, padded-len(encoded))
	}
}

// TestLinkPacketsMatchReferenceProbe replays testdata/link.golden, the output
// of testdata/oracle/LinkProbe.java: the reference's ServerStatus packets
// (single attributes, and the six-attribute packet sent on AuthResponse) and
// PlayerInGame packets.
func TestLinkPacketsMatchReferenceProbe(t *testing.T) {
	data, err := os.ReadFile("testdata/link.golden")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for i, text := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line := i + 1
		fields := strings.Fields(text)
		if len(fields) != 3 {
			t.Fatalf("malformed golden line %d %q", line, text)
		}
		counts[fields[0]]++
		switch fields[0] {
		case "serverstatus":
			referencePacket(t, line, fields[2], EncodeServerStatus(parseServerStatus(t, line, fields[1])))
		case "playeringame":
			encoded, err := EncodePlayerInGame(strings.Split(fields[1], ","))
			if err != nil {
				t.Fatal(err)
			}
			referencePacket(t, line, fields[2], encoded)
		default:
			t.Fatalf("malformed golden line %d %q", line, text)
		}
	}
	if counts["serverstatus"] < 100 || counts["playeringame"] != 2 {
		t.Fatalf("golden has %v lines, want every ServerStatus attribute and the six-attribute packet", counts)
	}
}

// parseServerStatus reads "attribute:value,..." into the status those
// attributes make up.
func parseServerStatus(t *testing.T, line int, attrs string) ServerStatus {
	t.Helper()
	var s ServerStatus
	for _, pair := range strings.Split(attrs, ",") {
		a, v, ok := strings.Cut(pair, ":")
		attr, err1 := strconv.Atoi(a)
		value, err2 := strconv.ParseInt(v, 10, 32)
		if !ok || err1 != nil || err2 != nil {
			t.Fatalf("line %d: malformed attribute %q", line, pair)
		}
		val, on := int32(value), value == serverStatusOn
		switch serverStatusAttr(attr) {
		case attrStatus:
			st := ServerType(val)
			s.Status = &st
		case attrClock:
			s.ShowClock = &on
		case attrBrackets:
			s.ShowBrackets = &on
		case attrAgeLimit:
			s.AgeLimit = &val
		case attrTestServer:
			s.TestServer = &on
		case attrPvp:
			s.Pvp = &on
		case attrMaxPlayers:
			s.MaxPlayers = &val
		default:
			t.Fatalf("line %d: unknown attribute %d", line, attr)
		}
	}
	return s
}
