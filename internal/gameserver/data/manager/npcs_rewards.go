package manager

import (
	"math"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
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
	geo        move.Geo
}

// playerRewardEntry is one acting player's reward share: its own damage
// plus every summon's it controls.
type playerRewardEntry struct {
	actor  *player.Character
	damage float64
}

// CalculateRewards implements creature.Rewarder.
func (d *deathRewards) CalculateRewards(killer attackable.Combatant) {
	d.scheduleDecay()

	// A corpse nobody fought pays nothing: no loot, no spoil, no exp.
	if d.hostile.AI().Threats().IsEmpty() {
		return
	}
	threats := d.hostile.AI().Threats().Snapshot()
	entries, summonDamage, totalDamage, maxDealer := d.rewardEntries(threats)
	// A top dealer who logged out forfeits the drops to the killer.
	if maxDealer != nil && maxDealer.SessionDetached() {
		maxDealer = nil
	}
	if receiver := dropReceiver(killer, maxDealer); receiver != nil {
		d.rollDrops(receiver, d.hostile.HighestAttackerLevel(receiver.Level()))
	}
	d.grantExpAndSp(entries, summonDamage, totalDamage)
}

// dropReceiver returns the player credited with the drops: the top damage
// dealer, else the killer's acting player (a summon's owner). Nil means no
// player earned the kill — an NPC, the victim itself, or no killer at all —
// and nothing drops.
func dropReceiver(killer attackable.Combatant, maxDealer *player.Character) attackable.Combatant {
	if maxDealer != nil {
		return maxDealer
	}
	if killer == nil {
		return nil
	}
	switch killer.Kind() {
	case actor.KindPlayer:
		return killer
	case actor.KindSummon:
		if owner, ok := killer.Owner(); ok {
			return owner
		}
	}
	return nil
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

// rewardEntries credits each playable attacker's damage to its acting
// player; a summon's damage is also kept under the summon's id for its
// owner's pet share. The top dealer is the first player whose combined
// damage exceeds every earlier total.
func (d *deathRewards) rewardEntries(threats []attackable.Threat) ([]playerRewardEntry, map[int32]float64, float64, *player.Character) {
	var entries []playerRewardEntry
	summonDamage := map[int32]float64{}
	var totalDamage, maxDamage float64
	var maxDealer *player.Character

	for _, threat := range threats {
		// An attacker the victim no longer knows (logged out, unsummoned,
		// gone out of sight) has left the fight; its entry only lingers
		// until the next AI refresh.
		if threat.Damage <= 1 || !d.hostile.Knows(threat.Attacker) || !d.inPartyRange(threat.Attacker) {
			continue
		}
		attacker, ok := actingCharacter(threat.Attacker)
		if !ok {
			continue
		}
		totalDamage += threat.Damage
		if threat.Attacker.Kind() == actor.KindSummon {
			summonDamage[threat.Attacker.ObjectID()] += threat.Damage
		}
		entry := slices.IndexFunc(entries, func(e playerRewardEntry) bool { return e.actor.ObjectID() == attacker.ObjectID() })
		if entry < 0 {
			entries = append(entries, playerRewardEntry{actor: attacker})
			entry = len(entries) - 1
		}
		entries[entry].damage += threat.Damage
		if entries[entry].damage > maxDamage {
			maxDealer = attacker
			maxDamage = entries[entry].damage
		}
	}
	return entries, summonDamage, totalDamage, maxDealer
}

// actingCharacter resolves a player or summon attacker to its acting
// player's model. Owners and online players reach here as network wrappers
// around *player.Character, so the model is reached through its promoted
// accessor instead of a concrete type assertion.
func actingCharacter(c attackable.Combatant) (*player.Character, bool) {
	switch c.Kind() {
	case actor.KindPlayer:
	case actor.KindSummon:
		owner, ok := c.Owner()
		if !ok {
			return nil, false
		}
		c = owner
	default:
		return nil, false
	}
	holder, ok := c.(interface{ PlayerCharacter() *player.Character })
	if !ok {
		return nil, false
	}
	return holder.PlayerCharacter(), true
}

// inPartyRange reports whether c is within the party range of the victim,
// body to body in 3D. A range of -1 is unlimited.
func (d *deathRewards) inPartyRange(c attackable.Combatant) bool {
	if d.config.PartyRange == -1 {
		return true
	}
	hx, hy, hz := d.hostile.Position()
	cx, cy, cz := c.Position()
	dx, dy, dz := int64(hx-cx), int64(hy-cy), int64(hz-cz)
	reach := float64(d.config.PartyRange) + d.hostile.CollisionRadius() + c.CollisionRadius()
	return float64(dx*dx+dy*dy+dz*dz) <= reach*reach
}

func (d *deathRewards) rollDrops(receiver attackable.Combatant, attackerLevel int) {
	if len(d.categories) == 0 {
		return
	}
	x, y, z := d.hostile.Position()
	heading := d.hostile.Heading()

	levelMultiplier := item.LevelPenaltyMultiplier(int32(attackerLevel), int32(d.tmpl.Level), d.raid, d.config.DeepBlueDropRules)
	autoLootItems := d.config.AutoLoot
	if d.raid {
		autoLootItems = d.config.AutoLootRaid
	}

	NewKillReward(d.categories, d.hostile.SpoilPool(), levelMultiplier, d.raid, d.config.Rates, autoLootItems, d.config.AutoLootHerbs, d.ids, d.items, d.ground, d.geo, x, y, z, heading, d.hostile.ObjectID()).CalculateRewards(receiver)
}

func (d *deathRewards) grantExpAndSp(entries []playerRewardEntry, summonDamage map[int32]float64, totalDamage float64) {
	if d.config.PlayerLevels == nil || totalDamage <= 0 {
		return
	}
	for _, entry := range entries {
		// Only real death forfeits the exp; Fake Death keeps it.
		if entry.actor.Dead() || !entry.actor.Knows(d.hostile) {
			continue
		}
		var own *summon.Actor
		if d.state != nil {
			if obj, ok := d.state.Summon(entry.actor.ObjectID()); ok {
				own, _ = obj.(*summon.Actor)
			}
		}
		exp, sp := player.KillRewardExpAndSp(d.tmpl.RewardExp, d.tmpl.RewardSp, entry.damage, totalDamage, entry.actor.Level()-d.tmpl.Level)
		var penalty float32
		if own != nil && !own.IsPet() {
			penalty = own.ExpPenalty()
		}
		// Scaled in single precision even without a servitor, so a large
		// reward loses its low bits the same way.
		exp = int64(float32(float32(exp) * (1 - penalty)))
		if d.hostile.OverhitValid(entry.actor) {
			entry.actor.NotifyOverHit()
			exp += d.hostile.OverhitBonus(exp)
		}
		if own != nil && own.CanReceiveKillReward(d.config.PartyRange) {
			petExp, petSp := petReward(own.ExpType(), summonDamage[own.ObjectID()], entry.damage, exp, sp)
			exp -= petExp
			sp -= petSp
			own.AddExpAndSp(petExp, petSp)
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
