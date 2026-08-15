package summon

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// baseBuffSlots is the non-toggle, non-seven-signs buff-slot count every
// pet or servitor holds, matching the same fixed cap player.Character uses
// (see baseBuffSlots in character_effects.go) since no passive modeled here
// raises it.
const baseBuffSlots = 20

// AI is the summon intention loop controlled by owner commands and effects.
type AI interface {
	TryToAttack(attackable.Combatant) bool
	TryToFollow(attackable.Combatant) bool
	TryToIdle()
	TryToCast(target attackable.Combatant, ref modelskill.Ref) bool
}

// Owner is the live player surface a summon needs for world placement and
// command preconditions.
type Owner interface {
	world.Tracked
	LevelValue() int
	Position() (int, int, int)
}

// Actor is a live pet or servitor placed in world.State next to its owner.
//
// State methods guard the embedded Presence. level, pet growth state, name,
// fed, belowUnsummonLimit, lifetime, combat stat bases, and invul are guarded
// by statusMu; HP, MP, and dead are guarded by vitals.mu. Both are safe to
// read from any goroutine,
// including the world-visibility goroutine driving Discover. The remaining
// fields are mutated by the goroutine handling the owner connection or by the
// actor's own tick callback, so callers must serialize command and tick calls
// per actor.
type Actor struct {
	world.Presence

	// movement is this summon's lifetime move state, matching
	// creature.Live.movement (internal/gameserver/model/actor/creature/live.go):
	// zero-value until InitMovement wires real geodata/speed, so Move().Moving()
	// stays false (not an error) for a summon with no movement controller.
	movement move.CreatureMove

	id      int32
	owner   Owner
	world   *world.State
	los     LineOfSight
	isPet   bool
	npcID   int
	radius  float64
	height  float64
	passive bool

	// statusMu guards level, pet growth state, name, fed, belowUnsummonLimit,
	// lifetime, combat stat bases, invul, and
	// statusUpdater:
	// petInfoSnapshot (internal/gameserver/network/visibility.go) reads them
	// from the world-visibility goroutine via Level/Name/Fed/Lifetime while
	// the owner-connection and tick goroutines write them, per
	// world.Observer's concurrency contract
	// (internal/gameserver/world/visibility.go).
	statusMu       sync.RWMutex
	invul          bool
	level          int
	name           string
	named          bool
	lifetime       LifetimeState
	statusUpdater  func()
	damageNotifier func(string, int32)
	expNotifier    func(int64)
	dead           bool
	disabled       bool
	combat         bool
	attack         bool
	brain          AI
	onDespawn      func()
	// skills maps skill id to the level this summon's npc template grants
	// it, used by TryUseSkill to resolve an owner-commanded action-bar
	// skill shortcut, matching Java's Summon.getSkill.
	skills map[int]int
	zones  peaceZoneQuery

	followActive       bool
	belowUnsummonLimit bool
	intent             Intent
	target             worldobject.Object

	ownerInventory   *itemcontainer.Inventory
	timeLostIdle     int
	timeLostActive   int
	itemConsumeID    int32
	itemConsumeCount int

	petInventory  *itemcontainer.Inventory
	petConfig     *petmodel.Config
	growth        *npc.PetData
	controlItemID int32
	exp           int64
	sp            int
	expType       int
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

	stats                  CombatStats
	statCalc               summonStatCalcs
	vitals                 summonVitals
	effects                *effect.List
	stateMu                sync.RWMutex
	paralyzed, teleporting bool

	abnormalEffect   atomic.Int32
	abnormalMu       sync.RWMutex
	onAbnormalUpdate func()

	shotsMu        sync.Mutex
	shotsMask      int32
	skillMu        sync.Mutex // guards disabledSkills
	disabledSkills map[int32]time.Time
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
	ObjectID        int32
	Owner           Owner
	ControlItemID   int32
	NPCID           int
	CollisionRadius float64
	CollisionHeight float64
	Name            string
	// Named reports whether Name is a player-assigned custom name rather
	// than a fallback to the npc template's name; it gates RequestChangePetName's
	// "pet is already named" rejection (Pet.getName() != null in the reference).
	Named   bool
	Level   int
	Exp     int64
	SP      int
	ExpType int
	CON     int
	Passive bool
	Config  *petmodel.Config
	Growth  *npc.PetData

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
	Stats         CombatStats
	// Skills maps skill id to level, from this pet's npc template. See
	// Actor.skills.
	Skills map[int]int
}

// ServitorConfig carries the minimum state needed to create a live servitor.
type ServitorConfig struct {
	ObjectID        int32
	Owner           Owner
	NPCID           int
	CollisionRadius float64
	CollisionHeight float64
	Name            string
	Level           int
	Passive         bool

	OwnerInventory   *itemcontainer.Inventory
	Lifetime         LifetimeState
	TimeLostIdle     int
	TimeLostActive   int
	ItemConsumeID    int32
	ItemConsumeCount int
	Roll             func(int) int
	Stats            CombatStats
	// Skills maps skill id to level, from this servitor's npc template.
	// See Actor.skills.
	Skills map[int]int
}

// NewServitor returns a live servitor actor.
func NewServitor(cfg ServitorConfig) *Actor {
	a := &Actor{
		id:               cfg.ObjectID,
		owner:            cfg.Owner,
		level:            cfg.Level,
		npcID:            cfg.NPCID,
		radius:           cfg.CollisionRadius,
		height:           cfg.CollisionHeight,
		name:             cfg.Name,
		passive:          cfg.Passive,
		followActive:     true,
		intent:           IntentFollowOwner,
		ownerInventory:   cfg.OwnerInventory,
		lifetime:         cfg.Lifetime,
		timeLostIdle:     defaultPositive(cfg.TimeLostIdle, 1000),
		timeLostActive:   defaultPositive(cfg.TimeLostActive, 1000),
		itemConsumeID:    cfg.ItemConsumeID,
		itemConsumeCount: cfg.ItemConsumeCount,
		roll:             defaultRoll(cfg.Roll),
		stats:            cfg.Stats,
		skills:           cfg.Skills,
	}
	a.initVitals()
	a.effects = effect.NewList(a)
	return a
}

// NewPet returns a live pet actor.
func NewPet(cfg PetConfig) *Actor {
	petCfg := copyPetConfig(cfg.Config)
	if petCfg != nil && cfg.Inventory != nil {
		slots, weight := petCfg.InventoryLimits(cfg.CON)
		cfg.Inventory.SlotLimit = slots
		cfg.Inventory.WeightLimit = weight
	}
	a := &Actor{
		id:            cfg.ObjectID,
		owner:         cfg.Owner,
		level:         cfg.Level,
		isPet:         true,
		npcID:         cfg.NPCID,
		radius:        cfg.CollisionRadius,
		height:        cfg.CollisionHeight,
		name:          cfg.Name,
		named:         cfg.Named,
		passive:       cfg.Passive,
		followActive:  true,
		intent:        IntentFollowOwner,
		petInventory:  cfg.Inventory,
		petConfig:     petCfg,
		growth:        cfg.Growth,
		controlItemID: cfg.ControlItemID,
		exp:           cfg.Exp,
		sp:            cfg.SP,
		expType:       cfg.ExpType,
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
		stats:         cfg.Stats,
		skills:        cfg.Skills,
	}
	a.initVitals()
	a.effects = effect.NewList(a)
	return a
}

func copyPetConfig(cfg *petmodel.Config) *petmodel.Config {
	if cfg == nil {
		return nil
	}
	copied := *cfg
	return &copied
}

// ObjectID returns the live world object id assigned to this summon.
