package cast

import modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

// Target is a live object a cast packet can identify and position.
type Target interface {
	ObjectID() int32
	Position() (x, y, z int)
}

// SelectTarget resolves the concrete target for def from the caster and the
// currently selected live object.
func SelectTarget(caster Target, selected any, def modelskill.Definition) (Target, bool) {
	switch def.Target {
	case modelskill.TargetNone, modelskill.TargetSelf, modelskill.TargetGround:
		return caster, caster != nil
	case modelskill.TargetOne:
		target, ok := selected.(Target)
		return target, ok
	}
	return nil, false
}
