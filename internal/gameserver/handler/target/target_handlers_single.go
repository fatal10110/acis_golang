package target

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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

func (oneHandler) CanCast(caster, target Creature, skill *modelskill.Definition, ctrl bool) bool {
	if target == nil {
		return false
	}
	if skill == nil {
		return true
	}
	return oneCastRejection(caster, target, skill, ctrl) == CastRejectNone
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
