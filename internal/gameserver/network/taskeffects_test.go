package network

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestTaskEffectsWaterSendsCyanGauge(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 100, capture)
	state.AddPlayer(live)

	water, err := task.NewWater(NewTaskEffects(state), time.Now)
	if err != nil {
		t.Fatalf("NewWater() error = %v", err)
	}
	water.Add(live, 10*time.Second)

	if len(capture.frames) != 1 {
		t.Fatalf("captured frames = %d, want 1", len(capture.frames))
	}
	got := capture.frames[0]
	if got[0] != serverpackets.OpcodeSetupGauge {
		t.Fatalf("opcode = %#x, want %#x", got[0], serverpackets.OpcodeSetupGauge)
	}
	if color := binary.LittleEndian.Uint32(got[1:5]); color != uint32(serverpackets.GaugeCyan) {
		t.Fatalf("gauge color = %d, want %d", color, serverpackets.GaugeCyan)
	}
	if duration := binary.LittleEndian.Uint32(got[9:13]); duration != 10_000 {
		t.Fatalf("gauge duration = %d, want 10000", duration)
	}
}

func TestTaskEffectsDrownDamagesAndNotifiesLivePlayer(t *testing.T) {
	state := world.New()
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 100, capture)
	state.AddPlayer(live)

	NewTaskEffects(state).Drown(live)

	if live.CurrentHP() >= 100 {
		t.Fatalf("current HP = %d, want drowning damage", live.CurrentHP())
	}
	if len(capture.frames) == 0 {
		t.Fatal("drowning sent no client frame")
	}
	got := capture.frames[len(capture.frames)-1]
	if got[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want %#x", got[0], serverpackets.OpcodeSystemMessage)
	}
	if message := binary.LittleEndian.Uint32(got[1:5]); message != serverpackets.SystemMessageDrownDamage {
		t.Fatalf("message = %d, want %d", message, serverpackets.SystemMessageDrownDamage)
	}
}
