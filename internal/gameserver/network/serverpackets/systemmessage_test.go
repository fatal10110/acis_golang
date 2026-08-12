package serverpackets

import (
	"bytes"
	"testing"
)

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
