package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// CharCreateOk
// CharSelectInfo

// SSQInfo
// CharSelected

// Spawned well within melee range so this exercises the immediate-attack
// path; out-of-range approach is covered separately.

// TestGameClientLinkAttackBroadcastsTargetHPStatusOnEveryHit is the
// regression test for the reported bug: hitting an NPC does eventually kill
// it, but nothing shows the HP bar dropping along the way — TakeDamage
// applied the damage and ran the death check, but never told any client the
// target's HP had changed, so the only visible sign of a hit landing was
// the target's corpse.

// CharCreateOk
// CharSelectInfo

// SSQInfo
// CharSelected

// A hostile NPC only broadcasts through a world it was registered
// into, matching how the production spawn manager wires it — see
// data/manager/npcs.go's newLiveHostile.

// MyTargetSelected
// StatusUpdate (selection snapshot, still full HP)

// attribute count
// MAX_HP type
// MAX_HP value

// wireLiveAttackHooks connects a newTestLivePlayer-built live player to gcl
// the same way production attachLivePlayer does — SetStarted,
// SetAttackBroadcaster, SetMoveBroadcaster, and SetArrived (which advances
// the player's world-grid position on arrival before re-thinking, the same
// step attachLivePlayer performs). newTestLivePlayer itself can't do this
// wiring because it has no GameClientLink to close over.

// TestGameClientLinkAttackLiveTargetOutOfRangeWalksBeforeSwinging is the
// regression test for the reported bug: attacking a target outside weapon
// range must start the player walking toward it, not stand still doing
// nothing, and must not land a swing until it actually arrives.

// A redundant re-evaluation of the same still-converging target (e.g.
// the client resending its Attack packet while walking) must not
// re-broadcast movement or fail the action.

// TestGameClientLinkAttackLiveTargetArrivesAndLandsASwing is the regression
// test for the "arrival never reaches world.Presence" review finding: it
// waits for the real arrival timer (no fake clock) and asserts the player's
// world-grid position actually advances and a swing actually lands —
// exercising the same real time.AfterFunc path the reviewer's scratch test
// found broken, rather than a fake clock that can't observe this bug.

// TestClearLiveTargetStopsAttackIntention is the regression test for the
// "endless attack loop has no cancel path" review finding: cancelling the
// target must stop the underlying ai.PlayerAttack loop, or the
// arrived/finished hooks keep re-evaluating and re-attacking forever.

// TestMoveLivePlayerStopsAttackIntention is the regression test for the
// "server chase fights the player's own movement" review finding: a
// client-initiated walk must cancel any attack-driven chase, or the
// server's own MaybeStartOffensiveFollow re-think steers the player back
// toward the old target underneath them.

// TestGameClientLinkSecondActionClickAttacksSelectedTarget is the
// regression test for the unresponsive-attack bug: the client attacks a mob
// by plain-clicking it twice (both clicks arrive as Action packets, not
// AttackRequest), and the second click must start the attack — in range it
// swings immediately, out of range it starts walking. Dropping that second
// click silently leaves the client's pending attack action unresolved, so
// the character walks up client-side, never swings, and stops responding
// to further input.

// CharCreateOk
// CharSelectInfo

// SSQInfo
// CharSelected

// StatusUpdate

// TestGameClientLinkSecondActionClickWalksTowardDistantTarget covers the
// out-of-range half of the same regression: the second plain click on a far
// mob must answer with MoveToPawn (the walk into range), not silence.

// CharCreateOk
// CharSelectInfo

// SSQInfo
// CharSelected

// StatusUpdate

// CharCreateOk
// CharSelectInfo

// SSQInfo
// CharSelected

// TestGameClientLinkActionBarStanceCommandsToggleStance covers the
// action-bar sit/stand and walk/run buttons, which arrive as action-use
// requests rather than the dedicated wait/move-type packets, and the
// release path for an action-bar command no handler claims yet: the client
// must get ActionFailed back, never silence.

// CharCreateOk
// CharSelectInfo

// SSQInfo
// CharSelected

// Walk/run button: a fresh character runs, so the first press walks and
// the second runs again.

// Sit/stand button: a fresh character stands, so the first press sits
// and the second stands back up.

// An action-bar command nothing claims (private store sell) must
// release the client with ActionFailed instead of silence.

// CharCreateOk
// CharSelectInfo

