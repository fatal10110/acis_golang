package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
)

// BossStandard is RaidBossStandard, the raid boss every other raid boss
// derives from. It binds no NPC of its own.
func BossStandard() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossStandard", Behavior: true, Hooks: BossStandardHooks()}
}

// striderSlow is the skill a raid boss casts on a player riding a strider.
func striderSlow() skill.Ref { return skill.Ref{ID: 4258, Level: 1} }

// raidMute is the skill a raid boss casts on a caster more than 8 levels
// above it.
func raidMute() skill.Ref { return skill.Ref{ID: 4215, Level: 1} }

// bossHate is the attack weight a raid boss gives a hit of damage, damage
// at least 1, scaled down by its level.
func bossHate(n *script.NPC, damage int32) float64 {
	return ((1.0 * float64(damage)) / float64(n.Level()+7)) * 20000
}

// BossStandardHooks are RaidBossStandard's hooks, which the other raid
// bosses derive theirs from. A raid boss:
//   - appears playing its RaidSpawnMusic (Rm01_A by default) and checks
//     itself 1 s later and every minute after: out of its territory and out
//     of a fight, on an even draw, it drops its attack desires and goes back
//     to its spawn point; one time in five it reshuffles its hate;
//   - hit, slows a player riding a strider, and curses an attacker more
//     than 8 levels above it, forgetting its hate, instead of fighting it;
//     it wants to attack a playable attacker by 20000 per point of damage
//     over its level plus 7;
//   - failing to find its path more than 10 times when hit, jumps onto its
//     top desire target within 1000, else onto the attacker, which it then
//     wants alone; hit inside a peace zone, it drops its attack desires and
//     goes back to its spawn point;
//   - seeing a spell, mutes a caster more than 8 levels above it; while
//     fighting another, it wants to attack a caster of an aggro-raising
//     spell, and jumps onto its top desire target that casts while it is
//     hurt and lost.
func BossStandardHooks() script.Hooks {
	base := monster.Hooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				n := e.NPC
				if !n.InMyTerritory() && s.RndBool() && n.Intention() != script.IntentionAttack {
					// The teleport also clears its target.
					n.AbortAll()
					n.RemoveAllAttackDesire()
					n.TeleportTo(n.SpawnLocation())
				}
				if s.Rnd(5) < 1 {
					n.RandomizeAttack()
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			damage := e.Damage
			if p, ok := attacker.(*script.Player); ok && p.MountType() == 1 {
				if s.AbnormalLevel(attacker, striderSlow()) <= 0 {
					n.AddCastDesire(attacker, striderSlow(), 1000000)
				}
			}
			if attacker.Level() > n.Level()+8 {
				if s.AbnormalLevel(attacker, raidCurse()) == -1 {
					n.AddCastDesire(attacker, raidCurse(), 1000000)
					n.StopHate(attacker)
					return
				}
			}
			if attacker.Playable() {
				if damage == 0 {
					damage = 1
				}
				n.AddAttackDesire(attacker, bossHate(n, damage))
			}
			if n.GeoPathFailCount() > 10 {
				if top := n.TopDesireTarget(); top != nil && n.Distance2D(top) < 1000 {
					n.AbortAll()
					n.TeleportTo(top)
				} else {
					n.RemoveAllAttackDesire()
					if attacker.Playable() {
						if damage == 0 {
							damage = 1
						}
						n.AddAttackDesire(attacker, bossHate(n, damage))
					}
					n.AbortAll()
					n.TeleportTo(attacker)
				}
			}
			if n.InsideZone(script.ZonePeace) {
				n.AbortAll()
				n.RemoveAllAttackDesire()
				n.TeleportTo(n.SpawnLocation())
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			music, ok := n.StringParam("RaidSpawnMusic")
			if !ok {
				music = "Rm01_A"
			}
			n.PlayMusic(music)
			s.StartTimerAtFixedRate("1001", n, nil, 1000*time.Millisecond, 60000*time.Millisecond)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if caster.Level() > n.Level()+8 {
				if s.AbnormalLevel(caster, raidMute()) <= 0 {
					n.AddCastDesire(caster, raidMute(), 1000000)
					return
				}
			}
			top := n.TopDesireTarget()
			if top == nil {
				return
			}
			// Integer division: any aggro short of the NPC's maximum HP
			// gives 0.
			if aggro := int32(s.Skill(e.Skill).AggroPoints()); aggro > 0 && n.Intention() == script.IntentionAttack && !script.Same(top, caster) {
				n.AddAttackDesire(caster, float64(((aggro/n.MaxHP())*4000)*150))
			}
			if n.GeoPathFailCount() > 10 && script.Same(caster, top) && n.HP() != float64(n.MaxHP()) {
				n.AbortAll()
				n.TeleportTo(caster)
			}
		},
	}).Named("RaidBossStandard")
}
