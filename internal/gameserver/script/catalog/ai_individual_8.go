package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/guard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/individual"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/royalrush"
)

// aiIndividual8 is the catalog of batch 8 of 8 of the individual NPC behaviors (#193).
func aiIndividual8() script.Catalog {
	return script.Catalog{
		"script.ai.individual.AgitWarrior.AgitWarrior":                                             nil,
		"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitThief":                               nil,
		"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitWarriorFlee":                         nil,
		"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitWizardFlee":                          nil,
		"script.ai.individual.AgitWarrior.AgitWizard":                                              nil,
		"script.ai.individual.AltarDoorController":                                                 individual.AltarDoorController,
		"script.ai.individual.AreadataSwitch":                                                      individual.AreadataSwitch,
		"script.ai.individual.Corpse":                                                              individual.Corpse,
		"script.ai.individual.DaimonTheWhiteEyed":                                                  individual.DaimonTheWhiteEyed,
		"script.ai.individual.Guard.GuardFixed":                                                    guard.Fixed,
		"script.ai.individual.Guard.GuardMoveAroundFixed":                                          guard.MoveAroundFixed,
		"script.ai.individual.ImperialGravekeeper":                                                 individual.ImperialGravekeeper,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAfflict":                                royalrush.Afflict,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAreaController":                         royalrush.AreaController,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushDefaultNpc":                             royalrush.DefaultNpc,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushKeyBox":                                 royalrush.KeyBox,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTrasureBox":                             royalrush.TreasureBox,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox1":    royalrush.TriggerBox1,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox2":    royalrush.TriggerBox2,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox3":    royalrush.TriggerBox3,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox4":    royalrush.TriggerBox4,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox5":    royalrush.TriggerBox5,
		"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBoxBase": royalrush.TriggerBoxBase,
		"script.ai.individual.RoyalRushMissionNpc":                                                 individual.RoyalRushMissionNpc,
		"script.ai.individual.RoyalRushNpc":                                                        individual.RoyalRushNpc,
		"script.ai.individual.SacrificialVictim":                                                   individual.SacrificialVictim,
		"script.ai.individual.TriolCamera":                                                         individual.TriolCamera,
		"script.ai.individual.WarriorPetForPC":                                                     individual.WarriorPetForPC,
	}
}
