package manager

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

type deathRewards struct {
	hostile    *npc.Hostile
	tmpl       *npc.Template
	categories []item.DropCategory
	config     KillRewardConfig
	raid       bool
	decay      *task.Decay
	ids        idAllocator
	items      *item.Table
	ground     groundPlacer
}

type playerRewardEntry struct {
	actor  *player.Character
	damage float64
}

// CalculateRewards implements creature.Rewarder.
func (d *deathRewards) CalculateRewards(killer creature.DeathActor) {
	d.scheduleDecay()

	entries, totalDamage, maxDealer, highestLevel := d.rewardEntries()
	d.rollDrops(killer, maxDealer, highestLevel)
	d.grantExpAndSp(entries, totalDamage)
}

func (d *deathRewards) scheduleDecay() {
	if d.tmpl.CorpseTime <= 0 {
		return
	}
	interval := time.Duration(d.tmpl.CorpseTime) * time.Second
	if d.hostile.Spoiled() || d.hostile.Seeded() {
		interval *= 2
	}
	deadline := d.decay.Add(d.hostile, interval)
	d.hostile.SetCorpseDeadline(deadline)
}

func (d *deathRewards) rewardEntries() ([]playerRewardEntry, float64, *player.Character, int) {
	var entries []playerRewardEntry
	var totalDamage float64
	var maxDealer *player.Character
	var maxDamage float64
	var highestLevel int

	for _, threat := range d.hostile.AI().Threats().Snapshot() {
		if threat.Damage <= 1 {
			continue
		}
		attacker, ok := threat.Attacker.(*player.Character)
		if !ok || attacker.AlikeDead() || !attacker.Knows(d.hostile) {
			continue
		}
		entries = append(entries, playerRewardEntry{actor: attacker, damage: threat.Damage})
		totalDamage += threat.Damage
		if maxDealer == nil || threat.Damage > maxDamage {
			maxDealer = attacker
			maxDamage = threat.Damage
		}
		if attacker.CharLevel > highestLevel {
			highestLevel = attacker.CharLevel
		}
	}

	return entries, totalDamage, maxDealer, highestLevel
}

func (d *deathRewards) rollDrops(killer creature.DeathActor, maxDealer *player.Character, highestLevel int) {
	if len(d.categories) == 0 {
		return
	}
	x, y, z := d.hostile.Position()
	heading := d.hostile.Heading()

	levelMultiplier := 1.0
	if highestLevel > 0 {
		levelMultiplier = item.LevelPenaltyMultiplier(int32(highestLevel), int32(d.tmpl.Level), d.raid, d.config.DeepBlueDropRules)
	}
	autoLootItems := d.config.AutoLoot
	if d.raid {
		autoLootItems = d.config.AutoLootRaid
	}

	receiver := killer
	if maxDealer != nil {
		receiver = maxDealer
	}
	NewKillReward(d.categories, d.hostile.SpoilPool(), levelMultiplier, d.raid, d.config.Rates, autoLootItems, d.config.AutoLootHerbs, d.ids, d.items, d.ground, x, y, z, heading, d.hostile.ObjectID()).CalculateRewards(receiver)
}

func (d *deathRewards) grantExpAndSp(entries []playerRewardEntry, totalDamage float64) {
	if d.config.PlayerLevels == nil || totalDamage <= 0 {
		return
	}
	for _, entry := range entries {
		exp, sp := player.KillRewardExpAndSp(d.tmpl.RewardExp, d.tmpl.RewardSp, entry.damage, totalDamage, entry.actor.CharLevel-d.tmpl.Level)
		entry.actor.RewardExpAndSp(d.config.PlayerLevels, exp, sp)
	}
}
