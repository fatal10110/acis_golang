package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// Claims returns, per scripts.xml path of a behavior not ported yet, the
// (NPC, event) bindings it takes from the behaviors listed before it
// (script.Config.Claims): while it is not ported, an NPC whose every
// behavior binding it takes keeps its stand-ins, even once the earlier
// behavior is ported. Porting the behavior removes its entry.
func Claims() map[string]script.Bindings {
	const (
		anais   = 29096
		andreas = 29062
		manager = 35596
	)
	circle := func() script.Bindings {
		return script.Bindings{script.EventFirstTalk: {manager}, script.EventTalked: {manager}}
	}
	return map[string]script.Bindings{
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3AnaisAggressive": {
			script.EventAttacked: {anais}, script.EventAttackFinished: {anais}, script.EventCreated: {anais},
			script.EventPartyAttacked: {anais}, script.EventPartyDied: {anais}, script.EventSeeSpell: {anais},
		},
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4AndreasAggressive": {
			script.EventAttacked: {andreas}, script.EventAttackFinished: {andreas}, script.EventCreated: {andreas},
			script.EventPartyAttacked: {andreas}, script.EventPartyDied: {andreas}, script.EventSeeSpell: {andreas},
		},
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle2": circle(),
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle3": circle(),
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle4": circle(),
	}
}
