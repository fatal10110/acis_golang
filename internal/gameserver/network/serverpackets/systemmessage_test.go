package serverpackets

import (
	"bytes"
	"testing"
	"time"
)

// ---- from confirmdlg_test.go ----
func TestFrameConfirmDlgSummonFriendRequest(t *testing.T) {
	got := framePayload(t, FrameConfirmDlgSummonFriendRequest("Bob", 12345, 10, 20, 30, 30*time.Second))
	want := []byte{
		OpcodeConfirmDlg,
		0x32, 0x07, 0x00, 0x00, // 1842
		0x02, 0x00, 0x00, 0x00, // 2 info entries
		confirmDlgTypeText, 0x00, 0x00, 0x00,
		'B', 0x00, 'o', 0x00, 'b', 0x00, 0x00, 0x00,
		confirmDlgTypeZoneName, 0x00, 0x00, 0x00,
		10, 0x00, 0x00, 0x00,
		20, 0x00, 0x00, 0x00,
		30, 0x00, 0x00, 0x00,
		0x30, 0x75, 0x00, 0x00, // 30000ms
		0x39, 0x30, 0x00, 0x00, // 12345
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameConfirmDlgSummonFriendRequest() = %x, want %x", got, want)
	}
}

// TestFrameConfirmDlgResurrectionRequest pins the resurrection offer's
// bytes: ConfirmDlg.writeImpl with one TYPE_TEXT entry (addCharName) and a
// zero time and requester id, which it leaves off the wire.
func TestFrameConfirmDlgResurrectionRequest(t *testing.T) {
	got := framePayload(t, FrameConfirmDlgResurrectionRequest("Bob"))
	want := []byte{
		0xed,
		0xe6, 0x05, 0x00, 0x00, // 1510 RESSURECTION_REQUEST_BY_S1
		0x01, 0x00, 0x00, 0x00, // 1 info entry
		0x00, 0x00, 0x00, 0x00, // TYPE_TEXT
		'B', 0x00, 'o', 0x00, 'b', 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameConfirmDlgResurrectionRequest() = %x, want %x", got, want)
	}
}

