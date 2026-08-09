package network

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// wireLiveAttackHooks connects a newTestLivePlayer-built live player to gcl
// the same way production attachLivePlayer does — SetStarted,
// SetAttackBroadcaster, SetMoveBroadcaster, and SetArrived (which advances
// the player's world-grid position on arrival before re-thinking, the same
// step attachLivePlayer performs). newTestLivePlayer itself can't do this
// wiring because it has no GameClientLink to close over.
func wireLiveAttackHooks(gcl *GameClientLink, live *livePlayer) {
	live.stopAttack = gcl.stopLiveAutoAttack
	live.attack.SetFinished(func() {
		gcl.finishDeferredPickup(live)
		live.combat.Think()
	})
	live.attack.SetStarted(func() {
		gcl.startLiveAutoAttack(live)
	})
	live.Character.SetAttackBroadcaster(func(snapshot attack.Snapshot) {
		gcl.broadcastAttack(live, snapshot)
	})
	live.Character.SetMoveBroadcaster(func(event move.Event) {
		gcl.broadcastLiveMoveEvent(live, event)
	})
	live.Character.SetStatusBroadcaster(func() {
		gcl.broadcastLiveStatus(live)
	})
	live.move.SetArrived(func() {
		pos := live.move.Position()
		gcl.updateLivePlayerPosition(live, pos, live.CurrentHeading())
		live.combat.Think()
	})
}

func TestGameClientLinkAttackLiveTargetReusesController(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Character.SetWorld(state)
	attacker.Character.SetRollSource(func(int) int { return 0 })
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	wireLiveAttackHooks(gcl, attacker)
	target := newTestHostileNPC(t, 3002)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(target, 30, 0, 0, 0)
	attackerFrames.frames = nil

	if !gcl.attackLiveTarget(attacker, target) {
		t.Fatal("first attackLiveTarget returned false")
	}
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeAutoAttackStart, serverpackets.OpcodeAttack}) {
		t.Fatalf("first attack opcodes = %x, want AutoAttackStart, Attack", got)
	}
	if attacker.attack == nil || !attacker.attack.AttackingNow() {
		t.Fatal("live player attack controller is not tracking the active attack")
	}

	attackerFrames.frames = nil
	if gcl.attackLiveTarget(attacker, target) {
		t.Fatal("second attackLiveTarget returned true while the first swing is active")
	}
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("second attack opcodes = %x, want ActionFailed only", got)
	}

	attacker.Stop()
	if attacker.attack.AttackingNow() {
		t.Fatal("live player Stop did not cancel the active attack controller")
	}
	if attacker.InCombat() {
		t.Fatal("live player Stop did not clear combat stance")
	}
}

// TestGameClientLinkAttackLiveTargetOutOfRangeWalksBeforeSwinging is the
// regression test for the reported bug: attacking a target outside weapon
// range must start the player walking toward it, not stand still doing
// nothing, and must not land a swing until it actually arrives.
func TestGameClientLinkAttackLiveTargetOutOfRangeWalksBeforeSwinging(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Character.SetWorld(state)
	attacker.Character.SetRollSource(func(int) int { return 0 })
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	wireLiveAttackHooks(gcl, attacker)
	target := newTestHostileNPC(t, 3003)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(target, 1000, 0, 0, 0)
	attackerFrames.frames = nil

	if !gcl.attackLiveTarget(attacker, target) {
		t.Fatal("attackLiveTarget returned false for a distant target, want true (start walking)")
	}
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeMoveToPawn}) {
		t.Fatalf("out-of-range attack opcodes = %x, want MoveToPawn only (no premature swing)", got)
	}
	if attacker.attack.AttackingNow() {
		t.Fatal("attack controller reports attacking while still closing distance")
	}
	if attacker.InCombat() {
		t.Fatal("combat stance started before any swing landed")
	}

	// A redundant re-evaluation of the same still-converging target (e.g.
	// the client resending its Attack packet while walking) must not
	// re-broadcast movement or fail the action.
	attackerFrames.frames = nil
	if !gcl.attackLiveTarget(attacker, target) {
		t.Fatal("second attackLiveTarget returned false while still closing distance")
	}
	if len(attackerFrames.frames) != 0 {
		t.Fatalf("redundant re-evaluation opcodes = %x, want none", frameOpcodes(attackerFrames.frames))
	}
}

