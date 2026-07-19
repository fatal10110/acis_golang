package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Definitions resolves loaded skill definitions.
type Definitions interface {
	Definition(modelskill.Ref) (modelskill.Definition, bool)
}

// PlayerSkillRequest is one live player skill-cast request after the network
// packet has been decoded.
type PlayerSkillRequest struct {
	Now         time.Time
	Controller  *Controller
	Caster      *player.Character
	Selected    any
	SkillID     int
	Definitions Definitions
}

// StartedSkill is a player skill request accepted by the cast controller.
type StartedSkill struct {
	Definition modelskill.Definition
	Target     Target
	Plan       Plan
}

// StartPlayerSkill validates and starts a live player skill cast.
func StartPlayerSkill(req PlayerSkillRequest) (StartedSkill, error) {
	if req.Caster == nil || req.Caster.AlikeDead() || req.SkillID <= 0 || req.Definitions == nil || req.Controller == nil {
		return StartedSkill{}, ErrSkillUnavailable
	}

	level := req.Caster.SkillLevel(req.SkillID)
	if level <= 0 {
		return StartedSkill{}, ErrSkillUnavailable
	}

	def, ok := req.Definitions.Definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: level})
	if !ok || def.Activation != modelskill.ActivationActive {
		return StartedSkill{}, ErrSkillUnavailable
	}

	target, ok := SelectTarget(req.Caster, req.Selected, def)
	started := StartedSkill{Definition: def, Target: target}
	if !ok {
		return started, ErrInvalidTarget
	}

	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	plan, err := req.Controller.Start(now, target, def)
	if err != nil {
		return started, err
	}
	started.Plan = plan
	return started, nil
}

// PlayerToggleRequest is one live player toggle-skill request after the
// network packet has been decoded.
type PlayerToggleRequest struct {
	Caster      *player.Character
	Selected    any
	SkillID     int
	Definitions Definitions
}

// ResolvePlayerToggle validates skillID against caster's known skills and
// resolves it to a toggle definition and target, without consuming any
// resource or touching effect state. ApplyToggle is the typical caller — it
// looks up the caster's live effect list to decide the on/off branch and
// drives Controller.CastToggle with the result.
func ResolvePlayerToggle(req PlayerToggleRequest) (modelskill.Definition, Target, error) {
	if req.Caster == nil || req.Caster.AlikeDead() || req.SkillID <= 0 || req.Definitions == nil {
		return modelskill.Definition{}, nil, ErrSkillUnavailable
	}

	level := req.Caster.SkillLevel(req.SkillID)
	if level <= 0 {
		return modelskill.Definition{}, nil, ErrSkillUnavailable
	}

	def, ok := req.Definitions.Definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: level})
	if !ok || def.Activation != modelskill.ActivationToggle {
		return modelskill.Definition{}, nil, ErrSkillUnavailable
	}

	target, ok := SelectTarget(req.Caster, req.Selected, def)
	if !ok {
		return def, nil, ErrInvalidTarget
	}
	return def, target, nil
}