// SSQInfo
// CharSelected

// TestLiveTargetReflectsDomainLevelRetarget is the regression test for
// issue #855: the network layer no longer keeps its own copy of the
// selected target, so a domain-level retarget (the AGGDEBUFF continuous
// effect's retargetableOnAggression branch, here simulated directly through
// player.Character.SetTarget) is immediately visible through the same
// live.Target() read the network click path uses, without any extra sync
// step.
func TestLiveTargetReflectsDomainLevelRetarget(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	other := newTestHostileNPC(t, 2)
	caster := newTestHostileNPC(t, 3)

	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(other, 100, 0, 0, 0)
	state.Spawn(caster, 200, 0, 0, 0)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	if !gcl.selectLiveTarget(live, other) {
		t.Fatal("selectLiveTarget returned false")
	}
	if live.Target() != world.Tracked(other) {
		t.Fatalf("Target() after click-select = %v, want %v", live.Target(), other)
	}

	live.Character.SetTarget(world.Tracked(caster))

	if got := live.Target(); got != world.Tracked(caster) {
		t.Fatalf("Target() after domain-level retarget = %v, want %v", got, caster)
	}
	if got := live.CurrentTarget(); got != worldobject.Object(world.Tracked(caster)) {
		t.Fatalf("CurrentTarget() = %v, want %v", got, caster)
	}
}

// TestRetargetHookSendsPlayerSetTargetPackets is the regression test for
// Finding 1 of pr-reviews/975.md: the AGGDEBUFF retarget branch
// (retargetableOnAggression.SetTarget) reduced to a silent field write, with
// none of Player.setTarget's packet funnel (Player.java:2439-2510) that the
// click path already sends via selectLiveTarget — the retargeted player's
// client kept the old selection, and witnesses never saw a TargetSelected
// broadcast. character_flow.go's attachLivePlayer now wires SetRetargetHook
// to the same selectLiveTarget the click path uses, so a domain-level
// retarget reproduces that funnel instead of only updating the field.
func TestRetargetHookSendsPlayerSetTargetPackets(t *testing.T) {
	state := world.New()
	retargetedFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	retargeted := newTestLivePlayer(t, 1, retargetedFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)
	caster := newTestHostileNPC(t, 3)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	retargeted.Character.SetRetargetHook(func(target world.Tracked) {
		gcl.selectLiveTarget(retargeted, target)
	})

	state.Spawn(retargeted, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	state.Spawn(caster, 150, 0, 0, 0)
	retargetedFrames.frames = nil
	observerFrames.frames = nil

	retargeted.Character.SetTarget(world.Tracked(caster))

	if got := retargeted.Target(); got != world.Tracked(caster) {
		t.Fatalf("Target() after domain-level retarget = %v, want %v", got, caster)
	}
	if got := frameOpcodes(retargetedFrames.frames); string(got) != string([]byte{serverpackets.OpcodeValidateLocation, serverpackets.OpcodeMyTargetSelected, serverpackets.OpcodeStatusUpdate}) {
		t.Fatalf("retargeted player opcodes = %x, want ValidateLocation, MyTargetSelected, StatusUpdate", got)
	}
	if got := frameOpcodes(observerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeTargetSelected}) {
		t.Fatalf("observer opcodes = %x, want TargetSelected", got)
	}
}

// TestAttackTargetHookStartsLiveAttackIntention is the regression test for
// the AGGDEBUFF "already targeting the caster" branch's wiring: the hook
// character_flow.go's attachLivePlayer registers via SetAttackTargetHook
// must route through the same attackLiveTarget path a client-initiated
// attack click uses.
func TestAttackTargetHookStartsLiveAttackIntention(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetWorld(state)
	live.Character.SetRollSource(func(int) int { return 0 })
	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	target := newTestHostileNPC(t, 4)
	target.Instance.Template.PDef = 1
	target.Instance.Template.DEX = 30
	target.SetRollSource(func(int) int { return 0 })

	live.Character.SetAttackTargetHook(func(t world.Tracked) {
		gcl.attackLiveTarget(live, t)
	})

	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(target, 30, 0, 0, 0)

	live.Character.AttackTarget(world.Tracked(target))

	if live.combat.Target() != target {
		t.Fatal("AttackTarget hook did not start the live attack intention on the caster")
	}
}
