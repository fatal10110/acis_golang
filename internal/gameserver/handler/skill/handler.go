package skill

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// Actor is the surface every participant in a cast — the caster and each
// resolved target — shares. Handlers narrow it further with focused
// assertions for the capabilities the skill type at hand needs; typing the
// boundary itself keeps a value that is not an actor from reaching a handler
// as an inert `any` that every one of those assertions then misses.
type Actor interface {
	ObjectID() int32
	Kind() actor.Kind
	Dead() bool
	Position() (x, y, z int)
	Heading() int
}

// Creature is a creature cast participant, caster or target: every player,
// NPC and summon. It takes part in combat and in effects; a method that does
// not apply to a kind returns the neutral value documented at its
// implementation.
type Creature interface {
	creature.FormulaActor
	effect.Actor

	Paralyzed() bool
	Undead() bool
	// SkillSuccessInput and EffectSuccessInput resolve an effect-landing roll
	// of caster's skill against this creature; ok is false when it can't be
	// rolled at all.
	SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (in formulas.SkillSuccessInput, ok bool)
	EffectSuccessInput(caster creature.FormulaActor, def modelskill.Definition, tmpl modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool)
	// SkillReflectInput is the state deciding whether this creature reflects
	// def back onto its caster.
	SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput

	// Attackable reports an NPC combat target; the aggro controls below only
	// act on one.
	Attackable() bool
	NotifyAggression(source attackable.Combatant, power int)
	ReduceAllAggroHate(amount float64)
	StopAggroHate(attacker attackable.Combatant)
	StopHateList(attacker attackable.Combatant)
	ClearAggroTables()
	// EnableOverhit arms overhit damage tracking for the current hit; only
	// attackable NPCs track it.
	EnableOverhit()

	// CurrentTarget, SetTarget and AttackTarget retarget a playable creature
	// provoked by aggression.
	CurrentTarget() world.Tracked
	SetTarget(world.Tracked)
	AttackTarget(world.Tracked)

	// Damage and resource surface. ReduceHP applies skill damage; the
	// *Input methods resolve a formula roll against this creature and
	// report false when it cannot be rolled.
	ReduceHP(amount float64, attacker attackable.Combatant, skill modelskill.Definition)
	PhysicalSkillInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.PhysicalSkillInput, bool)
	MagicDamageInput(caster creature.FormulaActor, skill modelskill.Definition, magicFailures bool) (formulas.MagicDamageInput, bool)
	BlowInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.BlowInput, bool)
	ManaDamageInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.ManaDamageInput, bool)
	LethalInput(caster creature.FormulaActor, skill modelskill.Definition) (formulas.LethalInput, bool)
	ApplyLethalOutcome(formulas.LethalOutcome, attackable.Combatant, modelskill.Definition)
	CounterSkillPhysical() float64
	Invulnerable() bool
	// HealInput resolves this creature's side of an outgoing HEAL; the
	// handler adds the spiritshot state it sampled.
	HealInput(modelskill.Definition) (formulas.HealInput, bool)
	MaxMPValue() float64
	SetHP(float64)
	// Kill runs this creature's death sequence at once, whatever its HP,
	// crediting killer, and reports whether this call killed it.
	Kill(killer attackable.Combatant) bool
}

// Player is the player-only cast participant surface: the resources, cast
// state and client notifications only a player character carries.
type Player interface {
	Creature

	CP() float64
	MaxCPValue() float64
	SetCP(float64)
	BreakCastOnDamage(damage float64)
	Charges() int
	// ReviveRestoringExp restores exp and then revives, only while the
	// player is still dead.
	ReviveRestoringExp(restorePercent float64) bool
	// CursedWeaponEquipped reports a cursed weapon in hand.
	CursedWeaponEquipped() bool
	// Summon-friend eligibility state.
	Operating() bool
	Rooted() bool
	InCombat() bool
	FestivalParticipant() bool
	// Client notifications.
	NotifyHPRestored(healerName string, amount int, byOther bool)
	NotifyMPRestored(healerName string, amount int, byOther bool)
	NotifyCPRestored(healerName string, amount int, byOther bool)
	NotifyAttackFailed()
	NotifyResistedSkill(targetName string, skillID modelskill.ID, level int)
	NotifyResistedMagic(attackerName string)
	NotifySkillDamage(amount int, magicCrit, blocked, petrified bool)
	NotifySpoilAlready()
	NotifySpoilSuccess()
	// Summon-friend eligibility state the caster and target share.
	Mounted() bool
	OlympiadMode() bool
	ObserverMode() bool
	NoSummonFriendZone() bool
}

