package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestGameClientLinkRoutesSummonActionUseToLiveSummon(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	ownerObject, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	owner, ok := ownerObject.(summon.Owner)
	if !ok {
		t.Fatalf("world.Player(%d) does not satisfy summon.Owner", objID)
	}
	liveSummon := summon.NewServitor(summon.ServitorConfig{ObjectID: 500, Owner: owner, Level: 40})
	summon.SpawnBesideOwner(state, liveSummon, owner, location.Location{})

	c.Send(encodeRequestActionUse(52, false, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodePetDelete {
		t.Fatalf("post-action opcode = %#x, want PetDelete (%#x)", reply[0], serverpackets.OpcodePetDelete)
	}

	if _, ok := state.Summon(objID); ok {
		t.Fatalf("owner %d still has active summon after action 52", objID)
	}
	if _, ok := state.Object(liveSummon.ObjectID()); ok {
		t.Fatalf("summon object %d still exists after action 52", liveSummon.ObjectID())
	}
}

func TestGameClientLinkSummonActionUseDispatchesSelectedTargetToAI(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 300)
	state.Spawn(hostile, 100, 0, 0, 0)

	liveSummon := summon.NewServitor(summon.ServitorConfig{ObjectID: 500, Owner: live, Level: 40})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	live.SetTargetTracked(hostile)
	gcl := &GameClientLink{world: state}
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16}) {
		t.Fatal("handleSummonActionUse returned false for a summon attack command")
	}
	if len(brain.attacks) != 1 || brain.attacks[0] != hostile.ObjectID() {
		t.Fatalf("AI attacks = %v, want selected hostile id %d", brain.attacks, hostile.ObjectID())
	}

	friendlyCreature := &summonActionCombatant{id: 301}
	state.Spawn(friendlyCreature, 150, 0, 0, 0)
	live.SetTargetTracked(friendlyCreature)
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16}) {
		t.Fatal("handleSummonActionUse returned false for a summon follow-target command")
	}
	if len(brain.follows) != 1 || brain.follows[0] != friendlyCreature.ObjectID() {
		t.Fatalf("AI follows = %v, want selected creature id %d", brain.follows, friendlyCreature.ObjectID())
	}
}

// TestGameClientLinkSummonActionUseWithNoActiveSummonAnswersActionFailed is
// the regression test for a pet-command shortcut (attack/follow/stop/
// return) pressed with no summon out: handleSummonActionUse recognized the
// action id and claimed the request as handled, but sent nothing back,
// silently swallowing the ActionFailed fallback the dispatch loop would
// otherwise have sent for an unclaimed action-bar command.
func TestGameClientLinkSummonActionUseWithNoActiveSummonAnswersActionFailed(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	chars.soleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	// Action id 16 is the pet-attack shortcut; no summon has been spawned.
	c.Send(encodeRequestActionUse(16, false, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("pet-command opcode with no active summon = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}

func TestGameClientLinkSummonSkillUseResolvesTargetKindAndDispatches(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 300)
	state.Spawn(hostile, 100, 0, 0, 0)
	live.SetTargetTracked(hostile)

	liveSummon := summon.NewServitor(summon.ServitorConfig{
		ObjectID: 500, Owner: live, Level: 40,
		Skills: map[int]int{4259: 1, 4378: 1, 4139: 8},
	})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	gcl := &GameClientLink{world: state}

	// Action 36 (Soulless - Toxic Smoke) targets the clicked target.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 36}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 1 || brain.casts[0] != hostile.ObjectID() {
		t.Fatalf("AI casts = %v, want clicked target %d", brain.casts, hostile.ObjectID())
	}

	// Action 42 (Kai the Cat - Self Damage Shield) targets the owner.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 42}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 2 || brain.casts[1] != live.ObjectID() {
		t.Fatalf("AI casts = %v, want owner target %d", brain.casts, live.ObjectID())
	}

	// Action 1001 (Sin Eater - Ultimate Bombastic Buster) targets the
	// summon itself.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 1001}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 3 || brain.casts[2] != liveSummon.ObjectID() {
		t.Fatalf("AI casts = %v, want self target %d", brain.casts, liveSummon.ObjectID())
	}
}