// TestGameClientLinkAttackLiveTargetArrivesAndLandsASwing is the regression
// test for the "arrival never reaches world.Presence" review finding: it
// waits for the real arrival timer (no fake clock) and asserts the player's
// world-grid position actually advances and a swing actually lands —
// exercising the same real time.AfterFunc path the reviewer's scratch test
// found broken, rather than a fake clock that can't observe this bug.
func TestGameClientLinkAttackLiveTargetArrivesAndLandsASwing(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Character.SetWorld(state)
	attacker.Character.SetRollSource(func(int) int { return 0 })
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	wireLiveAttackHooks(gcl, attacker)
	target := newTestHostileNPC(t, 3006)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(target, 200, 0, 0, 0)

	if !gcl.attackLiveTarget(attacker, target) {
		t.Fatal("attackLiveTarget returned false for an out-of-range target")
	}

	deadline := time.Now().Add(5 * time.Second)
	for target.CurrentHP() >= target.MaxHP() {
		if time.Now().After(deadline) {
			x, y, z := attacker.Position()
			t.Fatalf("swing never landed after arrival: position = (%d,%d,%d), attackingNow = %v, want near target at (200,0,0) with damage taken",
				x, y, z, attacker.attack.AttackingNow())
		}
		time.Sleep(10 * time.Millisecond)
	}

	x, _, _ := attacker.Position()
	if x < 150 {
		t.Fatalf("attacker position after arrival = %d, want advanced toward the target (>=150)", x)
	}
}

// TestClearLiveTargetStopsAttackIntention is the regression test for the
// "endless attack loop has no cancel path" review finding: cancelling the
// target must stop the underlying ai.PlayerAttack loop, or the
// arrived/finished hooks keep re-evaluating and re-attacking forever.
func TestClearLiveTargetStopsAttackIntention(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Character.SetWorld(state)
	attacker.Character.SetRollSource(func(int) int { return 0 })
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	target := newTestHostileNPC(t, 3004)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(target, 30, 0, 0, 0)
	attacker.SetTargetTracked(target)
	if !gcl.attackLiveTarget(attacker, target) {
		t.Fatal("attackLiveTarget returned false for an in-range target")
	}
	if attacker.combat.Target() != target {
		t.Fatal("attack intention did not latch onto the target")
	}

	gcl.clearLiveTarget(attacker)

	if attacker.combat.Target() != nil {
		t.Fatalf("attack intention target = %v after clearLiveTarget, want nil", attacker.combat.Target())
	}
}

// TestMoveLivePlayerStopsAttackIntention is the regression test for the
// "server chase fights the player's own movement" review finding: a
// client-initiated walk must cancel any attack-driven chase, or the
// server's own MaybeStartOffensiveFollow re-think steers the player back
// toward the old target underneath them.
func TestMoveLivePlayerStopsAttackIntention(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	attacker.Character.SetWorld(state)
	attacker.Character.SetRollSource(func(int) int { return 0 })
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	target := newTestHostileNPC(t, 3005)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(target, 1000, 0, 0, 0)
	if !gcl.attackLiveTarget(attacker, target) {
		t.Fatal("attackLiveTarget returned false for a distant target")
	}
	if attacker.combat.Target() == nil {
		t.Fatal("attack intention did not latch onto the distant target")
	}

	gcl.moveLivePlayer(attacker, location.Location{X: -500, Y: 0, Z: 0})

	if attacker.combat.Target() != nil {
		t.Fatalf("attack intention target = %v after a player-initiated walk, want nil", attacker.combat.Target())
	}
}

