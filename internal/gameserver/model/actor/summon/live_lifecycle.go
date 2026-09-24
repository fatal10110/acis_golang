package summon

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

func (a *Actor) ApplyCommand(ctx CommandContext) CommandResult {
	outcome := Resolve(a.resolveRequest(ctx))
	result := CommandResult{Outcome: outcome, Feedback: feedbackFor(outcome), Intent: a.Intent()}
	if outcome != OutcomeApplied {
		return result
	}

	switch ctx.Command {
	case CommandToggleFollow:
		if a.followOff.Load() {
			a.followOff.Store(false)
			a.setIntent(IntentFollowOwner)
			a.TryToFollow(a.owner)
		} else {
			a.followOff.Store(true)
			a.idle()
		}
	case CommandAttack:
		a.SetTarget(ctx.Target)
		if ctx.TargetIsCreature && ctx.TargetAttackable {
			a.setIntent(IntentAttackTarget)
			a.TryToAttack(ctx.Target)
		} else if ctx.TargetIsCreature {
			a.setIntent(IntentFollowTarget)
			a.TryToFollow(ctx.Target)
		} else {
			a.setIntent(IntentInteractTarget)
		}
	case CommandStop:
		a.idle()
	case CommandReturnPet, CommandUnsummonServitor:
		a.idle()
		a.despawn(ctx.World)
	case CommandMoveToTarget:
		a.followOff.Store(true)
		a.SetTarget(ctx.Target)
		if ctx.TargetIsCreature {
			a.setIntent(IntentFollowTarget)
			a.TryToFollow(ctx.Target)
		} else {
			a.setIntent(IntentInteractTarget)
		}
	}
	result.Intent = a.Intent()
	return result
}

// TickServitor advances a servitor's live lifetime and consumes owner
// upkeep when a checkpoint is crossed.
func (a *Actor) TickServitor(state *world.State) TickResult {
	if a == nil || a.isPet {
		return TickResult{}
	}

	cost := a.timeLostIdle
	if a.InCombat() {
		cost = a.timeLostActive
	}
	a.statusMu.Lock()
	next, expired, upkeep := Tick(a.lifetime, cost)
	a.lifetime = next
	a.statusMu.Unlock()

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
	if !upkeep || a.itemConsumeID == 0 || a.itemConsumeCount <= 0 || a.Dead() {
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

	inCombat := a.InCombat()
	a.statusMu.Lock()
	consume := a.mealInNormal
	if inCombat {
		consume = a.mealInBattle
	}
	a.fed = petmodel.NextFed(a.fed, consume)
	a.belowUnsummonLimit = petmodel.BelowShare(a.fed, a.maxMeal, a.unsummonLimit)
	fed, maxMeal := a.fed, a.maxMeal
	a.statusMu.Unlock()

	result := PetTickResult{Fed: fed}
	if a.petInventory != nil && petmodel.BelowShare(fed, maxMeal, a.autoFeedLimit) {
		food := a.petInventory.ItemByTemplateID(a.food1)
		restore := a.foodRestore1
		if food == nil && a.food2 != 0 {
			food = a.petInventory.ItemByTemplateID(a.food2)
			restore = a.foodRestore2
		}
		if food != nil && a.petInventory.DestroyItem(food, 1) != nil {
			a.statusMu.Lock()
			a.fed += restore
			if a.fed > a.maxMeal {
				a.fed = a.maxMeal
			}
			a.belowUnsummonLimit = petmodel.BelowShare(a.fed, a.maxMeal, a.unsummonLimit)
			fed = a.fed
			a.statusMu.Unlock()
			result.AutoFed = true
			result.Fed = fed
			return result
		}
	}
	result.Starvation = petmodel.Classify(fed, maxMeal)
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

// Unsummon despawns this summon and detaches it from its owner. A dead
// summon stays where it is: its corpse is not the owner's to recall.
func (a *Actor) Unsummon() {
	if a.Dead() {
		return
	}
	a.despawn(nil)
}

// LeaveWithOwner despawns this summon, dead or alive, because its owner is
// leaving the world. Summons are tracked under the owner's persistent object
// id, so a corpse left behind would still hold that owner's summon slot on
// the next login; nothing else would ever take it out until summon corpses
// decay (#2439).
func (a *Actor) LeaveWithOwner() {
	a.despawn(nil)
}

func (a *Actor) despawn(state *world.State) {
	if state == nil {
		state = a.world
	}
	if state == nil {
		return
	}
	// Only the first caller despawns, and a concurrent one returns only once
	// it has finished. An owner's command, a hostile Erase, a signet and the
	// owner's logout can each reach here on their own goroutines: a second
	// pass would settle the pet twice, its RemoveSummon could clear a newer
	// summon the owner has called since, and a logout that returned early
	// would flush the owner's inventory while the pet's items were still
	// moving into it.
	a.despawnOnce.Do(func() {
		// Unsummoning aborts in-flight actions and settles a pet, while
		// observers still know this summon.
		a.emit(event.Unsummoning{})
		// RemoveSummon runs before Despawn: Despawn's relocate step
		// synchronously fires the owner's Forget callback, which sends the
		// client-visible PetDelete frame. A caller (or a test synchronizing on
		// that frame, as TestGameClientLinkRoutesSummonActionUseToLiveSummon
		// does) must never observe world.State.Summon still reporting this
		// actor active once the client has been told it's gone.
		state.RemoveSummon(a.OwnerID())
		state.Despawn(a)
		// Stop the periodic effect sweep from reaching this summon's list
		// once it leaves the world for good, even if it still holds a buff.
		a.EffectList().Untrack()
		a.emit(event.Despawned{})
	})
}

func (a *Actor) resolveRequest(ctx CommandContext) Request {
	ownerLevel := 0
	if a.owner != nil {
		ownerLevel = a.owner.LevelValue()
	}
	a.statusMu.RLock()
	level, belowUnsummonLimit := a.level, a.belowUnsummonLimit
	a.statusMu.RUnlock()
	dead := a.Dead()
	return Request{
		Command:                ctx.Command,
		HasSummon:              a != nil,
		IsPet:                  a.isPet,
		SummonIsDead:           dead,
		OutOfControl:           a.OutOfControl(),
		InCombat:               a.InCombat(),
		IsAttackingNow:         a.IsAttackingNow(),
		HasTarget:              ctx.Target != nil,
		TargetIsSummon:         sameObject(ctx.Target, a),
		TargetIsOwner:          sameObject(ctx.Target, a.owner),
		TargetIsDeadCreature:   ctx.TargetIsDeadCreature,
		IsPassiveSummon:        a.passive,
		FollowActive:           !a.followOff.Load(),
		OwnerWithinFollowRange: a.ownerWithinFollowRange(),
		SummonLevel:            level,
		OwnerLevel:             ownerLevel,
		BelowUnsummonFeedShare: belowUnsummonLimit,
	}
}