func TestGameClientLinkSinEaterSkillUseBroadcastsFlavorLine(t *testing.T) {
	if got, want := sinEaterActionStrings, [4]string{
		"special skill? Abuses in this kind of place, can turn blood Knots...!",
		"Hey! Brother! What do you anticipate to me?",
		"shouts ha! Flap! Flap! Response?",
		", has not hit...!",
	}; got != want {
		t.Fatalf("Sin Eater flavor strings = %q, want %q", got, want)
	}

	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	liveSummon := summon.NewPet(summon.PetConfig{
		ObjectID: 500, Owner: live, NPCID: 12564, Level: live.LevelValue(),
		Skills: map[int]int{4139: 1},
		Roll:   func(int) int { return 0 },
	})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{world: state}
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 1001}) {
		t.Fatal("handleSummonActionUse returned false for Sin Eater skill action")
	}
	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string([]byte{serverpackets.OpcodeNpcSay, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("Sin Eater skill-use opcodes = %x, want NpcSay then ActionFailed", got)
	}
}

func TestGameClientLinkSinEaterSkillUseMissedFlavorRollDoesNotBroadcast(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	liveSummon := summon.NewPet(summon.PetConfig{
		ObjectID: 500, Owner: live, NPCID: 12564, Level: live.LevelValue(),
		Skills: map[int]int{4139: 1},
		Roll:   func(int) int { return 10 },
	})
	liveSummon.SetAI(&recordingNetworkSummonAI{})
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{world: state}
	gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 1001})
	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("missed Sin Eater flavor-roll opcodes = %x, want ActionFailed only", got)
	}
}

func TestGameClientLinkSummonSkillUsePetBeyondLevelGapIsBlocked(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)
	live.SetTargetTracked(live)

	livePet := summon.NewPet(summon.PetConfig{
		ObjectID: 500, Owner: live, Level: live.LevelValue() + 21,
		Skills: map[int]int{4259: 1},
	})
	brain := &recordingNetworkSummonAI{}
	livePet.SetAI(brain)
	summon.SpawnBesideOwner(state, livePet, live, location.Location{})

	gcl := &GameClientLink{world: state}
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 36}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 0 {
		t.Fatalf("AI casts = %v, want none for a pet beyond the level gap", brain.casts)
	}
}

func TestGameClientLinkSummonSkillUseUnmappedActionFallsThrough(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	gcl := &GameClientLink{world: state}
	if gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 9999}) {
		t.Fatal("handleSummonActionUse = true for an action id with no command or skill mapping")
	}
}

func TestGameClientLinkSummonSkillUseDoorOnlyActionNeverDispatchesYet(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 300)
	state.Spawn(hostile, 100, 0, 0, 0)
	live.SetTargetTracked(hostile)

	liveSummon := summon.NewServitor(summon.ServitorConfig{
		ObjectID: 500, Owner: live, Level: 40,
		Skills: map[int]int{4079: 1},
	})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	gcl := &GameClientLink{world: state}
	// Action 1000 (Siege Golem - Siege Hammer) requires a Door target; no
	// Door world-object type exists yet, so it must never dispatch.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 1000}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 0 {
		t.Fatalf("AI casts = %v, want none for a door-only action with no Door target", brain.casts)
	}
}

// TestGameClientLinkSummonActionUseAttackRequiresForceOrCtrl is the
// regression test for the review finding that summonTargetAttackable used a
// plain AttackableBy check, dispatching ATTACK for any living player target
// (party members included) where Java's RequestActionUse.java:177 requires
// AttackableWithoutForceBy or an explicit Ctrl-press.
func TestGameClientLinkSummonActionUseAttackRequiresForceOrCtrl(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	// A party-member-like target: attackable with force (Ctrl) only, not
	// attackable without it.
	partyMember := &summonActionAttackTarget{id: 301, attackableWith: true}
	state.Spawn(partyMember, 150, 0, 0, 0)
	live.SetTargetTracked(partyMember)

	liveSummon := summon.NewServitor(summon.ServitorConfig{ObjectID: 500, Owner: live, Level: 40})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	gcl := &GameClientLink{world: state}

	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16}) {
		t.Fatal("handleSummonActionUse returned false for a summon attack command")
	}
	if len(brain.attacks) != 0 {
		t.Fatalf("AI attacks = %v without Ctrl pressed, want none (party member requires force)", brain.attacks)
	}
	if len(brain.follows) != 1 || brain.follows[0] != partyMember.ObjectID() {
		t.Fatalf("AI follows = %v without Ctrl pressed, want a follow onto the party member", brain.follows)
	}

	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16, CtrlPressed: true}) {
		t.Fatal("handleSummonActionUse returned false for a Ctrl-pressed summon attack command")
	}
	if len(brain.attacks) != 1 || brain.attacks[0] != partyMember.ObjectID() {
		t.Fatalf("AI attacks = %v with Ctrl pressed, want an attack onto the party member", brain.attacks)
	}
}