// NPC is the NPC-only cast participant surface.
type NPC interface {
	Creature

	// Lethalable reports whether a lethal strike may apply at all.
	Lethalable() bool
	// SpoilPool is the NPC's sweepable drop pool, nil when it has none.
	SpoilPool() *item.SpoilPool
	// SeedState is the NPC life's manor sow/harvest lifecycle.
	SeedState() *npc.SeedState
	// MonsterKind reports a Monster-family NPC, the only one sown or
	// harvested.
	MonsterKind() bool
}

// Summon is the summon-only cast participant surface.
type Summon interface {
	Creature
	SiegeSummon() bool
	SummonOwner() summon.Owner
	UnSummon(owner summon.Owner)
}

// asCreature returns a as a creature, or false for a cast participant that
// is not one (a door or a signet effect point).
func asCreature(a Actor) (Creature, bool) {
	c, ok := a.(Creature)
	return c, ok
}

// asPlayer returns a as a player character, or false for any other kind.
func asPlayer(a Actor) (Player, bool) {
	if a == nil || a.Kind() != actor.KindPlayer {
		return nil, false
	}
	p, ok := a.(Player)
	return p, ok
}

// asNPC returns a as an NPC, or false for any other kind.
func asNPC(a Actor) (NPC, bool) {
	if a == nil || a.Kind() != actor.KindNPC {
		return nil, false
	}
	n, ok := a.(NPC)
	return n, ok
}

// asEffected returns a as an effect target: every creature plus a signet
// effect point, which holds an effect list without taking part in combat.
func asEffected(a Actor) (effect.Actor, bool) {
	e, ok := a.(effect.Actor)
	return e, ok
}

// Cast carries the already-resolved inputs a skill handler needs.
type Cast struct {
	Caster  Creature
	Skill   modelskill.Definition
	Targets []Actor
	// Item is a genuinely heterogeneous payload with unrelated consumers
	// (manor.go asserts it to a seed item, summon.go forwards it untouched),
	// left untyped deliberately rather than typed against one of them.
	Item any
	// Sink, when set, receives each handler message the moment the handler
	// produces it, so the message keeps its place among the frames the
	// cast's own state changes send at once (a target's status, death and
	// kill rewards). A message a sink received is not returned in
	// Result.Messages.
	Sink     MessageSink
	resisted *Result
	messages *messageLog
}

// MessageSink delivers one handler message.
type MessageSink func(message any)

// messageLog is the ordered message stream a registry dispatch shares
// between a cast and its result: each message goes to sink when there is
// one and is kept in pending otherwise.
type messageLog struct {
	sink    MessageSink
	pending []any
}

func (l *messageLog) add(message any) {
	if l.sink != nil {
		l.sink(message)
		return
	}
	l.pending = append(l.pending, message)
}

// record hands a caster-visible message to the dispatching registry's
// message stream; a cast used outside a registry drops it.
func (c Cast) record(message any) {
	if c.messages != nil {
		c.messages.add(message)
	}
}

func (c Cast) reportResisted(target Actor, def modelskill.Definition, count int) {
	appendResistedCount(c.resisted, target, def, count)
}

// Definitions resolves loaded skill definitions.
type Definitions interface {
	Definition(modelskill.Ref) (modelskill.Definition, bool)
	MaxLevel(modelskill.ID) int
}

// Handler applies one skill action to already-resolved targets.
type Handler interface {
	Types() []string
	Use(Cast)
}

// Counterattack reports the two participants in a countered physical skill.
type Counterattack struct {
	AttackerID   int32
	AttackerName string
	DefenderID   int32
	DefenderName string
}

// Lethal reports the participants in a successful lethal strike.
type Lethal struct {
	AttackerID int32
	TargetID   int32
}

