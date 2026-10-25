package zaken

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Zaken's script memory.
const (
	// slotHeld gates the night moves; nothing sets it to anything but 0.
	slotHeld = script.SlotIntAI0
	// slotDestX, slotDestY and slotDestZ hold where Instant Move takes
	// Zaken.
	slotDestX = script.SlotIntAI1
	slotDestY = script.SlotIntAI2
	slotDestZ = script.SlotIntAI3
	// slotWave is the next wave of privates timer 1003 places, 1 to 7; 8
	// once every wave stands.
	slotWave = script.SlotIntAI4
	// slotMarks counts the creatures Zaken marked as it saw them, up to 5.
	slotMarks = npc.IntQuest0
	// slotStreak counts the night checks Zaken spent on one top desire
	// target, slotStreakTarget.
	slotStreak       = npc.IntQuest1
	slotStreakTarget = script.SlotCreature0
	// slotQuarters is how many of its HP quarters Zaken may still escape
	// from by day.
	slotQuarters = npc.IntQuest2
)

// landed is a creature a mass teleport sent and the point it lands at.
type landed struct {
	c  script.Creature
	at script.Loc
}

// markSlot is the creature slot of mark k, 0 to 4.
func markSlot(k int) script.CreatureSlot {
	switch k {
	case 0:
		return npc.CreatureQuest0
	case 1:
		return npc.CreatureQuest1
	case 2:
		return npc.CreatureQuest2
	case 3:
		return npc.CreatureQuest3
	}
	return npc.CreatureQuest4
}

// setDest makes at the point Instant Move takes Zaken to.
func setDest(n *script.NPC, at script.Loc) {
	n.SetInt(slotDestX, int32(at.X))
	n.SetInt(slotDestY, int32(at.Y))
	n.SetInt(slotDestZ, int32(at.Z))
}

// dest is the point Instant Move takes Zaken to.
func dest(n *script.NPC) script.Loc {
	return script.Loc{X: int(n.Int(slotDestX)), Y: int(n.Int(slotDestY)), Z: int(n.Int(slotDestZ))}
}

// farther reports whether a lies more than r from b on the ground, in
// 32-bit arithmetic that wraps on overflow.
func farther(a, b script.Loc, r int32) bool {
	dx, dy := int32(a.X)-int32(b.X), int32(a.Y)-int32(b.Y)
	return dx*dx+dy*dy > r*r
}

// moveToRoom picks a room point as Zaken's destination and makes it want
// Instant Move.
func moveToRoom(s *script.Script, n *script.NPC) {
	setDest(n, randomRoomPoint(s))
	n.AddCastDesire(n, skill.Ref{ID: 4222, Level: 1}, 10000000)
}

// chooseSkill makes Zaken want one of its skills on c, after a draw of 225:
// Scatter Enemy, its mass teleport, its 4219 or its 4218, or, for c close
// to it and not its top desire target, its 4221 on itself. Then, c being
// its top desire target, it wants last on c one time in two.
func chooseSkill(s *script.Script, n *script.NPC, c script.Creature, last skill.ID) {
	top := n.TopDesireTarget()

	i0 := s.Rnd(15 * 15)
	switch {
	case i0 < 1:
		n.AddCastDesire(c, skill.Ref{ID: 4216, Level: 1}, 1000000)
	case i0 < 2:
		n.AddCastDesire(c, skill.Ref{ID: 4217, Level: 1}, 1000000)
	case i0 < 4:
		n.AddCastDesire(c, skill.Ref{ID: 4219, Level: 1}, 1000000)
	case i0 < 8:
		n.AddCastDesire(c, skill.Ref{ID: 4218, Level: 1}, 1000000)
	case i0 < 15:
		if !script.Same(c, top) && n.Distance3D(c) < 100 {
			n.AddCastDesire(n, skill.Ref{ID: 4221, Level: 1}, 1000000)
		}
	}

	if script.Same(c, top) && s.Rnd(2) < 1 {
		n.AddCastDesire(c, skill.Ref{ID: last, Level: 1}, 1000000)
	}
}

