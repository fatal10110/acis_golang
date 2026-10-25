package baium

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// dir is the directory the Baium scripts are listed under.
const dir = "ai/boss/baium"

// memoKey is the global memo key under which Baium leaves its object id
// while it lives.
const memoKey = "2"

// The NPCs Baium creates: its archangels, the stone it sleeps as, and the
// teleport cube it leaves behind.
const (
	archangelID = 29021
	stoneID     = 29025
	cubeID      = 31842
)

// The NPC's script memory as Baium uses it.
const (
	slotDamage      = script.SlotIntAI0 // damage taken from players since the last check
	slotPartyDamage = script.SlotIntAI1 // damage its party took since the last check
	slotLastHit     = script.SlotIntAI2 // tick of the last player hit
	slotAwake       = script.SlotIntAI3 // 1 once its archangels are out
	slotFighting    = npc.IntQuest3     // 1 from its wake until its reset
	slotWaker       = script.SlotParam1 // object id of the player that woke it
	slotFirstSeen   = script.SlotCreature1
	slotNonPlayer   = script.SlotCreature3
)

// targetSlots are the slots of the three targets Baium ranks: their
// points, and who each is.
func targetSlots() ([3]script.IntSlot, [3]script.CreatureSlot) {
	return [3]script.IntSlot{npc.IntQuest0, npc.IntQuest1, npc.IntQuest2},
		[3]script.CreatureSlot{npc.CreatureQuest0, npc.CreatureQuest1, npc.CreatureQuest2}
}