// Dodge reports a blow evasion and its two participants.
type Dodge struct {
	AttackerID   int32
	AttackerName string
	DefenderID   int32
	DefenderName string
}

// Resisted reports a target that resisted a skill effect.
type Resisted struct {
	TargetName string
	SkillID    modelskill.ID
	SkillLevel int
	// Unconditional marks a skill's own effect-landing resist — the
	// Mdam/Blow/Manadam/charge-damage report, sent to the caster with no
	// caster-type gate — as opposed to the generic per-effect-template
	// resist, which is gated to a Player caster. Only a summon forwards the
	// former to its owner unconditionally; the latter never fires for a
	// non-Player caster at all.
	Unconditional bool
}

// MagicResist reports a player target that resisted a magic-damage cast.
// Drain marks a DRAIN cast, which the target hears about in its own words.
type MagicResist struct {
	TargetID     int32
	AttackerName string
	Drain        bool
}

// ManaDrain reports MP drained from a player target by a MANADAM cast.
type ManaDrain struct {
	TargetID   int32
	CasterName string
	MP         int32
}

// DamageReceived reports a hit's damage to the player target that took it,
// naming the attacker.
type DamageReceived struct {
	TargetID     int32
	AttackerName string
	Amount       int32
}

// DamageSource selects the damage feedback family: a player's own, or a
// pet's or servitor's, which reaches the summon's owner.
type DamageSource uint8

const (
	DamageByPlayer DamageSource = iota
	DamageByPet
	DamageByServitor
)

// Damage reports a skill hit's damage to the attacking side. RecipientID is
// the attacking player, or the owner of the attacking summon.
type Damage struct {
	RecipientID  int32
	Source       DamageSource
	Amount       int32
	MagicCrit    bool
	PhysicalCrit bool
	// Blocked marks an invulnerable target; Petrified marks one that is
	// also paralyzed.
	Blocked   bool
	Petrified bool
}

// AttackFailedMessage, DrainHalfSucceededMessage, ManaDamageMissedMessage,
// InvalidTargetMessage and CasterVitalsChanged mark messages without data.
// DrainHalfSucceededMessage is a DRAIN cast's half-damage magic failure,
// reported in place of AttackFailedMessage. InvalidTargetMessage tells a
// player caster the skill cannot act on a target it reached (an unlock
// skill on neither a door nor a chest, a confusion on a non-NPC).
// CasterVitalsChanged marks where a player caster's own HP changed
// mid-cast, so its status reaches it at that point among the cast's other
// messages.
type (
	InvalidTargetMessage      struct{}
	AttackFailedMessage       struct{}
	DrainHalfSucceededMessage struct{}
	ManaDamageMissedMessage   struct{}
	CasterVitalsChanged       struct{}
	OpponentMPReducedMessage  struct{ MP int32 }
)

// Result reports player-visible outcomes produced while a skill handler ran.
type Result struct {
	// Messages retains the order in which handler messages were produced.
	Messages       []any
	messages       *messageLog
	AttackFailed   int
	Counterattacks []Counterattack
	Lethals        []Lethal
	Dodges         []Dodge
	Resisted       []Resisted
	MagicResists   []MagicResist
	// ManaDamageMissed counts MANADAM casts that missed their target
	// (invulnerable or the magic-affected roll failed), reported to the caster.
	ManaDamageMissed int
	// ManaDrains reports MP drained from a player target by a MANADAM cast,
	// delivered to the target as MP-drained.
	ManaDrains []ManaDrain
	// OpponentMPReduced reports, per successful MANADAM drain, the MP amount
	// to report to the caster.
	OpponentMPReduced []int32
	CubicAdded        bool
	// CubicTargets are non-caster targets whose cubic runtime was touched.
	CubicTargets []Actor
	// CubicAddedTargets are the non-caster targets whose visible cubic list changed.
	CubicAddedTargets []Actor
	// CubicTouched and CubicID report that a SUMMON cubic cast reached the
	// caster's own cubic list, whether newly admitted or refreshed, so a
	// caller can (re)sync the cubic's live action/disappear runtime either
	// way — unlike CubicAdded, which only fires on a fresh admit and drives
	// the character-info broadcast.
	CubicTouched bool
	CubicID      cubic.ID
}

