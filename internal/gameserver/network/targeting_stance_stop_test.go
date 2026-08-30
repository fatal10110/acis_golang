package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

type noopAttackStanceEffects struct{}

func (noopAttackStanceEffects) AutoAttackStop(task.AttackStanceActor) {}

// TestStopLiveAutoAttackTrackedActorRemovesAndBroadcasts pins that a normal
// auto-attack stop drops the combat-stance tracker entry before broadcasting
// AutoAttackStop.
func TestStopLiveAutoAttackTrackedActorRemovesAndBroadcasts(t *testing.T) {
	stance, err := task.NewAttackStance(noopAttackStanceEffects{}, nil)
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 42, frames)
	live.SetInCombat(true)
	stance.Add(live)

	gcl := &GameClientLink{attackStance: stance, log: zerolog.Nop()}
	testsupport.ResetCapture(frames)

	gcl.stopLiveAutoAttack(live)

	if stance.InAttackStance(live) {
		t.Fatal("attack stance entry should be removed")
	}
	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 1 || got[0] != serverpackets.OpcodeAutoAttackStop {
		t.Fatalf("opcodes = %x, want [AutoAttackStop]", got)
	}
}

// TestStopLiveAutoAttackUntrackedActorIsSilent pins that stopping combat on an
// actor that was never registered in the tracker sends no AutoAttackStop.
func TestStopLiveAutoAttackUntrackedActorIsSilent(t *testing.T) {
	stance, err := task.NewAttackStance(noopAttackStanceEffects{}, nil)
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 43, frames)
	live.SetInCombat(true)

	gcl := &GameClientLink{attackStance: stance, log: zerolog.Nop()}
	testsupport.ResetCapture(frames)

	gcl.stopLiveAutoAttack(live)

	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 0 {
		t.Fatalf("opcodes = %x, want none", got)
	}
	if live.InCombat() {
		t.Fatal("combat flag should clear even when tracker had no entry")
	}
}

// TestStopLiveAutoAttackRepeatedStopBroadcastsOnce pins that a second stop on
// an already-idle actor is a no-op.
func TestStopLiveAutoAttackRepeatedStopBroadcastsOnce(t *testing.T) {
	stance, err := task.NewAttackStance(noopAttackStanceEffects{}, nil)
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 44, frames)
	live.SetInCombat(true)
	stance.Add(live)

	gcl := &GameClientLink{attackStance: stance, log: zerolog.Nop()}
	testsupport.ResetCapture(frames)

	gcl.stopLiveAutoAttack(live)
	gcl.stopLiveAutoAttack(live)

	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 1 || got[0] != serverpackets.OpcodeAutoAttackStop {
		t.Fatalf("opcodes = %x, want single AutoAttackStop", got)
	}
}