// Baium is the grand boss Baium. Created outside its lair it sleeps: it
// leaves its stone there, and the stone's script event wakes it in the
// lair, where it greets the waker, calls its five archangels and starts
// fighting; created inside it resumes the fight at once. While it fights
// it ranks up to three targets by the hits and heals it sees, casts its
// skills at the best, and once 30 minutes pass without a player hit it
// sends the players of its lair away and goes back to sleep. Dying, it
// leaves a teleport cube.
func Baium() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29020}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.Int(slotAwake) == 1 {
				n.AddWanderDesire(5, 5)
				n.LookNeighbor()
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			n.SetInt(slotFighting, 1)
			n.SetInt(script.SlotFlag, 1)

			n.SetHeading(-25348)
			n.TeleportTo(script.Loc{X: 116033, Y: 17447, Z: 10107})

			n.SetDBValue(1)

			n.SetInt(slotWaker, e.Arg1)

			s.StartTimer("2004", n, nil, 11500*time.Millisecond)
			s.StartTimer("2006", n, nil, 2000*time.Millisecond)
			s.StartTimer("2002", n, nil, 60000*time.Millisecond)

			n.SetInt(slotLastHit, s.CurrentTick())

			s.StartTimer("2003", n, nil, time.Duration((s.Rnd(3)+2)+(60*1000))*time.Millisecond)

			n.SetInt(slotAwake, 0)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(slotFighting, 0)

			s.GlobalMemo().Set(memoKey, int(n.ObjectID()))
			n.SetDBValue(0)

			if n.InMyTerritory() {
				n.SetDBValue(1)

				n.SetInt(slotFighting, 1)
				n.SetInt(slotLastHit, s.CurrentTick())

				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				s.StartTimer("2002", n, nil, 60000*time.Millisecond)
				s.StartTimer("2003", n, nil, time.Duration((s.Rnd(3)+2)+(60*1000))*time.Millisecond)
			} else {
				s.Spawner().CreateOnePrivateEx(n, stoneID, script.Loc{X: 116033, Y: 17447, Z: 10107, Heading: -25348}, 0, script.Params{})
			}

			n.SetInt(script.SlotFlag, 1)
			n.SetInt(slotDamage, 1)
			n.SetInt(slotPartyDamage, 1)
			n.SetInt(slotAwake, 0)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			s.Spawner().CreateOnePrivateEx(n, cubeID, script.Loc{X: 115017, Y: 15549, Z: 10090}, 0, script.Params{})

			n.PlayMusic("BS01_D")

			s.GlobalMemo().Remove(memoKey)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.Int(slotAwake) != 1 {
				return
			}

			hinderStrider := skill.Ref{ID: 4258, Level: 1}
			if script.Riding(attacker) && s.AbnormalLevel(attacker, hinderStrider) <= 0 {
				n.AddCastDesire(attacker, hinderStrider, 1000000)
			}

			if attacker.Playable() {
				if _, ok := attacker.(*script.Player); ok {
					damage := e.Damage
					n.SetInt(slotDamage, n.Int(slotDamage)+damage)
					n.SetInt(slotLastHit, s.CurrentTick())

					switch hp, maxHP := n.HP(), float64(n.MaxHP()); {
					case e.Skill.ID == 0:
						rank(s, n, attacker, damage*1000)
					case hp < (maxHP*1.0)/4.0:
						rank(s, n, attacker, (damage/30)*1000)
					case hp < (maxHP*2.0)/4.0:
						rank(s, n, attacker, (damage/50)*1000)
					case hp < (maxHP*3.0)/4.0:
						rank(s, n, attacker, (damage/100)*1000)
					default:
						rank(s, n, attacker, (damage/150)*1000)
					}
				}
			} else {
				n.SetSlot(slotNonPlayer, attacker)
			}

			if n.Intention() == script.IntentionWander && n.Int(slotAwake) != 0 {
				castBaiumSkill(s, n)
			}
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			if !script.Same(e.Caller, e.Called) && !script.Same(e.Target, e.Called) {
				e.Called.SetInt(slotPartyDamage, e.Called.Int(slotPartyDamage)+e.Damage)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !n.InSpawnTerritory(c) {
				return
			}

			p := script.ActingPlayer(c)
			if p != nil && n.LifeTime() < 10 {
				n.SetSlot(slotFirstSeen, c)
			}

			if p != nil && script.ClassTypeOf(p.ClassID()) == player.ClassTypePriest {
				switch hpRatio := n.HPRatio(); {
				case hpRatio < 0.25:
					rank(s, n, c, 10*1000)
				case hpRatio < 0.5:
					rank(s, n, c, 6*1000)
				case hpRatio < 0.75:
					rank(s, n, c, 3*1000)
				default:
					rank(s, n, c, 2*1000)
				}
			} else {
				rank(s, n, c, 1*1000)
			}

			if n.Intention() == script.IntentionWander && n.Int(slotAwake) != 0 {
				castBaiumSkill(s, n)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if n.Int(slotAwake) == 0 {
				return
			}

			castBaiumSkill(s, n)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if n.Int(slotFighting) == 0 {
				return base.Timer(s, e)
			}

			switch e.Name {
			case "2001":
				s.Spawner().CreateOnePrivateEx(n, archangelID, script.Loc{X: 115792, Y: 16608, Z: 10136}, 0, script.Params{P1: 1})
				s.Spawner().CreateOnePrivateEx(n, archangelID, script.Loc{X: 115168, Y: 17200, Z: 10136}, 0, script.Params{P1: 2})
				s.Spawner().CreateOnePrivateEx(n, archangelID, script.Loc{X: 115780, Y: 15564, Z: 10136, Heading: 13620}, 0, script.Params{P1: 3})
				s.Spawner().CreateOnePrivateEx(n, archangelID, script.Loc{X: 114880, Y: 16236, Z: 10136, Heading: 5400}, 0, script.Params{P1: 4})
				s.Spawner().CreateOnePrivateEx(n, archangelID, script.Loc{X: 114239, Y: 17168, Z: 10136, Heading: -1992}, 0, script.Params{P1: 5})

				n.SetInt(slotAwake, 1)

				castBaiumSkill(s, n)
			case "2002":
				i0 := s.ElapsedTicks(n.Int(slotLastHit))
				switch {
				case i0 > 30*60:
					n.RemoveAllDesire()
					n.SetDBValue(0)
					s.TeleportPlayersInTerritory(n, script.Loc{X: 120112, Y: 18208, Z: -5152}, 900)
					n.TeleportTo(script.Loc{X: -105200, Y: -253104, Z: -15264})

					s.Spawner().CreateOnePrivateEx(n, stoneID, script.Loc{X: 116026, Y: 17435, Z: 10107, Heading: -25348}, 0, script.Params{})

					n.SetInt(slotFighting, 0)
					n.SetInt(slotAwake, 0)
					n.SetInt(script.SlotFlag, 6)
				case i0 > 5*60 && n.HPRatio() < 0.75:
					s.StartTimer("2002", n, nil, 60000*time.Millisecond)

					n.AddCastDesire(n, skill.Ref{ID: 4135, Level: 1}, 1000000)
				default:
					s.StartTimer("2002", n, nil, 60000*time.Millisecond)
				}
			case "2003":
				n.SetInt(slotPartyDamage, saturatedInt(float64(n.Int(slotPartyDamage))*2.73))

				damage, partyDamage := n.Int(slotDamage), n.Int(slotPartyDamage)
				i0 := damage + partyDamage

				var i1 int32
				if damage > partyDamage {
					i1 = damage - partyDamage
				} else {
					i1 = partyDamage - damage
				}

				if i1*10 > 3*i0 {
					n.SetInt(script.SlotFlag, 0)
				} else {
					n.SetInt(script.SlotFlag, 1+int32(s.Rnd(4)))
				}

				// A next delay is drawn, then the timer is armed with 30 s
				// regardless: the draw stays, as it moves the random
				// sequence.
				_ = (s.Rnd(3) + 2) + (60 * 1000)

				s.StartTimer("2003", n, nil, 30000*time.Millisecond)

				n.SetInt(slotDamage, 1)
				n.SetInt(slotPartyDamage, 1)
			case "2004":
				n.SocialAction(3)

				s.StartTimer("2005", n, nil, 7300*time.Millisecond)
			case "2005":
				n.SocialAction(1)

				var c0 script.Creature
				if waker := n.Int(slotWaker); waker != 0 {
					c0 = s.Object(waker)
				}
				if c0 != nil {
					if n.Distance2D(c0) < 16000 {
						n.Say(script.NpcString(22937))
						script.TeleportTo(c0, script.Loc{X: 115910, Y: 17337, Z: 10105})
						n.AddCastDesire(c0, skill.Ref{ID: 4136, Level: 1}, 1100000)
					}
				} else if first := n.Slot(slotFirstSeen); first != nil {
					n.Say(script.NpcString(22937))
					script.TeleportTo(first, script.Loc{X: 115910, Y: 17337, Z: 10105})
					n.AddCastDesire(first, skill.Ref{ID: 4136, Level: 1}, 1100000)
				}

				if hero := s.NeighborHero(n, 4000); hero != nil {
					n.BroadcastOnScreen(3000, script.NpcString(1000521, hero.Name()))
				}

				s.StartTimer("2001", n, nil, 8000*time.Millisecond)
			case "2006":
				n.Earthquake(40, 10)
				n.PlayMusic("BS02_A")
			}

			return base.Timer(s, e)
		},
	})}
}