func (r *Result) record(message any) {
	if r.messages != nil {
		r.messages.add(message)
		return
	}
	r.Messages = append(r.Messages, message)
}

type resultHandler interface {
	UseResult(Cast) Result
}

// Registry maps skill type names to their handlers.
type Registry struct {
	entries map[string]Handler
	// magicFailures is the server's MagicFailures switch, read by the
	// cubic damage path (UseCubic).
	magicFailures bool
}

// NewRegistry returns a registry populated with handlers.
func NewRegistry(handlers ...Handler) *Registry {
	r := &Registry{entries: make(map[string]Handler)}
	for _, h := range handlers {
		r.Register(h)
	}
	return r
}

// NewDefaultRegistry returns the representative handlers that currently have
// enough surrounding model support to run deterministically.
func NewDefaultRegistry() *Registry {
	return NewDefaultRegistryWithDefinitions(nil)
}

// NewDefaultRegistryWithDefinitions returns the default handlers with the
// shipped MagicFailures=true, providing loaded skill definitions to handlers
// that need cross-skill effect lookup.
func NewDefaultRegistryWithDefinitions(defs Definitions) *Registry {
	return newDefaultRegistry(defs, true, nil)
}

func newDefaultRegistry(defs Definitions, magicFailures bool, healSps *modelskill.HealSpsTable) *Registry {
	r := NewRegistry(
		pdamHandler{},
		chargeDamHandler{},
		mdamHandler{magicFailures: magicFailures},
		drainHandler{magicFailures: magicFailures},
		blowHandler{},
		manaDamageHandler{},
		healHandler{buff: continuousHandler{defs: defs}, healSps: healSps},
		healPercentHandler{buff: continuousHandler{defs: defs}},
		manaHealHandler{},
		combatPointHealHandler{buff: continuousHandler{defs: defs}},
		cpDamagePercentHandler{},
		balanceLifeHandler{buff: continuousHandler{defs: defs}},
		realDamageHandler{},
		giveSPHandler{},
		dummyHandler{},
		cancelHandler{},
		disablersHandler{},
		resurrectHandler{},
		instantJumpHandler{},
		getPlayerHandler{},
		recallHandler{},
		summonCreatureHandler{},
		summonFriendHandler{},
		cubicHandler{},
		unlockHandler{},
		extractableHandler{},
		sowHandler{},
		harvestHandler{},
		spoilHandler{},
		sweepHandler{},
		seedHandler{},
		continuousHandler{defs: defs},
		fusionHandler{defs: defs},
		craftHandler{},
		fishingHandler{},
		fishingSkillHandler{},
	)
	r.magicFailures = magicFailures
	return r
}

// SignetDeps carries the world-spawning collaborators the signet cast
// shape needs beyond skill definitions. IDs also gives the item-creating
// handlers (sweep, harvest, capsule extraction) their object ids.
type SignetDeps struct {
	Effects   effect.Env
	Templates signetTemplates
	IDs       objectIDAllocator
	World     *world.State
	// NewSink builds the event sink a spawned signet effect point reports
	// through; nil disables signet spawning.
	NewSink func(*npc.EffectPoint) event.Sink
	// Queues creates each spawned effect point's own queue; nil disables
	// signet spawning.
	Queues signetQueues
	Log    zerolog.Logger
	// ManorCropRate multiplies every harvested crop count.
	ManorCropRate int
}

// NewDefaultRegistryWithSignet returns the default handlers under the
// server's MagicFailures switch, with the healSps table a spiritshot-boosted
// heal reads, plus the signet cast shape wired with signet's own
// world-spawning collaborators, and the item-creating handlers wired with
// its object ids.
func NewDefaultRegistryWithSignet(defs Definitions, magicFailures bool, healSps *modelskill.HealSpsTable, signet SignetDeps) *Registry {
	r := newDefaultRegistry(defs, magicFailures, healSps)
	if signet.IDs != nil {
		r.Register(extractableHandler{ids: signet.IDs})
		r.Register(harvestHandler{ids: signet.IDs, cropRate: signet.ManorCropRate})
		r.Register(sweepHandler{ids: signet.IDs})
	}
	r.Register(signetHandler{defs: defs, magicFailures: magicFailures, templates: signet.Templates, ids: signet.IDs, world: signet.World, newSink: signet.NewSink, effects: signet.Effects, queues: signet.Queues, log: signet.Log})
	return r
}

