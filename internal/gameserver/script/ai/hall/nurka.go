package hall

import (
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
