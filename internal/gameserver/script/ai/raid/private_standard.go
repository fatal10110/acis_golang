// Package raid holds the raid behaviors: RaidPrivateStandard, the private a
// raid boss leads, which every other raid private derives from.
package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
)

// PrivateStandard is RaidPrivateStandard: a raid boss's private. It lives
// only while its master does: it leaves the world when it is created with
// no live master and when its master dies. It follows its live master,
// returns to it every minute when it strays 500 or more away out of a
// fight, and fights for its party, cursing any creature more than 8 levels
// above it that it fights.
func PrivateStandard() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidPrivate/RaidPrivateStandard", Behavior: true, NPCs: privateStandardIDs(), Hooks: PrivateStandardHooks()}
}

// privateStandardIDs are RaidPrivateStandard's own NPCs.
func privateStandardIDs() []int32 {
	return []int32{
		25006, 25009, 25012, 25015, 25018, 25022, 25025, 25028, 25031, 25037, 25040, 25043, 25046, 25049,
		25053, 25056, 25059, 25066, 25069, 25075, 25078, 25081, 25087, 25094, 25101, 25105, 25108, 25117,
		25124, 25133, 25136, 25142, 25145, 25148, 25151, 25154, 25161, 25168, 25172, 25175, 25178, 25184,
		25187, 25194, 25204, 25207, 25213, 25219, 25228, 25232, 25237, 25240, 25247, 25258, 25262, 25267,
		25271, 25279, 25288, 25289, 25295, 25301, 25308, 25311, 25314, 25318, 25327, 25341, 25355, 25363,
		25371, 25382, 25387, 25390, 25414, 25424, 25433, 25442, 25445, 25448, 25457, 25459, 25468, 25476,
		25492, 25495, 25500, 29032, 29035, 29042, 29058, 25503, 25511, 25526, 29097,
	}
}

// raidCurse is the skill a raid private casts on a creature more than 8
// levels above it.
func raidCurse() skill.Ref { return skill.Ref{ID: 4515, Level: 1} }

// partyHate is the attack weight a raid private gives a hit of damage on
// its party: the damage, 1 for none, scaled down by its level.
func partyHate(called *script.NPC, damage int32) float64 {
	if damage == 0 {
		damage = 1
	}
	return float64(commons.TruncInt32Sat(((1.0 * float64(damage)) / float64(called.Level()+7)) * 20000))
}

// PrivateStandardHooks are RaidPrivateStandard's hooks, which the other
// raid privates derive theirs from.
func PrivateStandardHooks() script.Hooks {
	base := monster.Hooks()
	return base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.HasMaster() && !n.Master().Dead() {
				n.AddFollowDesire(n.Master(), 5)
			} else {
				n.AddWanderDesire(10, 5)
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if !n.HasMaster() || n.Master().Dead() {
				n.DeleteMe()
				return
			}
			s.StartTimer("1001", n, nil, 2000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				n := e.NPC
				master := n.Master()
				if n.Distance2D(master) > 500 && !master.Dead() && n.Intention() != script.IntentionAttack {
					n.AbortAll()
					n.InstantTeleportTo(master)
				}
				if s.Rnd(3) < 1 {
					n.RandomizeAttack()
				}
				if master.Dead() {
					n.DeleteMe()
					return ""
				}
				s.StartTimerAtFixedRate("1001", n, e.Player, 60*1000*time.Millisecond, 60*1000*time.Millisecond)
			}
			return base.Timer(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			damage := e.Damage
			i0 := 0
			if master := called.Master(); master != nil && target.Level() <= master.Level()+8 {
				if target.Playable() {
					called.AddAttackDesire(target, partyHate(called, damage))
				}
			}
			if script.Same(caller, called) {
				i0 = 10
				if target.Level() > called.Level()+8 && s.AbnormalLevel(target, raidCurse()) == -1 {
					called.AddCastDesire(target, raidCurse(), 1000000)
					called.StopHate(target)
				}
			} else {
				i0 = 8 + s.Rnd(13)
			}
			if called.GeoPathFailCount() > i0 {
				if top := called.TopDesireTarget(); top != nil && called.Distance2D(top) < 1000 {
					called.AbortAll()
					called.InstantTeleportTo(top)
				} else {
					called.RemoveAllAttackDesire()
					if target.Playable() {
						called.AddAttackDesire(target, partyHate(called, damage))
					}
					called.AbortAll()
					called.InstantTeleportTo(target)
				}
			}
			if called.InsideZone(script.ZonePeace) {
				called.AbortAll()
				called.InstantTeleportTo(called.SpawnLocation())
				called.RemoveAllAttackDesire()
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if caster.Level() > n.Level()+8 && s.AbnormalLevel(caster, raidCurse()) <= 0 {
				n.AddCastDesire(caster, raidCurse(), 1000000)
				return
			}
			top := n.TopDesireTarget()
			if top == nil {
				return
			}
			// Integer division: any aggro
			// short of the NPC's maximum HP gives 0.
			if aggro := int32(s.Skill(e.Skill).AggroPoints()); aggro > 0 && n.Intention() == script.IntentionAttack && !script.Same(top, caster) {
				n.AddAttackDesire(caster, float64(((aggro/n.MaxHP())*4000)*150))
			}
			if n.GeoPathFailCount() > 10 && script.Same(caster, top) && n.HP() != float64(n.MaxHP()) {
				n.AbortAll()
				n.InstantTeleportTo(caster)
			}
		},
		OnPartyDied: func(_ *script.Script, e script.PartyDied) {
			if master := e.Called.Master(); master != nil && script.Same(e.Caller, master) {
				e.Called.DeleteMe()
			}
		},
	}).Named("RaidPrivateStandard")
}
