package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// aiSiegeGuard is the catalog of the castle siege guard behaviors (#207).
func aiSiegeGuard() script.Catalog {
	return script.Catalog{
		"script.ai.siegeguards.GludioCleric":                        nil,
		"script.ai.siegeguards.GludioClericStand":                   nil,
		"script.ai.siegeguards.GludioHold.GludioBowGuard":           nil,
		"script.ai.siegeguards.GludioHold.GludioCourtGuard":         nil,
		"script.ai.siegeguards.GludioHold.GludioDukeLewinWaldner":   nil,
		"script.ai.siegeguards.GludioHold.GludioHold":               nil,
		"script.ai.siegeguards.GludioHold.GludioKnight":             nil,
		"script.ai.siegeguards.GludioHold.GludioSirCronenberg":      nil,
		"script.ai.siegeguards.GludioHold.GludioSpearGuard":         nil,
		"script.ai.siegeguards.GludioHold.GludioSwordGuard":         nil,
		"script.ai.siegeguards.GludioHold.GludioSwordGuardUseSkill": nil,
		"script.ai.siegeguards.GludioStand.GludioBowGuardStand":     nil,
		"script.ai.siegeguards.GludioStand.GludioStand":             nil,
		"script.ai.siegeguards.GludioWizard.GludioWizard":           nil,
		"script.ai.siegeguards.GludioWizard.GludioWizardUseSkill":   nil,
		"script.ai.siegeguards.GludioWizardStand":                   nil,
	}
}
