package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyPrivateSplit is PartyPrivateSplit: a split-off PartyPrivateWarrior
// that takes up the creature it was spawned against, strikes it with one
// of its two physical special skills when it sees it, and attacks it when
// it is a player or a summon. A split whose master died leaves the world
// three minutes later unless it is fighting.
func PartyPrivateSplit() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22087, 22093}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			c0 := s.Object(n.Int(script.SlotParam2))
			n.SetSlot(script.SlotCreature0, c0)
			if c0 != nil {
				n.SetTopDesireTarget(c0)
			}
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if script.Same(c, n.TopDesireTarget()) {
				if s.Rnd(100) < 50 {
					castByType(n, c, npc.SkillTypePhysicalSpecial1)
				} else {
					castByType(n, c, npc.SkillTypePhysicalSpecial2)
				}
				if !c.Playable() {
					return
				}
				defaultnpc.TryToAttack(s, n, c)
			}
			base.SeeCreature(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1006"):
				if master := n.Master(); master == nil || master.Dead() {
					if i := n.Intention(); i != script.IntentionAttack && i != script.IntentionCast {
						n.DeleteMe()
						return ""
					}
					s.StartTimer("1006", n, e.Player, 180000*time.Millisecond)
				}
			case strings.EqualFold(e.Name, "1004"):
				s.StartTimer("1006", n, e.Player, 180000*time.Millisecond)
			case strings.EqualFold(e.Name, "1001"):
				// A split walks no route: it is a silhouette its leader puts
				// down and nothing in its chain gives it one, so idling and
				// wandering are all the peaceful intentions it can hold.
				switch i := n.Intention(); {
				case (i == script.IntentionIdle || i == script.IntentionWander) && n.HPRatio() > 0.4 && !n.Dead():
					moveAroundSocial := n.IntParam("MoveAroundSocial")
					moveAroundSocial1 := n.IntParam("MoveAroundSocial1")
					moveAroundSocial2 := n.IntParam("MoveAroundSocial2")
					if moveAroundSocial > 0 || moveAroundSocial1 > 0 || moveAroundSocial2 > 0 {
						if moveAroundSocial2 > 0 && s.Rnd(100) < 20 {
							n.AddSocialDesire(3, int(moveAroundSocial2*1000/30), 50)
						} else if moveAroundSocial1 > 0 && s.Rnd(100) < 20 {
							n.AddSocialDesire(2, int(moveAroundSocial1*1000/30), 50)
						} else if moveAroundSocial > 0 && s.Rnd(100) < 20 {
							n.AddSocialDesire(1, int(moveAroundSocial*1000/30), 50)
						}
					}
					if shoutMsg2 := n.IntParam("ShoutMsg2"); shoutMsg2 > 0 && s.Rnd(1000) < 17 {
						shoutOrSay(n, shoutMsg2)
					}
				case i == script.IntentionAttack:
					if shoutMsg3 := n.IntParam("ShoutMsg3"); shoutMsg3 > 0 && s.Rnd(100) < 10 {
						shoutOrSay(n, shoutMsg3)
					}
				}
				s.StartTimer("1001", n, e.Player, 10000*time.Millisecond)
			case strings.EqualFold(e.Name, "1"):
				if n.IntParam("AttackLowLevel") == 1 {
					n.LookNeighbor()
				}
			case strings.EqualFold(e.Name, "2"):
				if n.IntParam("IsVs") == 1 {
					n.SetSlot(script.SlotCreature0, n)
				}
			}
			return base.Timer(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if !script.Same(caller, called) && script.Same(caller, called.Master()) {
				s.StartTimer("1004", called, nil, 180000*time.Millisecond)
			}
		},
	})}
}

// shoutOrSay has n shout the NpcString id, or say it when its IsSay AI
// parameter is set.
func shoutOrSay(n *script.NPC, id int32) {
	if n.IntParam("IsSay") == 0 {
		n.Shout(script.NpcString(id))
	} else {
		n.Say(script.NpcString(id))
	}
}

// partyPrivateCoupleFollowerDir is the directory of PartyPrivateCoupleFollower
// and its children.
const partyPrivateCoupleFollowerDir = "ai/individual/Monster/WarriorBase/Warrior/PartyPrivateWarrior/PartyPrivateCoupleFollower"

// PartyPrivateCoupleFollower is PartyPrivateCoupleFollower: a
// PartyPrivateWarrior that, the first time a party member dies while its
// master is gone or dead and it is above 70% of its HP, puts three of its
// silhouettes around itself against the creature it most wants to act on.
// It binds no NPC of its own.
func PartyPrivateCoupleFollower() script.Script {
	return script.Script{Dir: partyPrivateCoupleFollowerDir, Behavior: true, Hooks: PartyPrivateCoupleFollowerHooks()}
}

// PartyPrivateCoupleFollowerHooks are PartyPrivateCoupleFollower's hooks,
// which its children derive theirs from.
func PartyPrivateCoupleFollowerHooks() script.Hooks {
	base := PartyPrivateHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			called := e.Called
			if master := called.Master(); (master == nil || master.Dead()) && called.Int(script.SlotIntAI0) == 0 && called.HPRatio() > 0.7 {
				if topDesireTarget := called.TopDesireTarget(); topDesireTarget != nil {
					silhouette := called.IntParam("silhouette")
					at := script.LocOf(called)
					x, y, z := at.X, at.Y, at.Z
					params := script.Params{P1: 1000, P2: topDesireTarget.ObjectID(), P3: 0}
					spawner := s.Spawner()
					spawner.CreateOnePrivateEx(called, silhouette, script.Loc{X: x + 10, Y: y, Z: z}, 0, params)
					spawner.CreateOnePrivateEx(called, silhouette, script.Loc{X: x, Y: y + 10, Z: z}, 0, params)
					spawner.CreateOnePrivateEx(called, silhouette, script.Loc{X: x + 5, Y: y + 5, Z: z}, 0, params)
					called.SetInt(script.SlotIntAI0, 1)
				}
			}
		},
	}).Named("PartyPrivateCoupleFollower")
}