// Zaken is the grand boss Zaken. It appears with its music and, inside its
// territory, fills the five rooms of each deck of its ship with seven waves
// of privates, 1.7 s apart; a dead private comes back 30 to 89 s later. It
// checks every 30 s: at night (game hour under 5) it ends its day form,
// keeps its regeneration up, moves to a random room when its fight and
// every creature it marked are more than 1500 from it, and drops a top
// desire target it held for over five checks; by day it takes its day
// form and may again escape three HP quarters. It also moves to a random
// room one time in 40. A private hit lightly while Zaken wanders at night
// may call it there. Seeing a player or summon on its deck it attacks it
// inside its territory, marks it one time in three, up to five, and may
// want a skill on it; hits and spells it sees make it want a skill the
// same way, a hit hates by its damage, and a hit by day under the HP
// quarter it may still escape sends it to a random room. Scatter Enemy
// sends its target to a random room; its mass teleport also sends the
// marks standing within 250 of where the target lands. Dying, it plays its
// death music.
func Zaken() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/zaken", Behavior: true, NPCs: []int32{29022}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.PlayMusic("BS01_A")

			n.SetInt(slotHeld, 0)
			setDest(n, script.LocOf(n))
			n.SetInt(slotMarks, 0)

			if !n.DBLoaded() {
				n.SetInt(slotQuarters, 3)
			}

			n.SetInt(slotStreak, 0)

			if n.InMyTerritory() {
				n.SetInt(slotWave, 1)

				s.StartTimer("1003", n, nil, 1700*time.Millisecond)
			}

			s.StartTimerAtFixedRate("1001", n, nil, 1000*time.Millisecond, 30000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1001"):
				check(s, n)
			case strings.EqualFold(e.Name, "1002"):
				n.SetInt(slotMarks, 0)
				n.AddCastDesire(n, skill.Ref{ID: 4222, Level: 1}, 10000000)
				n.SetInt(slotHeld, 0)
			case strings.EqualFold(e.Name, "1003"):
				placeWave(s, n, e.Player)
			}
			return ""
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n, c := e.NPC, e.Creature
			switch e.Skill {
			case skill.Ref{ID: 4222, Level: 1}:
				n.TeleportTo(dest(n))
				n.RemoveAllAttackDesire()
			case skill.Ref{ID: 4216, Level: 1}:
				script.TeleportTo(c, randomRoomPoint(s))
				n.StopHate(c)
			case skill.Ref{ID: 4217, Level: 1}:
				// Each creature this cast sent is measured where it lands:
				// at the point drawn for it, as its teleport only runs
				// once this task is done.
				landing := randomRoomPoint(s)
				script.TeleportTo(c, landing)
				n.StopHate(c)
				sent := []landed{{c, landing}}

				for k := range 5 {
					mark := n.Slot(markSlot(k))
					if mark == nil || n.Int(slotMarks) <= int32(k) || script.Same(mark, c) {
						continue
					}
					at := script.LocOf(mark)
					i := slices.IndexFunc(sent, func(l landed) bool { return script.Same(l.c, mark) })
					if i >= 0 {
						at = sent[i].at
					}
					if at.Z > landing.Z-100 && at.Z < landing.Z+100 && !farther(at, landing, 250) {
						to := randomRoomPoint(s)
						script.TeleportTo(mark, to)
						n.StopHate(mark)
						if i >= 0 {
							sent[i].at = to
						} else {
							sent = append(sent, landed{mark, to})
						}
					}
				}
			}
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called := e.Caller, e.Called
			if s.GameHour() < 5 {
				if called.Intention() == script.IntentionWander && called.Int(slotHeld) == 0 && e.Damage < 10 && s.Rnd(30*15) < 1 {
					called.SetInt(slotHeld, 0)
					setDest(called, script.LocOf(caller))

					s.StartTimer("1002", called, nil, 300*time.Millisecond)
				}
			}
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if !script.Same(called, caller) {
				// The way the private would face at Zaken's place; its
				// respawn places it at its own point, facing its own way,
				// so the heading is drawn and never used.
				s.Rnd(360)

				s.Spawner().ScheduleRespawn(caller, time.Duration(30+s.Rnd(60))*1000*time.Millisecond)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			z, cz := script.LocOf(n).Z, script.LocOf(c).Z
			if cz <= z-100 || cz >= z+100 {
				return
			}
			if !c.Playable() {
				return
			}

			if n.InMyTerritory() {
				n.AddAttackDesire(c, 200)
			}

			if marks := n.Int(slotMarks); marks < 5 && s.Rnd(3) < 1 {
				n.SetSlot(markSlot(int(marks)), c)
				n.SetInt(slotMarks, marks+1)
			}

			if s.Rnd(15) < 1 {
				chooseSkill(s, n, c, 1080321)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			striderSlow := skill.Ref{ID: 4258, Level: 1}
			if script.Riding(attacker) && s.AbnormalLevel(attacker, striderSlow) <= 0 {
				n.AddCastDesire(attacker, striderSlow, 1000000)
			}

			if attacker.Playable() {
				n.AddAttackDesire(attacker, float64(e.Damage)/float64(n.MaxHP())/0.05*20000)
			}

			if s.Rnd(10) < 1 && attacker.Playable() {
				chooseSkill(s, n, attacker, 4220)
			}

			if s.GameHour() >= 5 && n.HP() < float64(n.MaxHP()*n.Int(slotQuarters)/4) {
				n.SetInt(slotQuarters, n.Int(slotQuarters)-1)

				moveToRoom(s, n)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			info := s.Skill(e.Skill)
			if info.AggroPoints() > 0 && !info.Offensive() {
				n.AddAttackDesire(caster, float64(int32(info.AggroPoints())/n.MaxHP()*10*150))
			}

			if s.Rnd(12) < 1 {
				chooseSkill(s, n, caster, 4220)
			}
		},
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			e.NPC.PlayMusic("BS02_D")
		},
	})}
}

