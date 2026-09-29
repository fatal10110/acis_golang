package summon

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
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
		a.setFollowStatus(a.followOff.Load())
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
		a.TryToIdle()
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
// upkeep when a checkpoint is crossed. A dead servitor's lifetime stops.
func (a *Actor) TickServitor(state *world.State) TickResult {
	if a == nil || a.isPet || a.Dead() {
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
	if !upkeep || a.itemConsumeID == 0 || a.itemConsumeCount <= 0 {
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
// inventory when the auto-feed threshold is crossed. A dead pet is not fed:
// its gauge and food stay as they were until it is revived.
func (a *Actor) TickPet(state *world.State) PetTickResult {
	if a == nil || !a.isPet || a.Dead() {
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

// LeaveWithOwner handles this summon's owner leaving the world. A living
// summon leaves with it. A corpse stays where it lies until its decay removes
// it, and its owner's session is done with it (event.CorpseLeftBehind): a
// pet's items and row are settled now, while the owner's inventory is still
// here to take them, and the corpse's work moves to a queue of its own
// (AdoptCorpseQueue).
//
// A pet's corpse keeps its owner's summon slot, which the owner finds taken
// again on the next login until the corpse decays. A servitor's corpse gives
// the slot up, so the owner can summon again at once. A corpse whose owner
// already left once is not settled again.
//
// A corpse with no decay pending, a pet restored dead from its saved row,
// leaves with its owner like a living summon: nothing would ever remove it,
// and without a revive (#2679) its owner could not summon again until the
// server restarts.
func (a *Actor) LeaveWithOwner() {
	if !a.Dead() || !a.HasCorpse() {
		a.despawn(nil)
		return
	}
	if !a.ownerLeft.CompareAndSwap(false, true) {
		return
	}
	if !a.isPet && a.world != nil {
		a.world.RemoveSummon(a.OwnerID(), a)
	}
	a.emit(event.CorpseLeftBehind{})
}

// OwnerLeft reports whether this summon's owner left the world while it lay
// dead, leaving its corpse behind.
func (a *Actor) OwnerLeft() bool { return a.ownerLeft.Load() }

// ShownAsOwnedBy reports whether playerID sees this summon as their own. A
// pet's corpse stays its owner's across a relog; a servitor's corpse left
// behind is nobody's, since its owner may already have summoned another.
func (a *Actor) ShownAsOwnedBy(playerID int32) bool {
	return a.OwnerID() == playerID && (a.isPet || !a.OwnerLeft())
}

// AdoptCorpseQueue moves the work of a corpse whose owner left onto q, a
// queue of its own, since the owner's queue closes with the owner's session.
// Its decay runs there. Nothing else moves: a corpse no longer acts.
func (a *Actor) AdoptCorpseQueue(q *sim.Queue) {
	a.queue.Store(q)
}

// despawn takes a out of the world and reports whether this call did so.
func (a *Actor) despawn(state *world.State) bool {
	if state == nil {
		state = a.world
	}
	if state == nil {
		return false
	}
	ran := false
	// Only the first caller despawns, and a concurrent one returns only once
	// it has finished. An owner's command, a hostile Erase, a signet and the
	// owner's logout can each reach here on their own goroutines: a second
	// pass would settle the pet twice, its RemoveSummon could clear a newer
	// summon the owner has called since, and a logout that returned early
	// would flush the owner's inventory while the pet's items were still
	// moving into it.
	a.despawnOnce.Do(func() {
		ran = true
		// Unsummoning aborts in-flight actions and settles a pet, while
		// observers still know this summon.
		a.emit(event.Unsummoning{})
		state.RemoveSummon(a.OwnerID(), a)
		// The owner receives PetDelete even when outside the summon's view.
		// Despawn's Forget callbacks then send DeleteObject to old observers.
		a.emit(event.SummonRemoved{})
		state.Despawn(a)
		// Stop the periodic effect sweep from reaching this summon's list
		// once it leaves the world for good, even if it still holds a buff.
		a.EffectList().Untrack()
		a.emit(event.Despawned{})
	})
	return ran
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