// ---- from systemmessage_test.go ----
func TestFrameSystemMessage(t *testing.T) {
	got := framePayload(t, FrameSystemMessage(SystemMessagePetRefusingOrder))
	want := []byte{
		OpcodeSystemMessage,
		0x48, 0x07, 0x00, 0x00, // 1864
		0x00, 0x00, 0x00, 0x00, // no params
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessage() = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageCorpseTargetFailures(t *testing.T) {
	for _, id := range []int{
		SystemMessageSweeperFailedTargetNotSpoiled,
		SystemMessageHarvestFailedSeedNotSown,
		SystemMessageCorpseTooOldSkillNotUsed,
	} {
		got := framePayload(t, FrameSystemMessage(id))
		want := []byte{OpcodeSystemMessage, byte(id), byte(id >> 8), 0, 0, 0, 0, 0, 0}
		if !bytes.Equal(got, want) {
			t.Fatalf("FrameSystemMessage(%d) = %x, want %x", id, got, want)
		}
	}
}

func TestFrameSystemMessageOverHit(t *testing.T) {
	got := framePayload(t, FrameSystemMessage(SystemMessageOverHit))
	want := []byte{OpcodeSystemMessage, 0x69, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessage(OverHit) = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageCounterattackFeedback(t *testing.T) {
	tests := []struct {
		name string
		id   int
	}{
		{name: "performing", id: SystemMessageS1PerformingCounterattack},
		{name: "countered", id: SystemMessageCounteredS1Attack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := framePayload(t, FrameSystemMessageString(tt.id, "Target"))
			want := []byte{
				OpcodeSystemMessage,
				byte(tt.id), byte(tt.id >> 8), 0x00, 0x00,
				0x01, 0x00, 0x00, 0x00,
				SystemMessageParamText, 0x00, 0x00, 0x00,
				'T', 0x00, 'a', 0x00, 'r', 0x00, 'g', 0x00, 'e', 0x00, 't', 0x00, 0x00, 0x00,
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("counterattack frame = %x, want %x", got, want)
			}
		})
	}
}

func TestFrameSystemMessageResistedS1Magic(t *testing.T) {
	got := framePayload(t, FrameSystemMessageString(SystemMessageResistedS1Magic, "Mage"))
	want := []byte{
		OpcodeSystemMessage,
		0x9f, 0x00, 0x00, 0x00, // 159
		0x01, 0x00, 0x00, 0x00,
		SystemMessageParamText, 0x00, 0x00, 0x00,
		'M', 0x00, 'a', 0x00, 'g', 0x00, 'e', 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("resisted-magic frame = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageForceChargeFeedback(t *testing.T) {
	t.Run("increased", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessageNumber(SystemMessageForceIncreasedToS1, 3))
		want := []byte{
			OpcodeSystemMessage,
			0x43, 0x01, 0x00, 0x00, // 323
			0x01, 0x00, 0x00, 0x00, // one parameter
			0x01, 0x00, 0x00, 0x00, // number parameter
			0x03, 0x00, 0x00, 0x00, // three charges
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("force-increased frame = %x, want %x", got, want)
		}
	})

	t.Run("maximum", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessage(SystemMessageForceMaxLevelReached))
		want := []byte{
			OpcodeSystemMessage,
			0x44, 0x01, 0x00, 0x00, // 324
			0x00, 0x00, 0x00, 0x00, // no parameters
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("force-maximum frame = %x, want %x", got, want)
		}
	})
}

func TestFrameSystemMessageTwoNumbers(t *testing.T) {
	got := framePayload(t, FrameSystemMessageTwoNumbers(SystemMessageYouEarnedS1ExpAndS2SP, 1000, 25))
	want := []byte{
		OpcodeSystemMessage,
		0x5f, 0x00, 0x00, 0x00, // 95
		0x02, 0x00, 0x00, 0x00, // two params
		0x01, 0x00, 0x00, 0x00, // number param
		0xe8, 0x03, 0x00, 0x00, // 1000 exp
		0x01, 0x00, 0x00, 0x00, // number param
		0x19, 0x00, 0x00, 0x00, // 25 sp
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageTwoNumbers() = %x, want %x", got, want)
	}
}

// TestFrameSystemMessageItemNameNumber pins the pickup form of
// YOU_PICKED_UP_S2_S1: the count goes out as a plain number parameter
// (type 1), unlike the item-number (type 6) form an item created by id
// uses.
func TestFrameSystemMessageItemNameNumber(t *testing.T) {
	got := framePayload(t, FrameSystemMessageItemNameNumber(SystemMessageYouPickedUpS2S1, 1060, 5))
	want := []byte{
		OpcodeSystemMessage,
		0x1d, 0x00, 0x00, 0x00, // 29
		0x02, 0x00, 0x00, 0x00, // two params
		0x03, 0x00, 0x00, 0x00, // item-name param
		0x24, 0x04, 0x00, 0x00, // item 1060
		0x01, 0x00, 0x00, 0x00, // number param
		0x05, 0x00, 0x00, 0x00, // 5
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageItemNameNumber() = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageSkillName(t *testing.T) {
	got := framePayload(t, FrameSystemMessageSkillName(SystemMessageNightSkillEffectApplies, 294, 1))
	want := []byte{
		OpcodeSystemMessage,
		0x6b, 0x04, 0x00, 0x00, // 1131
		0x01, 0x00, 0x00, 0x00, // one param
		0x04, 0x00, 0x00, 0x00, // skill-name param
		0x26, 0x01, 0x00, 0x00, // skill 294
		0x01, 0x00, 0x00, 0x00, // level 1
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageSkillName() = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageStringSkillName(t *testing.T) {
	got := framePayload(t, FrameSystemMessageStringSkillName(SystemMessageS1ResistedYourS2, "Target", 123, 1))
	want := []byte{
		OpcodeSystemMessage,
		0x8b, 0x00, 0x00, 0x00, // 139
		0x02, 0x00, 0x00, 0x00, // two params
		0x00, 0x00, 0x00, 0x00, // text parameter
		'T', 0x00, 'a', 0x00, 'r', 0x00, 'g', 0x00, 'e', 0x00, 't', 0x00, 0x00, 0x00,
		0x04, 0x00, 0x00, 0x00, // skill-name parameter
		0x7b, 0x00, 0x00, 0x00, // skill 123
		0x01, 0x00, 0x00, 0x00, // level 1
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageStringSkillName() = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageStringNumber(t *testing.T) {
	got := framePayload(t, FrameSystemMessageStringNumber(1016, "Attacker", 12))
	want := []byte{
		OpcodeSystemMessage,
		0xf8, 0x03, 0x00, 0x00, // 1016
		0x02, 0x00, 0x00, 0x00, // two params
		0x00, 0x00, 0x00, 0x00, // text parameter
		'A', 0x00, 't', 0x00, 't', 0x00, 'a', 0x00, 'c', 0x00, 'k', 0x00, 'e', 0x00, 'r', 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00, // number parameter
		0x0c, 0x00, 0x00, 0x00, // 12
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageStringNumber() = %x, want %x", got, want)
	}
}

func TestFrameHPMPRestoredSystemMessages(t *testing.T) {
	t.Run("self HP", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessageNumber(SystemMessageS1HPRestored, 8))
		want := []byte{
			OpcodeSystemMessage,
			0x2a, 0x04, 0x00, 0x00, // 1066
			0x01, 0x00, 0x00, 0x00, // one param
			0x01, 0x00, 0x00, 0x00, // number
			0x08, 0x00, 0x00, 0x00, // 8
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("self HP restored frame = %x, want %x", got, want)
		}
	})
	t.Run("other HP", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessageStringNumber(SystemMessageS2HPRestoredByS1, "Healer", 4))
		want := []byte{
			OpcodeSystemMessage,
			0x2b, 0x04, 0x00, 0x00, // 1067
			0x02, 0x00, 0x00, 0x00, // two params
			0x00, 0x00, 0x00, 0x00, // text
			'H', 0x00, 'e', 0x00, 'a', 0x00, 'l', 0x00, 'e', 0x00, 'r', 0x00, 0x00, 0x00,
			0x01, 0x00, 0x00, 0x00, // number
			0x04, 0x00, 0x00, 0x00, // 4
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("other HP restored frame = %x, want %x", got, want)
		}
	})
	t.Run("self MP", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessageNumber(SystemMessageS1MPRestored, 4))
		want := []byte{
			OpcodeSystemMessage,
			0x2c, 0x04, 0x00, 0x00, // 1068
			0x01, 0x00, 0x00, 0x00,
			0x01, 0x00, 0x00, 0x00,
			0x04, 0x00, 0x00, 0x00,
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("self MP restored frame = %x, want %x", got, want)
		}
	})
	t.Run("other MP", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessageStringNumber(SystemMessageS2MPRestoredByS1, "Healer", 4))
		want := []byte{
			OpcodeSystemMessage,
			0x2d, 0x04, 0x00, 0x00, // 1069
			0x02, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			'H', 0x00, 'e', 0x00, 'a', 0x00, 'l', 0x00, 'e', 0x00, 'r', 0x00, 0x00, 0x00,
			0x01, 0x00, 0x00, 0x00,
			0x04, 0x00, 0x00, 0x00,
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("other MP restored frame = %x, want %x", got, want)
		}
	})
	t.Run("self CP", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessageNumber(SystemMessageS1CPWillBeRestored, 4))
		want := []byte{
			OpcodeSystemMessage,
			0x7d, 0x05, 0x00, 0x00, // 1405
			0x01, 0x00, 0x00, 0x00,
			0x01, 0x00, 0x00, 0x00,
			0x04, 0x00, 0x00, 0x00,
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("self CP restored frame = %x, want %x", got, want)
		}
	})
	t.Run("other CP", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessageStringNumber(SystemMessageS2CPWillBeRestoredByS1, "Healer", 4))
		want := []byte{
			OpcodeSystemMessage,
			0x7e, 0x05, 0x00, 0x00, // 1406
			0x02, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			'H', 0x00, 'e', 0x00, 'a', 0x00, 'l', 0x00, 'e', 0x00, 'r', 0x00, 0x00, 0x00,
			0x01, 0x00, 0x00, 0x00,
			0x04, 0x00, 0x00, 0x00,
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("other CP restored frame = %x, want %x", got, want)
		}
	})
}

// TestFrameSystemMessageParams pins the mixed-parameter SystemMessage: id,
// parameter count, then each parameter's type followed by its text or
// value.
func TestFrameSystemMessageParams(t *testing.T) {
	got := framePayload(t, FrameSystemMessageParams(SystemMessageS2S3SCreatedForS1ForS4Adena,
		TextParam("Al"), NumberParam(2), ItemNameParam(1060), ItemNumberParam(300)))
	want := []byte{0x64}
	want = appendD(want, 1147)
	want = appendD(want, 4)
	want = appendD(want, 0)
	want = append(want, 'A', 0, 'l', 0, 0, 0)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 3)
	want = appendD(want, 1060)
	want = appendD(want, 6)
	want = appendD(want, 300)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageParams() = %x, want %x", got, want)
	}
}
