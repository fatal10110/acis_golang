package network

import (
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestPreAuthPacketCapDisconnects drives the hard cap on packets processed
// pre-auth: three accepted CONNECTED-state frames run, the fourth closes
// the connection without a reply.
func TestPreAuthPacketCapDisconnects(t *testing.T) {
	log := zerolog.New(os.Stderr)
	addr, _, _, _ := newTestGameClientLinkWithLog(t, nil, nil, log)
	c := testsupport.Dial(t, addr)
	c.SendProtocolVersion(746) // processed packet 1

	for i := 0; i < 2; i++ { // processed packets 2 and 3
		c.Send(encodeProtocolVersion(746))
		if got := c.Read(); got == nil || got[0] != serverpackets.OpcodeVersionCheck {
			t.Fatalf("accepted pre-auth frame %d: opcode = %#x, want VersionCheck", i+2, got[0])
		}
	}

	c.Send(encodeProtocolVersion(746)) // processed packet 4: over the cap
	if !c.AwaitClose(2 * time.Second) {
		t.Fatal("frame past the pre-auth cap did not close the connection")
	}
}

func TestPerformFloodProtectedReuseGate(t *testing.T) {
	c := NewClient(nil)
	now := time.Unix(0, 0)
	const delay = 3 * time.Second

	if !c.performFloodProtected(floodProtectorCharacterSelect, delay, now) {
		t.Fatal("first call refused, want allowed")
	}
	if c.performFloodProtected(floodProtectorCharacterSelect, delay, now.Add(time.Second)) {
		t.Fatal("call inside the reuse window allowed, want refused")
	}
	// A refusal must not extend the deadline.
	if c.performFloodProtected(floodProtectorCharacterSelect, delay, now.Add(2*time.Second)) {
		t.Fatal("second refusal extended the reuse window")
	}
	if !c.performFloodProtected(floodProtectorCharacterSelect, delay, now.Add(delay)) {
		t.Fatal("call at the deadline not allowed, want allowed")
	}
	// Gates are independent per protector.
	if !c.performFloodProtected(floodProtectorServerBypass, delay, now) {
		t.Fatal("server-bypass gate refused while the character-select gate held")
	}
	// A zero delay never rate-limits.
	for i := 0; i < 3; i++ {
		if !c.performFloodProtected(floodProtectorServerBypass, 0, now) {
			t.Fatal("zero-delay gate refused a call")
		}
	}
}

// TestCharacterDeleteFloodGateAnswersDeletionFailed drives two deletes in
// quick succession: the first (unknown slot) refreshes the character list,
// the second is refused by the character-select reuse gate and answered
// with CharDeleteFail alone.
func TestCharacterDeleteFloodGateAnswersDeletionFailed(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterDelete(0))
	if got := c.Read(); got[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("first delete: opcode = %#x, want CharSelectInfo", got[0])
	}

	c.Send(encodeRequestCharacterDelete(0))
	got := c.Read()
	if len(got) == 0 || got[0] != serverpackets.OpcodeCharDeleteFail {
		t.Fatalf("gated delete: opcode = %#x, want CharDeleteFail", got[0])
	}
	c.ExpectNoFrame()
}

// TestCharacterRestoreFloodGateIsSilent drives two restores in quick
// succession: the first refreshes the character list, the second is refused
// silently.
func TestCharacterRestoreFloodGateIsSilent(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.Send(encodeCharacterRestore(0))
	if got := c.Read(); got[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("first restore: opcode = %#x, want CharSelectInfo", got[0])
	}

	c.Send(encodeCharacterRestore(0))
	c.ExpectNoFrame()
}

// TestGameStartFloodGateIsSilent selects a character, then re-selects in
// quick succession: the second selection is refused silently — no SSQInfo,
// no CharSelected.
func TestGameStartFloodGateIsSilent(t *testing.T) {
	c, _, _, _ := newLinkedGameClientSeedOneChar(t)

	c.Send(encodeRequestGameStart(0))
	if got := c.Read(); got[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("first selection: opcode = %#x, want SSQInfo", got[0])
	}
	if got := c.Read(); got[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("first selection: opcode = %#x, want CharSelected", got[0])
	}

	c.Send(encodeRequestGameStart(0))
	c.ExpectNoFrame()
}
