package cast

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Definitions resolves loaded skill definitions.
type Definitions interface {
	Definition(modelskill.Ref) (modelskill.Definition, bool)
}

// StartHooks are the phase callbacks StartPlayerSkill / StartItemSkill
// invoke while accepting a cast. Schedule uses Hooks for Launch / Hit /
// Finish after the cast has already started.
type StartHooks struct {
	ResolveTarget func(Target, world.Tracked, modelskill.Definition, bool) (Target, skilltarget.CastRejection)
	// StopMovement cancels an in-flight walk before final cast validation
	// when the skill's template hit time is long enough to freeze the caster.
	StopMovement func() error
	// AfterCanCast runs after cost/reuse validation succeeds and before the
	// caster faces a non-self target. Ground skills use it for signet LOS,
	// peace-zone, heading, and ValidateLocation.
	AfterCanCast func() error
}

// PlayerSkillRequest is one live player skill-cast request after the network
// packet has been decoded. Ctrl and Shift are the client's cast modifiers,
// carried through unconsumed for the domain cast-condition check and the
// post-cast offensive-follow decision to read once those rules exist; today
// neither consumer exists, so StartPlayerSkill only copies them onto
// StartedSkill.
type PlayerSkillRequest struct {
	Now         time.Time
	Controller  *Controller
	Caster      *player.Character
	Selected    world.Tracked
	SkillID     int
	Definitions Definitions
	Ctrl        bool
	Shift       bool
	Hooks       StartHooks
}

// fakeDeathSkillID is the Fake Death toggle skill. Recasting it while
// faking must be let through even though the caster is FakeDead(): Java's
// PlayerCast.canAttemptCast rejects every cast during fake death except this
// one skill id — `if (_actor.isFakeDeath() && skill.getId() != 60)`
// (PlayerCast.java:218-222) — so the player can un-toggle it.
const fakeDeathSkillID = 60

// StartedSkill is a player skill request accepted by the cast controller.
type StartedSkill struct {
	Definition     modelskill.Definition
	Target         Target
	Rejection      skilltarget.CastRejection
	CanCastFailure bool
	Plan           Plan
	Ctrl           bool
	Shift          bool
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

	started, err := startResolvedSkill(req.Now, req.Controller, req.Caster, req.Selected, def, req.Ctrl, req.Hooks)
	started.Ctrl = req.Ctrl
	started.Shift = req.Shift
	return started, err
}

// ItemSkillRequest is one item-carried skill cast request, routed through
// the AI cast path rather than the instant-cast (potion) path: Skill names
// the definition directly (an item's own attached-skill entry), instead of
// being looked up from the caster's learned skill list.
type ItemSkillRequest struct {
	Now         time.Time
	Controller  *Controller
	Caster      *player.Character
	Selected    world.Tracked
	Skill       modelskill.Ref
	Definitions Definitions
	Hooks       StartHooks
}

// StartItemSkill validates and starts an item-carried skill cast: the same
// cost/reuse/target machinery as StartPlayerSkill, but the skill definition
// comes from Skill directly rather than the caster's own skill level.
func StartItemSkill(req ItemSkillRequest) (StartedSkill, error) {
	if req.Caster == nil || req.Caster.AlikeDead() || req.Definitions == nil || req.Controller == nil {
		return StartedSkill{}, ErrSkillUnavailable
	}

	def, ok := req.Definitions.Definition(req.Skill)
	if !ok || def.Activation != modelskill.ActivationActive {
		return StartedSkill{}, ErrSkillUnavailable
	}

	return startResolvedSkill(req.Now, req.Controller, req.Caster, req.Selected, def, false, req.Hooks)
}

// startResolvedSkill runs the shared target-resolution and cost/reuse start
// sequence once a caller has already resolved def, regardless of whether
// def came from the caster's own skill list or an item's attached skill.
func startResolvedSkill(now time.Time, controller *Controller, caster *player.Character, selected world.Tracked, def modelskill.Definition, ctrl bool, hooks StartHooks) (StartedSkill, error) {
	var target Target
	var rejection skilltarget.CastRejection
	var ok bool
	if hooks.ResolveTarget != nil {
		target, rejection = hooks.ResolveTarget(caster, selected, def, ctrl)
		ok = target != nil
	} else {
		target, ok = SelectTarget(caster, selected, def)
	}
	started := StartedSkill{Definition: def, Target: target, Rejection: rejection}
	if !ok {
		return started, ErrInvalidTarget
	}
	if err := controller.CanAttemptCast(target, def); err != nil {
		return started, err
	}
	stopForCast(def, hooks.StopMovement)
	if err := controller.CanCast(target, def); err != nil {
		started.CanCastFailure = true
		return started, err
	}
	if rejection != skilltarget.CastRejectNone {
		return started, ErrInvalidTarget
	}
	if hooks.AfterCanCast != nil {
		if err := hooks.AfterCanCast(); err != nil {
			return started, err
		}
	}
	faceCastTarget(caster, target, def)

	if now.IsZero() {
		now = time.Now()
	}
	plan, err := controller.Start(now, target, def)
	if err != nil {
		return started, err
	}
	started.Plan = plan

	// PlayerCast.doCast clears the post-fake-death grace unconditionally
	// right after committing to a successfully started cast
	// (_actor.clearRecentFakeDeath(), PlayerCast.java:181-185). It never
	// runs for doInstantCast (potions) or doToggleCast, which
	// startResolvedSkill's ActivationActive-only callers already exclude.
	// FUSION and SIGNET_CASTTIME never reach doCast at all — PlayerAI
	// dispatches them to doFusionCast instead (PlayerAI.java:299-301),
	// which never calls clearRecentFakeDeath — so they're excluded here
	// too.
	if def.SkillType != "FUSION" && def.SkillType != "SIGNET_CASTTIME" {
		caster.ClearRecentFakeDeath()
	}
	return started, nil
}

// stopForCast cancels an in-flight walk when the skill's template hit time
// is long enough that the caster must stand still. Callers must already
// have passed the pre-movement reuse and disable gates.
func stopForCast(def modelskill.Definition, stopMovement func() error) {
	if def.HitTime <= 50 || stopMovement == nil {
		return
	}
	_ = stopMovement()
}

// faceCastTarget orients the caster toward a non-self target after the
// cast has been accepted.
func faceCastTarget(caster *player.Character, target Target, def modelskill.Definition) {
	if caster == nil || target == nil || def.HitTime <= 50 || target.ObjectID() == caster.ObjectID() {
		return
	}
	tx, ty, _ := target.Position()
	caster.SetHeading(caster.CurrentLocation().HeadingTo(location.Location{X: tx, Y: ty}))
}

// PlayerToggleRequest is one live player toggle-skill request after the
// network packet has been decoded.
type PlayerToggleRequest struct {
	Caster      *player.Character
	Selected    world.Tracked
	SkillID     int
	Definitions Definitions
}

// ResolvePlayerToggle validates skillID against caster's known skills and
// resolves it to a toggle definition and target, without consuming any
// resource or touching effect state. ApplyToggle is the typical caller — it
// looks up the caster's live effect list to decide the on/off branch and
// drives Controller.CastToggle with the result.
func ResolvePlayerToggle(req PlayerToggleRequest) (modelskill.Definition, Target, error) {
	if req.Caster == nil || req.Caster.Dead() || (req.Caster.FakeDead() && req.SkillID != fakeDeathSkillID) || req.SkillID <= 0 || req.Definitions == nil {
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
