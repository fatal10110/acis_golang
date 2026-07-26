package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
)

// cubicSummoner is the narrow surface a cubic-granting SUMMON skill needs
// from a target: admit or refresh the cubic on that target's own list.
type cubicSummoner interface {
	AddOrRefreshCubic(id cubic.ID, givenByOther bool) (added bool)
}

// cubicHandler applies the cubic branch of a SkillType.SUMMON cast: adding
// or refreshing a cubic on the caster's cubic list, or on every targeted
// player's for a mass-cubic skill. The servitor branch of the same skill
// type (def.IsCubic false) is out of this handler's scope — tracked by
// #960 — and is a silent no-op here, matching how an unregistered skill
// type produces no effect elsewhere in this registry.
type cubicHandler struct{}

func (cubicHandler) Types() []string { return []string{"SUMMON"} }

func (cubicHandler) Use(cast Cast) {
	cubicHandler{}.UseResult(cast)
}

func (cubicHandler) UseResult(cast Cast) Result {
	if !cast.Skill.IsCubic {
		return Result{}
	}
	id := cubic.ID(cast.Skill.NpcID)

	var added bool
	if len(cast.Targets) > 1 {
		// Mass-cubic cast: every targeted player except the caster
		// receives the cubic as "given by another player". No shipped
		// cubic skill in the reference data actually resolves more than
		// one target (every one declares target SELF), so a recipient
		// other than the caster never gets its own character-info
		// refreshed here — only the caster's CubicAdded is reported,
		// matching what an actual multi-target cubic skill would need if
		// one existed.
		for _, target := range cast.Targets {
			summoner, ok := target.(cubicSummoner)
			if !ok {
				continue
			}
			givenByOther := !sameObject(cast.Caster, target)
			if summoner.AddOrRefreshCubic(id, givenByOther) && !givenByOther {
				added = true
			}
		}
		return Result{CubicAdded: added}
	}

	summoner, ok := cast.Caster.(cubicSummoner)
	if !ok {
		return Result{}
	}
	added = summoner.AddOrRefreshCubic(id, false)
	return Result{CubicAdded: added}
}
