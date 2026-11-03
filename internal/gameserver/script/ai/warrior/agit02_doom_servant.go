package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Agit02DoomServantAgit is Agit02DoomServantAgit: a clan hall siege
// servant, a WarriorCastDDMagicAggressive that never wanders. Every 60 s
// it may jump home when out of its territory and may shuffle its hate.
// It answers only the clan members that share its flag: it may cast its DD
// magic from where it stands at their attacker, and wants to attack it by
// the damage dealt.
func Agit02DoomServantAgit() script.Script {
	base := CastDDMagicAggressiveHooks()
	return script.Script{Dir: castDDMagicDir, Behavior: true, NPCs: []int32{35632, 35411}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("3001", e.NPC, nil, 1000*time.Millisecond, 60000*time.Millisecond)
			e.NPC.SetInt(script.SlotFlag, 0)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				n := e.NPC
				if !n.InMyTerritory() && s.Rnd(3) < 1 {
					n.TeleportTo(n.SpawnLocation())
					n.RemoveAllAttackDesire()
				}
				if s.Rnd(5) < 1 {
					n.RandomizeAttack()
				}
			}
			return base.Timer(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			sameFlag := called.Int(script.SlotFlag) == caller.Int(script.SlotFlag)
			if called.LifeTime() > 7 && attacker.Playable() && called.Intention() != script.IntentionAttack && sameFlag {
				ddMagic, ok := called.SkillByType(npc.SkillTypeDDMagic)
				if s.Rnd(100) < 10 && called.Distance2D(attacker) > 100 && ok {
					called.AddCastDesireHold(attacker, ddMagic, 1000000)
				}
			}
			if sameFlag && attacker.Playable() {
				called.AddAttackDesire(attacker, float64(e.Damage)/float64(called.MaxHP())/0.05*100)
			}
		},
	})}
}
