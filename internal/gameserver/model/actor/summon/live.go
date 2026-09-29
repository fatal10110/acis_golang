package summon

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// baseBuffSlots is the shipped players.properties MaxBuffsAmount default.
const baseBuffSlots = 20

// AI is the summon intention loop controlled by owner commands and effects.
type AI interface {
	TryToAttack(attackable.Combatant) bool
	TryToFollow(attackable.Combatant) bool
	TryToIdle()
	// WaitOutIdle reports whether an idle request has to wait for the
	// summon's swing or cast to end, dropping the queued intention if so.
	WaitOutIdle() bool
	// Think continues the current intention once.
	Think()
	// FinishedAttack runs the queued intention, if any, once a swing ends,
	// and otherwise continues the current one.
	FinishedAttack()
	// FinishedCasting runs the queued intention, or the attack the cast
	// replaced, once a cast completes; otherwise it goes idle in the same
	// critical section, following follow when non-nil, and reports true.
	FinishedCasting(follow attackable.Combatant) bool
	TryToCast(target attackable.Combatant, ref modelskill.Ref, ctrl bool) bool
	// AbortAll stops movement, the attack cycle and any in-flight cast.
	AbortAll()
	// FollowInstead makes following target the current intention.
	FollowInstead(attackable.Combatant)
	// TryToMoveTo makes walking to dest the current intention and starts
	// the walk.
	TryToMoveTo(dest location.Location) bool
	// StepAside walks to dest when the summon is idle or following,
	// reporting whether the walk started.
	StepAside(dest location.Location) bool
	StopMove()
	StopAttack()
	// AttackingNow reports whether this summon's own attack cycle is
	// currently in flight, matching CreatureAttack.isAttackingNow
	// (CreatureAttack.java:56-59): a plain start/stop flag on the summon's
	// own attack component, independent of its owner.
	AttackingNow() bool
}

