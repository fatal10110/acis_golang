package network

import (
	"context"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func (l *GameClientLink) handleSummonActionUse(ctx context.Context, live *livePlayer, req clientpackets.RequestActionUse) bool {
	command, ok := summonCommandForActionID(req.ActionID)
	if !ok {
		return l.handleSummonSkillUse(live, req)
	}
	if l.world == nil {
		return false
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		// A pet-command shortcut with no active summon to command is
		// still a claimed, resolved action — the client must be
		// released, not left waiting for a response that never comes.
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	actor, ok := obj.(*summon.Actor)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	result := actor.ApplyCommand(l.summonCommandContext(live, command))
	if id, ok := systemMessageForSummonFeedback(result.Feedback); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(id))
	}
	if result.Outcome == summon.OutcomeApplied && (command == summon.CommandReturnPet || command == summon.CommandUnsummonServitor) {
		l.savePet(context.WithoutCancel(ctx), actor)
		l.transferPetInventory(actor, live.Inventory())
		// ApplyCommand's despawn (inside CommandReturnPet/CommandUnsummonServitor)
		// synchronously triggers world visibility's Forget callback, which
		// sends the owner PetDelete (network/visibility.go) — no explicit
		// send needed here.
		//
		// Unsummoning detaches the pet inventory's notifier so its closure
		// stops holding live; lifecycle.go does the same for a still-active
		// pet on logout. The pet's container also goes away here, so its
		// items are flushed and unregistered on the same path logout uses —
		// otherwise they'd sit in the persistence task's pending set
		// referencing a despawned pet.
		if inv := actor.PetInventory(); inv != nil {
			inv.SetUpdateNotifier(nil)
			flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), livePlayerDetachSaveTimeout)
			defer cancel()
			l.flushItemPersistence(flushCtx, inv)
		}
	}
	return true
}

// summonSkillTargetKind identifies which live object an owner-commanded
// special-skill action targets, mirroring the WorldObject argument Java's
// RequestActionUse passes to each useSkill(skillId, ...) call.
type summonSkillTargetKind uint8

const (
	// summonSkillTargetClicked uses the owner's current client-side target.
	summonSkillTargetClicked summonSkillTargetKind = iota
	// summonSkillTargetOwner uses the commanding player themself.
	summonSkillTargetOwner
	// summonSkillTargetSelf uses the summon itself (e.g. Sin Eater's
	// Ultimate Bombastic Buster casting on its own owner's summon).
	summonSkillTargetSelf
)

// summonSkillEntry is one RequestActionUse action-id-to-skill mapping for a
// pet/servitor's commanded special skill.
type summonSkillEntry struct {
	SkillID    int
	TargetKind summonSkillTargetKind
	// DoorOnly marks the two siege actions (Siege Golem's Siege Hammer,
	// Wild Hog Cannon's Attack) Java restricts to a Door target. No Door
	// world-object type is modeled in Go yet, so this always evaluates to
	// "not a Door" and these two actions correctly stay unusable until
	// siege doors are ported, matching what a target-less/non-door click
	// already does in the reference.
	DoorOnly bool
}

const sinEaterNPCID = 12564

var sinEaterActionStrings = [...]string{
	"special skill? Abuses in this kind of place, can turn blood Knots...!",
	"Hey! Brother! What do you anticipate to me?",
	"shouts ha! Flap! Flap! Response?",
	", has not hit...!",
}

// summonSkillActionTable mirrors every commanded special-skill case in
// Java's RequestActionUse.runImpl. Action 32 (Wild Hog Cannon Mode Change)
// is intentionally absent: the reference's own case body is commented out,
// making it a real no-op in Java too.
var summonSkillActionTable = map[int32]summonSkillEntry{
	36:   {SkillID: 4259, TargetKind: summonSkillTargetClicked},
	39:   {SkillID: 4138, TargetKind: summonSkillTargetClicked},
	41:   {SkillID: 4230, TargetKind: summonSkillTargetClicked, DoorOnly: true},
	42:   {SkillID: 4378, TargetKind: summonSkillTargetOwner},
	43:   {SkillID: 4137, TargetKind: summonSkillTargetClicked},
	44:   {SkillID: 4139, TargetKind: summonSkillTargetClicked},
	45:   {SkillID: 4025, TargetKind: summonSkillTargetOwner},
	46:   {SkillID: 4261, TargetKind: summonSkillTargetClicked},
	47:   {SkillID: 4260, TargetKind: summonSkillTargetClicked},
	48:   {SkillID: 4068, TargetKind: summonSkillTargetClicked},
	1000: {SkillID: 4079, TargetKind: summonSkillTargetClicked, DoorOnly: true},
	// 1001 (Sin Eater's Ultimate Bombastic Buster) targets the summon itself.
	1001: {SkillID: 4139, TargetKind: summonSkillTargetSelf},
	1003: {SkillID: 4710, TargetKind: summonSkillTargetClicked},
	1004: {SkillID: 4711, TargetKind: summonSkillTargetOwner},
	1005: {SkillID: 4712, TargetKind: summonSkillTargetClicked},
	1006: {SkillID: 4713, TargetKind: summonSkillTargetOwner},
	1007: {SkillID: 4699, TargetKind: summonSkillTargetOwner},
	1008: {SkillID: 4700, TargetKind: summonSkillTargetOwner},
	1009: {SkillID: 4701, TargetKind: summonSkillTargetClicked},
	1010: {SkillID: 4702, TargetKind: summonSkillTargetOwner},
	1011: {SkillID: 4703, TargetKind: summonSkillTargetOwner},
	1012: {SkillID: 4704, TargetKind: summonSkillTargetClicked},
	1013: {SkillID: 4705, TargetKind: summonSkillTargetClicked},
	1014: {SkillID: 4706, TargetKind: summonSkillTargetOwner},
	1015: {SkillID: 4707, TargetKind: summonSkillTargetClicked},
	1016: {SkillID: 4709, TargetKind: summonSkillTargetClicked},
	1017: {SkillID: 4708, TargetKind: summonSkillTargetClicked},
	1031: {SkillID: 5135, TargetKind: summonSkillTargetClicked},
	1032: {SkillID: 5136, TargetKind: summonSkillTargetClicked},
	1033: {SkillID: 5137, TargetKind: summonSkillTargetClicked},
	1034: {SkillID: 5138, TargetKind: summonSkillTargetClicked},
	1035: {SkillID: 5139, TargetKind: summonSkillTargetClicked},
	1036: {SkillID: 5142, TargetKind: summonSkillTargetClicked},
	1037: {SkillID: 5141, TargetKind: summonSkillTargetClicked},
	1038: {SkillID: 5140, TargetKind: summonSkillTargetClicked},
	// 1039/1040 (Swoop Cannon) reject a Door target in Java. No Door type
	// exists in Go yet, so no target can match one and this restriction is
	// vacuously satisfied — safe to omit without behavior divergence.
	1039: {SkillID: 5110, TargetKind: summonSkillTargetClicked},
	1040: {SkillID: 5111, TargetKind: summonSkillTargetClicked},
}

