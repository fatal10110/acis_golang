package antharas

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// behemothID is the Behemoth Dragon Antharas calls under half its HP;
// cubeID the teleport cube it leaves dying.
const (
	behemothID = 29069
	cubeID     = 31859
)

// The NPC's script memory as Antharas uses it.
const (
	slotCalled  = script.SlotIntAI0 // 1 for 4 hours once it called its Behemoth Dragon
	slotLastHit = script.SlotIntAI1 // tick of its last hit, or of its awakening
)

// targetSlots are the slots of the three targets Antharas ranks: their
// points, and who each is.
func targetSlots() ([3]script.IntSlot, [3]script.CreatureSlot) {
	return [3]script.IntSlot{script.SlotIntAI2, script.SlotIntAI3, script.SlotIntAI4},
		[3]script.CreatureSlot{script.SlotCreature2, script.SlotCreature3, script.SlotCreature4}
}

// The places Antharas uses: where its move-in starts and ends, where its
// reset sends the players of its lair, and where it waits out of the lair.
var (
	lairEntry = script.Loc{X: 185452, Y: 114835, Z: -8221}
	lairStage = script.Loc{X: 181911, Y: 114835, Z: -7678}
	exitPoint = script.Loc{X: 80464, Y: 152294, Z: -3534}
	hidePoint = script.Loc{X: -105200, Y: -253104, Z: -15264}
)

// memoKey is the global memo key under which Antharas leaves its object id
// while it lives: its GM_ID spawn parameter.
func memoKey(n *script.NPC) string { return strconv.Itoa(int(n.IntParam("GM_ID"))) }

// moveIn teleports Antharas to its lair's entry and sends it walking to
// its stage with its music.
func moveIn(n *script.NPC) {
	n.TeleportTo(lairEntry)
	n.AddMoveToDesire(lairStage, 100000)
	n.PlayMusic("BS02_A")
}

