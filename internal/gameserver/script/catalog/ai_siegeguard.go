package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/siegeguard"
)

// aiSiegeGuard is the catalog of the castle siege guard behaviors (#207).
func aiSiegeGuard() script.Catalog {
	return script.Catalog{
		"script.ai.siegeguards.GludioCleric":                        siegeguard.Cleric,
		"script.ai.siegeguards.GludioClericStand":                   siegeguard.ClericStand,
		"script.ai.siegeguards.GludioHold.GludioBowGuard":           siegeguard.BowGuard,
		"script.ai.siegeguards.GludioHold.GludioCourtGuard":         siegeguard.CourtGuard,
		"script.ai.siegeguards.GludioHold.GludioDukeLewinWaldner":   siegeguard.DukeLewinWaldner,
		"script.ai.siegeguards.GludioHold.GludioHold":               siegeguard.Hold,
		"script.ai.siegeguards.GludioHold.GludioKnight":             siegeguard.Knight,
		"script.ai.siegeguards.GludioHold.GludioSirCronenberg":      nil,
		"script.ai.siegeguards.GludioHold.GludioSpearGuard":         nil,
		"script.ai.siegeguards.GludioHold.GludioSwordGuard":         nil,
		"script.ai.siegeguards.GludioHold.GludioSwordGuardUseSkill": nil,
		"script.ai.siegeguards.GludioStand.GludioBowGuardStand":     nil,
		"script.ai.siegeguards.GludioStand.GludioStand":             nil,
		"script.ai.siegeguards.GludioWizard.GludioWizard":           nil,
		"script.ai.siegeguards.GludioWizard.GludioWizardUseSkill":   nil,
		"script.ai.siegeguards.GludioWizardStand":                   siegeguard.WizardStand,
	}
}
