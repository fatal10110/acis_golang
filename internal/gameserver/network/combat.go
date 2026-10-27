package network

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// CombatConfig holds the collaborators of combat and targeting: the attack
// stances, the actor AI registrations, the PvP flags and the spiritshot
// heal corrections the skill handlers apply.
type CombatConfig struct {
	AttackStance AttackStanceTracker
	AI           AIRegistry
	PvPFlags     *task.PvPFlags
	// HealSps holds the spiritshot heal corrections; nil gives a charged
	// heal no correction term.
	HealSps *modelskill.HealSpsTable
}

// combat is combat and targeting: who is in attack stance, which actors'
// AI runs, who is PvP flagged, how a skill finds its targets, what each
// skill does when it lands, and the chance procs hits and casts trigger.
// NewGameClientLink builds it whole and nothing in it is replaced
// afterwards, so every client goroutine reads it without a lock. A nil
// attackStance, ai or pvpFlags leaves that part out.
type combat struct {
	attackStance  AttackStanceTracker
	ai            AIRegistry
	pvpFlags      *task.PvPFlags
	targets       *skilltarget.Registry
	skillHandlers *handlerskill.Registry
	// chance needs the link's skill definitions and delivery, so
	// NewGameClientLink sets it once the link exists.
	chance *actorcast.ChanceProcs
}

// newCombat builds combat from cfg's combat collaborators and the world,
// skill, zone and effect wiring its skill handlers act through. The
// handlers that need the link itself are registered by NewGameClientLink.
func newCombat(cfg GameClientLinkConfig) combat {
	return combat{
		attackStance: cfg.Combat.AttackStance,
		ai:           cfg.Combat.AI,
		pvpFlags:     cfg.Combat.PvPFlags,
		targets:      skilltarget.NewRegistry(skilltarget.WorldKnown{State: cfg.World}),
		skillHandlers: handlerskill.NewDefaultRegistryWithSignet(cfg.Skills, cfg.PlayerConfig.MagicFailures, cfg.Combat.HealSps, handlerskill.SignetDeps{
			Templates: cfg.NPCs,
			IDs:       cfg.IDs,
			World:     cfg.World,
			NewSink:   EffectPointSinks(cfg.World),
			Zones:     cfg.Zones,
			Effects:   cfg.Effects,
			Queues:    cfg.Queues,
			Log:       cfg.Log,

			ManorCropRate: cfg.Manor.CropRate,
		}),
	}
}
