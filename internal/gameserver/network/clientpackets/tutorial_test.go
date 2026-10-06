package clientpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestDecodeTutorialRequests pins the four tutorial requests and the event
// name each hands the tutorial quest: the link and the command as sent,
// "QM" and "CE" followed by the int32 number in decimal.
func TestDecodeTutorialRequests(t *testing.T) {
	str := func(opcode byte, s string) []byte {
		w := wire.NewPacketWriter(opcode)
		w.WriteString(s)
		return w.Bytes()
	}
	num := func(opcode byte, n int32) []byte {
		w := wire.NewPacketWriter(opcode)
		w.WriteInt32(n)
		return w.Bytes()
	}
	for _, tc := range []struct {
		name    string
		decode  func([]byte) (TutorialEvent, error)
		payload []byte
		want    string
	}{
		{"link", DecodeRequestTutorialLinkHTML, str(OpcodeRequestTutorialLinkHTML, "link tutorial_02.htm"), "link tutorial_02.htm"},
		{"command", DecodeRequestTutorialPassCmdToServer, str(OpcodeRequestTutorialPassCmdToServer, "TE1"), "TE1"},
		{"question mark", DecodeRequestTutorialQuestionMark, num(OpcodeRequestTutorialQuestionMark, 26), "QM26"},
		{"client event", DecodeRequestTutorialClientEvent, num(OpcodeRequestTutorialClientEvent, 8388608), "CE8388608"},
		{"negative client event", DecodeRequestTutorialClientEvent, num(OpcodeRequestTutorialClientEvent, -1), "CE-1"},
	} {
		got, err := tc.decode(tc.payload)
		if err != nil || got.Name != tc.want {
			t.Errorf("%s: decode = %q, %v; want %q", tc.name, got.Name, err, tc.want)
		}
	}
}

// TestDecodeTutorialRequestsShort rejects an unterminated string and a
// number shorter than four bytes.
func TestDecodeTutorialRequestsShort(t *testing.T) {
	for name, tc := range map[string]struct {
		decode  func([]byte) (TutorialEvent, error)
		payload []byte
	}{
		"link":          {DecodeRequestTutorialLinkHTML, []byte{OpcodeRequestTutorialLinkHTML, 'x'}},
		"command":       {DecodeRequestTutorialPassCmdToServer, []byte{OpcodeRequestTutorialPassCmdToServer, 'x'}},
		"question mark": {DecodeRequestTutorialQuestionMark, []byte{OpcodeRequestTutorialQuestionMark, 1, 0, 0}},
		"client event":  {DecodeRequestTutorialClientEvent, []byte{OpcodeRequestTutorialClientEvent, 1}},
	} {
		if _, err := tc.decode(tc.payload); err == nil {
			t.Errorf("%s: want error on a short payload, got nil", name)
		}
	}
}
