package baium

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The NPC's script memory as an archangel uses it.
const (
	slotIndex        = script.SlotIntAI0    // which of Baium's five it is, 1 to 5
	slotOnMaster     = script.SlotIntAI1    // 1 while it turns on Baium
	slotLastDamage   = script.SlotIntAI2    // damage of the last player hit
	slotIdleChecks   = script.SlotIntAI3    // checks spent out of a fight
	slotLastAttacker = script.SlotCreature2 // the player that hit it last
)

// Archangel is one of the five archangels Baium calls. Every 5 to 9 s it
// reads the mode Baium's damage balance left it in: in mode 0 every
// archangel fights the players, the last one that hit it first; in modes 1
// to 4 two or three of the five, by their index, turn on Baium while the
// rest fight the players, and each also wants to hit Baium. Every 5 s it
// leaves the world once Baium is gone, dead or back asleep, and otherwise
// keeps after Baium or looks around. Hit, it may cast its skills at the
// attacker, or at Baium while it turns on it.
func Archangel() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29021}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(slotIndex, n.Int(script.SlotParam1))
			n.SetInt(slotOnMaster, 1)
			n.SetInt(slotLastDamage, 100)
			n.SetInt(slotIdleChecks, 0)

			s.StartTimer("2001", n, nil, 5000*time.Millisecond)
			s.StartTimer("2002", n, nil, 5000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch e.Name {
			case "2001":
				s.StartTimer("2002", n, nil, 5000*time.Millisecond)
				s.StartTimer("2001", n, nil, time.Duration(5000+(s.Rnd(5)*1000))*time.Millisecond)

				master := n.Master()
				if master == nil {
					return base.Timer(s, e)
				}

				topDesireTarget := n.TopDesireTarget()

				if master.Int(script.SlotFlag) == 0 {
					if _, ok := topDesireTarget.(*script.Player); !ok {
						n.StopHate(master)
						n.LookNeighbor()
					}

					n.SetInt(slotOnMaster, 0)

					if c := n.Slot(slotLastAttacker); c != nil && !c.Dead() {
						n.AddAttackDesire(c, (float64(n.Int(slotLastDamage)/n.MaxHP())/0.05)*100)
					}
				} else {
					n.SetInt(slotOnMaster, 0)

					flag, index := master.Int(script.SlotFlag), n.Int(slotIndex)
					switch {
					case flag == 1 && (index == 1 || index == 2):
						n.SetInt(slotOnMaster, 1)
					case flag == 2 && (index == 3 || index == 4):
						n.SetInt(slotOnMaster, 1)
					case flag == 3 && (index == 1 || index == 3):
						n.SetInt(slotOnMaster, 1)
					case flag == 4 && (index == 2 || index == 4 || index == 5):
						n.SetInt(slotOnMaster, 1)
					}

					if n.Int(slotOnMaster) == 0 {
						if topDesireTarget != nil {
							if !topDesireTarget.Playable() {
								n.StopHate(master)
							}

							if c := n.Slot(slotLastAttacker); c != nil {
								n.AddAttackDesire(c, ((1.0*float64(n.Int(slotLastDamage)))/float64(n.Level()+7))*100)
							}
						}
					} else if topDesireTarget != nil && topDesireTarget.Playable() {
						n.RemoveAllAttackDesire()
					}

					n.AddAttackDesire(master, 10000)
				}
			case "2002":
				master := n.Master()
				if master == nil || master.Dead() || master.Int(script.SlotFlag) == 6 {
					n.DeleteMe()
				} else {
					s.StartTimer("2002", n, nil, 5000*time.Millisecond)

					switch {
					case n.Int(slotOnMaster) == 1:
						n.AddAttackDesire(master, 10000)
					case n.Int(slotIdleChecks) == 3:
						n.SetInt(slotIdleChecks, 0)
						n.AddAttackDesire(master, 1000)
					case n.Intention() != script.IntentionAttack:
						n.SetInt(slotIdleChecks, n.Int(slotIdleChecks)+1)
						n.LookNeighbor()
					}
				}
			}

			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			var c0 script.Creature

			if _, ok := attacker.(*script.Player); ok {
				n.SetSlot(slotLastAttacker, attacker)
				n.SetInt(slotLastDamage, e.Damage)
			}

			if n.Int(slotOnMaster) == 0 {
				c0 = attacker
				if attacker.Playable() {
					damage := e.Damage
					if damage == 0 {
						damage = 1
					}

					n.AddAttackDesire(attacker, ((1.0*float64(damage))/float64(n.Level()+7))*100)
				}
			} else if master := n.Master(); master != nil {
				c0 = master
			}

			if s.Rnd(100) < 10 {
				n.AddCastDesire(c0, skill.Ref{ID: 4132, Level: 1}, 1000000)
			}

			if s.Rnd(100) < 5 && n.HPRatio() < 0.5 {
				n.AddCastDesire(c0, skill.Ref{ID: 4133, Level: 1}, 1000000)
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() || c.Dead() || !n.InSpawnTerritory(c) {
				return
			}

			if n.Int(slotOnMaster) == 0 {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget == nil || !topDesireTarget.Playable() {
					if master := n.Master(); master != nil {
						n.StopHate(master)
					}
				}

				n.AddAttackDesire(c, 200)
			}

			if last := n.Slot(slotLastAttacker); last == nil || last.Dead() {
				n.SetSlot(slotLastAttacker, c)
				n.SetInt(slotLastDamage, 100)
			}
		},
	})}
}
