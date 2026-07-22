package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestPlayerClockEffectsSendsSystemMessagesToLivePlayer(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 100, capture)
	state.AddPlayer(live)

	effects := NewPlayerClockEffects(state)
	effects.NotifyPlayingTooLong(100)
	effects.NotifyDayNightSkillTransition(100, true, 294, 1)

	if len(capture.frames) != 2 {
		t.Fatalf("captured frames = %d, want 2", len(capture.frames))
	}
	wantTooLong := []byte{
		serverpackets.OpcodeSystemMessage,
		0xfc, 0x02, 0x00, 0x00, // 764
		0x00, 0x00, 0x00, 0x00, // no params
	}
	wantNight := []byte{
		serverpackets.OpcodeSystemMessage,
		0x6b, 0x04, 0x00, 0x00, // 1131
		0x01, 0x00, 0x00, 0x00, // one param
		0x04, 0x00, 0x00, 0x00, // skill-name param
		0x26, 0x01, 0x00, 0x00, // skill 294
		0x01, 0x00, 0x00, 0x00, // level 1
	}
	if !bytes.Equal(capture.frames[0], wantTooLong) {
		t.Fatalf("playing-too-long frame = %x, want %x", capture.frames[0], wantTooLong)
	}
	if !bytes.Equal(capture.frames[1], wantNight) {
		t.Fatalf("night skill frame = %x, want %x", capture.frames[1], wantNight)
	}
}

func TestPlayerClockEffectsMissingPlayerNoop(t *testing.T) {
	effects := NewPlayerClockEffects(world.New())
	effects.NotifyPlayingTooLong(404)
	effects.NotifyDayNightSkillTransition(404, false, 294, 1)
}