// check is Zaken's 30 s check (timer 1001).
func check(s *script.Script, n *script.NPC) {
	nightToDay := skill.Ref{ID: 4223, Level: 1}
	dayToNight := skill.Ref{ID: 4224, Level: 1}
	regeneration := skill.Ref{ID: 4227, Level: 1}

	if s.GameHour() < 5 {
		if s.AbnormalLevel(n, nightToDay) == 1 {
			n.AddCastDesire(n, dayToNight, 10000000)
			setDest(n, script.LocOf(n))
		}

		intention := n.Intention()
		top := n.TopDesireTarget()

		if s.AbnormalLevel(n, regeneration) == -1 {
			n.AddCastDesire(n, regeneration, 10000000)
		}
		if intention == script.IntentionAttack && n.Int(slotHeld) == 0 {
			awayFromAll := true
			if top == nil || !farther(script.LocOf(top), dest(n), 1500) {
				awayFromAll = false
			}
			for k := range 5 {
				if n.Int(slotMarks) <= int32(k) {
					break
				}
				mark := n.Slot(markSlot(k))
				if mark != nil && !farther(script.LocOf(mark), dest(n), 1500) {
					awayFromAll = false
				}
			}
			if awayFromAll {
				n.SetInt(slotMarks, 0)
				moveToRoom(s, n)
			}
		}

		if s.Rnd(20) < 1 && n.Int(slotHeld) == 0 {
			setDest(n, script.LocOf(n))
		}

		if intention == script.IntentionAttack && top != nil {
			if n.Int(slotStreak) != 0 && script.Same(n.Slot(slotStreakTarget), top) {
				n.SetInt(slotStreak, n.Int(slotStreak)+1)
			} else {
				n.SetSlot(slotStreakTarget, top)
				n.SetInt(slotStreak, 1)
			}
		}

		if intention == script.IntentionWander {
			n.SetInt(slotStreak, 0)
		}

		if n.Int(slotStreak) > 5 {
			n.StopHate(n.Slot(slotStreakTarget))
			n.SetInt(slotStreak, 0)
		}
	} else if s.AbnormalLevel(n, nightToDay) == -1 {
		n.AddCastDesire(n, nightToDay, 10000000)
		n.SetInt(slotQuarters, 3)
	}
	if s.AbnormalLevel(n, regeneration) == 11 {
		n.AddCastDesire(n, skill.Ref{ID: 4242, Level: 1}, 10000000)
	}
	if s.Rnd(40) < 1 {
		moveToRoom(s, n)
	}
}

// placeWave places Zaken's next wave of privates (timer 1003) and, before
// the last, starts the timer again for the one after, bound to p.
func placeWave(s *script.Script, n *script.NPC, p *script.Player) {
	wave := n.Int(slotWave)
	var id int32
	perRoom := 1
	switch wave {
	case 1:
		id = 29026
	case 2:
		id = 29023
	case 3:
		id, perRoom = 29024, 2
	case 4:
		id, perRoom = 29027, 5
	case 5, 6, 7:
		for _, pvt := range fixedWave(wave) {
			at := pvt.at
			at.Heading = s.Rnd(360)
			s.Spawner().CreateOnePrivateEx(n, pvt.id, at, 0, script.Params{})
		}
	default:
		return
	}
	if id != 0 {
		for room := 1; room <= 15; room++ {
			for range perRoom {
				at := roomPoint(s, room)
				at.Heading = s.Rnd(360)
				s.Spawner().CreateOnePrivateEx(n, id, at, 0, script.Params{})
			}
		}
	}

	n.SetInt(slotWave, wave+1)
	if wave < 7 {
		s.StartTimer("1003", n, p, 1700*time.Millisecond)
	}
}
