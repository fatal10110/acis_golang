package orfen

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
)

// slotRetreated is 1 while Orfen rests at its retreat point, after a hit
// took it under half its HP, and 0 once it is back on one of its three
// lairs.
const slotRetreated = script.SlotIntAI0

// param reads one of Orfen's integer spawn parameters.
func param(n *script.NPC, name string) int { return int(n.IntParam(name)) }

// retreatPoint is where Orfen rests after a hit took it under half its HP.
func retreatPoint(n *script.NPC) script.Loc {
	return script.Loc{X: param(n, "b03_x1"), Y: param(n, "b03_y1"), Z: param(n, "b03_z1")}
}

// toLair teleports Orfen to one of its three lairs, each one time in three.
func toLair(s *script.Script, n *script.NPC) {
	i6 := s.Rnd(100)
	switch {
	case i6 < 33:
		n.TeleportTo(script.Loc{X: param(n, "b03_x2"), Y: param(n, "b03_y2"), Z: param(n, "b03_z2")})
	case i6 < 66:
		n.TeleportTo(script.Loc{X: param(n, "b03_x3"), Y: param(n, "b03_y3"), Z: param(n, "b03_z3")})
	default:
		n.TeleportTo(script.Loc{X: param(n, "b03_x4"), Y: param(n, "b03_y4"), Z: param(n, "b03_z4")})
	}
}

// bumpFlag counts one more move of Orfen in its flag.
func bumpFlag(n *script.NPC) {
	n.SetInt(script.SlotFlag, n.Int(script.SlotFlag)+1)
}

// Orfen is the grand boss Orfen. It appears with its music and checks
// every 10 s where it stands: resting at its retreat point, it goes back
// to one of its three lairs once over 95% of its HP, dropping its fight,
// and is brought back to the retreat point when it left its territory.
// Leaving its territory drops its fight and sends it to the retreat point
// while it rests, otherwise to a lair. A hit under half its HP sends it to
// rest at the retreat point, dropping its fight. A hit, or a spell it
// sees, from a player more than 8 levels above it makes it want a curse on
// that player, and a strider rider free of its slow gets the slow. A
// player or summon hitting it from 300 to 1000 away is pulled to it one
// time in 10, named in a line and wanted paralyzed; otherwise it may want
// its area paralysis. A spell adding hate that it sees cast from within
// 1000 pulls the caster to it one time in 5 the same way. It hates by the damage it takes
// and by the hits on its party and clan. Dying, it plays its death music.
func Orfen() script.Script {
	base := warrior.PartyLeaderWarriorAggressiveHooks()
	return script.Script{Dir: "ai/boss/orfen", Behavior: true, NPCs: []int32{29014}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.PlayMusic("BS01_A")

			n.SetInt(script.SlotFlag, 0)
			n.SetInt(slotRetreated, 0)

			s.StartTimerAtFixedRate("3001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)

			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				n := e.NPC
				if n.Int(slotRetreated) == 1 {
					if n.HPRatio() > 0.95 {
						toLair(s, n)

						n.SetInt(slotRetreated, 0)
						bumpFlag(n)

						n.RemoveAllAttackDesire()
					} else if !n.InMyTerritory() {
						n.TeleportTo(retreatPoint(n))
					}
				}
			}
			return base.Timer(s, e)
		},
		OnOutOfTerritory: func(s *script.Script, e script.OutOfTerritory) {
			n := e.NPC
			n.RemoveAllAttackDesire()

			switch n.Int(slotRetreated) {
			case 1:
				n.TeleportTo(retreatPoint(n))
			case 0:
				toLair(s, n)
			}

			bumpFlag(n)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Level() > n.Level()+8 {
				n.AddCastDesire(attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}

			hinderStrider := skill.Ref{ID: 4258, Level: 1}
			if script.Riding(attacker) && s.AbnormalLevel(attacker, hinderStrider) <= 0 {
				n.AddCastDesire(attacker, hinderStrider, 1000000)
			}

			if n.Int(slotRetreated) == 0 && n.HPRatio() < 0.5 {
				n.TeleportTo(retreatPoint(n))

				n.SetInt(slotRetreated, 1)
				bumpFlag(n)

				n.RemoveAllAttackDesire()
			}

			if attacker.Playable() {
				if n.Distance2D(attacker) > 300 && s.Rnd(100) < 10 && n.Distance2D(attacker) < 1000 {
					i0 := s.Rnd(100)
					switch {
					case i0 < 33:
						n.Say(script.NpcString(1000028, attacker.Name()))
					case i0 < 66:
						n.Say(script.NpcString(1000029, attacker.Name()))
					default:
						n.Say(script.NpcString(1000030, attacker.Name()))
					}

					script.TeleportTo(attacker, n)

					n.AddCastDesire(attacker, skill.Ref{ID: 4063, Level: 1}, 1000000)
				} else if s.Rnd(100) < 20 {
					n.AddCastDesire(n, skill.Ref{ID: 4064, Level: 1}, 1000000)
				}

				n.AddAttackDesire(attacker, float64(commons.TruncInt32Sat(float64(e.Damage)/float64(n.MaxMP())/0.05*10000)))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if n.Dead() {
				return
			}

			if caster.Level() > n.Level()+8 {
				n.AddCastDesire(caster, skill.Ref{ID: 4215, Level: 1}, 1000000)
				return
			}

			if s.Skill(e.Skill).AggroPoints() > 0 && s.Rnd(100) < 20 && n.Distance2D(caster) < 1000 {
				i0 := s.Rnd(100)
				switch {
				case i0 < 25:
					n.Say(script.NpcString(1000028, caster.Name()))
				case i0 < 50:
					n.Say(script.NpcString(1000029, caster.Name()))
				case i0 < 75:
					n.Say(script.NpcString(1000030, caster.Name()))
				default:
					n.Say(script.NpcString(1000031, caster.Name()))
				}

				caster.TeleportTo(n, 0)

				n.AddCastDesire(caster, skill.Ref{ID: 4063, Level: 1}, 1000000)
			}

			base.SeeSpell(s, e)
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if !script.Same(caller, called) && target.Playable() {
				damage := float64(e.Damage)
				called.AddAttackDesire(target, float64(commons.TruncInt32Sat(damage/float64(called.MaxMP())/0.05*damage*float64(caller.Int(script.SlotWeightPoint))*1000)))
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() {
				called.AddAttackDesire(attacker, float64(commons.TruncInt32Sat(float64(e.Damage)/float64(called.MaxMP())/0.05*3000)))
			}
		},
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			e.NPC.PlayMusic("BS02_D")
		},
	})}
}
