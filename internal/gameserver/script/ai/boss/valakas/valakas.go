package valakas

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// memoKey is the global memo key under which Valakas leaves its object id.
const memoKey = "3"

// lavasaurusID is the Lavasaurus Elder Valakas raises against a creature
// that hits it.
const lavasaurusID = 29029

// The NPC's script memory as Valakas uses it.
const (
	slotHighDamage   = script.SlotIntAI0 // damage taken from above, toward its flight
	slotGroundDamage = script.SlotIntAI1 // damage taken from its ground, toward its flight
	slotFlying       = script.SlotIntAI2 // 1 while it moves to a flee point
	slotRearDamage   = script.SlotIntAI3 // ground damage taken from behind, toward its rear attack
	slotNearDamage   = script.SlotIntAI4 // ground damage taken, toward its rear attack
	slotRearReady    = npc.IntQuest0     // 1 once it readied its rear attack
	slotLastHit      = npc.IntQuest1     // tick of its last hit, or of its awakening
)

// targetSlots are the slots of the three targets Valakas ranks: their
// points, and who each is.
func targetSlots() ([3]script.IntSlot, [3]script.CreatureSlot) {
	return [3]script.IntSlot{npc.IntQuest2, npc.IntQuest3, npc.IntQuest4},
		[3]script.CreatureSlot{npc.CreatureQuest2, npc.CreatureQuest3, npc.CreatureQuest4}
}

