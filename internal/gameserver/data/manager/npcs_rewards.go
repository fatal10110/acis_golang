package manager

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type deathRewards struct {
	hostile    *npc.Hostile
	state      *world.State
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
	actor       *player.Character
	damage      float64
	ownerDamage float64
	pet         *summon.Actor
	petDamage   float64
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

	for _, threat := range d.hostile.AI().Threats().Snapshot() {
		if threat.Damage <= 1 {
			continue
		}
		attacker, ok := threat.Attacker.(*player.Character)
		var pet *summon.Actor
		if !ok {
			pet, ok = threat.Attacker.(*summon.Actor)
			if !ok || !pet.IsPet() {
				continue
			}
			attacker, ok = pet.ActingPlayer().(*player.Character)
		}
		if !ok || attacker.AlikeDead() || !attacker.Knows(d.hostile) {
			continue
		}
		entry := -1
		for i := range entries {
			if entries[i].actor == attacker {
				entry = i
				break
			}
		}
		if entry < 0 {
			entries = append(entries, playerRewardEntry{actor: attacker})
			entry = len(entries) - 1
		}
		entries[entry].damage += threat.Damage
		if pet != nil {
			entries[entry].pet = pet
			entries[entry].petDamage += threat.Damage
		} else {
			entries[entry].ownerDamage += threat.Damage
		}
		totalDamage += threat.Damage
	}

	maxDealer, highestLevel := rewardLeader(entries)
	return entries, totalDamage, maxDealer, highestLevel
}

func rewardLeader(entries []playerRewardEntry) (*player.Character, int) {
	var maxDealer *player.Character
	var maxDamage float64
	var highestLevel int
	for _, entry := range entries {
		if maxDealer == nil || entry.damage > maxDamage {
			maxDealer = entry.actor
			maxDamage = entry.damage
		}
		if entry.actor.CharLevel > highestLevel {
			highestLevel = entry.actor.CharLevel
		}
	}
	return maxDealer, highestLevel
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
		if entry.pet == nil && d.state != nil {
			if obj, ok := d.state.Summon(entry.actor.ObjectID()); ok {
				entry.pet, _ = obj.(*summon.Actor)
			}
		}
		if entry.pet != nil && entry.pet.CanReceiveKillReward(d.config.PartyRange) {
			petExp, petSp := petReward(entry.pet.ExpType(), entry.petDamage, entry.ownerDamage, exp, sp)
			exp -= petExp
			sp -= petSp
			entry.pet.AddExpAndSp(petExp, petSp)
		}
		entry.actor.RewardExpAndSp(d.config.PlayerLevels, exp, sp)
	}
}

func petReward(expType int, petDamage, totalDamage float64, exp int64, sp int) (int64, int) {
	if expType == -1 {
		if totalDamage <= 0 {
			return 0, 0
		}
		share := petDamage / totalDamage
		return int64(float64(exp) * share), int(float64(sp) * share)
	}
	if expType > 100 {
		expType = 100
	}
	share := 1 - float64(expType)/100
	return int64(math.Round(float64(exp) * share)), int(math.Round(float64(sp) * share))
}
