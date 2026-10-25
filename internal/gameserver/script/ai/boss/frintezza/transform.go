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

// transformGMID is the global memo key that holds the transform's object id.
const transformGMID = 6

// The transform's own skills; it also swings 5014 level 1 and casts the
// anti-gravity 5016 level 1 of the follower.
func dashVer3() skill.Ref     { return skill.Ref{ID: 5015, Level: 3} }
func dashAllVer3() skill.Ref  { return skill.Ref{ID: 5015, Level: 6} }
func magicCircle2() skill.Ref { return skill.Ref{ID: 5018, Level: 2} }
func vampiric() skill.Ref     { return skill.Ref{ID: 5019, Level: 1} }

// FollowerOfFrintezzaTransform is Scarlet van Halisha's transform (29047),
// which the follower calls onto its place as it leaves. Global memo key 6
// names it; it tells Frintezza (key 4) how it fares by script events
// carrying its object id as the event id: hit, 30021 below 75% of its HP,
// 30024 at 75% or more. Spawned with no row it tells Frintezza 0 and,
// through key 6, sends itself to sleep at db value 0; restored at db value
// 7 it tells Frintezza 7 (30021 below 75%) and starts.
//
// Script event 7, from the follower, starts it 2 s later: it tells
// Frintezza 7, 6 s after that takes a social pose, and 7 s after the pose
// runs maker event 1000 on both evil spirit makers, tells the NPCs within
// 8000 arg1 20000 and fights. Every minute, 15 minutes with no hit sends it
// back to its sleeping point at db value 0, telling Frintezza and the
// follower (key 5) so and sending the players in its territory out of the
// tomb. Arg1 0 sends it to sleep at once. Dying, it gives up its memo key,
// tells the NPCs within 8000 event 8 and films its death in three shots.
//
// It fights as the follower does, the hardest of the last three creatures
// that hit or met it, with odds that change as its HP drops.
func FollowerOfFrintezzaTransform() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29047}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			c0 := memoNPC(s, strconv.Itoa(transformGMID))
			if c0 == nil {
				s.GlobalMemo().Set(strconv.Itoa(transformGMID), int(n.ObjectID()))
			}
			if !n.DBLoaded() {
				n.SetDBValue(0)
				if c1 := memoNPC(s, "4"); c1 != nil {
					s.SendScriptEvent(c1, n.ObjectID(), 0, 0)
				}
				n.SetDBValue(5)
				if c1 := memoNPC(s, "6"); c1 != nil {
					s.SendScriptEvent(c1, n.ObjectID(), 0, 0)
				}
			} else if n.DBValue() == 7 {
				stage := int32(7)
				if n.HPRatio() < 0.75 {
					stage = 30021
				}
				if c0 := memoNPC(s, "4"); c0 != nil {
					s.SendScriptEvent(c0, n.ObjectID(), stage, 0)
				}
				n.SetInt(script.SlotIntAI4, stage)
				s.StartTimer("1001", n, nil, 2000*time.Millisecond)
			}
			n.SetInt(script.SlotIntAI1, 0)
			n.SetInt(script.SlotIntAI3, 0)
			n.SetInt(script.SlotIntAI4, 0)
			n.SetInt(script.SlotIntAI2, 0)
			n.SetInt(npc.IntQuest0, 0)
			n.SetInt(npc.IntQuest1, s.CurrentTick())
			if n.Int(script.SlotIntAI0) == 7 {
				s.StartTimer("3000", n, nil, 60000*time.Millisecond)
			}
			s.StartTimer("3001", n, nil, 60000*time.Millisecond)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			n.SetInt(npc.IntQuest1, s.CurrentTick())
			stage := int32(30024)
			if n.HPRatio() < 0.75 {
				stage = 30021
			}
			if c0 := memoNPC(s, "4"); c0 != nil {
				s.SendScriptEvent(c0, n.ObjectID(), stage, 0)
			}
			n.SetInt(script.SlotIntAI4, stage)
			hinderStrider := skill.Ref{ID: 4258, Level: 1}
			if script.Riding(attacker) && s.AbnormalLevel(attacker, hinderStrider) <= 0 {
				n.AddCastDesire(attacker, hinderStrider, 1000000)
			}
			rank(s, n, attacker, e.Damage*1000)
			if e.Skill == (skill.Ref{ID: 5008, Level: 2}) {
				// The dash chain's first step, at the attacker.
				n.SetInt(script.SlotIntAI2, 1)
				n.AddCastDesire(attacker, dashVer3(), 10000)
				n.SetInt(script.SlotIntAI2, 2)
			}
			transformAttack(s, n)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 0 || script.ActingPlayer(e.Creature) == nil {
				return
			}
			rank(s, n, e.Creature, 1*1000)
			transformAttack(s, n)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if e.Skill == dashAllVer3() && e.Success && n.Int(script.SlotIntAI2) == 3 {
				n.SetInt(script.SlotIntAI2, 0)
			}
			transformAttack(s, n)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID == 7 {
				s.StartTimer("1001", n, nil, 2000*time.Millisecond)
				s.StartTimer("1001", n, nil, 7000*time.Millisecond)
			} else if e.Arg1 == 0 {
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
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				if c3 := memoNPC(s, "4"); c3 != nil {
					s.SendScriptEvent(c3, n.ObjectID(), 7, 0)
				}
				s.StartTimer("1900", n, nil, 2000*time.Millisecond)
			case strings.EqualFold(e.Name, "1900"):
				s.StartTimer("5001", n, nil, 4000*time.Millisecond)
			case strings.EqualFold(e.Name, "5001"):
				n.SetInt(script.SlotIntAI0, 30020)
				n.AddSocialDesire(2, 5000, 10000000)
				s.StartTimer("5002", n, nil, 5000*time.Millisecond)
			case strings.EqualFold(e.Name, "5002"):
				n.LookNeighbor()
				transformAttack(s, n)
				s.StartTimer("5004", n, nil, 2000*time.Millisecond)
			case strings.EqualFold(e.Name, "5004"):
				s.Spawner().SendMakerEvent("frintessa_evilate_maker1", "1000", 0, 0)
				s.Spawner().SendMakerEvent("frintessa_evilate_maker2", "1000", 0, 0)
				s.BroadcastScriptEvent(n, 0, 20000, 8000)
				n.SetInt(script.SlotIntAI0, 7)
				n.SetInt(script.SlotIntAI4, 1)
				transformAttack(s, n)
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				s.StartTimer("3000", n, nil, 60000*time.Millisecond)
			case strings.EqualFold(e.Name, "2000"):
				n.SpecialCamera(script.CameraShot{Distance: 430, Yaw: 300, Pitch: 80, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2001", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2001"):
				n.PlayMusic("BS05_D")
				n.SpecialCamera(script.CameraShot{Distance: 430, Yaw: 300, Pitch: 80, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2002", n, nil, 400*time.Millisecond)
			case strings.EqualFold(e.Name, "2002"):
				n.SpecialCamera(script.CameraShot{Distance: 0, Yaw: 180, Pitch: 80, Time: 4000, Duration: 6000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				// The timer it starts has no branch of its own.
				s.StartTimer("2003", n, nil, 6000*time.Millisecond)
			case strings.EqualFold(e.Name, "3000"):
				if n.Int(script.SlotIntAI0) == 7 {
					if s.ElapsedTicks(n.Int(npc.IntQuest1)) > 15*60 {
						n.SetDBValue(0)
						if c0 := memoNPC(s, "4"); c0 != nil {
							s.SendScriptEvent(c0, n.ObjectID(), 0, 0)
						}
						if c0 := memoNPC(s, "5"); c0 != nil {
							s.SendScriptEvent(c0, n.ObjectID(), 0, 0)
						}
						n.RemoveAllDesire()
						s.TeleportPlayersInTerritory(n, script.Loc{X: 150037, Y: -57255, Z: -2976}, 150)
						n.SetHeading(16384)
						n.TeleportTo(script.Loc{X: -105200, Y: -253104, Z: -15264})
						n.SetInt(npc.IntQuest2, 0)
						n.SetInt(npc.IntQuest3, 0)
						n.SetInt(npc.IntQuest4, 0)
					} else {
						n.LookNeighbor()
						s.StartTimer("3000", n, nil, 60000*time.Millisecond)
					}
				}
			case strings.EqualFold(e.Name, "3001"):
				if n.Int(script.SlotIntAI0) == 7 {
					transformAttack(s, n)
				}
				n.LookNeighbor()
				s.StartTimer("3001", n, nil, 60000*time.Millisecond)
			}
			return base.Timer(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			s.GlobalMemo().Remove(strconv.Itoa(transformGMID))
			s.BroadcastScriptEvent(n, n.ObjectID(), 8, 8000)
			s.StartTimer("2000", n, nil, 1*time.Millisecond)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.LookNeighbor()
		},
	})}
}

// transformAttack wants n to use a skill on the best scored of the
// creatures it remembers, as doAttack does for the follower: one that is
// gone, dead or over 5000 away on the ground scores 0, and one time in 20
// the best one's score drops to 500. A chain under way dashes on; else one
// time in five it would charge every creature around from its no-charge
// territory and does nothing, and past that its HP band's odds choose,
// the swing when none matches.
func transformAttack(s *script.Script, n *script.NPC) {
	for _, q := range []struct {
		score npc.IntSlot
		who   npc.CreatureSlot
	}{{npc.IntQuest2, npc.CreatureQuest2}, {npc.IntQuest3, npc.CreatureQuest3}, {npc.IntQuest4, npc.CreatureQuest4}} {
		if c := n.Slot(q.who); c == nil || n.Distance2D(c) > 5000 || c.Dead() {
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
	if n.Int(script.SlotIntAI2) > 0 {
		dash(n, c2, dashVer3(), dashAllVer3())
		return
	}
	// The odds of 10000, per HP band, of the dash chain, the anti-gravity,
	// the magic circle and the vampiric touch, each drawn in turn.
	var odds [4]int
	switch hpRatio := n.HPRatio(); {
	case hpRatio > 0.75:
		odds = [4]int{1000, 1000, 0, 0}
	case hpRatio > 0.5:
		odds = [4]int{1000, 750, 500, 500}
	case hpRatio > 0.25:
		odds = [4]int{1000, 750, 1000, 500}
	case hpRatio > 0.1:
		odds = [4]int{1000, 1000, 1000, 1000}
	default:
		odds = [4]int{1000, 500, 500, 0}
	}
	switch {
	case s.Rnd(10000) < 2000:
	case s.Rnd(10000) < odds[0]:
		n.SetInt(script.SlotIntAI2, 1)
		dash(n, c2, dashVer3(), dashAllVer3())
	case s.Rnd(10000) < odds[1]:
		n.AddCastDesireUnchecked(c2, antiGravity(), 10000)
	case s.Rnd(10000) < odds[2]:
		n.AddCastDesireUnchecked(c2, magicCircle2(), 10000)
	case s.Rnd(10000) < odds[3]:
		n.AddCastDesireUnchecked(c2, vampiric(), 10000)
	default:
		n.AddCastDesireUnchecked(c2, swingVer1(), 10000)
	}
}