// Valakas is the grand boss Valakas. A fresh spawn stores db value 0; one
// restored from its row resumes where it stood: at 1 it appears 10 minutes
// later, at 3 it fights. Asleep at 0, a script event wakes it: once the
// ValakasWaitTime has passed it stores 2, appears in its lair with its music
// and a camera sequence that ends at 3 with its fight. Fighting, it ranks
// up to three targets by the hits and the creatures it sees and casts its
// skills at the best by its HP and where the target stands; it may raise
// Lavasaurus Elders around a creature that hits it, flies to its nearest
// flee point once enough damage reaches it, readies its rear attack once
// enough of it comes from behind, and keeps up its regeneration by its HP
// every minute. A player hitting it while it sleeps is sent out of its
// lair; 15 minutes without a hit send the players of its lair away and
// reset it to 0 out of its lair. Dying, it clears its memo key, stores 0
// and plays its music and death cameras.
func Valakas() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/valakas", Behavior: true, NPCs: []int32{29028}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			switch n.DBValue() {
			case 3:
				n.AddWanderDesire(5, 5)
			case 0:
				n.RemoveAllDesire()
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if s.GlobalMemo().Int(memoKey, -1) == -1 {
				s.GlobalMemo().Set(memoKey, int(n.ObjectID()))
			}

			if !n.DBLoaded() {
				n.SetDBValue(0)
			} else if n.DBValue() == 1 {
				s.StartTimer("1001", n, nil, 600000*time.Millisecond)
			}

			n.SetInt(slotHighDamage, 0)
			n.SetInt(slotGroundDamage, 0)
			n.SetInt(slotFlying, 0)
			n.SetInt(slotRearDamage, 0)
			n.SetInt(slotNearDamage, 0)
			n.SetInt(slotRearReady, 0)
			n.SetInt(slotLastHit, s.CurrentTick())

			if n.DBValue() == 3 {
				n.AddWanderDesire(5, 5)

				s.StartTimer("1002", n, nil, 60000*time.Millisecond)
			}

			s.StartTimer("1003", n, nil, 600000*time.Millisecond)

			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker, damage := e.NPC, e.Attacker, e.Damage
			if p, ok := attacker.(*script.Player); ok && n.DBValue() == 0 && !n.Dead() {
				p.TeleportTo(script.Loc{X: 150037, Y: -57255, Z: -2976}, 0)
			}

			n.SetInt(slotLastHit, s.CurrentTick())

			if script.LocOf(attacker).Z < script.LocOf(n).Z+200 {
				if n.Int(slotFlying) == 0 {
					n.SetInt(slotGroundDamage, n.Int(slotGroundDamage)+damage)
				}

				if n.Int(slotRearReady) == 0 {
					n.SetInt(slotNearDamage, n.Int(slotNearDamage)+damage)
				}

				if angle := script.AngleFrom(n, attacker); angle > 180-30 && angle < 180+30 {
					if n.Int(slotRearReady) == 0 {
						n.SetInt(slotRearDamage, n.Int(slotRearDamage)+damage)
					}
				}
			} else if n.Int(slotFlying) == 0 {
				n.SetInt(slotHighDamage, n.Int(slotHighDamage)+damage)
			}

			if s.Rnd(100) < 2 {
				raisePrivates(s, n, attacker, firstPrivateSpots())
				raisePrivates(s, n, attacker, secondPrivateSpots())
			}

			maxHP := n.MaxHP()
			if n.Int(slotFlying) == 0 {
				high, ground := n.Int(slotHighDamage), n.Int(slotGroundDamage)
				share := ((high + ground) / maxHP) * 100

				if (share > 3 && high > ground*2) || share > 5 {
					n.SetInt(slotHighDamage, 0)
					n.SetInt(slotGroundDamage, 0)
					n.SetInt(slotFlying, 1)

					n.RemoveAllAttackDesire()

					if at, ok := script.Closest(n, fleePoints()); ok {
						if s.Rnd(3) == 2 {
							s.Rnd(3)
						}

						n.AddMoveToDesire(at, 100000000)
					}
				}
			}

			if n.Int(slotRearReady) == 0 {
				if (n.Int(slotNearDamage)/maxHP)*100 > 1 {
					if rear, near := n.Int(slotRearDamage), n.Int(slotNearDamage); rear > near-rear {
						n.SetInt(slotRearDamage, 0)
						n.SetInt(slotNearDamage, 0)
						n.AddCastDesire(n, skill.Ref{ID: 4687, Level: 1}, 1000000)
						n.SetInt(slotRearReady, 1)
					}
				}
			}

			hinderStrider := skill.Ref{ID: 4258, Level: 1}
			if script.Riding(attacker) && s.AbnormalLevel(attacker, hinderStrider) <= 0 {
				n.AddCastDesire(attacker, hinderStrider, 1000000)
			}

			hp, maxHPf := n.HP(), float64(maxHP)
			switch {
			case e.Skill.ID == 0:
				rankInt(s, n, attacker, damage*1000)
			case hp < (maxHPf*1.0)/4.0:
				rankScaled(s, n, attacker, float64(damage)/30.0*1000)
			case hp < (maxHPf*2.0)/4.0:
				rankScaled(s, n, attacker, float64(damage)/50.0*1000)
			case hp < (maxHPf*3.0)/4.0:
				rankScaled(s, n, attacker, float64(damage)/100.0*1000)
			default:
				rankScaled(s, n, attacker, float64(damage)/150.0*1000)
			}

			if n.DBValue() == 3 && idleOrWandering(n) {
				castAtBest(s, n)
			}
		},
		OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
			n := e.NPC
			if n.Int(slotFlying) == 1 {
				n.SetInt(slotFlying, 0)

				n.AddCastDesire(n, skill.Ref{ID: 4680, Level: 1}, 1000000)
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if n.DBValue() == 0 {
				s.StartTimer("1001", n, nil, s.BossWaitTimes().Valakas)

				n.SetDBValue(1)
			}

			base.ScriptEvent(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n, name, p := e.NPC, e.Name, e.Player
			camera := func(distance, yaw, pitch, at, duration, turn, rise, unknown int32, next string, after time.Duration) {
				n.SpecialCamera(script.CameraShot{Distance: distance, Yaw: yaw, Pitch: pitch, Time: at, Duration: duration, Turn: turn, Rise: rise, Widescreen: 1, Unknown: unknown})
				if next != "" {
					s.StartTimer(next, n, p, after)
				}
			}
			switch {
			case strings.EqualFold(name, "1001"):
				n.SetInt(slotLastHit, s.CurrentTick())

				n.SetDBValue(2)
				n.TeleportTo(script.Loc{X: 212852, Y: -114842, Z: -1632})
				n.PlayMusic("B03_A")

				s.StartTimer("1004", n, nil, 2000*time.Millisecond)
			case strings.EqualFold(name, "1002"):
				if n.DBValue() == 3 {
					hp, maxHP := n.HP(), float64(n.MaxHP())
					regen := func(level int, below int32) {
						if ref := (skill.Ref{ID: 4691, Level: level}); s.AbnormalLevel(n, ref) < below {
							n.AddCastDesire(n, ref, 4000000)
						}
					}
					switch {
					case s.ElapsedTicks(n.Int(slotLastHit)) > 15*60:
						n.SetDBValue(0)
						n.RemoveAllDesire()
						s.TeleportPlayersInTerritory(n, script.Loc{X: 150037, Y: -57255, Z: -2976}, 150)
						n.TeleportTo(script.Loc{X: -105200, Y: -253104, Z: -15264})

						pts, _ := targetSlots()
						for _, slot := range pts {
							n.SetInt(slot, 0)
						}
					case hp < (maxHP*1.0)/4.0:
						regen(4, 14)
					case hp < (maxHP*2.0)/4.0:
						regen(3, 13)
					case hp < (maxHP*3.0)/4.0:
						regen(2, 12)
					default:
						regen(1, 11)
					}

					s.StartTimer("1002", n, p, 60000*time.Millisecond)
				}
			case strings.EqualFold(name, "1003"):
				if n.DBValue() == 3 && idleOrWandering(n) {
					castAtBest(s, n)
				}

				s.StartTimer("1003", n, p, 60000*time.Millisecond)
			case strings.EqualFold(name, "1004"):
				camera(1800, 180, -1, 1500, 10000, 0, 0, 0, "1102", 1500*time.Millisecond)
			case strings.EqualFold(name, "1102"):
				camera(1300, 180, -5, 3000, 10000, 0, -5, 0, "1103", 3300*time.Millisecond)
			case strings.EqualFold(name, "1103"):
				camera(500, 180, -8, 600, 10000, 0, 60, 0, "1104", 2900*time.Millisecond)
			case strings.EqualFold(name, "1104"):
				camera(800, 180, -8, 2700, 10000, 0, 30, 0, "1105", 2700*time.Millisecond)
			case strings.EqualFold(name, "1105"):
				camera(200, 250, 70, 0, 10000, 30, 80, 0, "1106", 1*time.Millisecond)
			case strings.EqualFold(name, "1106"):
				camera(1100, 250, 70, 2500, 10000, 30, 80, 0, "1107", 3200*time.Millisecond)
			case strings.EqualFold(name, "1107"):
				camera(700, 150, 30, 0, 10000, -10, 60, 0, "1108", 1400*time.Millisecond)
			case strings.EqualFold(name, "1108"):
				camera(1200, 150, 20, 2900, 10000, -10, 30, 0, "1109", 6700*time.Millisecond)
			case strings.EqualFold(name, "1109"):
				camera(750, 170, -10, 3400, 4000, 10, -15, 0, "1110", 5700*time.Millisecond)
			case strings.EqualFold(name, "1110"):
				n.AddCastDesire(n, skill.Ref{ID: 4691, Level: 1}, 4000000)
				n.SetDBValue(3)

				s.StartTimer("1002", n, p, 60000*time.Millisecond)

				if hero := s.NeighborHero(n, 4000); hero != nil {
					n.BroadcastOnScreen(3000, script.NpcString(1000519))
				}

				castAtBest(s, n)
			case strings.EqualFold(name, "1111"):
				camera(1100, 210, -5, 3000, 10000, -13, 0, 1, "1112", 3500*time.Millisecond)
			case strings.EqualFold(name, "1112"):
				camera(1300, 200, -8, 3000, 10000, 0, 15, 1, "1113", 4500*time.Millisecond)
			case strings.EqualFold(name, "1113"):
				camera(1000, 190, 0, 500, 10000, 0, 10, 1, "1114", 500*time.Millisecond)
			case strings.EqualFold(name, "1114"):
				camera(1700, 120, 0, 2500, 10000, 12, 40, 1, "1115", 4600*time.Millisecond)
			case strings.EqualFold(name, "1115"):
				camera(1700, 20, 0, 700, 10000, 10, 10, 1, "1116", 750*time.Millisecond)
			case strings.EqualFold(name, "1116"):
				camera(1700, 10, 0, 1000, 10000, 20, 70, 1, "1117", 2500*time.Millisecond)
			case strings.EqualFold(name, "1117"):
				camera(1700, 10, 0, 300, 250, 20, -20, 1, "", 0)
			}

			return base.Timer(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			p := script.ActingPlayer(c)
			if p == nil {
				return
			}

			// No class is the occupation "@cleric_group", a class group: the
			// sighting always takes the last branch.
			if n.DBValue() == 3 && p.SameOccupation("@cleric_group") {
				switch hpRatio := n.HPRatio(); {
				case hpRatio < 0.25:
					rankInt(s, n, c, 10*1000)
				case hpRatio < 0.5:
					rankInt(s, n, c, 6*1000)
				case hpRatio < 0.75:
					rankInt(s, n, c, 3*1000)
				default:
					rankInt(s, n, c, 2*1000)
				}
			} else {
				rankInt(s, n, c, 1*1000)
			}

			if n.DBValue() == 3 && idleOrWandering(n) {
				castAtBest(s, n)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			castAtBest(s, e.NPC)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			s.GlobalMemo().Remove(memoKey)

			n.SetDBValue(0)
			n.PlayMusic("B03_D")
			n.SpecialCamera(script.CameraShot{Distance: 2000, Yaw: 130, Pitch: -1, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})

			s.StartTimer("1111", n, nil, 500*time.Millisecond)
		},
	})}
}

