package summon

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (a *Actor) ApplyCommand(ctx CommandContext) CommandResult {
	outcome := Resolve(a.resolveRequest(ctx))
	result := CommandResult{Outcome: outcome, Feedback: feedbackFor(outcome), Intent: a.intent}
	if outcome != OutcomeApplied {
		return result
	}

	switch ctx.Command {
	case CommandToggleFollow:
		a.followActive = !a.followActive
		if a.followActive {
			a.intent = IntentFollowOwner
			a.TryToFollow(a.OwnerCombatant())
		} else {
			a.TryToIdle()
		}
	case CommandAttack:
		a.target = ctx.Target
		if ctx.TargetIsCreature && ctx.TargetAttackable {
			a.intent = IntentAttackTarget
			a.TryToAttack(ctx.Target)
		} else if ctx.TargetIsCreature {
			a.intent = IntentFollowTarget
			a.TryToFollow(ctx.Target)
		} else {
			a.intent = IntentInteractTarget
		}
	case CommandStop:
		a.TryToIdle()
	case CommandReturnPet, CommandUnsummonServitor:
		a.TryToIdle()
		a.despawn(ctx.World)
	case CommandMoveToTarget:
		a.followActive = false
		a.target = ctx.Target
		if ctx.TargetIsCreature {
			a.intent = IntentFollowTarget
			a.TryToFollow(ctx.Target)
		} else {
			a.intent = IntentInteractTarget
		}
	}
	result.Intent = a.intent
	return result
}

// TickServitor advances a servitor's live lifetime and consumes owner
// upkeep when a checkpoint is crossed.
func (a *Actor) TickServitor(state *world.State) TickResult {
	if a == nil || a.isPet {
		return TickResult{}
	}

	cost := a.timeLostIdle
	if a.combat {
		cost = a.timeLostActive
	}
	next, expired, upkeep := Tick(a.lifetime, cost)
	a.lifetime = next

	result := TickResult{
		TimeRemaining: next.TimeRemaining,
		Expired:       expired,
		UpkeepDue:     upkeep,
	}
	if expired {
		a.despawn(state)
		result.Unsummoned = true
		return result
	}
	if !upkeep || a.itemConsumeID == 0 || a.itemConsumeCount <= 0 || a.dead {
		return result
	}
	if a.ownerInventory == nil || a.ownerInventory.DestroyByTemplateID(a.itemConsumeID, a.itemConsumeCount) == nil {
		a.despawn(state)
		result.Unsummoned = true
		return result
	}
	result.UpkeepConsumed = true
	return result
}

// StartServitorTicks schedules fixed-rate servitor lifetime/upkeep ticks.
func (a *Actor) StartServitorTicks(period time.Duration, state *world.State, log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(period, func() {
		a.TickServitor(state)
	}, log)
}

// TickPet advances a pet's live food gauge and consumes food from its own
// inventory when the auto-feed threshold is crossed.
func (a *Actor) TickPet(state *world.State) PetTickResult {
	if a == nil || !a.isPet {
		return PetTickResult{}
	}

	consume := a.mealInNormal
	if a.combat {
		consume = a.mealInBattle
	}
	a.fed = petmodel.NextFed(a.fed, consume)
	a.belowUnsummonLimit = petmodel.BelowShare(a.fed, a.maxMeal, a.unsummonLimit)

	result := PetTickResult{Fed: a.fed}
	if a.petInventory != nil && petmodel.BelowShare(a.fed, a.maxMeal, a.autoFeedLimit) {
		food := a.petInventory.ItemByTemplateID(a.food1)
		if food == nil && a.food2 != 0 {
			food = a.petInventory.ItemByTemplateID(a.food2)
		}
		if food != nil && a.petInventory.DestroyItem(food, 1) != nil {
			a.fed += a.foodRestore
			if a.fed > a.maxMeal {
				a.fed = a.maxMeal
			}
			a.belowUnsummonLimit = petmodel.BelowShare(a.fed, a.maxMeal, a.unsummonLimit)
			result.AutoFed = true
			result.Fed = a.fed
			return result
		}
	}
	result.Starvation = petmodel.Classify(a.fed, a.maxMeal)
	if result.Starvation != petmodel.StarvationNone && a.roll(100) < result.Starvation.LeaveChancePercent() {
		a.despawn(state)
		result.LeftOwner = true
		result.Unsummoned = true
	}
	return result
}

// StartPetFeed schedules pet feeding/starvation ticks.
func (a *Actor) StartPetFeed(period time.Duration, state *world.State, log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(period, func() {
		a.TickPet(state)
	}, log)
}

// Unsummon despawns this summon and detaches it from its owner.
func (a *Actor) Unsummon() {
	a.despawn(nil)
}

func (a *Actor) despawn(state *world.State) {
	if state == nil {
		state = a.world
	}
	if state == nil {
		return
	}
	state.Despawn(a)
	state.RemoveSummon(a.OwnerID())
}

func (a *Actor) resolveRequest(ctx CommandContext) Request {
	ownerLevel := 0
	if a.owner != nil {
		ownerLevel = a.owner.LevelValue()
	}
	return Request{
		Command:                ctx.Command,
		HasSummon:              a != nil,
		IsPet:                  a.isPet,
		SummonIsDead:           a.dead,
		OutOfControl:           a.disabled,
		InCombat:               a.combat,
		IsAttackingNow:         a.attack,
		HasTarget:              ctx.Target != nil,
		TargetIsSummon:         sameObject(ctx.Target, a),
		TargetIsOwner:          sameObject(ctx.Target, a.owner),
		TargetIsDeadCreature:   ctx.TargetIsDeadCreature,
		IsPassiveSummon:        a.passive,
		FollowActive:           a.followActive,
		OwnerWithinFollowRange: a.ownerWithinFollowRange(),
		SummonLevel:            a.level,
		OwnerLevel:             ownerLevel,
		BelowUnsummonFeedShare: a.belowUnsummonLimit,
	}
}
