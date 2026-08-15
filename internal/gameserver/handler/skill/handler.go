package skill

import (
	"reflect"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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
	Dead() bool
}

// Cast carries the already-resolved inputs a skill handler needs.
type Cast struct {
	Caster  Actor
	Skill   modelskill.Definition
	Targets []Actor
	// Item is a genuinely heterogeneous payload with unrelated consumers
	// (manor.go asserts it to a seed item, summon.go forwards it untouched),
	// left untyped deliberately rather than typed against one of them.
	Item     any
	resisted *Result
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
}

// Result reports player-visible outcomes produced while a skill handler ran.
type Result struct {
	AttackFailed   int
	Counterattacks []Counterattack
	Lethals        []Lethal
	Dodges         []Dodge
	Resisted       []Resisted
	CubicAdded     bool
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

type resultHandler interface {
	UseResult(Cast) Result
}

// Registry maps skill type names to their handlers.
type Registry struct {
	entries map[string]Handler
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

// NewDefaultRegistryWithDefinitions returns the default handlers, providing
// loaded skill definitions to handlers that need cross-skill effect lookup.
func NewDefaultRegistryWithDefinitions(defs Definitions) *Registry {
	return NewRegistry(
		pdamHandler{},
		chargeDamHandler{},
		mdamHandler{},
		blowHandler{},
		manaDamageHandler{},
		healHandler{},
		healPercentHandler{},
		manaHealHandler{},
		combatPointHealHandler{},
		cpDamagePercentHandler{},
		balanceLifeHandler{},
		realDamageHandler{},
		giveSPHandler{},
		dummyHandler{},
		cancelHandler{},
		disablersHandler{},
		resurrectHandler{},
		instantJumpHandler{},
		getPlayerHandler{},
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
	)
}

// SignetDeps carries the world-spawning collaborators the signet cast
// shape needs beyond skill definitions.
type SignetDeps struct {
	Templates signetTemplates
	IDs       signetIDAllocator
	World     *world.State
	Log       zerolog.Logger
}

// NewDefaultRegistryWithSignet returns the same handlers as
// NewDefaultRegistryWithDefinitions, plus the signet cast shape wired with
// signet's own world-spawning collaborators.
func NewDefaultRegistryWithSignet(defs Definitions, signet SignetDeps) *Registry {
	r := NewDefaultRegistryWithDefinitions(defs)
	r.Register(signetHandler{defs: defs, templates: signet.Templates, ids: signet.IDs, world: signet.World, log: signet.Log})
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
func (r *Registry) UseResult(cast Cast) (Result, bool) {
	var reported Result
	cast.resisted = &reported
	h, ok := r.Handler(cast.Skill.SkillType)
	if !ok {
		return Result{}, false
	}
	if rh, ok := h.(resultHandler); ok {
		result := rh.UseResult(cast)
		result.Resisted = append(result.Resisted, reported.Resisted...)
		return result, true
	}
	h.Use(cast)
	return reported, true
}

func skillTypeKey(skillType string) string {
	return strings.ToUpper(strings.TrimSpace(skillType))
}

type dummyHandler struct{}

func (dummyHandler) Types() []string { return []string{"DUMMY", "BEAST_FEED"} }

func (dummyHandler) Use(Cast) {}

// alikeDeadSource optionally reports an actor's death-like state (fake
// death, a pending resurrect) on top of plain death; an actor without one
// is alike-dead exactly when it is dead.
type alikeDeadSource interface {
	AlikeDead() bool
}

// alikeDead reports whether a is dead or in a death-like state, matching the
// reference's isAlikeDead() default of falling back to isDead().
func alikeDead(a Actor) bool {
	if a == nil {
		return false
	}
	if d, ok := a.(alikeDeadSource); ok {
		return d.AlikeDead()
	}
	return a.Dead()
}

func sameObject(a, b Actor) bool {
	if a == nil || b == nil {
		return a == b
	}

	ta := reflect.TypeOf(a)
	tb := reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}

	return a == b
}

// cursedWeaponHolder optionally reports whether an actor currently wields a
// cursed weapon; an actor without one never does.
type cursedWeaponHolder interface {
	CursedWeaponEquipped() bool
}

func cursed(a Actor) bool {
	c, ok := a.(cursedWeaponHolder)
	return ok && c.CursedWeaponEquipped()
}
