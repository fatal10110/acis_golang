// Package summon models the behavior shared by pets and servitors that
// isn't specific to feeding or leveling: owner-issued control commands and
// a servitor's timed lifetime.
package summon

// Command identifies an owner-issued instruction targeting their active
// pet or servitor.
type Command int

const (
	// CommandToggleFollow switches the summon between following its owner
	// and holding its current position.
	CommandToggleFollow Command = iota
	// CommandAttack orders the summon at the owner's current target.
	CommandAttack
	// CommandStop cancels whatever the summon is currently doing.
	CommandStop
	// CommandReturnPet sends a pet back into its collar item.
	CommandReturnPet
	// CommandUnsummonServitor dismisses a servitor early.
	CommandUnsummonServitor
	// CommandMoveToTarget orders the summon to approach or interact with
	// the owner's current target, dropping follow mode.
	CommandMoveToTarget
)

// Outcome is what accepting or rejecting a Command should cause.
type Outcome int

const (
	// OutcomeIgnored means the command has no effect and produces no
	// owner-visible feedback: the preconditions for even considering it
	// were not met (no active summon, no valid target, and so on).
	OutcomeIgnored Outcome = iota
	// OutcomeRefusedOutOfControl means the summon rejects direct orders
	// right now, and the owner is told so.
	OutcomeRefusedOutOfControl
	// OutcomeRefusedDead means the command needs a living summon.
	OutcomeRefusedDead
	// OutcomeRefusedInCombat means the command can't be carried out while
	// the summon is fighting or already attacking.
	OutcomeRefusedInCombat
	// OutcomeRefusedHungry means a pet's meal gauge is too low to send it
	// back into its collar right now.
	OutcomeRefusedHungry
	// OutcomeRefusedLevelGap means the summon has outgrown its owner's
	// ability to direct it into combat.
	OutcomeRefusedLevelGap
	// OutcomeApplied means the command's preconditions are met; the
	// caller carries out its actual effect through whichever system owns
	// that behavior (AI intentions, movement, unsummoning).
	OutcomeApplied
)

// Request carries every precondition needed to resolve a Command. Not every
// field applies to every Command; each command reads only the ones its own
// rules need.
type Request struct {
	Command Command

	// HasSummon reports whether the owner currently has an active pet or
	// servitor at all.
	HasSummon bool
	// IsPet is false when the active summon is a servitor.
	IsPet          bool
	SummonIsDead   bool
	OutOfControl   bool
	InCombat       bool
	IsAttackingNow bool

	HasTarget            bool
	TargetIsSummon       bool // the command's target is the owner's own summon
	TargetIsOwner        bool
	TargetIsDeadCreature bool
	// IsPassiveSummon marks a summon npc id that never takes a direct
	// attack order (e.g. a non-combat companion).
	IsPassiveSummon bool

	// FollowActive is the summon's current follow-owner state.
	FollowActive bool
	// OwnerWithinFollowRange only matters while FollowActive is true: an
	// owner who has drifted too far from a following summon can't call it
	// to a stop.
	OwnerWithinFollowRange bool

	SummonLevel int
	OwnerLevel  int

	// BelowUnsummonFeedShare is a pet's own precondition: its meal gauge
	// is under the share of its max that blocks sending it back into its
	// collar.
	BelowUnsummonFeedShare bool
}

// Resolve decides what a Command should cause, given the preconditions in
// req. It never touches world state; OutcomeApplied means the caller should
// carry out the command through the systems that own movement, AI
// intentions, and unsummoning.
func Resolve(req Request) Outcome {
	switch req.Command {
	case CommandToggleFollow:
		return resolveToggleFollow(req)
	case CommandAttack:
		return resolveAttack(req)
	case CommandStop:
		return resolveStop(req)
	case CommandReturnPet:
		return resolveReturnPet(req)
	case CommandUnsummonServitor:
		return resolveUnsummonServitor(req)
	case CommandMoveToTarget:
		return resolveMoveToTarget(req)
	default:
		return OutcomeIgnored
	}
}

func resolveToggleFollow(req Request) Outcome {
	if !req.HasSummon {
		return OutcomeIgnored
	}
	// An owner who has drifted too far from a following summon can't
	// silently call it to a halt.
	if req.FollowActive && !req.OwnerWithinFollowRange {
		return OutcomeIgnored
	}
	if req.OutOfControl {
		return OutcomeRefusedOutOfControl
	}
	return OutcomeApplied
}

func resolveAttack(req Request) Outcome {
	if !req.HasTarget || !req.HasSummon || req.TargetIsSummon || req.TargetIsOwner {
		return OutcomeIgnored
	}
	if req.TargetIsDeadCreature {
		return OutcomeIgnored
	}
	if req.IsPassiveSummon {
		return OutcomeIgnored
	}
	if req.OutOfControl {
		return OutcomeRefusedOutOfControl
	}
	if req.IsPet && req.SummonLevel-req.OwnerLevel > 20 {
		return OutcomeRefusedLevelGap
	}
	return OutcomeApplied
}

func resolveStop(req Request) Outcome {
	if !req.HasSummon {
		return OutcomeIgnored
	}
	if req.OutOfControl {
		return OutcomeRefusedOutOfControl
	}
	return OutcomeApplied
}

func resolveReturnPet(req Request) Outcome {
	if !req.IsPet {
		return OutcomeIgnored
	}
	if req.SummonIsDead {
		return OutcomeRefusedDead
	}
	if req.OutOfControl {
		return OutcomeRefusedOutOfControl
	}
	if req.IsAttackingNow || req.InCombat {
		return OutcomeRefusedInCombat
	}
	if req.BelowUnsummonFeedShare {
		return OutcomeRefusedHungry
	}
	return OutcomeApplied
}

func resolveUnsummonServitor(req Request) Outcome {
	if req.IsPet || !req.HasSummon {
		return OutcomeIgnored
	}
	if req.SummonIsDead {
		return OutcomeRefusedDead
	}
	if req.OutOfControl {
		return OutcomeRefusedOutOfControl
	}
	if req.IsAttackingNow || req.InCombat {
		return OutcomeRefusedInCombat
	}
	return OutcomeApplied
}

func resolveMoveToTarget(req Request) Outcome {
	if !req.HasTarget || !req.HasSummon || req.TargetIsSummon {
		return OutcomeIgnored
	}
	if req.OutOfControl {
		return OutcomeRefusedOutOfControl
	}
	return OutcomeApplied
}
