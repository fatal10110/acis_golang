package item

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Handler names of the manor items: seeds sown on a living monster and the
// harvester used on a sown corpse.
const (
	SeedsHandler      = "Seeds"
	HarvestersHandler = "Harvesters"
)

// harvestSkill is the skill a harvester casts, whatever the item carries.
var harvestSkill = modelskill.Ref{ID: 2098, Level: 1}

// ManorRules is what the manor items read: whether the manor is on at all,
// the seed rows by seed item, and the manor areas monsters are sown in.
type ManorRules struct {
	Allowed bool
	Seeds   *manor.Table
	Areas   *manor.AreaIndex
}

// ManorRefusal is why a manor item casts nothing.
type ManorRefusal uint8

const (
	// ManorAllowed means the item's skill is cast on the target.
	ManorAllowed ManorRefusal = iota
	// ManorSilent means the item is refused with no answer.
	ManorSilent
	// ManorUnavailableForSeeding means the target is no seedable monster
	// spawned in a manor area (THE_TARGET_IS_UNAVAILABLE_FOR_SEEDING).
	ManorUnavailableForSeeding
	// ManorSeedNotHere means the target's manor area belongs to another
	// castle than the seed (THIS_SEED_MAY_NOT_BE_SOWN_HERE).
	ManorSeedNotHere
	// ManorInvalidTarget means a dead sow target, or a harvester target that
	// is no corpse (INVALID_TARGET).
	ManorInvalidTarget
	// ManorAlreadySown means the target was sown before
	// (THE_SEED_HAS_BEEN_SOWN).
	ManorAlreadySown
)

// ManorUse is the decision on one seed or harvester use.
type ManorUse struct {
	// Handled reports that the item is a seed or a harvester; nothing else
	// is decided otherwise.
	Handled bool
	Refusal ManorRefusal
	// Skill is cast on the user's current target, with no modifiers, when
	// Refusal is ManorAllowed.
	Skill modelskill.Definition
	// Carrier reports that the item itself carries the cast: it is spent as
	// the cast starts and is the seed the sow reads. A harvester's cast is
	// an ordinary skill cast.
	Carrier bool
}

// ManorUser is the player using a manor item.
type ManorUser interface {
	Target() world.Tracked
}

// seedTarget is a monster a seed may be sown on.
type seedTarget interface {
	MonsterKind() bool
	Seedable() bool
	SpawnLocation() (location.Location, bool)
	Dead() bool
	Seeded() bool
}

// ResolveManorItem decides a seed or harvester use by user against the
// user's current target.
func ResolveManorItem(tmpl *modelitem.Template, user ManorUser, rules ManorRules, defs actorcast.Definitions) ManorUse {
	if tmpl == nil || tmpl.Kind != modelitem.KindEtcItem || tmpl.EtcItem == nil || user == nil {
		return ManorUse{}
	}
	switch tmpl.EtcItem.Handler {
	case SeedsHandler:
		return resolveSeed(tmpl, user, rules, defs)
	case HarvestersHandler:
		return resolveHarvester(user, rules, defs)
	}
	return ManorUse{}
}

// resolveSeed refuses, in order: a disabled manor; a target that is no
// seedable monster spawned inside a manor area; an item no seed row names;
// a seed of another castle than the area's; a dead target; and a target
// already sown. It then casts the seed's first skill with the seed as the
// cast's carrier.
func resolveSeed(tmpl *modelitem.Template, user ManorUser, rules ManorRules, defs actorcast.Definitions) ManorUse {
	if !rules.Allowed {
		return ManorUse{Handled: true, Refusal: ManorSilent}
	}
	target, ok := user.Target().(seedTarget)
	if !ok || !target.MonsterKind() {
		return ManorUse{Handled: true, Refusal: ManorUnavailableForSeeding}
	}
	area, inArea := spawnArea(target, rules.Areas)
	if !target.Seedable() || !inArea {
		return ManorUse{Handled: true, Refusal: ManorUnavailableForSeeding}
	}
	seed, ok := rules.Seeds.Seed(tmpl.ID)
	if !ok {
		return ManorUse{Handled: true, Refusal: ManorSilent}
	}
	if area.CastleID != seed.CastleID {
		return ManorUse{Handled: true, Refusal: ManorSeedNotHere}
	}
	if target.Dead() {
		return ManorUse{Handled: true, Refusal: ManorInvalidTarget}
	}
	if target.Seeded() {
		return ManorUse{Handled: true, Refusal: ManorAlreadySown}
	}
	if len(tmpl.AttachedSkills) == 0 {
		return ManorUse{Handled: true, Refusal: ManorSilent}
	}
	def, ok := resolveSkillRef(tmpl.AttachedSkills[0], defs)
	if !ok {
		return ManorUse{Handled: true, Refusal: ManorSilent}
	}
	return ManorUse{Handled: true, Skill: def, Carrier: true}
}

// spawnArea is the manor area holding target's spawn point, wherever the
// target stands now.
func spawnArea(target seedTarget, areas *manor.AreaIndex) (manor.Area, bool) {
	home, ok := target.SpawnLocation()
	if !ok {
		return manor.Area{}, false
	}
	return areas.AreaAt(home.X, home.Y, home.Z)
}

// resolveHarvester refuses a disabled manor silently, then any target that
// is no dead creature, and casts the harvest skill on the corpse.
func resolveHarvester(user ManorUser, rules ManorRules, defs actorcast.Definitions) ManorUse {
	if !rules.Allowed {
		return ManorUse{Handled: true, Refusal: ManorSilent}
	}
	target, ok := user.Target().(skilltarget.Actor)
	if !ok || !target.Dead() {
		return ManorUse{Handled: true, Refusal: ManorInvalidTarget}
	}
	if defs == nil {
		return ManorUse{Handled: true, Refusal: ManorSilent}
	}
	def, ok := defs.Definition(harvestSkill)
	if !ok {
		return ManorUse{Handled: true, Refusal: ManorSilent}
	}
	return ManorUse{Handled: true, Skill: def}
}
