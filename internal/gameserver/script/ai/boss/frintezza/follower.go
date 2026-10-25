package frintezza

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// dir is the page directory of the Frintezza behaviors.
const dir = "ai/boss/frintezza"

// The follower's weapons, and the global memo key that holds its object id.
const (
	changeWeapon      = 7903
	changeWeaponStart = 8204
	gmID              = 5
)

// The follower's skills.
func swingVer1() skill.Ref       { return skill.Ref{ID: 5014, Level: 1} }
func swingVer2() skill.Ref       { return skill.Ref{ID: 5014, Level: 2} }
func dashVer2() skill.Ref        { return skill.Ref{ID: 5015, Level: 2} }
func dashAllVer2() skill.Ref     { return skill.Ref{ID: 5015, Level: 4} }
func antiGravity() skill.Ref     { return skill.Ref{ID: 5016, Level: 1} }
func magicCircle() skill.Ref     { return skill.Ref{ID: 5018, Level: 1} }
func changeBodySkill() skill.Ref { return skill.Ref{ID: 5017, Level: 1} }

// FollowerOfFrintezza is Scarlet van Halisha (29046), Frintezza's follower.
// Its db value is its stage: 0 sleeping, 4 rising, 5 the first form, 6 the
// second form, 50000 beaten in its second form. Global memo key 5 names it;
// keys 4 and 6 name Frintezza and the follower's transform, which it tells
// of its stage by script events carrying its object id as the event id.
//
// Script event arg1 4 wakes it: it leaves its dummy at the hall, jumps there
// 6 s later and fights 18 s after that, from stage 5. Hit below 60% of its
// HP at stage 5 it changes body and weapon into stage 6; below 50% it tells
// Frintezza 30010; at 20% or less it tells the NPCs within 6000 that it is
// beaten (arg1 50000), stops and, 10 s later, plays its exit: a walk, two
// socials and nine camera shots, calling its transform onto its place, then
// leaves the world at stage 0. Dying does the same from 0.5 s later. Every
// minute at stage 5 or 6, 15 minutes with no hit puts it back to sleep: at
// stage 0 it sends the players in its territory out of the tomb, jumps to
// its sleeping point and runs maker event 1000 on both evil spirit makers.
// Arg1 0 sends it to sleep at once.
//
// It fights the hardest of the last three creatures that hit or met it,
// ranked by a damage score, with its swings, dashes, the anti-gravity and
// the magic circle; a strider rider that hits it gets Hinder Strider.
func FollowerOfFrintezza() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29046}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			c0 := memoNPC(s, strconv.Itoa(gmID))
			if c0 == nil {
				s.GlobalMemo().Set(strconv.Itoa(gmID), int(n.ObjectID()))
			}
			if !n.DBLoaded() {
				n.SetDBValue(0)
				if c1 := memoNPC(s, "4"); c1 != nil {
					s.SendScriptEvent(c1, n.ObjectID(), 0, 0)
				}
				if c1 := memoNPC(s, "6"); c1 != nil {
					s.SendScriptEvent(c1, n.ObjectID(), 0, 0)
				}
			} else if n.DBValue() == 5 {
				s.Spawner().SendMakerEvent("frintessa_evilate_maker1", "1001", 0, 0)
				s.Spawner().SendMakerEvent("frintessa_evilate_maker2", "1001", 0, 0)
				s.StartTimer("3000", n, nil, 1000*time.Millisecond)
			} else if n.DBValue() == 6 {
				s.Spawner().SendMakerEvent("frintessa_evilate_maker1", "1001", 0, 0)
				s.Spawner().SendMakerEvent("frintessa_evilate_maker2", "1001", 0, 0)
				n.RemoveAllDesire()
				n.SetInt(script.SlotIntAI4, 1)
				n.SetInt(npc.IntQuest2, 0)
				n.SetInt(npc.IntQuest3, 0)
				n.SetInt(npc.IntQuest4, 0)
				n.AddCastDesire(n, changeBodySkill(), 1000000)
				n.SetDBValue(6)
				if c0 := memoNPC(s, "4"); c0 != nil {
					s.SendScriptEvent(c0, n.ObjectID(), 6, 0)
				}
				n.SetInt(script.SlotIntAI3, 6)
				s.StartTimer("4000", n, nil, 2000*time.Millisecond)
				hpRatio := n.HPRatio()
				switch {
				case hpRatio >= 0.5:
					n.SetInt(script.SlotIntAI3, 6)
				case hpRatio > 0.2:
					n.SetInt(script.SlotIntAI3, 30010)
				default:
					n.SetInt(script.SlotIntAI3, 30011)
				}
			}
			n.SetRunning(true)
			n.SetInt(script.SlotIntAI0, 0)
			n.SetInt(script.SlotIntAI1, 0)
			n.SetInt(script.SlotIntAI3, 5)
			n.SetInt(script.SlotIntAI4, 0)
			n.SetInt(script.SlotIntAI2, 0)
			n.SetInt(npc.IntQuest0, 0)
			n.SetInt(npc.IntQuest1, s.CurrentTick())
			if n.DBValue() == 5 || n.DBValue() == 6 {
				n.AddWanderDesire(5, 5)
				s.StartTimer("3000", n, nil, 60000*time.Millisecond)
			}
			s.StartTimer("3001", n, nil, 60000*time.Millisecond)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.Int(script.SlotIntAI4) == 1 {
				return
			}
			hinderStrider := skill.Ref{ID: 4258, Level: 1}
			if script.Riding(attacker) && s.AbnormalLevel(attacker, hinderStrider) <= 0 {
				n.AddCastDesire(attacker, hinderStrider, 1000000)
			}
			n.SetInt(npc.IntQuest1, s.CurrentTick())
			hpRatio := n.HPRatio()
			switch {
			case n.DBValue() == 5 && hpRatio < 0.6:
				n.RemoveAllDesire()
				n.SetInt(script.SlotIntAI4, 1)
				n.SetInt(npc.IntQuest2, 0)
				n.SetInt(npc.IntQuest3, 0)
				n.SetInt(npc.IntQuest4, 0)
				n.AddCastDesire(n, changeBodySkill(), 1000000)
				n.SetDBValue(6)
				if c0 := memoNPC(s, "4"); c0 != nil {
					s.SendScriptEvent(c0, n.ObjectID(), 6, 0)
				}
				n.SetInt(script.SlotIntAI3, 6)
				s.StartTimer("4000", n, nil, 2000*time.Millisecond)
			case n.DBValue() == 6 && hpRatio < 0.5 && n.Int(script.SlotIntAI3) == 6:
				if c0 := memoNPC(s, "4"); c0 != nil {
					s.SendScriptEvent(c0, n.ObjectID(), 30010, 0)
				}
				n.SetInt(script.SlotIntAI3, 30010)
			case n.DBValue() == 6 && hpRatio <= 0.2 && n.Int(script.SlotIntAI3) == 30010 && n.Int(script.SlotIntAI4) == 0:
				n.SetDBValue(50000)
				n.SetInt(script.SlotIntAI3, 30011)
				s.BroadcastScriptEvent(n, 0, 50000, 6000)
				n.RemoveAllDesire()
				n.SetInt(script.SlotIntAI4, 1)
				s.StartTimer("2000", n, nil, 10000*time.Millisecond)
			}
			if top := n.TopDesireTarget(); n.GeoPathFailCount() > 10 && top != nil && script.Same(attacker, top) && hpRatio < 1.0 {
				n.TeleportTo(top)
			}
			rank(s, n, attacker, e.Damage*1000)
			if e.Skill == (skill.Ref{ID: 5008, Level: 2}) {
				s.Spawner().SendMakerEvent("frintessa_evilate_maker1", "1001", 0, 0)
				s.Spawner().SendMakerEvent("frintessa_evilate_maker2", "1001", 0, 0)
				if n.DBValue() == 6 {
					n.SetInt(script.SlotIntAI2, 1)
				}
			}
			if fighting(n) {
				doAttack(s, n)
			}
			base.Attacked(s, e)
		},
		OnSpelled: func(s *script.Script, e script.Spelled) {
			n := e.NPC
			if e.Skill == (skill.Ref{ID: 5008, Level: 2}) {
				s.Spawner().SendMakerEvent("frintessa_evilate_maker1", "1001", 0, 0)
				s.Spawner().SendMakerEvent("frintessa_evilate_maker2", "1001", 0, 0)
				if n.DBValue() == 6 {
					n.SetInt(script.SlotIntAI2, 1)
					doAttack(s, n)
				}
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.Arg1 {
			case 4:
				n.SetDBValue(4)
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				s.Spawner().CreateOnePrivateEx(n, 29053, script.Loc{X: 174230, Y: -88012, Z: -5112, Heading: 16384}, 0, script.Params{})
				s.StartTimer("1001", n, nil, 5500*time.Millisecond)
			case 0:
				n.SetDBValue(0)
				n.RemoveAllDesire()
				n.SetInt(npc.IntQuest2, 0)
				n.SetInt(npc.IntQuest3, 0)
				n.SetInt(npc.IntQuest4, 0)
				n.SetHeading(16384)
				n.TeleportTo(script.Loc{X: -105200, Y: -253104, Z: -15264})
			}
			base.ScriptEvent(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1001"):
				s.StartTimer("1002", n, nil, 500*time.Millisecond)
			case strings.EqualFold(e.Name, "1002"):
				n.SetHeading(16384)
				n.TeleportTo(script.Loc{X: 174230, Y: -88012, Z: -5112})
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				s.StartTimer("1003", n, nil, 8000*time.Millisecond)
			case strings.EqualFold(e.Name, "1003"):
				s.StartTimer("1900", n, nil, 10000*time.Millisecond)
			case strings.EqualFold(e.Name, "1900"):
				n.SetDBValue(5)
				n.SetInt(script.SlotIntAI4, 0)
				n.LookNeighbor()
				doAttack(s, n)
				s.StartTimer("3000", n, nil, 60*1000*time.Millisecond)
			case strings.EqualFold(e.Name, "2000"):
				s.StartTimer("2002", n, nil, 2000*time.Millisecond)
			case strings.EqualFold(e.Name, "2002"):
				stepNorth(n)
				s.StartTimer("2003", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2003"):
				stepNorth(n)
				s.StartTimer("2004", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2004"):
				n.AddSocialDesire(2, 84000, 10000000)
				s.StartTimer("2005", n, nil, 7000*time.Millisecond)
			case strings.EqualFold(e.Name, "2005"):
				n.SpecialCamera(script.CameraShot{Distance: 250, Yaw: 180, Pitch: 20, Time: 0, Duration: 10000, Turn: 0, Rise: 6, Widescreen: 1, Unknown: 1})
				s.StartTimer("2006", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2006"):
				n.SpecialCamera(script.CameraShot{Distance: 250, Yaw: 180, Pitch: 20, Time: 0, Duration: 10000, Turn: 0, Rise: 6, Widescreen: 1, Unknown: 1})
				s.StartTimer("2100", n, nil, 400*time.Millisecond)
			case strings.EqualFold(e.Name, "2100"):
				if c4 := memoNPC(s, "6"); c4 != nil {
					c4.SetHeading(16384)
					c4.TeleportTo(n)
					s.SendScriptEvent(c4, 7, 0, 0)
				}
				s.StartTimer("2101", n, nil, 1000*time.Millisecond)
			case strings.EqualFold(e.Name, "2101"):
				n.AddSocialDesire(3, 50000, 10000000)
				s.StartTimer("2102", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2102"):
				n.SpecialCamera(script.CameraShot{Distance: 250, Yaw: 180, Pitch: 20, Time: 0, Duration: 10000, Turn: 0, Rise: 6, Widescreen: 1, Unknown: 1})
				s.StartTimer("2103", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2103"):
				n.SpecialCamera(script.CameraShot{Distance: 250, Yaw: 180, Pitch: 20, Time: 0, Duration: 10000, Turn: 0, Rise: 6, Widescreen: 1, Unknown: 1})
				s.StartTimer("2104", n, nil, 400*time.Millisecond)
			case strings.EqualFold(e.Name, "2104"):
				n.SpecialCamera(script.CameraShot{Distance: 300, Yaw: 220, Pitch: 0, Time: 3000, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2105", n, nil, 3000*time.Millisecond)
			case strings.EqualFold(e.Name, "2105"):
				n.SpecialCamera(script.CameraShot{Distance: 100, Yaw: 150, Pitch: -3, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2106", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2106"):
				n.SpecialCamera(script.CameraShot{Distance: 100, Yaw: 150, Pitch: -3, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2107", n, nil, 400*time.Millisecond)
			case strings.EqualFold(e.Name, "2107"):
				n.SpecialCamera(script.CameraShot{Distance: 100, Yaw: 150, Pitch: 10, Time: 3000, Duration: 5000, Turn: 0, Rise: 15, Widescreen: 1, Unknown: 1})
				s.StartTimer("2108", n, nil, 4000*time.Millisecond)
			case strings.EqualFold(e.Name, "2108"):
				n.SpecialCamera(script.CameraShot{Distance: 300, Yaw: 180, Pitch: 10, Time: 2000, Duration: 2000, Turn: 0, Rise: 5, Widescreen: 1, Unknown: 1})
				s.StartTimer("2109", n, nil, 2000*time.Millisecond)
			case strings.EqualFold(e.Name, "2109"):
				s.GlobalMemo().Remove(strconv.Itoa(gmID))
				n.SetDBValue(0)
				n.DeleteMe()
			case strings.EqualFold(e.Name, "3000"):
				if n.DBValue() == 5 || n.DBValue() == 6 {
					if s.ElapsedTicks(n.Int(npc.IntQuest1)) > 15*60 {
						fallAsleep(s, n)
					} else {
						n.LookNeighbor()
						s.StartTimer("3000", n, nil, 60000*time.Millisecond)
					}
				}
			case strings.EqualFold(e.Name, "4000"):
				n.RemoveAllDesire()
				n.EquipItem(changeWeapon, 0)
				s.StartTimer("4001", n, nil, 3000*time.Millisecond)
			case strings.EqualFold(e.Name, "4001"):
				n.SetInt(script.SlotIntAI4, 0)
				s.StartTimer("3000", n, nil, 1000*time.Millisecond)
			case strings.EqualFold(e.Name, "3001"):
				if fighting(n) {
					doAttack(s, n)
				}
				s.StartTimer("3001", n, nil, 60000*time.Millisecond)
			case strings.EqualFold(e.Name, "8000"):
				doAttack(s, n)
			}
			return base.Timer(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 1 {
				return
			}
			dbValue := n.DBValue()
			if script.ActingPlayer(e.Creature) == nil {
				if s.Rnd(100) < 1 && dbValue == 6 {
					// Anything but a player or its summon is an NPC here;
					// another creature panics.
					s.SendScriptEvent(e.Creature.(*script.NPC), 0, 20000, 0)
					n.RemoveAllDesire()
					n.AddSocialDesire(4, 1500, 10000000)
				}
				return
			}
			if dbValue == 5 || dbValue == 6 {
				rank(s, n, e.Creature, 1*1000)
				doAttack(s, n)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if e.Skill == dashAllVer2() && e.Success && n.Int(script.SlotIntAI2) == 3 {
				n.SetInt(script.SlotIntAI2, 0)
			}
			doAttack(s, n)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if c0 := memoNPC(s, "4"); c0 != nil {
				s.SendScriptEvent(c0, n.ObjectID(), 30011, 0)
			}
			s.GlobalMemo().Remove(strconv.Itoa(gmID))
			n.SetDBValue(0)
			s.StartTimer("2000", n, nil, 500*time.Millisecond)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			dbValue := n.DBValue()
			if (dbValue == 5 || dbValue == 6) && n.Int(script.SlotIntAI4) == 0 {
				n.LookNeighbor()
			} else if dbValue == 0 {
				n.RemoveAllDesire()
			}
		},
	})}
}

// memoNPC returns the NPC the global memo names under key, nil for none. A
// key naming another creature panics.
func memoNPC(s *script.Script, key string) *script.NPC {
	c := s.GlobalMemo().Creature(key)
	if c == nil {
		return nil
	}
	return c.(*script.NPC)
}

// stepNorth asks n to walk 50 north of where it stands.
func stepNorth(n *script.NPC) {
	at := script.LocOf(n)
	n.AddMoveToDesire(script.Loc{X: at.X, Y: at.Y + 50, Z: at.Z}, 10000000)
}

// fighting reports whether n is awake, at stage 5 or 6, with nothing to do
// but stand or wander.
func fighting(n *script.NPC) bool {
	intention := n.Intention()
	return (n.DBValue() == 5 || n.DBValue() == 6) && (intention == script.IntentionIdle || intention == script.IntentionWander)
}

// fallAsleep puts n back to sleep after 15 minutes with no hit.
func fallAsleep(s *script.Script, n *script.NPC) {
	n.SetDBValue(0)
	if c0 := memoNPC(s, "4"); c0 != nil {
		s.SendScriptEvent(c0, n.ObjectID(), 0, 0)
	}
	if c0 := memoNPC(s, "6"); c0 != nil {
		s.SendScriptEvent(c0, n.ObjectID(), 0, 0)
	}
	s.BroadcastScriptEvent(n, 0, 40000, 8000)
	n.RemoveAllDesire()
	s.TeleportPlayersInTerritory(n, script.Loc{X: 150037, Y: -57255, Z: -2976}, 150)
	n.TeleportTo(script.Loc{X: -105200, Y: -253104, Z: -15264})
	n.SetHeading(16384)
	n.EquipItem(changeWeaponStart, 0)
	s.Spawner().SendMakerEvent("frintessa_evilate_maker1", "1000", 0, 0)
	s.Spawner().SendMakerEvent("frintessa_evilate_maker2", "1000", 0, 0)
	n.SetInt(npc.IntQuest2, 0)
	n.SetInt(npc.IntQuest3, 0)
	n.SetInt(npc.IntQuest4, 0)
}

// rank scores c, which hit n for score or came in sight (score 1000), among
// the three creatures n remembers: a remembered one keeps the higher of its
// score and score plus a random 0 to 2999 once score plus 1000 tops it; a
// new one takes the place of the lower of the second and third, or of the
// third and fourth, ties broken at random.
func rank(s *script.Script, n *script.NPC, c script.Creature, score int32) {
	q2, q3, q4 := n.Int(npc.IntQuest2), n.Int(npc.IntQuest3), n.Int(npc.IntQuest4)
	i1 := 0
	switch {
	case script.Same(c, n.Slot(npc.CreatureQuest2)):
		if q2 < score+1000 {
			n.SetInt(npc.IntQuest2, score+int32(s.Rnd(3000)))
		}
	case script.Same(c, n.Slot(npc.CreatureQuest3)):
		if q3 < score+1000 {
			n.SetInt(npc.IntQuest3, score+int32(s.Rnd(3000)))
		}
	case script.Same(c, n.Slot(npc.CreatureQuest4)):
		if q4 < score+1000 {
			n.SetInt(npc.IntQuest4, score+int32(s.Rnd(3000)))
		}
	case q2 > q3:
		i1 = 3
	case q2 == q3:
		i1 = pick(s, 2, 3)
	case q2 < q3:
		i1 = 2
	}
	switch i1 {
	case 2:
		switch {
		case q2 > q4:
			i1 = 4
		case q2 == q4:
			i1 = pick(s, 2, 4)
		case q2 < q4:
			i1 = 2
		}
	case 3:
		switch {
		case q3 > q4:
			i1 = 4
		case q3 == q4:
			i1 = pick(s, 3, 4)
		case q3 < q4:
			i1 = 3
		}
	}
	switch i1 {
	case 2:
		n.SetInt(npc.IntQuest2, score+int32(s.Rnd(3000)))
		n.SetSlot(npc.CreatureQuest2, c)
	case 3:
		n.SetInt(npc.IntQuest3, score+int32(s.Rnd(3000)))
		n.SetSlot(npc.CreatureQuest3, c)
	case 4:
		n.SetInt(npc.IntQuest4, score+int32(s.Rnd(3000)))
		n.SetSlot(npc.CreatureQuest4, c)
	}
}

// pick returns a or b, at even odds.
func pick(s *script.Script, a, b int) int {
	if s.RndBool() {
		return a
	}
	return b
}

// doAttack wants n to use a skill on the best scored of the creatures it
// remembers: one that is gone, dead or over 5000 away scores 0, and one
// time in 20 the best one's score drops to 500.
func doAttack(s *script.Script, n *script.NPC) {
	for _, q := range []struct {
		score npc.IntSlot
		who   npc.CreatureSlot
	}{{npc.IntQuest2, npc.CreatureQuest2}, {npc.IntQuest3, npc.CreatureQuest3}, {npc.IntQuest4, npc.CreatureQuest4}} {
		if c := n.Slot(q.who); c == nil || n.Distance3D(c) > 5000 || c.Dead() {
			n.SetInt(q.score, 0)
		}
	}
	var i1 npc.IntSlot
	var i2 int32
	var c2 script.Creature
	if n.Int(npc.IntQuest2) > n.Int(npc.IntQuest3) {
		i1, i2, c2 = npc.IntQuest2, n.Int(npc.IntQuest2), n.Slot(npc.CreatureQuest2)
	} else {
		i1, i2, c2 = npc.IntQuest3, n.Int(npc.IntQuest3), n.Slot(npc.CreatureQuest3)
	}
	if n.Int(npc.IntQuest4) > i2 {
		i1, i2, c2 = npc.IntQuest4, n.Int(npc.IntQuest4), n.Slot(npc.CreatureQuest4)
	}
	if i2 <= 0 {
		return
	}
	if s.Rnd(100) < 5 {
		n.SetInt(i1, 500)
	}
	if c2 == nil {
		return
	}
	switch n.DBValue() {
	case 5:
		// One time in five the first form would charge every creature
		// around from its no-charge territory; it does nothing then.
		if s.Rnd(10000) >= 2000 {
			if s.Rnd(10000) < 500 {
				n.AddCastDesireUnchecked(c2, antiGravity(), 10000)
			} else {
				n.AddCastDesireUnchecked(c2, swingVer1(), 10000)
			}
		}
	case 6:
		switch {
		case n.Int(script.SlotIntAI2) > 0:
			dash(n, c2, dashVer2(), dashAllVer2())
		case n.HPRatio() > 0.5:
			// One time in five it would charge from its no-charge
			// territory; it does nothing then.
			if s.Rnd(10000) >= 2000 {
				switch {
				case s.Rnd(10000) < 500:
					n.SetInt(script.SlotIntAI2, 1)
					dash(n, c2, dashVer2(), dashAllVer2())
				case s.Rnd(10000) < 500:
					n.AddCastDesireUnchecked(c2, antiGravity(), 10000)
				case s.Rnd(10000) < 500:
					n.AddCastDesireUnchecked(c2, magicCircle(), 10000)
				default:
					n.AddCastDesireUnchecked(c2, swingVer2(), 10000)
				}
			}
		default:
			if s.Rnd(10000) >= 2000 {
				switch {
				case s.Rnd(10000) < 1500:
					n.SetInt(script.SlotIntAI2, 1)
					dash(n, c2, dashVer2(), dashAllVer2())
				case s.Rnd(10000) < 1500:
					n.AddCastDesireUnchecked(c2, antiGravity(), 10000)
				case s.Rnd(10000) < 1000:
					n.AddCastDesireUnchecked(c2, magicCircle(), 10000)
				default:
					n.AddCastDesireUnchecked(c2, swingVer2(), 10000)
				}
			}
		}
	}
}

// dash runs n's dash chain on c: the first two steps dash at c with step,
// the third dashes at every creature around it with all and ends the chain.
func dash(n *script.NPC, c script.Creature, step, all skill.Ref) {
	switch ai2 := n.Int(script.SlotIntAI2); {
	case ai2 > 0 && ai2 < 3:
		n.AddCastDesireUnchecked(c, step, 10000)
		n.SetInt(script.SlotIntAI2, ai2+1)
	case ai2 == 3:
		n.AddCastDesireUnchecked(c, all, 10000)
		n.SetInt(script.SlotIntAI2, 0)
	default:
		n.SetInt(script.SlotIntAI2, 0)
	}
}