// rank weighs c with points among Baium's three targets. A creature
// already ranked keeps its slot, raised to points (plus a random share)
// when that beats its standing by more than 1000; any other takes the
// weakest slot, a tie broken at random.
func rank(s *script.Script, n *script.NPC, c script.Creature, points int32) {
	pts, who := targetSlots()
	q0, q1, q2 := n.Int(pts[0]), n.Int(pts[1]), n.Int(pts[2])
	var i1 int32
	switch {
	case script.Same(c, n.Slot(who[0])):
		if points+1000 > q0 {
			n.SetInt(pts[0], points+int32(s.Rnd(3000)))
		}
	case script.Same(c, n.Slot(who[1])):
		if points+1000 > q1 {
			n.SetInt(pts[1], points+int32(s.Rnd(3000)))
		}
	case script.Same(c, n.Slot(who[2])):
		if points+1000 > q2 {
			n.SetInt(pts[2], points+int32(s.Rnd(3000)))
		}
	case q0 > q1:
		i1 = 3
	case q0 == q1:
		i1 = pick(s, 2, 3)
	case q0 < q1:
		i1 = 2
	}

	switch i1 {
	case 2:
		switch {
		case q0 > q2:
			i1 = 4
		case q0 == q2:
			i1 = pick(s, 2, 4)
		case q0 < q2:
			i1 = 2
		}
	case 3:
		switch {
		case q1 > q2:
			i1 = 4
		case q1 == q2:
			i1 = pick(s, 3, 4)
		case q1 < q2:
			i1 = 3
		}
	}

	switch i1 {
	case 2, 3, 4:
		n.SetInt(pts[i1-2], points+int32(s.Rnd(3000)))
		n.SetSlot(who[i1-2], c)
	}
}

