package xml

import (
	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// compileSkillConditions compiles every condition clause of defs at load,
// keeping the compiled tree for the casts that test it. A clause that does
// not compile still loads and is logged: every cast it guards is refused.
func compileSkillConditions(defs []skill.Definition, log zerolog.Logger) {
	for i := range defs {
		def := &defs[i]
		for j := range def.Conditions {
			if _, err := conditions.CompiledClause(&def.Conditions[j]); err != nil {
				log.Warn().Err(err).Int("skill", int(def.ID)).Int("level", def.Level).
					Msg("data/xml: skill condition does not compile; every cast it guards is refused")
			}
		}
	}
}

// compileItemConditions compiles every use-condition block of templates at
// load, keeping the compiled tree for the uses that test it. A block that
// does not compile still loads and is logged: every use it guards is
// refused.
func compileItemConditions(templates []*item.Template, log zerolog.Logger) {
	for _, tpl := range templates {
		for i := range tpl.UseConditions {
			if _, err := conditions.CompiledItemUse(&tpl.UseConditions[i]); err != nil {
				log.Warn().Err(err).Int("item", int(tpl.ID)).
					Msg("data/xml: item use condition does not compile; every use it guards is refused")
			}
		}
	}
}