// raisePrivates raises a Lavasaurus Elder against attacker at the first of
// the two spots picked from spots around it, and on an even draw a second
// one at the other, each told who attacker is.
func raisePrivates(s *script.Script, n *script.NPC, attacker script.Creature, spots []script.Loc) {
	picked := script.Farthest(attacker, spots, 2)
	if len(picked) == 0 {
		return
	}
	s.Spawner().CreateOnePrivateEx(n, lavasaurusID, picked[0], 0, script.Params{P1: attacker.ObjectID()})

	if s.Rnd(100) < 50 {
		s.Spawner().CreateOnePrivateEx(n, lavasaurusID, picked[1], 0, script.Params{P1: attacker.ObjectID()})
	}
}

// idleOrWandering reports whether Valakas's AI is idle or wandering.
func idleOrWandering(n *script.NPC) bool {
	switch n.Intention() {
	case script.IntentionWander, script.IntentionIdle:
		return true
	}
	return false
}

// points are the points a hit or a sighting ranks a creature with.
type points struct {
	stored int32   // what a slot takes, before its random share
	exact  float64 // the unrounded points of a spell's damage
	scaled bool    // whether exact, not stored, is compared
}

// beats reports whether p beats a standing of cur by more than 1000: in
// whole numbers for a plain hit or a sighting, unrounded for a spell.
func (p points) beats(cur int32) bool {
	if p.scaled {
		return p.exact+1000 > float64(cur)
	}
	return p.stored+1000 > cur
}

