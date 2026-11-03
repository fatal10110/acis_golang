package hall

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// standDir is the page directory of the Partisan Hideaway's standing
// guards.
const standDir = "ai/siegeablehall/Agit01Stand1"

// The guards of Bloody Lord Nurka's escort: the swordsmen and the archers.
const (
	guardSwordID = 35377
	guardBowID   = 35378
)

// The number of posts of each kind of the escort.
const (
	swordPosts = 8
	bowPosts   = 4
)

// swordPost and bowPost are post i of the escort's swordsmen and archers:
// the offset from the master it first walks to, then the two points it
// walks to as the master's flag reaches 1 and then any other value.
func swordPost(i int) [8]int {
	return [swordPosts][8]int{
		{40, 0, 45842, 109981, -1970, 44565, 108867, -2020},
		{28, 28, 45830, 110009, -1970, 44553, 108895, -2020},
		{0, 28, 45802, 110009, -1970, 44525, 108895, -2020},
		{-28, 28, 45774, 110009, -1970, 44497, 108895, -2020},
		{-40, 0, 45762, 109981, -1970, 44485, 108867, -2020},
		{-28, -28, 45774, 109953, -1970, 44497, 108839, -2020},
		{0, -40, 45802, 109941, -1970, 44525, 108827, -2020},
		{28, -28, 45830, 109953, -1970, 44553, 108839, -2020},
	}[i]
}

func bowPost(i int) [8]int {
	return [bowPosts][8]int{
		{10, 17, 45812, 109998, -1970, 44535, 108884, -2020},
		{-10, 17, 45792, 109998, -1970, 44515, 108884, -2020},
		{-10, -17, 45792, 109964, -1970, 44515, 108850, -2020},
		{10, -17, 45812, 109964, -1970, 44535, 108850, -2020},
	}[i]
}

// stand1Hooks are Agit01Stand1's hooks, which the escort derives its own
// from: the guard holds its ground and fights the playables it sees, at
// weight 200, those that hit it or a clan mate, by the damage, and those
// whose spells it sees, by the spell's aggro.
func stand1Hooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Attacker.Playable() {
				e.NPC.AddAttackDesireHold(e.Attacker, damageWeight(e.NPC, e.Damage, 100))
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() {
				e.Called.AddAttackDesireHold(e.Attacker, damageWeight(e.Called, e.Damage, 100))
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			e.NPC.AddAttackDesireHold(e.Creature, 200)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 {
				e.NPC.AddAttackDesireHold(e.Caster, damageWeight(e.NPC, int32(aggro), 150))
			}
		},
	}).Named("Agit01Stand1")
}

// Stand1 is Agit01Stand1: a Partisan Hideaway guard that fights from where
// it stands the playables it sees, those that hit it or a clan mate, and
// those whose spells it sees.
func Stand1() script.Script {
	return script.Script{Dir: standDir, Behavior: true, NPCs: []int32{35370, 35371, 35374}, Hooks: stand1Hooks()}
}

// Stand2 is Agit01Stand2: an Agit01Stand1 guard of Bloody Lord Nurka's
// escort. As it appears it takes the first free post of its kind, and
// every 10 seconds, while it has a master, it is meant to walk to its
// post by the master's flag. The kind test there compares with the
// archer's id, so each guard looks itself up among the other kind's
// posts, holds none, and its check ends there: no guard of the escort
// ever walks to a post. Dying frees its post.
func Stand2() script.Script {
	base := stand1Hooks()
	swordGuards := &script.ListCell[int32]{}
	swordGuards.Add(make([]int32, swordPosts)...)
	bowGuards := &script.ListCell[int32]{}
	bowGuards.Add(make([]int32, bowPosts)...)
	return script.Script{Dir: standDir, Behavior: true, NPCs: []int32{guardSwordID, guardBowID}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.NpcID() == guardSwordID {
				swordGuards.ReplaceFirst(0, n.ObjectID())
			}
			if n.NpcID() == guardBowID {
				bowGuards.ReplaceFirst(0, n.ObjectID())
			}
			n.SetInt(script.SlotIntAI0, 0)
			s.StartTimerAtFixedRate("2001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if n.HasMaster() {
					isSwordGuard := n.NpcID() == guardBowID
					guards := bowGuards
					if isSwordGuard {
						guards = swordGuards
					}
					i := guards.Index(n.ObjectID())
					if i < 0 {
						// The other kind's posts never hold this guard:
						// the check ends here, with no walk and no
						// parent timer hook.
						return ""
					}
					post := bowPost(i)
					if isSwordGuard {
						post = swordPost(i)
					}
					master := n.Master()
					switch flag := master.Int(script.SlotFlag); {
					case flag == 0:
						if n.Int(script.SlotIntAI0) < 1 {
							at := script.LocOf(master)
							n.AddMoveToDesire(script.Loc{X: at.X + post[0], Y: at.Y + post[1], Z: at.Z}, 100000000)
							n.SetInt(script.SlotIntAI0, 1)
						}
					case flag == 1:
						if n.Int(script.SlotIntAI0) < 2 {
							n.AddMoveToDesire(script.Loc{X: post[2], Y: post[3], Z: post[4]}, 100000000)
							n.SetInt(script.SlotIntAI0, 2)
						}
					case n.Int(script.SlotIntAI0) < 3:
						n.AddMoveToDesire(script.Loc{X: post[5], Y: post[6], Z: post[7]}, 100000000)
						n.SetInt(script.SlotIntAI0, 3)
					}
				}
			}
			return base.Timer(s, e)
		},
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			n := e.NPC
			if n.NpcID() == guardSwordID {
				swordGuards.ReplaceFirst(n.ObjectID(), 0)
			}
			if n.NpcID() == guardBowID {
				bowGuards.ReplaceFirst(n.ObjectID(), 0)
			}
		},
	})}
}
