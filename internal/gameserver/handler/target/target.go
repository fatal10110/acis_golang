package target

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Category classifies the runtime shape a target handler needs for
// selection rules.
type Category uint8

const (
	// CategoryPlayable marks player-controlled actors and summons.
	CategoryPlayable Category = 1 << iota
	// CategoryAttackable marks hostile or otherwise attackable NPC actors.
	CategoryAttackable
	// CategoryFolk marks NPC actors that can affect nearby playable actors.
	CategoryFolk
)

// Has reports whether c includes all bits in want.
func (c Category) Has(want Category) bool { return c&want == want }

// Creature is the actor surface target handlers need to resolve affected
// skill targets.
type Creature interface {
	ObjectID() int32
	Position() (x, y, z int)
	Heading() int
	Dead() bool
	Category() Category
}

// AttackRules is implemented by creatures that can answer whether a caster
// may affect them offensively.
type AttackRules interface {
	AttackableBy(caster Creature) bool
	AttackableWithoutForceBy(caster Creature) bool
}

// SightChecker is implemented by creatures that can answer line-of-sight
// checks against another creature.
type SightChecker interface {
	CanSeeTarget(target Creature) bool
}

// Summoner is implemented by creatures that expose a current summon.
type Summoner interface {
	Summon() (Creature, bool)
}

// OwnedCreature is implemented by summons that expose their owner.
type OwnedCreature interface {
	Owner() (Creature, bool)
}

// HolyTarget is implemented by creatures that can receive artifact-targeted
// skills.
type HolyTarget interface {
	Holy() bool
}

// UnlockableTarget is implemented by creatures that can receive unlock
// skills.
type UnlockableTarget interface {
	Unlockable() bool
}

// UndeadTarget is implemented by creatures that expose undead race state to
// skill targeting.
type UndeadTarget interface {
	Undead() bool
}

// CorpseTarget is implemented by creatures that can report whether they
// currently have a pending, lootable corpse available to corpse-targeted
// skills.
type CorpseTarget interface {
	HasCorpse() bool
}

// MonsterTarget identifies the Monster-family corpses accepted by harvest and
// sweep skills.
type MonsterTarget interface {
	MonsterKind() bool
}

// CorpseDeadlineTarget is optionally implemented by mob corpses that expose
// the decay deadline used for Java's too-old corpse targeting cutoff.
type CorpseDeadlineTarget interface {
	CorpseDeadline() (time.Time, bool)
	CorpseTime() time.Duration
}

// SpoiledCorpse is optionally implemented by mob corpses that bypass the
// too-old targeting cutoff after a successful spoil.
type SpoiledCorpse interface {
	Spoiled() bool
}

// SeededCorpse is optionally implemented by mob corpses that bypass the
// too-old targeting cutoff after being sown.
type SeededCorpse interface {
	Seeded() bool
}

// PetTarget is implemented by summons that can report whether they are pets
// rather than servitors.
type PetTarget interface {
	IsPet() bool
}

// PeaceZoner is implemented by creatures that can report whether hostilities
// are blocked by their current zone.
type PeaceZoner interface {
	InPeaceZone() bool
}

// Known enumerates nearby creatures for radius-based target handlers.
type Known interface {
	ForEachKnownCreatureInRadius(anchor Creature, radius int, fn func(Creature))
}

// WorldKnown adapts the world grid to target-handler radius scans.
type WorldKnown struct {
	State *world.State
}

// ForEachKnownCreatureInRadius calls fn for every known creature within
// radius of anchor.
func (w WorldKnown) ForEachKnownCreatureInRadius(anchor Creature, radius int, fn func(Creature)) {
	if w.State == nil {
		return
	}
	tracked, ok := anchor.(world.Tracked)
	if !ok {
		return
	}
	w.State.ForEachKnownInRadius(tracked, radius, func(obj world.Tracked) {
		creature, ok := obj.(Creature)
		if ok {
			fn(creature)
		}
	})
}

// Handler resolves a skill's final target and affected target list.
type Handler interface {
	Target() modelskill.Target
	Targets(caster, target Creature, skill *modelskill.Definition) []Creature
	FinalTarget(caster, target Creature, skill *modelskill.Definition) Creature
	CanCast(caster, target Creature, skill *modelskill.Definition, ctrl bool) bool
}

// Registry owns the target handlers available to the cast pipeline.
type Registry struct {
	handlers map[modelskill.Target]Handler
}

// NewRegistry returns a registry with the currently ported target handlers.
func NewRegistry(known Known) *Registry {
	r := &Registry{handlers: make(map[modelskill.Target]Handler)}
	r.Register(selfHandler{})
	r.Register(oneHandler{})
	r.Register(areaHandler{known: known})
	r.Register(frontAreaHandler{known: known})
	r.Register(auraHandler{known: known})
	r.Register(frontAuraHandler{known: known})
	r.Register(behindAuraHandler{known: known})
	r.Register(undeadHandler{})
	r.Register(auraUndeadHandler{known: known})
	r.Register(unlockableHandler{})
	r.Register(holyHandler{})
	r.Register(summonHandler{})
	r.Register(areaSummonHandler{known: known})
	r.Register(ownerPetHandler{})
	r.Register(corpseMobHandler{})
	r.Register(areaCorpseMobHandler{known: known})
	r.Register(corpsePlayerHandler{})
	r.Register(corpsePetHandler{})
	r.Register(groundHandler{})
	r.Register(partyHandler{known: known})
	r.Register(allyHandler{known: known})
	r.Register(clanHandler{known: known})
	r.Register(partyMemberHandler{})
	r.Register(partyOtherHandler{})
	r.Register(corpseAllyHandler{known: known})
	return r
}

// Register adds or replaces a handler by target type.
func (r *Registry) Register(handler Handler) {
	if r.handlers == nil {
		r.handlers = make(map[modelskill.Target]Handler)
	}
	r.handlers[handler.Target()] = handler
}

// Handler returns the handler for typ, if one is registered.
func (r *Registry) Handler(typ modelskill.Target) (Handler, bool) {
	if r == nil {
		return nil, false
	}
	handler, ok := r.handlers[typ]
	return handler, ok
}
