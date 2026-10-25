package benom

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
)

// Benom's script memory.
const (
	slotSieged   = script.SlotIntAI0 // the first argument of 10100: 1 once a siege moved it
	slotHomeX    = script.SlotIntAI1 // where it stood at its last 1001 check
	slotHomeY    = script.SlotIntAI2
	slotHomeZ    = script.SlotIntAI3
	slotTargeted = npc.IntQuest0 // players it marked to send away together
	slotHunting  = npc.IntQuest1 // 1 once a siege moved it: it hunts every playable it sees
)

// throne is where a siege moves Benom, and where it goes back to when it
// strays out of its territory.
func throne() script.Loc { return script.Loc{X: 11563, Y: -49152, Z: -537} }

// targetTeleports are the points Benom's cancels send a creature to; each
// heading is the scatter of the landing around its point.
func targetTeleports() []script.Loc {
	return []script.Loc{
		{X: 12860, Y: -49158, Z: -976, Heading: 650},
		{X: 14878, Y: -51339, Z: 1024, Heading: 100},
		{X: 15674, Y: -49970, Z: 864, Heading: 100},
		{X: 15696, Y: -48326, Z: 864, Heading: 100},
		{X: 14873, Y: -46956, Z: 1024, Heading: 100},
		{X: 12157, Y: -49135, Z: -1088, Heading: 650},
		{X: 12875, Y: -46392, Z: -288, Heading: 200},
		{X: 14087, Y: -46706, Z: -288, Heading: 200},
		{X: 14086, Y: -51593, Z: -288, Heading: 200},
		{X: 12864, Y: -51898, Z: -288, Heading: 200},
		{X: 15538, Y: -49153, Z: -1056, Heading: 200},
		{X: 17001, Y: -49149, Z: -1064, Heading: 650},
	}
}

// sendAway teleports c to one of the target points drawn at random. A
// player lands scattered around it by the point's heading; any other
// creature lands on the point itself (#4210).
func sendAway(s *script.Script, c script.Creature) {
	points := targetTeleports()
	loc := points[s.Rnd(len(points))]
	if p, ok := c.(*script.Player); ok {
		p.TeleportTo(loc, loc.Heading)
		return
	}
	script.TeleportTo(c, loc)
}

// castByType has n want the skill its template lists under t on target at
// 1000000, without the reuse and cost checks unless checked; nothing when
// it lists none.
func castByType(n *script.NPC, target script.Creature, t npc.SkillType, checked bool) {
	k, ok := n.SkillByType(t)
	if !ok {
		return
	}
	if checked {
		n.AddCastDesire(target, k, 1000000)
		return
	}
	n.AddCastDesireUnchecked(target, k, 1000000)
}

// isSkill reports whether sk is the skill n's template lists under t.
func isSkill(n *script.NPC, sk skill.Ref, t npc.SkillType) bool {
	k, ok := n.SkillByType(t)
	return ok && sk == k
}