// rankInt ranks c with points counted in whole numbers, as a plain hit and
// a sighting are.
func rankInt(s *script.Script, n *script.NPC, c script.Creature, v int32) {
	rank(s, n, c, points{stored: v})
}

// rankScaled ranks c with points scaled from a spell's damage, compared
// unrounded and stored truncated.
func rankScaled(s *script.Script, n *script.NPC, c script.Creature, v float64) {
	rank(s, n, c, points{stored: commons.TruncInt32Sat(v), exact: v, scaled: true})
}

// rank weighs c with p among Valakas's three targets. A creature already
// ranked keeps its slot, raised to p (plus a random share) when p beats its
// standing; any other takes the weakest slot, a tie broken at random.
func rank(s *script.Script, n *script.NPC, c script.Creature, p points) {
	pts, who := targetSlots()
	q0, q1, q2 := n.Int(pts[0]), n.Int(pts[1]), n.Int(pts[2])
	var i1 int
	switch {
	case script.Same(c, n.Slot(who[0])):
		if p.beats(q0) {
			n.SetInt(pts[0], p.stored+int32(s.Rnd(3000)))
		}
	case script.Same(c, n.Slot(who[1])):
		if p.beats(q1) {
			n.SetInt(pts[1], p.stored+int32(s.Rnd(3000)))
		}
	case script.Same(c, n.Slot(who[2])):
		if p.beats(q2) {
			n.SetInt(pts[2], p.stored+int32(s.Rnd(3000)))
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
		n.SetInt(pts[i1-2], p.stored+int32(s.Rnd(3000)))
		n.SetSlot(who[i1-2], c)
	}
}

// pick returns a or b on an even draw.
func pick(s *script.Script, a, b int) int {
	if s.RndBool() {
		return a
	}
	return b
}

// castAtBest drops the targets Valakas can no longer rank, empty, over 5000
// away in 3D or dead, and casts one of its skills at the best of the
// others, if any: the lower its HP, the likelier its stronger skills.
func castAtBest(s *script.Script, n *script.NPC) {
	pts, who := targetSlots()
	for i := range pts {
		if c := n.Slot(who[i]); c == nil || n.Distance3D(c) > 5000 || c.Dead() {
			n.SetInt(pts[i], 0)
		}
	}

	var i1 int
	var i2 int32
	var c2 script.Creature
	if q0, q1 := n.Int(pts[0]), n.Int(pts[1]); q0 > q1 {
		i1, i2, c2 = 0, q0, n.Slot(who[0])
	} else {
		i1, i2, c2 = 1, q1, n.Slot(who[1])
	}

	if q2 := n.Int(pts[2]); q2 > i2 {
		i1, i2, c2 = 2, q2, n.Slot(who[2])
	}

	if i2 <= 0 {
		return
	}

	if s.Rnd(100) < 70 {
		n.SetInt(pts[i1], 500)
	}

	switch hpRatio := n.HPRatio(); {
	case hpRatio < 0.25:
		castSkills(s, n, c2, chances{20, 15, 15, 10, 35, 20, 15})
	case hpRatio < 0.5:
		castSkills(s, n, c2, chances{5, 10, 10, 10, 20, 5, 10})
	case hpRatio < 0.75:
		castSkills(s, n, c2, chances{0, 5, 5, 10, 15, 0, 5})
	default:
		castSkills(s, n, c2, chances{0, 10, 5, 10, 15, 0, 10})
	}
}

// chances are the percent chances of Valakas's skills against a target on
// its ground (meteor, fear, rear strike, rear throw, low breath) and above
// it (meteor, fear).
type chances struct {
	meteor, fear, rearStrike, rearThrow, breath, highMeteor, highFear int
}

// castSkills makes Valakas cast one skill at or around target by chance:
// against a target on its ground, its meteor, fear, its rear strike (once
// readied) or rear throw while the target stands behind it, its low
// breath, else a strike by the side the target stands on; against one
// above it, its meteor, fear, else its high breath.
func castSkills(s *script.Script, n *script.NPC, target script.Creature, ch chances) {
	if target == nil {
		return
	}

	direction := script.AngleFrom(n, target)
	distance := n.Distance3D(target)

	behind := (distance < 1423 && direction < 180+8 && direction > 180-8) || (distance < 802 && direction < 180+14 && direction > 180-14)

	at := func(id skill.ID) { n.AddCastDesire(target, skill.Ref{ID: id, Level: 1}, 1000000) }
	self := func(id skill.ID) { n.AddCastDesire(n, skill.Ref{ID: id, Level: 1}, 1000000) }

	if script.LocOf(target).Z < script.LocOf(n).Z+200 {
		switch {
		case s.Rnd(100) < ch.meteor:
			self(4690)
		case s.Rnd(100) < ch.fear:
			at(4689)
		case s.Rnd(100) < ch.rearStrike && behind && n.Int(slotRearReady) == 1:
			at(4685)
		case s.Rnd(100) < ch.rearThrow && behind:
			at(4688)
		case s.Rnd(100) < ch.breath:
			at(4683)
		case direction > 0 && direction <= 180:
			at(4682)
		default:
			at(4681)
		}
		return
	}

	switch {
	case s.Rnd(100) < ch.highMeteor:
		self(4690)
	case s.Rnd(100) < ch.highFear:
		at(4689)
	default:
		at(4684)
	}
}
