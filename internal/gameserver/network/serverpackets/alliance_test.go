package serverpackets

import (
	"bytes"
	"testing"
)

// Reference: AskJoinAlly.writeImpl writes C 0xa8, D requester object id, S
// the name it is given (RequestJoinAlly passes the alliance's name).
func TestFrameAskJoinAlly(t *testing.T) {
	got := framePayload(t, FrameAskJoinAlly(0x10000001, "Ally"))
	want := appendD([]byte{0xa8}, 0x10000001)
	want = append(want, encodeUTF16Z("Ally")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("AskJoinAlly = %x, want %x", got, want)
	}
}

// Reference: AllianceInfo.writeImpl writes C 0xb4, S alliance name, D
// total, D online, S leading clan, S its leader, D clan count, then per
// clan S name, D 0, D level, S leader, D total, D online.
func TestFrameAllianceInfo(t *testing.T) {
	got := framePayload(t, FrameAllianceInfo(Alliance{
		Name: "Ally", Total: 7, Online: 2, LeaderClan: "Knights", LeaderName: "Founder",
		Clans: []AllianceClan{
			{Name: "Knights", Level: 5, LeaderName: "Founder", Total: 4, Online: 1},
			{Name: "Rivals", Level: 3, LeaderName: "Rival", Total: 3, Online: 1},
		},
	}))
	want := append([]byte{0xb4}, encodeUTF16Z("Ally")...)
	want = appendD(want, 7)
	want = appendD(want, 2)
	want = append(want, encodeUTF16Z("Knights")...)
	want = append(want, encodeUTF16Z("Founder")...)
	want = appendD(want, 2)
	want = append(want, encodeUTF16Z("Knights")...)
	want = appendD(want, 0)
	want = appendD(want, 5)
	want = append(want, encodeUTF16Z("Founder")...)
	want = appendD(want, 4)
	want = appendD(want, 1)
	want = append(want, encodeUTF16Z("Rivals")...)
	want = appendD(want, 0)
	want = appendD(want, 3)
	want = append(want, encodeUTF16Z("Rival")...)
	want = appendD(want, 3)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("AllianceInfo = %x, want %x", got, want)
	}
}