// Antharas is the grand boss Antharas, in its three sizes. A fresh spawn
// stores db value 0 and shakes the screens around; a spawn restored from
// its row resumes where it stood: at 1 it moves in 10 minutes later, at 2
// it moves in at once, at 3 it fights. Asleep at 0, a script event wakes
// it: 10 s later it stores 2 and moves in, and once at its stage a camera
// sequence ends at 3 with the fight. Fighting, it ranks up to three
// targets by the hits and the players it sees, casts its skills at the
// best, buffs itself by its HP every minute, and calls its Behemoth Dragon
// once under half its HP (again 4 hours later). A hit while it sleeps
// sends the players of its lair away; 15 minutes without a hit reset it to
// 0 out of its lair. Dying, it plays its camera and music, stores 0,
// clears its global memo key and leaves a teleport cube for 15 minutes.
func Antharas() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/antharas", Behavior: true, NPCs: []int32{29066, 29067, 29068}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(slotCalled, 0)

			s.StartTimerAtFixedRate("2001", n, nil, 2*60*1000*time.Millisecond, 2*60*1000*time.Millisecond)

			if !n.DBLoaded() {
				n.SetDBValue(0)
				n.Earthquake(20, 10)
			}

			n.SetInt(slotLastHit, s.CurrentTick())
			n.SetInt(script.SlotIntAI2, 0)
			n.SetInt(script.SlotIntAI3, 0)
			n.SetInt(script.SlotIntAI4, 0)

			if n.IntParam("GM_ID") != 0 {
				if s.GlobalMemo().Int(memoKey(n), -1) == -1 {
					s.GlobalMemo().Set(memoKey(n), int(n.ObjectID()))
				}
			}

			switch n.DBValue() {
			case 1:
				s.StartTimer("1001", n, nil, 10*60*1000*time.Millisecond)
			case 2:
				moveIn(n)
			case 3:
				s.StartTimer("1002", n, nil, 60*1000*time.Millisecond)

				n.SetInt(slotLastHit, s.CurrentTick())
				n.AddWanderDesire(5, 5)
			}
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			switch n.DBValue() {
			case 3:
				n.AddWanderDesire(5, 5)
			case 0:
				n.RemoveAllDesire()
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if n.DBValue() == 0 {
				s.StartTimer("1001", n, nil, 10*1000*time.Millisecond)

				n.SetDBValue(1)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n, name := e.NPC, e.Name
			switch {
			case strings.EqualFold(name, "2001"):
				if n.Int(slotCalled) == 0 && n.HPRatio() < 0.5 {
					at := script.LocOf(n)
					at.Heading = 0
					s.Spawner().CreateOnePrivateEx(n, behemothID, at, 0, script.Params{P1: n.IntParam("GM_ID")})

					n.SetInt(slotCalled, 1)

					s.StartTimer("2002", n, e.Player, 240*60*1000*time.Millisecond)
				}
			case strings.EqualFold(name, "2002"):
				n.SetInt(slotCalled, 0)
			case strings.EqualFold(name, "1001"):
				n.SetDBValue(2)
				moveIn(n)
			case strings.EqualFold(name, "1201"):
				n.SpecialCamera(script.CameraShot{Distance: 700, Yaw: 13, Pitch: -19, Time: 0, Duration: 20000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})

				s.StartTimer("1202", n, e.Player, 3000*time.Millisecond)
			case strings.EqualFold(name, "1202"):
				n.SpecialCamera(script.CameraShot{Distance: 700, Yaw: 13, Pitch: 0, Time: 6000, Duration: 20000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})

				s.StartTimer("1203", n, e.Player, 10000*time.Millisecond)
			case strings.EqualFold(name, "1203"):
				n.SpecialCamera(script.CameraShot{Distance: 3700, Yaw: 0, Pitch: -3, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})
				n.AddSocialDesire(1, 8000, 10000000)
				n.AddSocialDesire(2, 6000, 5000000)

				s.StartTimer("1204", n, e.Player, 200*time.Millisecond)
			case strings.EqualFold(name, "1204"):
				n.SpecialCamera(script.CameraShot{Distance: 1100, Yaw: 0, Pitch: -3, Time: 22000, Duration: 30000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})

				s.StartTimer("1205", n, e.Player, 10800*time.Millisecond)
			case strings.EqualFold(name, "1205"):
				n.SpecialCamera(script.CameraShot{Distance: 1100, Yaw: 0, Pitch: -3, Time: 300, Duration: 7000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})
				n.SetDBValue(3)

				s.StartTimer("1002", n, e.Player, 60000*time.Millisecond)

				n.SetInt(slotLastHit, s.CurrentTick())

				if hero := s.NeighborHero(n, 4000); hero != nil {
					n.BroadcastOnScreen(3000, script.NpcString(1000520))
				}

				n.AddCastDesire(n, skill.Ref{ID: 4125, Level: 1}, 4000000)

				castAntharasSkill(s, n)

				n.AddWanderDesire(5, 5)
			case strings.EqualFold(name, "1002"):
				if s.ElapsedTicks(n.Int(slotLastHit)) > 15*60 {
					n.SetDBValue(0)
					n.RemoveAllDesire()
					s.TeleportPlayersInTerritory(n, exitPoint, 100)
					n.TeleportTo(hidePoint)

					n.SetInt(script.SlotIntAI2, 0)
					n.SetInt(script.SlotIntAI3, 0)
					n.SetInt(script.SlotIntAI4, 0)
					n.SetInt(npc.IntQuest0, 0)
				} else {
					s.StartTimer("1002", n, e.Player, 60000*time.Millisecond)
				}

				selfBuff := func(id skill.ID, below int32) {
					if ref := (skill.Ref{ID: id, Level: 1}); s.AbnormalLevel(n, ref) < below {
						n.AddCastDesire(n, ref, 4000000)
					}
				}
				switch hpRatio := n.HPRatio(); {
				case hpRatio < 0.25:
					selfBuff(4241, 14)
				case hpRatio < 0.5:
					selfBuff(4240, 13)
				case hpRatio < 0.75:
					selfBuff(4239, 12)
				default:
					selfBuff(4125, 11)
				}

				pts, _ := targetSlots()
				for _, slot := range pts {
					if v := n.Int(slot); v > 10 {
						n.SetInt(slot, v-int32(s.Rnd(9)+1))
					}
				}

				if n.DBValue() == 3 && idleOrWandering(n) {
					castAntharasSkill(s, n)
				}
			}

			return base.Timer(s, e)
		},
		OnMoveToFinished: func(s *script.Script, e script.MoveToFinished) {
			s.StartTimer("1201", e.NPC, nil, 2000*time.Millisecond)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker, damage := e.NPC, e.Attacker, e.Damage
			if n.DBValue() == 0 && !n.Dead() {
				s.TeleportPlayersInTerritory(n, exitPoint, 100)
			}

			n.SetInt(slotLastHit, s.CurrentTick())

			hinderStrider := skill.Ref{ID: 4258, Level: 1}
			if script.Riding(attacker) && s.AbnormalLevel(attacker, hinderStrider) <= 0 {
				n.AddCastDesire(attacker, hinderStrider, 1000000)
			}

			hp, maxHP := n.HP(), float64(n.MaxHP())
			switch {
			case e.Skill.ID == 0:
				rankInt(s, n, attacker, damage*1000)
			case hp < (maxHP*1.0)/4.0:
				rankScaled(s, n, attacker, float64(damage)/30.0*1000)
			case hp < (maxHP*2.0)/4.0:
				rankScaled(s, n, attacker, float64(damage)/50.0*1000)
			case hp < (maxHP*3.0)/4.0:
				rankScaled(s, n, attacker, float64(damage)/100.0*1000)
			default:
				rankScaled(s, n, attacker, float64(damage)/150.0*1000)
			}

			if n.DBValue() == 3 && idleOrWandering(n) {
				castAntharasSkill(s, n)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			// No class is the occupation "@cleric_group", a class group:
			// the seen player always takes the last branch.
			if p, ok := c.(*script.Player); ok && n.DBValue() == 3 && p.SameOccupation("@cleric_group") {
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
				castAntharasSkill(s, n)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			castAntharasSkill(s, e.NPC)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			n.SpecialCamera(script.CameraShot{Distance: 1200, Yaw: 20, Pitch: -10, Time: 0, Duration: 13000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})
			n.PlayMusic("BS01_D")
			n.SetDBValue(0)

			s.GlobalMemo().Remove(memoKey(n))

			s.Spawner().AddSpawn(cubeID, script.Loc{X: 177615, Y: 114941, Z: -7709, Heading: 0}, false, 900000*time.Millisecond)
		},
	})}
}

