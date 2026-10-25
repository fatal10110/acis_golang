package hall

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// nurkaDir is the page directory of Bloody Lord Nurka.
const nurkaDir = "ai/siegeablehall/BloodyLordNurka1"

// Bloody Lord Nurka's skills: the debuff he casts on a much higher level
// attacker, and his magic strike.
const (
	nurkaDebuff  = skill.ID(5456)
	nurkaDDMagic = skill.ID(4042)
)

// nurka1Hooks are BloodyLordNurka1's hooks, which Nurka's second form
// derives its own from. As he appears he sends every player in his
// territory to 51952, 111060, -1970, up to 200 away. He debuffs and stops
// hating an attacker more than 8 levels above him that does not hold his
// debuff, and fights back the playables that hit him or a clan mate,
// sometimes with his magic strike, which he also casts at some of the
// players whose spells he sees.
func nurka1Hooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.TeleportPlayersInTerritory(e.NPC, script.Loc{X: 51952, Y: 111060, Z: -1970}, 200)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if e.Attacker.Level() > n.Level()+8 && s.AbnormalLevel(e.Attacker, level1(nurkaDebuff)) == -1 {
				n.AddCastDesireHold(e.Attacker, level1(nurkaDebuff), 1000000)
				n.StopHate(e.Attacker)
			}
			if e.Attacker.Playable() {
				if s.Rnd(100) < 10 {
					n.AddCastDesireHold(e.Attacker, level1(nurkaDDMagic), 1000000)
				}
				n.AddAttackDesireHold(e.Attacker, damageWeight(n, e.Damage, 10000))
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() {
				if s.Rnd(100) < 2 {
					e.Called.AddCastDesireHold(e.Attacker, level1(nurkaDDMagic), 1000000)
				}
				e.Called.AddAttackDesireHold(e.Attacker, damageWeight(e.Called, e.Damage, 5000))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if s.Skill(e.Skill).AggroPoints() > 0 && s.Rnd(100) < 15 {
				e.NPC.AddCastDesireHold(e.Caster, level1(nurkaDDMagic), 1000000)
			}
		},
	}).Named("BloodyLordNurka1")
}

// Nurka1 is BloodyLordNurka1: Bloody Lord Nurka, the Partisan Hideaway's
// lord.
func Nurka1() script.Script {
	return script.Script{Dir: nurkaDir, Behavior: true, NPCs: []int32{35368}, Hooks: nurka1Hooks()}
}

// Nurka2 is BloodyLordNurka2: Bloody Lord Nurka as a hall siege raises
// him, with his escort. As he appears he sends every player in his
// territory to 51952, 111060, -1970, up to 200 away, clears his flag and
// raises his escort. Every 10 seconds he reads how long his hall's siege
// date is away: from an hour on he walks once to 44525, 108867, -2020 and
// sets his flag to 2; from half an hour to an hour, while his flag is
// still 0, he walks to 45802, 109981, -1970 and sets it to 1. His escort
// follows the flag. A dead private of his comes back once its own respawn
// delay has passed.
func Nurka2() script.Script {
	base := nurka1Hooks()
	return script.Script{Dir: nurkaDir, Behavior: true, NPCs: []int32{35375}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.TeleportPlayersInTerritory(n, script.Loc{X: 51952, Y: 111060, Z: -1970}, 200)
			s.StartTimerAtFixedRate("2001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			n.SetInt(script.SlotFlag, 0)
			defaultnpc.CreatePrivates(s, n)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				h := s.HallOf(n)
				if h == nil {
					// A Nurka serving no hall has no siege date to read:
					// the check fails there, with no walk and no parent
					// timer hook.
					return ""
				}
				untilSiege := (h.SiegeDate().UnixMilli() - s.Now().UnixMilli()) / 1000
				if untilSiege >= 1800 {
					if untilSiege < 3600 {
						if n.Int(script.SlotFlag) < 1 {
							n.AddMoveToDesire(script.Loc{X: 45802, Y: 109981, Z: -1970}, 100000000)
							n.SetInt(script.SlotFlag, 1)
						}
					} else if n.Int(script.SlotFlag) < 2 {
						n.AddMoveToDesire(script.Loc{X: 44525, Y: 108867, Z: -2020}, 100000000)
						n.SetInt(script.SlotFlag, 2)
					}
				}
			}
			return base.Timer(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			if !script.Same(e.Caller, e.Called) {
				s.Spawner().ScheduleRespawn(e.Caller, time.Duration(s.Spawner().RespawnDelay(e.Caller)*1000)*time.Millisecond)
			}
		},
	})}
}
