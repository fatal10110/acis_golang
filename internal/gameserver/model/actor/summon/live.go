package summon

import (
	"math/rand/v2"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Owner is the live player surface a summon needs for world placement and
// command preconditions.
type Owner interface {
	world.Tracked
	LevelValue() int
	Position() (int, int, int)
}

// Actor is a live pet or servitor placed in world.State next to its owner.
//
// State methods guard the embedded Presence. The remaining fields are
// mutated by the goroutine handling the owner connection or by the actor's
// own tick callback, so callers must serialize command and tick calls per
// actor.
type Actor struct {
	world.Presence

	id       int32
	owner    Owner
	world    *world.State
	level    int
	isPet    bool
	npcID    int
	passive  bool
	dead     bool
	disabled bool
	combat   bool
	attack   bool

	followActive       bool
	belowUnsummonLimit bool
	intent             Intent
	target             worldobject.Object

	ownerInventory   *itemcontainer.Inventory
	lifetime         LifetimeState
	timeLostIdle     int
	timeLostActive   int
	itemConsumeID    int32
	itemConsumeCount int

	petInventory  *itemcontainer.Inventory
	petConfig     *petmodel.Config
	fed           int
	maxMeal       int
	mealInNormal  int
	mealInBattle  int
	food1         int32
	food2         int32
	foodRestore   int
	autoFeedLimit float64
	hungryLimit   float64
	unsummonLimit float64
	roll          func(int) int
}

// Intent is the live action this actor is currently trying to carry out.
type Intent uint8

const (
	// IntentIdle means the summon is not actively moving, attacking, or
	// interacting.
	IntentIdle Intent = iota
	// IntentFollowOwner means the summon is following its owner.
	IntentFollowOwner
	// IntentAttackTarget means the summon is attacking its selected target.
	IntentAttackTarget
	// IntentFollowTarget means the summon is approaching a creature target.
	IntentFollowTarget
	// IntentInteractTarget means the summon is moving toward or using a
	// non-creature target.
	IntentInteractTarget
)

// Feedback identifies the owner-visible message an unapplied command should
// produce.
type Feedback uint8

const (
	// FeedbackNone means no owner-visible response is needed.
	FeedbackNone Feedback = iota
	// FeedbackPetRefusingOrder is shown when the summon is out of control.
	FeedbackPetRefusingOrder
	// FeedbackDeadPetCannotBeReturned is shown when a dead summon is
	// ordered back into its item or dismissed.
	FeedbackDeadPetCannotBeReturned
	// FeedbackPetCannotBeSentBackDuringBattle is shown while the summon is
	// fighting.
	FeedbackPetCannotBeSentBackDuringBattle
	// FeedbackCannotRestoreHungryPet is shown when a pet is too hungry to
	// return to its collar.
	FeedbackCannotRestoreHungryPet
	// FeedbackPetTooHighToControl is shown when a pet has outleveled its
	// owner by more than the allowed gap.
	FeedbackPetTooHighToControl
)

// CommandContext carries the live target and world state needed to apply an
// owner-issued summon command.
type CommandContext struct {
	Command Command
	World   *world.State
	Target  worldobject.Object

	TargetIsCreature     bool
	TargetIsDeadCreature bool
	TargetAttackable     bool
}

// CommandResult reports what applying a command did.
type CommandResult struct {
	Outcome  Outcome
	Feedback Feedback
	Intent   Intent
}

// TickResult reports the side effects of one live summon tick.
type TickResult struct {
	TimeRemaining  int
	Expired        bool
	UpkeepDue      bool
	UpkeepConsumed bool
	Unsummoned     bool
}

// PetTickResult reports the side effects of one live pet feeding tick.
type PetTickResult struct {
	Fed        int
	AutoFed    bool
	Starvation petmodel.StarvationTier
	LeftOwner  bool
	Unsummoned bool
}

// PetConfig carries the minimum state needed to create a live pet.
type PetConfig struct {
	ObjectID int32
	Owner    Owner
	NPCID    int
	Level    int
	CON      int
	Passive  bool
	Config   *petmodel.Config

	Inventory     *itemcontainer.Inventory
	Fed           int
	MaxMeal       int
	MealInNormal  int
	MealInBattle  int
	Food1         int32
	Food2         int32
	FoodRestore   int
	AutoFeedLimit float64
	HungryLimit   float64
	UnsummonLimit float64
	Roll          func(int) int
}

// ServitorConfig carries the minimum state needed to create a live servitor.
type ServitorConfig struct {
	ObjectID int32
	Owner    Owner
	NPCID    int
	Level    int
	Passive  bool

	OwnerInventory   *itemcontainer.Inventory
	Lifetime         LifetimeState
	TimeLostIdle     int
	TimeLostActive   int
	ItemConsumeID    int32
	ItemConsumeCount int
}

// NewServitor returns a live servitor actor.
func NewServitor(cfg ServitorConfig) *Actor {
	return &Actor{
		id:               cfg.ObjectID,
		owner:            cfg.Owner,
		level:            cfg.Level,
		npcID:            cfg.NPCID,
		passive:          cfg.Passive,
		followActive:     true,
		intent:           IntentFollowOwner,
		ownerInventory:   cfg.OwnerInventory,
		lifetime:         cfg.Lifetime,
		timeLostIdle:     defaultPositive(cfg.TimeLostIdle, 1000),
		timeLostActive:   defaultPositive(cfg.TimeLostActive, 1000),
		itemConsumeID:    cfg.ItemConsumeID,
		itemConsumeCount: cfg.ItemConsumeCount,
	}
}

// NewPet returns a live pet actor.
func NewPet(cfg PetConfig) *Actor {
	petCfg := copyPetConfig(cfg.Config)
	if petCfg != nil && cfg.Inventory != nil {
		slots, weight := petCfg.InventoryLimits(cfg.CON)
		cfg.Inventory.SlotLimit = slots
		cfg.Inventory.WeightLimit = weight
	}
	return &Actor{
		id:            cfg.ObjectID,
		owner:         cfg.Owner,
		level:         cfg.Level,
		isPet:         true,
		npcID:         cfg.NPCID,
		passive:       cfg.Passive,
		followActive:  true,
		intent:        IntentFollowOwner,
		petInventory:  cfg.Inventory,
		petConfig:     petCfg,
		fed:           cfg.Fed,
		maxMeal:       cfg.MaxMeal,
		mealInNormal:  cfg.MealInNormal,
		mealInBattle:  cfg.MealInBattle,
		food1:         cfg.Food1,
		food2:         cfg.Food2,
		foodRestore:   cfg.FoodRestore,
		autoFeedLimit: cfg.AutoFeedLimit,
		hungryLimit:   cfg.HungryLimit,
		unsummonLimit: cfg.UnsummonLimit,
		roll:          defaultRoll(cfg.Roll),
	}
}

func copyPetConfig(cfg *petmodel.Config) *petmodel.Config {
	if cfg == nil {
		return nil
	}
	copied := *cfg
	return &copied
}

// ObjectID returns the live world object id assigned to this summon.
func (a *Actor) ObjectID() int32 { return a.id }

// OwnerID returns the owning player's world object id.
func (a *Actor) OwnerID() int32 {
	if a.owner == nil {
		return 0
	}
	return a.owner.ObjectID()
}

// Level returns the summon's current level.
func (a *Actor) Level() int { return a.level }

// IsPet reports whether this live summon is a pet rather than a servitor.
func (a *Actor) IsPet() bool { return a.isPet }

// SummonType returns the client-visible summon type code.
func (a *Actor) SummonType() int {
	if a.isPet {
		return 2
	}
	return 1
}

// NPCID returns the template id backing this summon.
func (a *Actor) NPCID() int { return a.npcID }

// ScaledExpGain returns rawExp multiplied by this pet's configured
// experience rate.
func (a *Actor) ScaledExpGain(rawExp int64) int64 {
	if a == nil || !a.isPet {
		return 0
	}
	if a.petConfig == nil {
		return petmodel.DefaultConfig().ScaledExpGain(a.npcID, rawExp)
	}
	return a.petConfig.ScaledExpGain(a.npcID, rawExp)
}

// CanWearPetItem reports whether this pet can equip tmpl.
func (a *Actor) CanWearPetItem(tmpl *item.Template) bool {
	if a == nil || tmpl == nil {
		return false
	}
	switch a.npcID {
	case 12311, 12312, 12313:
		return tmpl.Slot == item.SlotHatchling
	case 12077:
		return tmpl.Slot == item.SlotWolf
	case 12526, 12527, 12528:
		return tmpl.Slot == item.SlotStrider
	case 12780, 12781, 12782:
		return tmpl.Slot == item.SlotBabyPet
	default:
		return false
	}
}

// Dead reports whether the summon is dead.
func (a *Actor) Dead() bool { return a.dead }

// OutOfControl reports whether the owner cannot currently command this
// summon.
func (a *Actor) OutOfControl() bool { return a.disabled }

// PetInventory returns the pet's inventory, or nil for servitors.
func (a *Actor) PetInventory() *itemcontainer.Inventory {
	if !a.isPet {
		return nil
	}
	return a.petInventory
}

// Fed returns a pet's current meal gauge.
func (a *Actor) Fed() int { return a.fed }

// FollowActive reports whether this actor is following its owner.
func (a *Actor) FollowActive() bool { return a.followActive }

// Intent returns the live action this actor is currently pursuing.
func (a *Actor) Intent() Intent { return a.intent }

// ApplyCommand resolves and applies an owner-issued control command.
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
		} else {
			a.intent = IntentIdle
		}
	case CommandAttack:
		a.target = ctx.Target
		if ctx.TargetIsCreature && ctx.TargetAttackable {
			a.intent = IntentAttackTarget
		} else if ctx.TargetIsCreature {
			a.intent = IntentFollowTarget
		} else {
			a.intent = IntentInteractTarget
		}
	case CommandStop:
		a.intent = IntentIdle
	case CommandReturnPet, CommandUnsummonServitor:
		a.intent = IntentIdle
		a.despawn(ctx.World)
	case CommandMoveToTarget:
		a.followActive = false
		a.target = ctx.Target
		if ctx.TargetIsCreature {
			a.intent = IntentFollowTarget
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

func sameObject(a, b worldobject.Object) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ObjectID() == b.ObjectID()
}