// TestGameClientLinkSummonActionUseAttackIgnoresFakeDeathButHonorsTrueDeath
// is the regression test for the review finding that the attack command's
// dead-target gate used AlikeDead() (fake death included) where Java's
// RequestActionUse.java:155-156 checks isDead() only ("Fake Death is
// handled elsewhere (attack task)").
func TestGameClientLinkSummonActionUseAttackIgnoresFakeDeathButHonorsTrueDeath(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	fakeDead := &summonActionAttackTarget{id: 301, attackableWithoutForce: true, alikeDead: true}
	state.Spawn(fakeDead, 150, 0, 0, 0)
	live.SetTargetTracked(fakeDead)

	liveSummon := summon.NewServitor(summon.ServitorConfig{ObjectID: 500, Owner: live, Level: 40})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	gcl := &GameClientLink{world: state}
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16}) {
		t.Fatal("handleSummonActionUse returned false for a summon attack command")
	}
	if len(brain.attacks) != 1 || brain.attacks[0] != fakeDead.ObjectID() {
		t.Fatalf("AI attacks = %v against a fake-dead target, want the swing to proceed", brain.attacks)
	}

	trueDead := &summonActionAttackTarget{id: 302, attackableWithoutForce: true, alikeDead: true, trueDead: true}
	state.Spawn(trueDead, 150, 0, 0, 0)
	live.SetTargetTracked(trueDead)

	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16}) {
		t.Fatal("handleSummonActionUse returned false for a summon attack command")
	}
	if len(brain.attacks) != 1 {
		t.Fatalf("AI attacks = %v against a truly dead target, want the command rejected", brain.attacks)
	}
}

type summonActionAttackTarget struct {
	world.Presence
	id                     int32
	alikeDead, trueDead    bool
	attackableWith         bool
	attackableWithoutForce bool
}

func (c *summonActionAttackTarget) ObjectID() int32  { return c.id }
func (c *summonActionAttackTarget) SiegeGuard() bool { return false }
func (c *summonActionAttackTarget) AlikeDead() bool  { return c.alikeDead }
func (c *summonActionAttackTarget) Dead() bool       { return c.trueDead }
func (c *summonActionAttackTarget) AttackableBy(target.Creature) bool {
	return c.attackableWith
}
func (c *summonActionAttackTarget) AttackableWithoutForceBy(target.Creature) bool {
	return c.attackableWithoutForce
}

type recordingNetworkSummonAI struct {
	attacks []int32
	follows []int32
	idles   int
	casts   []int32
}

func (a *recordingNetworkSummonAI) TryToAttack(target attackable.Combatant) bool {
	a.attacks = append(a.attacks, target.ObjectID())
	return true
}

func (a *recordingNetworkSummonAI) TryToFollow(target attackable.Combatant) bool {
	a.follows = append(a.follows, target.ObjectID())
	return true
}

func (a *recordingNetworkSummonAI) TryToIdle() {
	a.idles++
}

func (a *recordingNetworkSummonAI) TryToCast(target attackable.Combatant, ref modelskill.Ref) bool {
	a.casts = append(a.casts, target.ObjectID())
	return true
}

func (a *recordingNetworkSummonAI) AttackingNow() bool { return false }

type summonActionCombatant struct {
	world.Presence
	id   int32
	dead bool
}

func (c *summonActionCombatant) ObjectID() int32  { return c.id }
func (c *summonActionCombatant) SiegeGuard() bool { return false }
func (c *summonActionCombatant) AlikeDead() bool  { return c.dead }
