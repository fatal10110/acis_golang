package xml

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// errNestedEffect rejects an <effect> inside an <effect>.
var errNestedEffect = errors.New("nested effects")

// readEffectTemplate reads an <effect> element's values into an effect
// template; skills and items share this grammar. vals holds the attributes
// after the caller's table resolution. stackType is never resolved: it is
// read as written.
func readEffectTemplate(vals map[string]string) (skill.EffectTemplate, error) {
	a := newAttrValues(vals, "effect")
	name := a.str("name")
	if err := a.Err(); err != nil {
		return skill.EffectTemplate{}, err
	}
	if name == "" {
		return skill.EffectTemplate{}, errors.New(`effect: attribute "name" is empty`)
	}
	if !effect.Known(name) {
		return skill.EffectTemplate{}, fmt.Errorf("effect: no effect implementation named %q", name)
	}
	a.prefix = "effect " + name

	eff := skill.EffectTemplate{
		Name:             name,
		Value:            a.float64("val"),
		Count:            int(a.int32LiteralDefault("count", 1)),
		Time:             int(a.int32LiteralDefault("time", 1)),
		Self:             a.int32LiteralDefault("self", 0) == 1,
		Icon:             a.int32LiteralDefault("noicon", 0) != 1,
		StackType:        a.strDefault("stackType", "none"),
		StackOrder:       a.float64Default("stackOrder", 0),
		EffectPower:      a.float64Default("effectPower", -1),
		EffectPowerSet:   a.has("effectPower"),
		EffectType:       a.strDefault("effectType", ""),
		TriggeredID:      int(a.int32Default("triggeredId", 0)),
		TriggeredLevel:   int(a.int32Default("triggeredLevel", 1)),
		ChanceType:       a.strDefault("chanceType", ""),
		ActivationChance: int(a.int32Default("activationChance", -1)),
	}
	if a.has("effectType") && !skill.KnownSkillType(eff.EffectType) {
		a.fail(fmt.Errorf("attribute %q: unknown skill type %q", "effectType", eff.EffectType))
	}
	if a.has("chanceType") {
		// A present chanceType must name a trigger; an empty one is not
		// "no trigger" but a malformed value.
		if _, ok, err := skill.ParseChanceCondition(eff.ChanceType, eff.ActivationChance); err != nil || !ok {
			a.fail(fmt.Errorf("attribute %q: unknown trigger type %q", "chanceType", eff.ChanceType))
		}
	}
	if a.has("abnormal") {
		mask, err := skill.ParseAbnormalEffect(a.str("abnormal"))
		if err != nil {
			a.fail(fmt.Errorf("attribute %q: %w", "abnormal", err))
		}
		eff.AbnormalEffect = mask
	}
	if err := a.Err(); err != nil {
		return skill.EffectTemplate{}, err
	}
	return eff, nil
}

// readFuncStat reads a stat func's required stat, which must name a known
// stat exactly as written.
func readFuncStat(a *attrValues) string {
	name := a.str("stat")
	if a.err != nil {
		return ""
	}
	if _, err := stat.ByName(name); err != nil {
		a.fail(err)
		return ""
	}
	return name
}
