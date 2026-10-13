package network

import (
	enchantflow "github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// characterRules are the rules settings give every character and summon.
func (l *GameClientLink) characterRules(settings *PlayerConfig) player.Rules {
	return player.Rules{
		RateKarmaExpLost:       settings.RateKarmaExpLost,
		RespawnRestoreHP:       settings.RespawnRestoreHP,
		WeightLimitMultiplier:  settings.WeightLimitMultiplier,
		InventorySlots:         settings.InventorySlots,
		StorageSlots:           settings.StorageSlots,
		PerfectShieldBlockRate: settings.PerfectShieldBlockRate,
		MaxBuffsAmount:         settings.MaxBuffsAmount,
		DeathPenaltyChance:     settings.DeathPenaltyChance,
		AllowDelevel:           settings.AllowDelevel,
		RaidCursesDisabled:     l.disableRaidCurse,
		AwardPKKillPVPPoint:    settings.AwardPKKillPVPPoint,
		SingleItemDrop:         !settings.MultipleItemDrop,
		DeathDrop:              settings.DeathDrop,
	}
}

// enchantRates are the scroll enchant rates and limits settings give, the
// shipped ones when it names none.
func enchantRates(settings *PlayerConfig) enchantflow.Config {
	if settings.Enchant != nil {
		return *settings.Enchant
	}
	return enchantflow.DefaultConfig()
}

// applyPlayerConfig puts settings in force in place of the ones before: the
// link's own reads, the rules of every character and summon, whether online
// or still to come, and the switches the enchant, skill, craft and
// multisell services read. Each group changes whole, so one action reads
// one group's settings from before or after, never a mix.
func (l *GameClientLink) applyPlayerConfig(settings *PlayerConfig) {
	l.playerSettings.Store(settings)
	l.playerRules.Set(l.characterRules(settings))
	l.enchanting.items.SetConfig(enchantRates(settings))
	l.skillHandlers.SetMagicFailures(settings.MagicFailures)
	l.craft.SetEnabled(!settings.CraftingDisabled)
	l.exchange.SetKeepMaintained(settings.KeepMaintainedIngredients)
}