// Owner is the live player surface a summon needs for world placement and
// command preconditions.
type Owner interface {
	world.Tracked
	attackable.Combatant
	LevelValue() int
	// InCombat reports the owner's own attack-stance state. Summon.isInCombat
	// (Summon.java:302-305) delegates straight to _owner.isInCombat() — a
	// pet/servitor's "in combat" status is entirely owner-derived, never
	// tracked on the summon itself.
	InCombat() bool
	// ServitorVanished tells the owner its servitor was erased.
	ServitorVanished()
	// PvPFlagState is the owner's PvP flag, which its summon carries too.
	PvPFlagState() task.PvPFlagState
	// AwardSummonKillKarma credits killer for killing the owner's summon.
	AwardSummonKillKarma(killer attackable.Combatant)
	// OfferSummonRevive offers the owner the resurrection of its summon,
	// which just died under a Phoenix Blessing.
	OfferSummonRevive()
	// ClearReviveOffer closes the owner's pending resurrection offer, if
	// any, as its revived pet does.
	ClearReviveOffer()
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
	// queue is the queue this summon's work runs on: its owner's, set once
	// before the summon is published, and a queue of its own once its corpse
	// outlives its owner's session (AdoptCorpseQueue).
	queue atomic.Pointer[sim.Queue]

	id             int32
	owner          Owner
	world          *world.State
	los            LineOfSight
	isPet          bool
	npcID          int
	radius         float64
	height         float64
	passive        bool
	maxBuffsAmount int

	// statusMu guards level, pet growth state, name, fed, belowUnsummonLimit,
	// lifetime, combat stat bases, and invul:
	// petInfoSnapshot (internal/gameserver/network/visibility.go) reads them
	// via Level/Name/Fed/Lifetime when another player discovers this summon,
	// on the queue that drives that visibility change, while the owner's
	// queue writes them (world.Observer's contract,
	// internal/gameserver/world/visibility.go).
	statusMu sync.RWMutex
	invul    bool
	level    int
	name     string
	named    bool
	lifetime LifetimeState
	dead     bool
	disabled bool
	// corpseTime is how long a servitor's corpse lasts; see DecayDelay.
	corpseTime time.Duration
	// corpseDeadline is when this summon's corpse decays, zero while it has
	// none; guarded by vitals.mu, with dead.
	corpseDeadline time.Time
	// decayed is set once a pet's corpse decay has claimed it; a revive
	// racing that decay then loses. Guarded by vitals.mu.
	decayed bool
	// ownerLeft is set once this summon's owner has left the world while it
	// lay dead; see LeaveWithOwner.
	ownerLeft atomic.Bool
	// despawnOnce runs the one despawn that takes this summon out of the
	// world; see despawn. Nothing it runs may despawn this summon again.
	despawnOnce sync.Once
	brain       AI
	// cast is the live cast controller. Attach installs it with brain and
	// sink.
	cast CastControl
	// sink receives this summon's events. Attach installs it before
	// SpawnBesideOwner publishes this summon into world.State; that publish
	// takes a registry mutex, giving every other goroutine's read a
	// happens-before edge over this unsynchronized write.
	sink event.Sink
	// skills maps skill id to the level this summon's npc template grants
	// it, used by TryUseSkill to resolve an owner-commanded action-bar
	// skill shortcut, matching Java's Summon.getSkill.
	skills map[int]int
	zones  ZoneQuery

	// followOff is set while the owner has told the summon to stop following
	// it; the zero value follows. Atomic because an effect landing on the
	// summon reads it from the queue of whoever applied the effect.
	followOff          atomic.Bool
	belowUnsummonLimit bool

	ownerInventory   *itemcontainer.Inventory
	timeLostIdle     int
	timeLostActive   int
	itemConsumeID    int32
	itemConsumeCount int
	// expPenalty is immutable after construction.
	expPenalty float32

	petInventory  *itemcontainer.Inventory
	petConfig     *petmodel.Config
	growth        *npc.PetData
	controlItemID int32
	exp           int64
	// expBeforeDeath is a pet's experience before its last death penalty,
	// until a resurrection gives some of it back; guarded by statusMu.
	expBeforeDeath int64
	sp             int
	expType        int
	fed            int
	maxMeal        int
	mealInNormal   int
	mealInBattle   int
	food1          int32
	food2          int32
	// foodRestore1/foodRestore2 hold the meal gauge each food item
	// restores on auto-feed: PetFoods.java's feed skill for that item
	// (skill.getFeed() * PET_FOOD_RATE), since food1 and food2 can map to
	// different feed skills with different amounts (e.g. Strider's food
	// vs Clan Hall Strider's food).
	foodRestore1  int
	foodRestore2  int
	autoFeedLimit float64
	hungryLimit   float64
	unsummonLimit float64
	roll          func(int) int
	// respawnRestoreHP is the share of max HP a revive restores without a
	// Phoenix Blessing; immutable after construction.
	respawnRestoreHP float64

	stats              CombatStats
	statCalc           summonStatCalcs
	vitals             summonVitals
	effects            *effect.List
	skillDefs          skillLookup
	raidCursesDisabled bool
	// stateMu also guards intent and target: another actor's skill landing
	// on this summon retargets or idles it from that actor's queue.
	stateMu                             sync.RWMutex
	paralyzed, teleporting, immobilized bool
	// unfollowBeforeImmobilized is whether the summon had follow mode off
	// when its movement lock was last set; clearing the lock restores that
	// mode. Its zero value restores following, the default before any set.
	unfollowBeforeImmobilized bool
	intent                    Intent
	target                    world.Tracked

	abnormalEffect  atomic.Int32
	ownerDiscovered atomic.Bool

	// shotsMu is taken from another actor's queue: Betray, applied by its
	// caster, makes the summon attack its owner there (TryToAttack), and
	// the attack launch reads the shot mask.
	shotsMu   sync.Mutex
	shotsMask int32
	// skillsMu guards disabledSkills. It is taken from another actor's
	// queue: a chance proc the summon sets off when that actor hits it runs
	// in the hit's own call.
	skillsMu       sync.Mutex
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
	Target  world.Tracked

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
	Effects         effect.Env
	ObjectID        int32
	Owner           Owner
	ControlItemID   int32
	OwnerInventory  *itemcontainer.Inventory
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

	Inventory      *itemcontainer.Inventory
	Fed            int
	MaxMeal        int
	MealInNormal   int
	MealInBattle   int
	Food1          int32
	Food2          int32
	FoodRestore1   int
	FoodRestore2   int
	AutoFeedLimit  float64
	HungryLimit    float64
	UnsummonLimit  float64
	Roll           func(int) int
	Stats          CombatStats
	MaxBuffsAmount int
	// RespawnRestoreHP is the share of max HP a revive restores without a
	// Phoenix Blessing.
	RespawnRestoreHP float64
	// Skills maps skill id to level, from this pet's npc template. See
	// Actor.skills.
	Skills map[int]int
	// Passives are the npc template's type="PASSIVE" skill refs. SkillDefs
	// resolves them into stat funcs attached before current HP/MP seed.
	Passives  []modelskill.Ref
	SkillDefs skillLookup
	Zones     ZoneQuery
	LOS       LineOfSight
}

// ServitorConfig carries the minimum state needed to create a live servitor.
type ServitorConfig struct {
	Effects         effect.Env
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
	// ExpPenalty is the share of kill exp this servitor withholds from its
	// owner, from the summoning skill.
	ExpPenalty float32
	// CorpseTime is how long this servitor's corpse lasts before it decays,
	// from its npc template.
	CorpseTime     time.Duration
	Roll           func(int) int
	Stats          CombatStats
	MaxBuffsAmount int
	// RespawnRestoreHP is the share of max HP a revive restores without a
	// Phoenix Blessing.
	RespawnRestoreHP float64
	// Skills maps skill id to level, from this servitor's npc template.
	// See Actor.skills.
	Skills map[int]int
	// Passives are the npc template's type="PASSIVE" skill refs. SkillDefs
	// resolves them into stat funcs attached before current HP/MP seed.
	Passives  []modelskill.Ref
	SkillDefs skillLookup
	Zones     ZoneQuery
	LOS       LineOfSight
}

