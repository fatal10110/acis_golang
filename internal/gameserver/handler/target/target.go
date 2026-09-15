package target

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Actor is a skill caster or target: a player, NPC, summon or door. Every
// kind implements every method; a method that does not apply to a kind
// returns the neutral value documented at its implementation.
type Actor interface {
	world.Tracked
	Position() (x, y, z int)
	Heading() int
	Dead() bool

	// AttackableBy reports whether caster may affect this actor offensively;
	// AttackableWithoutForceBy whether it may without a forced attack.
	AttackableBy(caster Actor) bool
	AttackableWithoutForceBy(caster Actor) bool
	// CanSeeTarget reports line of sight from this actor to target.
	CanSeeTarget(target Actor) bool
	InPeaceZone() bool
	// EffectRangeInPeaceZone reports whether an effect of effectRange centered
	// on (x, y, z) overlaps a peace zone in the actor's region.
	EffectRangeInPeaceZone(x, y, z, effectRange int) bool
	// GroundTarget is the pending ground-click point of a ground-targeted
	// cast, and CanSeePoint the line of sight to a point; only players track
	// one.
	GroundTarget() (x, y, z int)
	CanSeePoint(x, y, z int) bool

	// Folk reports a civilian NPC that can affect nearby playable actors.
	Folk() bool
	// FolkOrGuard reports a civilian NPC kind that only accepts CTRL-pressed
	// damage skills as offensive single targets.
	FolkOrGuard() bool
	MonsterKind() bool
	Undead() bool
	// Holy reports an artifact that accepts holy-targeted skills.
	Holy() bool
	Unlockable() bool
	// IsPet reports a pet rather than a servitor.
	IsPet() bool

	// HasCorpse reports a pending, lootable corpse.
	HasCorpse() bool
	// CorpseDeadline and CorpseTime drive the too-old corpse cutoff.
	CorpseDeadline() (time.Time, bool)
	CorpseTime() time.Duration
	// Spoiled and Seeded bypass the too-old corpse cutoff.
	Spoiled() bool
	Seeded() bool

	// Summon returns the actor's active summon.
	Summon() (Actor, bool)
	// Owner returns the player controlling a summon.
	Owner() (attackable.Combatant, bool)

	// CanCastOnPlayable applies a playable caster's relationship policy to a
	// playable target.
	CanCastOnPlayable(target Actor, skill *modelskill.Definition, ctrl, offensive bool) bool
	OlympiadMode() bool
	OlympiadStarted() bool
	IsInParty() bool
	PartyContains(other Actor) bool
	IsInSameParty(other Actor) bool
	IsInSameClan(other Actor) bool
	IsInSameAlly(other Actor) bool
	HasClan() bool
	// DuelID is 0 when not dueling.
	DuelID() int32
	DuelTeam() int
	MageClass() bool
	// ClanGroups are the social-group tags a monster template carries.
	ClanGroups() []string
}

// Known enumerates nearby creatures for radius-based target handlers.
type Known interface {
	ForEachKnownCreatureInRadius(anchor Actor, radius int, fn func(Actor))
}

// WorldKnown adapts the world grid to target-handler radius scans.
type WorldKnown struct {
	State *world.State
}

// ForEachKnownCreatureInRadius calls fn for every known creature within
// radius of anchor.
func (w WorldKnown) ForEachKnownCreatureInRadius(anchor Actor, radius int, fn func(Actor)) {
	if w.State == nil || anchor == nil {
		return
	}
	w.State.ForEachKnownInRadius(anchor, radius, func(obj world.Tracked) {
		// Items and static objects on the grid are never skill targets.
		if a, ok := obj.(Actor); ok {
			fn(a)
		}
	})
}

// Handler resolves a skill's final target and affected target list.
type Handler interface {
	Target() modelskill.Target
	Targets(caster, target Actor, skill *modelskill.Definition) []Actor
	FinalTarget(caster, target Actor, skill *modelskill.Definition) Actor
	CanCast(caster, target Actor, skill *modelskill.Definition, ctrl bool) bool
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