// doorTarget is satisfied by a future siege-door world object. Nothing in
// the codebase implements it yet, so doorOnlyBlocked always returns true
// for a DoorOnly entry until siege doors are ported.
type doorTarget interface {
	IsSiegeDoor() bool
}

func doorOnlyBlocked(entry summonSkillEntry, target attackable.Combatant) bool {
	if !entry.DoorOnly {
		return false
	}
	door, ok := target.(doorTarget)
	return !ok || !door.IsSiegeDoor()
}

// handleSummonSkillUse dispatches an owner-commanded pet/servitor special
// skill (e.g. Wild Hog Cannon Attack), matching Java's
// RequestActionUse.useSkill. Unlike handleSummonActionUse's movement/status
// commands, an unresolvable summon or skill still returns true so the
// client's input is released, matching the existing action-bar contract;
// only an unmapped action id returns false.
func (l *GameClientLink) handleSummonSkillUse(live *livePlayer, req clientpackets.RequestActionUse) bool {
	entry, ok := summonSkillActionTable[req.ActionID]
	if !ok || l.world == nil {
		return false
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	actor, ok := obj.(*summon.Actor)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}

	target := l.summonSkillTarget(live, actor, entry.TargetKind)
	if !doorOnlyBlocked(entry, target) {
		if actor.TryUseSkill(entry.SkillID, target) && req.ActionID == 1001 && actor.NPCID() == sinEaterNPCID && actor.Roll(100) < 10 {
			actor.BroadcastFrame(serverpackets.FrameNpcSay(actor.ObjectID(), actor.NPCID(), serverpackets.SayTypeAll, sinEaterActionStrings[actor.Roll(len(sinEaterActionStrings))]))
		}
	}
	live.SendFrame(serverpackets.FrameActionFailed())
	return true
}

func (l *GameClientLink) summonSkillTarget(live *livePlayer, actor *summon.Actor, kind summonSkillTargetKind) attackable.Combatant {
	switch kind {
	case summonSkillTargetOwner:
		return live.Character
	case summonSkillTargetSelf:
		return actor
	default:
		target, _ := live.Target().(attackable.Combatant)
		return target
	}
}

func (l *GameClientLink) summonCommandContext(live *livePlayer, command summon.Command) summon.CommandContext {
	ctx := summon.CommandContext{Command: command, World: l.world}
	if live == nil || live.Target() == nil {
		return ctx
	}
	ctx.Target = live.Target()
	target, ok := live.Target().(attackable.Combatant)
	if !ok {
		return ctx
	}
	ctx.TargetIsCreature = true
	ctx.TargetIsDeadCreature = target.AlikeDead()
	ctx.TargetAttackable = summonTargetAttackable(live, target)
	return ctx
}

func summonTargetAttackable(live *livePlayer, target attackable.Combatant) bool {
	if live == nil || target == nil {
		return false
	}
	attackableTarget, ok := target.(interface {
		AttackableBy(skilltarget.Creature) bool
	})
	return ok && attackableTarget.AttackableBy(live.Character)
}

func summonCommandForActionID(actionID int32) (summon.Command, bool) {
	switch actionID {
	case 15, 21:
		return summon.CommandToggleFollow, true
	case 16, 22:
		return summon.CommandAttack, true
	case 17, 23:
		return summon.CommandStop, true
	case 19:
		return summon.CommandReturnPet, true
	case 52:
		return summon.CommandUnsummonServitor, true
	case 53, 54:
		return summon.CommandMoveToTarget, true
	default:
		return 0, false
	}
}

func systemMessageForSummonFeedback(feedback summon.Feedback) (int, bool) {
	switch feedback {
	case summon.FeedbackPetRefusingOrder:
		return serverpackets.SystemMessagePetRefusingOrder, true
	case summon.FeedbackDeadPetCannotBeReturned:
		return serverpackets.SystemMessageDeadPetCannotBeReturned, true
	case summon.FeedbackPetCannotBeSentBackDuringBattle:
		return serverpackets.SystemMessagePetCannotSentBackDuringBattle, true
	case summon.FeedbackCannotRestoreHungryPet:
		return serverpackets.SystemMessageYouCannotRestoreHungryPets, true
	case summon.FeedbackPetTooHighToControl:
		return serverpackets.SystemMessagePetTooHighToControl, true
	default:
		return 0, false
	}
}