// Register adds h for every skill type it reports.
func (r *Registry) Register(h Handler) {
	if r == nil || h == nil {
		return
	}
	if r.entries == nil {
		r.entries = make(map[string]Handler)
	}
	for _, skillType := range h.Types() {
		key := skillTypeKey(skillType)
		if key != "" {
			r.entries[key] = h
		}
	}
}

// Handler returns the handler for skillType.
func (r *Registry) Handler(skillType string) (Handler, bool) {
	if r == nil {
		return nil, false
	}
	h, ok := r.entries[skillTypeKey(skillType)]
	return h, ok
}

// Use dispatches cast to the handler registered for cast.Skill.SkillType.
func (r *Registry) Use(cast Cast) bool {
	_, ok := r.UseResult(cast)
	return ok
}

// UseResult dispatches cast and returns any caster-visible handler result.
// Messages cast.Sink received are left out of the result's Messages.
func (r *Registry) UseResult(cast Cast) (Result, bool) {
	messages := &messageLog{sink: cast.Sink}
	cast.messages = messages
	reported := Result{messages: messages}
	cast.resisted = &reported
	h, ok := r.Handler(cast.Skill.SkillType)
	if !ok {
		return Result{}, false
	}
	if rh, ok := h.(resultHandler); ok {
		result := rh.UseResult(cast)
		result.Resisted = append(result.Resisted, reported.Resisted...)
		result.Messages = messages.pending
		return result, true
	}
	h.Use(cast)
	reported.Messages = messages.pending
	return reported, true
}

func skillTypeKey(skillType string) string {
	return strings.ToUpper(strings.TrimSpace(skillType))
}

type dummyHandler struct{}

func (dummyHandler) Types() []string { return []string{"DUMMY", "BEAST_FEED"} }

func (dummyHandler) Use(Cast) {}

// alikeDead reports whether a is dead or in a death-like state, falling back
// to plain death by default.
func alikeDead(a Actor) bool {
	if a == nil {
		return false
	}
	if c, ok := asCreature(a); ok {
		return c.AlikeDead()
	}
	return a.Dead()
}

// sameObject reports whether a and b are the same world object. Identity is
// the object id: every live world object owns a unique non-zero id, so two
// Go values carrying the same id (an actor and a wrapper around it, say) are
// one object, and two values with different ids are distinct even when they
// are equal Go values. An actor with id 0 has not been placed in the world
// and has no identity to share, so it is the same object as nothing — not
// even itself. Two nil actors are the same (absent) object; nil and a
// non-nil actor never are.
func sameObject(a, b Actor) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	id := a.ObjectID()
	return id != 0 && id == b.ObjectID()
}

// clanHallManager reports whether a is a clan hall manager, whose buffs
// land on a cursed-weapon holder.
func clanHallManager(a Actor) bool {
	f, ok := a.(*npc.Folk)
	return ok && f.ClanHallManager()
}

// cursed reports whether a wields a cursed weapon; only a player can.
func cursed(a Actor) bool {
	p, ok := asPlayer(a)
	return ok && p.CursedWeaponEquipped()
}

// formulaCasterOf returns a as a formula caster, or nil for a cast
// participant that is not a creature (a door or a signet effect point).
// Formula inputs treat a nil caster as one that cannot roll.
//
// Do not read a damage permission through this narrowing: a combatant that
// is not a formula caster also comes back nil, and creature.CanDealDamage(nil)
// reports true because sourceless damage is legitimate. Such a gate would
// lift for that combatant instead of consulting it. Narrow to
// attackable.Combatant for permission checks instead.
func formulaCasterOf(a Actor) creature.FormulaActor {
	c, _ := a.(creature.FormulaActor)
	return c
}

var (
	_ Player       = (*player.Character)(nil)
	_ NPC          = (*npc.Hostile)(nil)
	_ Creature     = (*player.Character)(nil)
	_ Creature     = (*npc.Hostile)(nil)
	_ Summon       = (*summon.Actor)(nil)
	_ damageSummon = (*summon.Actor)(nil)
)