// NewServitor returns a live servitor actor.
func NewServitor(cfg ServitorConfig) (*Actor, error) {
	a := &Actor{
		id:               cfg.ObjectID,
		owner:            cfg.Owner,
		level:            cfg.Level,
		npcID:            cfg.NPCID,
		radius:           cfg.CollisionRadius,
		height:           cfg.CollisionHeight,
		name:             cfg.Name,
		passive:          cfg.Passive,
		intent:           IntentFollowOwner,
		ownerInventory:   cfg.OwnerInventory,
		lifetime:         cfg.Lifetime,
		timeLostIdle:     defaultPositive(cfg.TimeLostIdle, 1000),
		timeLostActive:   defaultPositive(cfg.TimeLostActive, 1000),
		itemConsumeID:    cfg.ItemConsumeID,
		itemConsumeCount: cfg.ItemConsumeCount,
		expPenalty:       cfg.ExpPenalty,
		corpseTime:       cfg.CorpseTime,
		roll:             defaultRoll(cfg.Roll),
		stats:            cfg.Stats,
		skills:           cfg.Skills,
		skillDefs:        cfg.SkillDefs,
		maxBuffsAmount:   defaultPositive(cfg.MaxBuffsAmount, baseBuffSlots),
		zones:            cfg.Zones,
		los:              cfg.LOS,
	}
	if err := a.attachTemplatePassives(cfg.SkillDefs, cfg.Passives); err != nil {
		return nil, err
	}
	a.respawnRestoreHP = cfg.RespawnRestoreHP
	a.initVitals()
	a.effects = effect.NewList(a, effect.WithEnv(cfg.Effects))
	return a, nil
}

// NewPet returns a live pet actor.
func NewPet(cfg PetConfig) (*Actor, error) {
	petCfg := copyPetConfig(cfg.Config)
	if petCfg != nil && cfg.Inventory != nil {
		slots, weight := petCfg.InventoryLimits(cfg.CON)
		cfg.Inventory.SlotLimit = slots
		cfg.Inventory.WeightLimit = weight
	}
	a := &Actor{
		id:             cfg.ObjectID,
		owner:          cfg.Owner,
		level:          cfg.Level,
		isPet:          true,
		npcID:          cfg.NPCID,
		radius:         cfg.CollisionRadius,
		height:         cfg.CollisionHeight,
		name:           cfg.Name,
		named:          cfg.Named,
		passive:        cfg.Passive,
		intent:         IntentFollowOwner,
		petInventory:   cfg.Inventory,
		petConfig:      petCfg,
		growth:         cfg.Growth,
		controlItemID:  cfg.ControlItemID,
		ownerInventory: cfg.OwnerInventory,
		exp:            cfg.Exp,
		sp:             cfg.SP,
		expType:        cfg.ExpType,
		fed:            cfg.Fed,
		maxMeal:        cfg.MaxMeal,
		mealInNormal:   cfg.MealInNormal,
		mealInBattle:   cfg.MealInBattle,
		food1:          cfg.Food1,
		food2:          cfg.Food2,
		foodRestore1:   cfg.FoodRestore1,
		foodRestore2:   cfg.FoodRestore2,
		autoFeedLimit:  cfg.AutoFeedLimit,
		hungryLimit:    cfg.HungryLimit,
		unsummonLimit:  cfg.UnsummonLimit,
		roll:           defaultRoll(cfg.Roll),
		stats:          cfg.Stats,
		skills:         cfg.Skills,
		skillDefs:      cfg.SkillDefs,
		maxBuffsAmount: defaultPositive(cfg.MaxBuffsAmount, baseBuffSlots),
		zones:          cfg.Zones,
		los:            cfg.LOS,
	}
	if err := a.attachTemplatePassives(cfg.SkillDefs, cfg.Passives); err != nil {
		return nil, err
	}
	a.respawnRestoreHP = cfg.RespawnRestoreHP
	a.initVitals()
	a.effects = effect.NewList(a, effect.WithEnv(cfg.Effects))
	return a, nil
}

type skillLookup interface {
	Definition(modelskill.Ref) (modelskill.Definition, bool)
}

func (a *Actor) attachTemplatePassives(lookup skillLookup, passives []modelskill.Ref) error {
	mods, err := effect.TemplatePassiveMods(lookup, passives)
	if err != nil {
		return fmt.Errorf("summon npc %d template passives: %w", a.npcID, err)
	}
	a.AddStatFuncs(mods)
	return nil
}

func copyPetConfig(cfg *petmodel.Config) *petmodel.Config {
	if cfg == nil {
		return nil
	}
	copied := *cfg
	return &copied
}

// ObjectID returns the live world object id assigned to this summon.
