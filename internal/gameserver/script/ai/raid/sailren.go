package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Sailren is the raid boss of Sailren's nest. As it appears it runs maker
// script event 11041 on rune16_npc2017_01m1. Event 11042 brings it into
// its lair: it jumps there, plays social animation 2 18 s later, and from
// a minute later, every minute, reshuffles its hate one time in five and
// wants its self buff one time in five. Event 11055 makes it aggressive
// and wants it to attack, at 200, every member of the party of the player
// arg1 names that stands in its spawn territory within 3000. Every 5
// minutes, once it lived 7 AI cycles and stands in its territory, 600 s
// with no hit, spell or sighting send it home: it refreshes its hate, runs
// 11046 on rune16_npc2017_13m1, jumps back to its waiting point and runs
// 11043 on rune16_npc2017_01m1. Dying, it tells the NPCs within 8000 event
// 11054.
func Sailren() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossType1/Sailren", Behavior: true, NPCs: []int32{29065}, Hooks: sailrenHooks()}
}

// sailrenHooks are Sailren's hooks. Its timer, see-spell, script event and
// dying hooks call no parent.
func sailrenHooks() script.Hooks {
	base := BossType1Hooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI1, s.CurrentTick())
			n.SetInt(script.SlotIntAI2, 0)
			n.SetInt(script.SlotIntAI3, 0)
			s.Spawner().SendMakerEvent("rune16_npc2017_01m1", "11041", 0, 0)
			s.StartTimerAtFixedRate("3003", n, nil, 300000*time.Millisecond, 300000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1001"):
				if s.Rnd(5) < 1 {
					n.RandomizeAttack()
				}
				if s.Rnd(5) < 1 {
					castByType(n, n, npc.SkillTypeSelfBuffA)
				}
			case strings.EqualFold(e.Name, "3000"):
				n.AddSocialDesire(2, 3000, 50)
			case strings.EqualFold(e.Name, "3003"):
				if n.LifeTime() > 7 && n.InMyTerritory() {
					if s.ElapsedTicks(n.Int(script.SlotIntAI1)) > 600 {
						n.HateList().Refresh()
						s.Spawner().SendMakerEvent("rune16_npc2017_13m1", "11046", 0, 0)
						n.TeleportTo(script.Loc{X: -113091, Y: -243942, Z: -15536, Heading: 0})
						n.SetInt(script.SlotIntAI2, 0)
						n.SetInt(script.SlotIntAI3, 0)
						s.Spawner().SendMakerEvent("rune16_npc2017_01m1", "11043", 0, 0)
					}
				}
			}
			return ""
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			e.NPC.SetInt(script.SlotIntAI1, s.CurrentTick())
			base.Attacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			n.SetInt(script.SlotIntAI1, s.CurrentTick())
			if caster.Level() > n.Level()+8 {
				if s.AbnormalLevel(caster, raidMute()) <= 0 {
					n.AddCastDesire(caster, raidMute(), 1000000)
					return
				}
			}
			top := n.TopDesireTarget()
			// Integer division: any aggro short of the NPC's maximum HP
			// gives 0.
			if aggro := int32(s.Skill(e.Skill).AggroPoints()); top != nil && !script.Same(top, caster) && aggro > 0 && n.Intention() == script.IntentionAttack {
				n.AddAttackDesire(caster, float64(((aggro/n.MaxHP())*4000)*150))
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			n.SetInt(script.SlotIntAI1, s.CurrentTick())
			if n.Int(script.SlotIntAI3) == 1 {
				n.AddAttackDesire(e.Creature, 200)
			}
			base.SeeCreature(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case 11042:
				n.SetInt(script.SlotIntAI1, s.CurrentTick())
				n.TeleportTo(script.Loc{X: 27549, Y: -6638, Z: -2008, Heading: 0})
				s.StartTimer("3000", n, nil, 18000*time.Millisecond)
				s.StartTimerAtFixedRate("1001", n, nil, 60000*time.Millisecond, 60000*time.Millisecond)
			case 11055:
				n.SetInt(script.SlotIntAI3, 1)
				// arg1 names no player: the hook ends here.
				c0, ok := s.Object(e.Arg1).(*script.Player)
				if !ok {
					return
				}
				for _, member := range c0.PartyMembers() {
					if n.InSpawnTerritory(member) && n.Distance3D(member) < 3000 {
						n.AddAttackDesire(member, 200)
					}
				}
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			s.BroadcastScriptEvent(e.NPC, 11054, 0, 8000)
		},
	}).Named("Sailren")
}