// TestCastFinishResumesPendingAttackIntention pins issue #1016's remaining
// acceptance criterion, gated the way the reference actually gates it:
// PlayableAI.onEvtFinishedCasting (PlayableAI.java:43-63) only resumes the
// attack when the just-finished skill has nextActionIsAttack() set (Go:
// modelskill.Definition.NextActionIsAttack); any other skill goes idle. The
// manual attacker.attack.Stop() below does not mirror a real production
// call site — the swing loop has no cast-awareness of its own yet (that gap
// is tracked separately, #1174) — it only isolates the finish-observer gate
// under test from that unrelated, still-open pause behavior.
func TestCastFinishResumesPendingAttackIntention(t *testing.T) {
	gatedDef := modelskill.Definition{
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 5000, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
		NextActionIsAttack: true,
	}
	ungatedDef := gatedDef
	ungatedDef.NextActionIsAttack = false

	tests := []struct {
		name       string
		end        func(*actorcast.Controller)
		def        modelskill.Definition
		wantResume bool
	}{
		{name: "natural finish, nextActionAttack", end: func(c *actorcast.Controller) { c.Finish() }, def: gatedDef, wantResume: true},
		{name: "abort, nextActionAttack", end: func(c *actorcast.Controller) { c.Stop() }, def: gatedDef, wantResume: true},
		{name: "natural finish, no nextActionAttack", end: func(c *actorcast.Controller) { c.Finish() }, def: ungatedDef, wantResume: false},
		{name: "abort, no nextActionAttack", end: func(c *actorcast.Controller) { c.Stop() }, def: ungatedDef, wantResume: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := world.New()
			attackerFrames := &frameCapture{}
			attacker := newTestLivePlayer(t, 1, attackerFrames)
			attacker.Character.SetWorld(state)
			attacker.Character.SetRollSource(func(int) int { return 0 })
			gcl := &GameClientLink{world: state, log: zerolog.Nop()}
			wireLiveAttackHooks(gcl, attacker)
			target := newTestHostileNPC(t, 3200)
			target.Instance.Template.PDef = 1
			target.Instance.Template.DEX = 30
			target.SetRollSource(func(int) int { return 0 })

			state.Spawn(attacker, 0, 0, 0, 0)
			state.Spawn(target, 30, 0, 0, 0)

			if !gcl.attackLiveTarget(attacker, target) {
				t.Fatal("attackLiveTarget returned false for an in-range target")
			}
			if !attacker.attack.AttackingNow() {
				t.Fatal("expected the first swing to be in flight")
			}

			// Isolates the finish-observer gate: see the doc comment above
			// on why this doesn't reflect a real cast-start call site yet.
			attacker.attack.Stop()
			attackerFrames.frames = nil

			controller := gcl.castController(attacker)
			if _, err := controller.Start(time.Now(), skillCastObject(attacker), tt.def); err != nil {
				t.Fatalf("Start() error: %v", err)
			}
			if attacker.combat.Target() != target {
				t.Fatal("attack intention target was cleared by starting a cast, want it queued")
			}

			tt.end(controller)

			if tt.wantResume {
				// The abort path additionally broadcasts MagicSkillCanceled
				// and ActionFailed ahead of the resumed swing; AutoAttackStart
				// does not repeat since the actor never left combat stance.
				// In both cases the last frame must be the resumed swing's
				// Attack.
				frames := frameOpcodes(attackerFrames.frames)
				if len(frames) == 0 || frames[len(frames)-1] != serverpackets.OpcodeAttack {
					t.Fatalf("attack opcodes after cast %s = %#x, want the last frame to be Attack (the resumed intention)", tt.name, frames)
				}
				if !attacker.attack.AttackingNow() {
					t.Fatal("attack intention did not resume after the cast ended")
				}
				return
			}

			if attacker.attack.AttackingNow() {
				t.Fatal("attack intention resumed after a cast whose skill has no nextActionAttack, want idle")
			}
			if attacker.combat.Target() != nil {
				t.Fatalf("attack intention target = %v after a cast with no nextActionAttack, want nil (idle)", attacker.combat.Target())
			}
		})
	}
}
