package network

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// zeroInterruptDef is a skill whose interrupt window is already closed the
// instant Start returns (InterruptAfter floors to 0 for HitTime 0), so a
// RequestTargetCancel issued right after Start lands strictly outside
// canAbortCast()'s window without needing a real sleep.
var zeroInterruptDef = modelskill.Definition{
	ID: 11, Level: 1, HitTime: 0, StaticHitTime: true, StaticReuse: true,
}

// TestRequestTargetCancelAbortsCastInsideInterruptWindow pins
// RequestTargetCancel.java:23-29 + PlayerAI.java:160-165 onEvtCancel: Esc
// (Unselect == 0) while casting and still inside the interrupt window fires
// AiEventType.CANCEL -> getCast().stop() (unconditional stop, no
// CASTING_INTERRUPTED) and leaves the target untouched.
func TestRequestTargetCancelAbortsCastInsideInterruptWindow(t *testing.T) {
	live := newTestLivePlayer(t, 1, &frameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), skillCastObject(live), castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	live.SetTargetTracked(live)

	gcl.requestTargetCancel(live, clientpackets.RequestTargetCancel{Unselect: 0})

	if controller.CastingNow() {
		t.Fatal("CastingNow() = true after Esc inside the interrupt window, want aborted")
	}
	if live.Target() != live {
		t.Fatal("target cleared by Esc cast-cancel, want left untouched (stop() never touches target)")
	}
}

// TestRequestTargetCancelIsNoOpOutsideInterruptWindow pins
// RequestTargetCancel.java:26 canAbortCast() gate: once the interrupt
// window has closed, Esc never fires AiEventType.CANCEL at all — the cast
// lands undisturbed.
func TestRequestTargetCancelIsNoOpOutsideInterruptWindow(t *testing.T) {
	live := newTestLivePlayer(t, 1, &frameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), skillCastObject(live), zeroInterruptDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	gcl.requestTargetCancel(live, clientpackets.RequestTargetCancel{Unselect: 0})

	if !controller.CastingNow() {
		t.Fatal("CastingNow() = false after Esc outside the interrupt window, want cast left running")
	}
}

// TestRequestTargetCancelClearsTargetWhenNotCasting pins
// RequestTargetCancel.java:28-29: with Unselect == 0 and no active cast,
// Esc clears the target exactly like today's clearLiveTarget path.
func TestRequestTargetCancelClearsTargetWhenNotCasting(t *testing.T) {
	live := newTestLivePlayer(t, 1, &frameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}
	live.SetTargetTracked(live)

	gcl.requestTargetCancel(live, clientpackets.RequestTargetCancel{Unselect: 0})

	if live.Target() != nil {
		t.Fatal("target not cleared by Esc while not casting")
	}
}

// TestRequestTargetCancelUnselectAlwaysClearsTargetDuringCast pins
// RequestTargetCancel.java:31-32: Unselect != 0 always clears the target,
// regardless of an in-flight cast, and never touches the cast itself.
func TestRequestTargetCancelUnselectAlwaysClearsTargetDuringCast(t *testing.T) {
	live := newTestLivePlayer(t, 1, &frameCapture{})
	gcl := &GameClientLink{log: zerolog.Nop()}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), skillCastObject(live), castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	live.SetTargetTracked(live)

	gcl.requestTargetCancel(live, clientpackets.RequestTargetCancel{Unselect: 1})

	if live.Target() != nil {
		t.Fatal("target not cleared by Unselect != 0")
	}
	if !controller.CastingNow() {
		t.Fatal("CastingNow() = false after Unselect != 0, want cast left running")
	}
}
