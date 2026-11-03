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
		"script.ai.siegeguards.GludioHold.GludioSirCronenberg":      siegeguard.SirCronenberg,
		"script.ai.siegeguards.GludioHold.GludioSpearGuard":         siegeguard.SpearGuard,
		"script.ai.siegeguards.GludioHold.GludioSwordGuard":         siegeguard.SwordGuard,
		"script.ai.siegeguards.GludioHold.GludioSwordGuardUseSkill": siegeguard.SwordGuardUseSkill,
		"script.ai.siegeguards.GludioStand.GludioBowGuardStand":     siegeguard.BowGuardStand,
		"script.ai.siegeguards.GludioStand.GludioStand":             siegeguard.Stand,
		"script.ai.siegeguards.GludioWizard.GludioWizard":           siegeguard.Wizard,
		"script.ai.siegeguards.GludioWizard.GludioWizardUseSkill":   siegeguard.WizardUseSkill,
		"script.ai.siegeguards.GludioWizardStand":                   siegeguard.WizardStand,
	}
}