func (a *Actor) ownerWithinFollowRange() bool {
	if a.owner == nil {
		return false
	}
	ax, ay, az := a.Position()
	bx, by, bz := a.owner.Position()
	return location.In3DRange(ax, ay, az, bx, by, bz, 2000)
}

func feedbackFor(outcome Outcome) Feedback {
	switch outcome {
	case OutcomeRefusedOutOfControl:
		return FeedbackPetRefusingOrder
	case OutcomeRefusedDead:
		return FeedbackDeadPetCannotBeReturned
	case OutcomeRefusedInCombat:
		return FeedbackPetCannotBeSentBackDuringBattle
	case OutcomeRefusedHungry:
		return FeedbackCannotRestoreHungryPet
	case OutcomeRefusedLevelGap:
		return FeedbackPetTooHighToControl
	default:
		return FeedbackNone
	}
}

func defaultPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultRoll(roll func(int) int) func(int) int {
	if roll != nil {
		return roll
	}
	return rand.IntN
}

// SpawnBesideOwner places actor in state at owner plus offset and registers
// it as the owner's active summon.
func SpawnBesideOwner(state *world.State, actor *Actor, owner Owner, offset location.Location) {
	if state == nil || actor == nil || owner == nil {
		return
	}
	actor.owner = owner
	actor.world = state
	x, y, z := owner.Position()
	state.Spawn(actor, x+offset.X, y+offset.Y, z+offset.Z, ownerHeading(owner))
	state.AddSummon(owner.ObjectID(), actor)
}

func ownerHeading(owner Owner) int {
	h, ok := owner.(interface{ Heading() int })
	if !ok {
		return 0
	}
	return h.Heading()
}
