package target

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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

type selfHandler struct{}

func (selfHandler) Target() modelskill.Target { return modelskill.TargetSelf }

func (selfHandler) Targets(caster, _ Creature, _ *modelskill.Definition) []Creature {
	return []Creature{caster}
}

func (selfHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (selfHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool { return true }

type oneHandler struct{}

func (oneHandler) Target() modelskill.Target { return modelskill.TargetOne }

func (oneHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (oneHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (oneHandler) CanCast(caster, target Creature, skill *modelskill.Definition, _ bool) bool {
	if target == nil {
		return false
	}
	if skill != nil && skill.Offensive && (sameCreature(caster, target) || target.Dead()) {
		return false
	}
	return true
}

type holyHandler struct{}

func (holyHandler) Target() modelskill.Target { return modelskill.TargetHoly }

func (holyHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (holyHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (holyHandler) CanCast(_, target Creature, _ *modelskill.Definition, _ bool) bool {
	holy, ok := target.(HolyTarget)
	return ok && holy.Holy()
}

type unlockableHandler struct{}

func (unlockableHandler) Target() modelskill.Target { return modelskill.TargetUnlockable }

func (unlockableHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (unlockableHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (unlockableHandler) CanCast(_, target Creature, _ *modelskill.Definition, _ bool) bool {
	unlockable, ok := target.(UnlockableTarget)
	return ok && unlockable.Unlockable()
}

type undeadHandler struct{}

func (undeadHandler) Target() modelskill.Target { return modelskill.TargetUndead }

func (undeadHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (undeadHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (undeadHandler) CanCast(_, target Creature, _ *modelskill.Definition, _ bool) bool {
	return validUndeadSingleTarget(target)
}

type areaHandler struct {
	known Known
}

func (areaHandler) Target() modelskill.Target { return modelskill.TargetArea }

func (h areaHandler) Targets(caster, target Creature, skill *modelskill.Definition) []Creature {
	if target == nil {
		return nil
	}
	out := []Creature{target}
	h.forEachAreaTarget(caster, target, skillRadius(skill), nil, func(creature Creature) {
		out = append(out, creature)
	})
	return out
}

func (areaHandler) FinalTarget(caster, target Creature, _ *modelskill.Definition) Creature {
	if target == nil || sameCreature(caster, target) || target.Dead() {
		return nil
	}
	return target
}

func (h areaHandler) CanCast(caster, target Creature, skill *modelskill.Definition, ctrl bool) bool {
	if skill == nil || !skill.Offensive {
		return true
	}
	if h.FinalTarget(caster, target, skill) == nil {
		return false
	}
	if !attackableBy(target, caster) {
		return false
	}
	return ctrl || attackableWithoutForceBy(target, caster)
}

func (h areaHandler) forEachAreaTarget(caster, anchor Creature, radius int, keep func(Creature) bool, fn func(Creature)) {
	if h.known == nil {
		return
	}
	h.known.ForEachKnownCreatureInRadius(anchor, radius, func(creature Creature) {
		if sameCreature(caster, creature) || creature.Dead() || !canSee(anchor, creature) {
			return
		}
		if keep != nil && !keep(creature) {
			return
		}
		if areaCanAffect(caster, creature) {
			fn(creature)
		}
	})
}

type frontAreaHandler struct {
	known Known
}

func (frontAreaHandler) Target() modelskill.Target { return modelskill.TargetFrontArea }

func (h frontAreaHandler) Targets(caster, target Creature, skill *modelskill.Definition) []Creature {
	if target == nil {
		return nil
	}
	out := []Creature{target}
	areaHandler{known: h.known}.forEachAreaTarget(caster, target, skillRadius(skill), func(creature Creature) bool {
		return creatureOrientedLocation(caster).IsInFrontOf(creatureLocation(creature))
	}, func(creature Creature) {
		out = append(out, creature)
	})
	return out
}

func (frontAreaHandler) FinalTarget(caster, target Creature, _ *modelskill.Definition) Creature {
	if target == nil || sameCreature(caster, target) || target.Dead() {
		return nil
	}
	return target
}

func (h frontAreaHandler) CanCast(caster, target Creature, skill *modelskill.Definition, ctrl bool) bool {
	return areaHandler{known: h.known}.CanCast(caster, target, skill, ctrl)
}

type auraHandler struct {
	known Known
}

func (auraHandler) Target() modelskill.Target { return modelskill.TargetAura }

func (h auraHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	return h.collect(caster, skillRadius(skill), nil)
}

func (auraHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (auraHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool { return true }

func (h auraHandler) collect(caster Creature, radius int, keep func(Creature) bool) []Creature {
	if h.known == nil {
		return nil
	}
	var out []Creature
	h.known.ForEachKnownCreatureInRadius(caster, radius, func(creature Creature) {
		if creature.Dead() || !canSee(caster, creature) {
			return
		}
		if keep != nil && !keep(creature) {
			return
		}
		if auraCanAffect(caster, creature) {
			out = append(out, creature)
		}
	})
	return out
}

type frontAuraHandler struct {
	known Known
}

func (frontAuraHandler) Target() modelskill.Target { return modelskill.TargetFrontAura }

func (h frontAuraHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	return auraHandler{known: h.known}.collect(caster, skillRadius(skill), func(creature Creature) bool {
		return creatureOrientedLocation(caster).IsInFrontOf(creatureLocation(creature))
	})
}

func (frontAuraHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (frontAuraHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool {
	return true
}

type behindAuraHandler struct {
	known Known
}

func (behindAuraHandler) Target() modelskill.Target { return modelskill.TargetBehindAura }

func (h behindAuraHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	return auraHandler{known: h.known}.collect(caster, skillRadius(skill), func(creature Creature) bool {
		return creatureOrientedLocation(caster).IsBehind(creatureLocation(creature))
	})
}

func (behindAuraHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (behindAuraHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool {
	return true
}

type auraUndeadHandler struct {
	known Known
}

func (auraUndeadHandler) Target() modelskill.Target { return modelskill.TargetAuraUndead }

func (h auraUndeadHandler) Targets(caster, _ Creature, skill *modelskill.Definition) []Creature {
	if h.known == nil {
		return nil
	}
	var out []Creature
	h.known.ForEachKnownCreatureInRadius(caster, skillRadius(skill), func(creature Creature) {
		if creature.Dead() || !isUndead(creature) || !canSee(caster, creature) {
			return
		}
		if areaCanAffect(caster, creature) {
			out = append(out, creature)
		}
	})
	return out
}

func (auraUndeadHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

func (auraUndeadHandler) CanCast(caster, _ Creature, skill *modelskill.Definition, _ bool) bool {
	return skill == nil || !skill.Offensive || !inPeaceZone(caster)
}

type summonHandler struct{}

func (summonHandler) Target() modelskill.Target { return modelskill.TargetSummon }

func (summonHandler) Targets(caster, _ Creature, _ *modelskill.Definition) []Creature {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return []Creature{summon}
}

func (summonHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return summon
}

func (summonHandler) CanCast(caster, _ Creature, _ *modelskill.Definition, _ bool) bool {
	summon, ok := summonOf(caster)
	return ok && !summon.Dead()
}

type areaSummonHandler struct {
	known Known
}

func (areaSummonHandler) Target() modelskill.Target { return modelskill.TargetAreaSummon }

func (h areaSummonHandler) Targets(caster, target Creature, skill *modelskill.Definition) []Creature {
	if !caster.Category().Has(CategoryPlayable) || target == nil {
		return nil
	}
	var out []Creature
	areaHandler{known: h.known}.forEachAreaTarget(caster, target, skillRadius(skill), nil, func(creature Creature) {
		out = append(out, creature)
	})
	return out
}

func (areaSummonHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	summon, ok := summonOf(caster)
	if !ok {
		return nil
	}
	return summon
}

func (areaSummonHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool {
	return true
}

type ownerPetHandler struct{}

func (ownerPetHandler) Target() modelskill.Target { return modelskill.TargetOwnerPet }

func (ownerPetHandler) Targets(caster, _ Creature, _ *modelskill.Definition) []Creature {
	owner, ok := ownerOf(caster)
	if !ok {
		return nil
	}
	return []Creature{owner}
}

func (ownerPetHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	owner, ok := ownerOf(caster)
	if !ok {
		return nil
	}
	return owner
}

func (ownerPetHandler) CanCast(caster, target Creature, _ *modelskill.Definition, _ bool) bool {
	owner, ok := ownerOf(caster)
	return ok && sameCreature(owner, target) && !target.Dead()
}

type corpseMobHandler struct{}

func (corpseMobHandler) Target() modelskill.Target { return modelskill.TargetCorpseMob }

func (corpseMobHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (corpseMobHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (corpseMobHandler) CanCast(_, target Creature, skill *modelskill.Definition, _ bool) bool {
	return corpseMobCanCast(target, skill)
}

type areaCorpseMobHandler struct {
	known Known
}

func (areaCorpseMobHandler) Target() modelskill.Target { return modelskill.TargetAreaCorpseMob }

// harvestGrandBoxSkillID is the one skill (Harvest Grand Box, id 444) that
// widens the corpse-mob area scan to also sweep in every already-dead
// attackable creature nearby, instead of the usual live-target splash.
const harvestGrandBoxSkillID = 444

func (h areaCorpseMobHandler) Targets(caster, target Creature, skill *modelskill.Definition) []Creature {
	if target == nil {
		return nil
	}
	out := []Creature{target}
	if h.known == nil {
		return out
	}
	h.known.ForEachKnownCreatureInRadius(target, skillRadius(skill), func(creature Creature) {
		if sameCreature(caster, creature) || !canSee(target, creature) {
			return
		}
		if skill != nil && skill.ID == harvestGrandBoxSkillID {
			if creature.Category().Has(CategoryAttackable) && creature.Dead() {
				out = append(out, creature)
			}
			return
		}
		if creature.Dead() {
			return
		}
		if areaCanAffect(caster, creature) {
			out = append(out, creature)
		}
	})
	return out
}

func (areaCorpseMobHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (areaCorpseMobHandler) CanCast(_, target Creature, skill *modelskill.Definition, _ bool) bool {
	return corpseMobCanCast(target, skill)
}

// corpseMobCanCast applies the mob-corpse cast-eligibility rule shared by
// the single-target and area corpse-mob handlers: the target must have a
// pending corpse and not be a player-controlled actor or summon, a harvest
// skill always succeeds against an attackable corpse regardless of its age,
// and a sweep skill only ever succeeds against an attackable corpse. Mob
// corpses that expose decay deadline state stop accepting generic corpse
// skills once they are past the halfway age cutoff, unless seeded/spoiled.
func corpseMobCanCast(target Creature, skill *modelskill.Definition) bool {
	if target == nil || !hasCorpse(target) || target.Category().Has(CategoryPlayable) {
		return false
	}
	if skill != nil && skill.SkillType == "HARVEST" {
		return target.Category().Has(CategoryAttackable)
	}
	if skill != nil && skill.SkillType == "SWEEP" && !target.Category().Has(CategoryAttackable) {
		return false
	}
	if target.Category().Has(CategoryAttackable) && corpseTooOld(target) && !corpseAgeBypass(target) {
		return false
	}
	return true
}

type corpsePlayerHandler struct{}

func (corpsePlayerHandler) Target() modelskill.Target { return modelskill.TargetCorpsePlayer }

func (corpsePlayerHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (corpsePlayerHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

func (corpsePlayerHandler) CanCast(_, target Creature, _ *modelskill.Definition, _ bool) bool {
	return target != nil && target.Dead() && target.Category().Has(CategoryPlayable)
}

type corpsePetHandler struct{}

func (corpsePetHandler) Target() modelskill.Target { return modelskill.TargetCorpsePet }

func (corpsePetHandler) Targets(_, target Creature, _ *modelskill.Definition) []Creature {
	return []Creature{target}
}

func (corpsePetHandler) FinalTarget(_, target Creature, _ *modelskill.Definition) Creature {
	return target
}

// CanCast requires the target be dead and owned by another creature — the
// closest available signal to a pet, since no actor type distinguishes pets
// from other player-owned summons yet.
func (corpsePetHandler) CanCast(_, target Creature, _ *modelskill.Definition, _ bool) bool {
	if target == nil || !target.Dead() {
		return false
	}
	_, ok := ownerOf(target)
	return ok
}

type groundHandler struct{}

func (groundHandler) Target() modelskill.Target { return modelskill.TargetGround }

func (groundHandler) Targets(caster, _ Creature, _ *modelskill.Definition) []Creature {
	return []Creature{caster}
}

func (groundHandler) FinalTarget(caster, _ Creature, _ *modelskill.Definition) Creature {
	return caster
}

// CanCast reports true unconditionally. The real cast-eligibility checks —
// line of sight from the caster to the cast's target point and whether that
// point falls inside a peace zone — depend on a per-cast target-point field
// and a location-aware line-of-sight query that don't exist on the caster
// yet; gating on them is deferred until that plumbing lands.
func (groundHandler) CanCast(Creature, Creature, *modelskill.Definition, bool) bool {
	return true
}

func skillRadius(skill *modelskill.Definition) int {
	if skill == nil {
		return 0
	}
	return skill.Radius
}

func sameCreature(a, b Creature) bool {
	return a != nil && b != nil && a.ObjectID() == b.ObjectID()
}

func canSee(origin, target Creature) bool {
	checker, ok := origin.(SightChecker)
	return !ok || checker.CanSeeTarget(target)
}

func areaCanAffect(caster, creature Creature) bool {
	if caster.Category().Has(CategoryPlayable) && creature.Category()&(CategoryAttackable|CategoryPlayable) != 0 {
		return attackableWithoutForceBy(creature, caster)
	}
	if caster.Category().Has(CategoryAttackable) && creature.Category().Has(CategoryPlayable) {
		return attackableBy(creature, caster)
	}
	return false
}

func auraCanAffect(caster, creature Creature) bool {
	if areaCanAffect(caster, creature) {
		return true
	}
	return caster.Category().Has(CategoryFolk) && creature.Category().Has(CategoryPlayable)
}

func attackableBy(creature, caster Creature) bool {
	rules, ok := creature.(AttackRules)
	return ok && rules.AttackableBy(caster)
}

func attackableWithoutForceBy(creature, caster Creature) bool {
	rules, ok := creature.(AttackRules)
	return ok && rules.AttackableWithoutForceBy(caster)
}

func validUndeadSingleTarget(creature Creature) bool {
	if creature == nil || creature.Dead() || !isUndead(creature) {
		return false
	}
	if creature.Category().Has(CategoryAttackable) {
		return true
	}
	if creature.Category().Has(CategoryPlayable) {
		_, ok := ownerOf(creature)
		return ok
	}
	return false
}

func isUndead(creature Creature) bool {
	undead, ok := creature.(UndeadTarget)
	return ok && undead.Undead()
}

func hasCorpse(creature Creature) bool {
	corpse, ok := creature.(CorpseTarget)
	return ok && corpse.HasCorpse()
}

func corpseTooOld(creature Creature) bool {
	target, ok := creature.(CorpseDeadlineTarget)
	if !ok {
		return false
	}
	deadline, ok := target.CorpseDeadline()
	if !ok {
		return false
	}
	corpseTime := target.CorpseTime()
	if corpseTime <= 0 {
		return false
	}
	cutoff := deadline.Add(-corpseTime / 2)
	return !time.Now().Before(cutoff)
}

func corpseAgeBypass(creature Creature) bool {
	if spoiled, ok := creature.(SpoiledCorpse); ok && spoiled.Spoiled() {
		return true
	}
	seeded, ok := creature.(SeededCorpse)
	return ok && seeded.Seeded()
}

func inPeaceZone(creature Creature) bool {
	zoner, ok := creature.(PeaceZoner)
	return ok && zoner.InPeaceZone()
}

func summonOf(creature Creature) (Creature, bool) {
	summoner, ok := creature.(Summoner)
	if !ok {
		return nil, false
	}
	summon, ok := summoner.Summon()
	return summon, ok && summon != nil
}

func ownerOf(creature Creature) (Creature, bool) {
	owned, ok := creature.(OwnedCreature)
	if !ok {
		return nil, false
	}
	owner, ok := owned.Owner()
	return owner, ok && owner != nil
}

func creatureLocation(creature Creature) location.Location {
	if creature == nil {
		return location.Location{}
	}
	x, y, z := creature.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func creatureOrientedLocation(creature Creature) location.OrientedLocation {
	if creature == nil {
		return location.OrientedLocation{}
	}
	return location.OrientedLocation{Location: creatureLocation(creature), Heading: creature.Heading()}
}