// Benom is the raid boss of Rune castle's dungeon. It appears with a shout
// and never wanders on its own. A siege that leaves the castle short of
// life towers (script event 10100) moves it to its throne: from then on it
// hunts every playable it sees, remembers where it stands 5 s later, and
// every 2 minutes, strayed out of its territory and out of a fight, goes
// back to its throne on an even draw, one time in five reshuffling its
// hate. It hates by the damage it takes over its HP. Moved by a siege, a
// hit may make it want its teleport cancels, the second only under a third
// of its HP, and a player it sees may be marked, up to ten; its cancels
// send their target away and drop its hate, the second also sending away
// again for each marked creature near the target. It may want its magic on
// an attacker over 300 away and its physical special on any. Its Instant
// Move after a siege brings it back where it last stood. Dying, it shouts,
// and leaves the exit teleporter unless a siege moved it.
func Benom() script.Script {
	base := raid.BossStandardHooks()
	targets := &script.ListCell[script.Creature]{}
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29054}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(*script.Script, script.NoDesire) {
			// Benom stays where it stands.
		},
		OnCreated: func(_ *script.Script, e script.Created) {
			n := e.NPC
			n.Shout(script.NpcString(1010623))

			targets.Clear()

			n.SetInt(slotTargeted, 0)
			n.SetInt(slotHunting, 0)

			at := script.LocOf(n)
			n.SetInt(slotSieged, 0)
			n.SetInt(slotHomeX, int32(at.X))
			n.SetInt(slotHomeY, int32(at.Y))
			n.SetInt(slotHomeZ, int32(at.Z))
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1001") {
				at := script.LocOf(n)
				n.SetInt(slotHomeX, int32(at.X))
				n.SetInt(slotHomeY, int32(at.Y))
				n.SetInt(slotHomeZ, int32(at.Z))
			} else if strings.EqualFold(e.Name, "1002") {
				if !n.InMyTerritory() && s.Rnd(2) < 1 && n.Intention() != script.IntentionAttack {
					n.CleanAllHate()
					n.TeleportTo(throne())
					n.AddMoveToDesire(throne(), 1000000)
				}

				if s.Rnd(5) < 1 {
					n.RandomizeAttack()
				}

				s.StartTimer("1002", n, nil, 120000*time.Millisecond)
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				n.AddAttackDesire(attacker, float64(e.Damage)/float64(n.MaxHP())/0.05*2000)
			}

			if n.Int(slotSieged) == 1 {
				if s.Rnd(25*25) < 1 {
					castByType(n, attacker, npc.SkillTypeSelfRangeCancelA1, false)
				}

				if n.HPRatio() < 0.333 && s.Rnd(25*25) < 1 {
					castByType(n, attacker, npc.SkillTypeSelfRangeCancelA2, false)
				}
			}

			if n.Distance2D(attacker) > 300 && s.Rnd(25*10) < 1 {
				castByType(n, attacker, npc.SkillTypeDDMagic, true)
			}

			if s.Rnd(25*10) < 1 {
				castByType(n, attacker, npc.SkillTypePhysicalSpecial, true)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}

			if n.Int(slotHunting) == 1 {
				n.AddAttackDesire(c, 200)
			}

			if _, ok := c.(*script.Player); ok && n.Int(slotSieged) == 1 && n.Int(slotTargeted) < 10 && s.Rnd(3) < 1 && targets.Index(c) < 0 {
				targets.Add(c)

				n.SetInt(slotTargeted, n.Int(slotTargeted)+1)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n, c := e.NPC, e.Creature
			switch {
			case e.Skill.ID == 4222 && n.Int(slotSieged) == 1:
				n.TeleportTo(script.Loc{X: int(n.Int(slotHomeX)), Y: int(n.Int(slotHomeY)), Z: int(n.Int(slotHomeZ))})
				n.CleanAllHate()
			case isSkill(n, e.Skill, npc.SkillTypeSelfRangeCancelA1):
				sendAway(s, c)

				n.StopHate(c)
			case isSkill(n, e.Skill, npc.SkillTypeSelfRangeCancelA2):
				sendAway(s, c)

				n.StopHate(c)

				if n.Int(slotTargeted) > 0 {
					for _, marked := range targets.Items() {
						if !script.Same(marked, c) && script.InRadius3D(c, marked, 250) {
							// The marked creature's hate drops, but the target
							// is the one sent away again.
							sendAway(s, c)

							n.StopHate(marked)
						}
					}
				}

				n.SetInt(slotTargeted, 0)

				targets.Clear()

				n.LookNeighbor()
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.EventID == 10100 {
				n := e.NPC
				n.TeleportTo(throne())

				n.SetInt(slotSieged, e.Arg1)
				n.SetInt(slotHunting, 1)

				s.StartTimer("1002", n, nil, 120000*time.Millisecond)
				s.StartTimer("1001", n, nil, 5000*time.Millisecond)
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			targets.Clear()

			n := e.NPC
			n.Shout(script.NpcString(1010626))

			if n.Int(slotSieged) == 0 {
				s.Spawner().CreateOnePrivateEx(n, 29055, script.Loc{X: 12589, Y: -49044, Z: -3008}, 0, script.Params{})
			}
		},
	})}
}