// pick returns a or b on an even draw.
func pick(s *script.Script, a, b int32) int32 {
	if s.RndBool() {
		return a
	}
	return b
}

// castBaiumSkill makes Baium cast one of its skills at its best target,
// the creature that hit it last when it has none: the lower its HP, the
// more of its skills it draws from.
func castBaiumSkill(s *script.Script, n *script.NPC) {
	pts, who := targetSlots()

	// The last non-player attacker drawn here is replaced below by the best
	// target anyway: only the draw, which moves the random sequence, and
	// the skipped reset of the first target count.
	drawn := n.Slot(slotNonPlayer) != nil && n.Int(script.SlotFlag) == 0 && s.Rnd(100) < 10
	if !drawn && gone(n, n.Slot(who[0])) {
		n.SetInt(pts[0], 0)
	}

	if gone(n, n.Slot(who[1])) {
		n.SetInt(pts[1], 0)
	}

	if gone(n, n.Slot(who[2])) {
		n.SetInt(pts[2], 0)
	}

	var c2 script.Creature
	var i1 int
	var i2 int32
	if q0, q1 := n.Int(pts[0]), n.Int(pts[1]); q0 > q1 {
		i1, i2, c2 = 0, q0, n.Slot(who[0])
	} else {
		i1, i2, c2 = 1, q1, n.Slot(who[1])
	}

	if q2 := n.Int(pts[2]); q2 > i2 {
		i1, i2, c2 = 2, q2, n.Slot(who[2])
	}

	if i2 > 0 && s.Rnd(100) < 70 {
		n.SetInt(pts[i1], 500)
	}

	if c2 == nil || i2 == 0 {
		c2 = n.Slot(slotNonPlayer)
	}

	if c2 == nil || c2.Dead() {
		return
	}

	cast := func(id skill.ID) { n.AddCastDesire(c2, skill.Ref{ID: id, Level: 1}, 1000000) }
	switch hpRatio := n.HPRatio(); {
	case hpRatio > 0.75:
		switch {
		case s.Rnd(100) < 10:
			cast(4128)
		case s.Rnd(100) < 10:
			cast(4129)
		default:
			cast(4127)
		}
	case hpRatio > 0.5:
		switch {
		case s.Rnd(100) < 10:
			cast(4131)
		case s.Rnd(100) < 10:
			cast(4128)
		case s.Rnd(100) < 10:
			cast(4129)
		default:
			cast(4127)
		}
	default:
		switch {
		case s.Rnd(100) < 10:
			cast(4130)
		case s.Rnd(100) < 10:
			cast(4131)
		case s.Rnd(100) < 10:
			cast(4128)
		case s.Rnd(100) < 10:
			cast(4129)
		default:
			cast(4127)
		}
	}
}

// gone reports whether target slot c holds no one Baium can still rank:
// empty, over 9000 away or dead.
func gone(n *script.NPC, c script.Creature) bool {
	return c == nil || n.Distance2D(c) > 9000 || c.Dead()
}

// saturatedInt converts f to an int32 as a cast of a double to an int
// does: rounded toward zero, clamped to the int32 range, NaN to 0.
func saturatedInt(f float64) int32 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	}
	return int32(f)
}
