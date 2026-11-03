package raid

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// BossParty is RaidBossParty: a RaidBossStandard that leads a party. It
// binds no NPC of its own.
func BossParty() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossParty", Behavior: true, Hooks: bossPartyHooks()}
}

// partyBossHate is the attack weight a party raid boss gives a hit of
// damage on its party, damage at least 1: 20000 per point over its level
// plus 7, truncated toward zero within the int32 range.
func partyBossHate(n *script.NPC, damage int32) float64 {
	if damage == 0 {
		damage = 1
	}
	return float64(commons.TruncInt32Sat(((1.0 * float64(damage)) / float64(n.Level()+7)) * 20000))
}

// bossPartyHooks are RaidBossParty's hooks, which its children derive
// theirs from. A party raid boss:
//   - appears with a weight point of 10, a master from then on, and
//     creates its privates;
//   - on a hit on one of its privates by a playable at most 8 levels above
//     it, wants to attack the playable (see partyBossHate); failing to find
//     its path more than 8 to 20 times, it jumps onto its top desire target
//     within 1000, else onto the attacker, which it then wants alone;
//   - on any hit on its party while inside a peace zone, goes back to its
//     spawn point and drops its attack desires;
//   - while it lives, brings each of its dying privates back after its
//     spawn's respawn delay.
//
// Its party-attacked and party-died hooks call no parent.
func bossPartyHooks() script.Hooks {
	base := BossStandardHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotWeightPoint, 10)
			// Reading the privates makes the NPC a master; creating them
			// forgets the ones it had.
			n.Minions()
			defaultnpc.CreatePrivates(s, n)
			base.Created(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if !script.Same(caller, called) {
				if target.Level() <= called.Level()+8 {
					if target.Playable() {
						called.AddAttackDesire(target, partyBossHate(called, e.Damage))
					}
				}
				if called.GeoPathFailCount() > 8+s.Rnd(13) {
					if top := called.TopDesireTarget(); top != nil && called.Distance2D(top) < 1000 {
						called.AbortAll()
						called.InstantTeleportTo(top)
					} else {
						called.RemoveAllAttackDesire()
						if target.Playable() {
							called.AddAttackDesire(target, partyBossHate(called, e.Damage))
						}
						called.InstantTeleportTo(target)
					}
				}
			}
			if called.InsideZone(script.ZonePeace) {
				called.AbortAll()
				called.InstantTeleportTo(called.SpawnLocation())
				called.RemoveAllAttackDesire()
			}
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if !script.Same(caller, called) && called.IsMaster() && !called.Dead() {
				s.Spawner().ScheduleRespawn(caller, time.Duration(s.Spawner().RespawnDelay(caller))*1000*time.Millisecond)
			}
		},
	}).Named("RaidBossParty")
}

// BossType2 is RaidBossType2: a RaidBossParty that, one time in 150, wants
// its ranged DD magic at 1000000 on a playable hitting its party and on a
// caster it sees.
func BossType2() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossParty/RaidBossType2", Behavior: true, NPCs: bossType2IDs(), Hooks: bossType2Hooks()}
}

// bossType2IDs are RaidBossType2's own NPCs.
func bossType2IDs() []int32 {
	return []int32{
		25286, 25524, 25339, 25202, 25051, 25312, 25007, 25016, 25020, 25029, 25044, 25073, 25079, 25099,
		25134, 25143, 25159, 25166, 25173, 25182, 25226, 25235, 25238, 25277, 25357, 25366, 25375, 25395,
		25431, 25447, 25456, 25463, 25481, 29030,
	}
}

// bossType2Hooks are RaidBossType2's hooks.
func bossType2Hooks() script.Hooks {
	base := bossPartyHooks()
	return base.With(script.Hooks{
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if e.Target.Playable() && s.Rnd(150) < 1 {
				castByType(e.Called, e.Target, npc.SkillTypeRangeDDMagicA)
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if s.Rnd(150) < 1 {
				castByType(e.NPC, e.Caster, npc.SkillTypeRangeDDMagicA)
			}
			base.SeeSpell(s, e)
		},
	}).Named("RaidBossType2")
}