// idleOrWandering reports whether Antharas's AI is idle or wandering.
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

// rank weighs c with p among Antharas's three targets. A creature already
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

// gone reports whether target slot c holds no one Antharas can still
// rank: empty, over 9000 away in 3D or dead.
func gone(n *script.NPC, c script.Creature) bool {
	return c == nil || n.Distance3D(c) > 9000 || c.Dead()
}

// inCone reports whether c stands within distance of Antharas, in 3D, and
// within half degrees of due west of it on the ground plane.
func inCone(n *script.NPC, c script.Creature, distance, half float64) bool {
	return n.Distance3D(c) < distance && script.AngleFrom(n, c) < 180+half && script.AngleFrom(n, c) > 180-half
}

// castAntharasSkill makes Antharas cast one of its skills at or around its
// best target: the lower its HP, the more of its skills it draws from.
func castAntharasSkill(s *script.Script, n *script.NPC) {
	pts, who := targetSlots()
	for i := range pts {
		if gone(n, n.Slot(who[i])) {
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

	if i2 <= 0 || c2 == nil {
		return
	}

	if s.Rnd(100) < 70 {
		n.SetInt(pts[i1], 500)
	}

	at := func(id skill.ID) { n.AddCastDesireUnchecked(c2, skill.Ref{ID: id, Level: 1}, 1000000) }
	self := func(id skill.ID) { n.AddCastDesireUnchecked(n, skill.Ref{ID: id, Level: 1}, 1000000) }
	narrow := func() bool { return inCone(n, c2, 1423, 8) || inCone(n, c2, 802, 14) }
	wide := func() bool { return inCone(n, c2, 850, 30) || inCone(n, c2, 425, 90) }

	switch hpRatio := n.HPRatio(); {
	case hpRatio < 0.25:
		switch {
		case s.Rnd(100) < 30:
			at(4110)
		case s.Rnd(100) < 80 && narrow():
			self(4107)
		case s.Rnd(100) < 40 && wide():
			self(4107)
		case s.Rnd(100) < 10 && n.Distance3D(c2) < 1100:
			self(4106)
		case s.Rnd(100) < 10:
			at(4108)
		case s.Rnd(100) < 6:
			at(4111)
		case s.Rnd(100) < 50:
			at(4112)
		default:
			at(4113)
		}
	case hpRatio < 0.5:
		switch {
		case s.Rnd(100) < 80 && narrow():
			self(4107)
		case s.Rnd(100) < 40 && wide():
			self(4107)
		case s.Rnd(100) < 10 && n.Distance3D(c2) < 1100:
			self(4106)
		case s.Rnd(100) < 10:
			at(4108)
		case s.Rnd(100) < 6:
			at(4111)
		case s.Rnd(100) < 50:
			at(4112)
		default:
			at(4113)
		}
	case hpRatio < 0.75:
		switch {
		case s.Rnd(100) < 80 && narrow():
			self(4107)
		case s.Rnd(100) < 10 && n.Distance3D(c2) < 1100:
			self(4106)
		case s.Rnd(100) < 10:
			at(4108)
		case s.Rnd(100) < 6:
			at(4111)
		case s.Rnd(100) < 50:
			at(4112)
		default:
			at(4113)
		}
	default:
		switch {
		case s.Rnd(100) < 80 && narrow():
			self(4107)
		case s.Rnd(100) < 10:
			at(4108)
		case s.Rnd(100) < 6:
			at(4111)
		case s.Rnd(100) < 50:
			at(4112)
		default:
			at(4113)
		}
	}
}
