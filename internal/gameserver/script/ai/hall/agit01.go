package hall

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The page directories of the Partisan Hideaway's soldiers and healers.
const (
	move1Dir  = "ai/siegeablehall/Agit01Move1"
	healerDir = "ai/siegeablehall/Agit01PartisanHealer1"
)

// move1Hooks are Agit01Move1's hooks, which the Partisan sorcerer derives
// its own from: the soldier fights the playables it sees, at weight 200,
// those that hit it or a clan mate, by the damage, and those whose spells
// it sees, by the spell's aggro.
func move1Hooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Attacker.Playable() {
				e.NPC.AddAttackDesire(e.Attacker, damageWeight(e.NPC, e.Damage, 100))
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() {
				e.Called.AddAttackDesire(e.Attacker, damageWeight(e.Called, e.Damage, 100))
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			e.NPC.AddAttackDesire(e.Creature, 200)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 {
				e.NPC.AddAttackDesire(e.Caster, damageWeight(e.NPC, int32(aggro), 150))
			}
		},
	}).Named("Agit01Move1")
}

// Move1 is Agit01Move1: a Partisan Hideaway soldier or archer that fights
// the playables it sees, those that hit it or a clan mate, and those whose
// spells it sees.
func Move1() script.Script {
	return script.Script{Dir: move1Dir, Behavior: true, NPCs: []int32{35372, 35379, 35381}, Hooks: move1Hooks()}
}

// PartisanSorcerer2 is Agit01PartisanSorcerer2: an Agit01Move1 sorcerer
// that, in place of fighting them, casts skill 4043 from where it stands
// at every playable it sees, and on a draw under 10 at one attacking a
// clan mate.
func PartisanSorcerer2() script.Script {
	base := move1Hooks()
	return script.Script{Dir: move1Dir, Behavior: true, NPCs: []int32{35380}, Hooks: base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() && s.Rnd(100) < 10 {
				e.Called.AddCastDesireHold(e.Attacker, level1(4043), 1000000)
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			e.NPC.AddCastDesireHold(e.Creature, level1(4043), 1000000)
		},
	})}
}

// partisanHealer1Hooks are Agit01PartisanHealer1's hooks, which the
// healer's child derives its own from: on a draw under 20 the healer wants
// to cast its heal 4044 on Bloody Lord Nurka when he is hit below 60% of
// his hit points.
func partisanHealer1Hooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if id := e.Caller.NpcID(); (id == 35368 || id == 35375) && e.Caller.HPRatio() < 0.6 && s.Rnd(100) < 20 {
				e.Called.AddCastDesire(e.Caller, level1(4044), 1000000)
			}
		},
	}).Named("Agit01PartisanHealer1")
}

// PartisanHealer1 is Agit01PartisanHealer1: a Partisan Hideaway healer
// that heals Bloody Lord Nurka when he is hit low.
func PartisanHealer1() script.Script {
	return script.Script{Dir: healerDir, Behavior: true, NPCs: []int32{35369}, Hooks: partisanHealer1Hooks()}
}

// PartisanHealer2 is Agit01PartisanHealer2: an Agit01PartisanHealer1
// healer of Bloody Lord Nurka's escort. Every 10 seconds, while it has a
// master, it walks once to each of three places by the master's flag:
// beside the master while the flag is 0, to a first fixed place when it is
// 1, and to a second for any other value. The first healer to appear,
// until it dies, takes the place of each pair 40 further along x.
func PartisanHealer2() script.Script {
	base := partisanHealer1Hooks()
	first := &script.NPCCell{}
	return script.Script{Dir: healerDir, Behavior: true, NPCs: []int32{35376}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			first.SetIfEmpty(n)
			n.SetInt(script.SlotIntAI0, 0)
			s.StartTimerAtFixedRate("2001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
		},
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			first.ClearIf(e.NPC)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if n.HasMaster() {
					right := func(r, l int) int {
						if held := first.Get(); held != nil && script.Same(held, n) {
							return r
						}
						return l
					}
					master := n.Master()
					switch flag := master.Int(script.SlotFlag); {
					case flag == 0:
						if n.Int(script.SlotIntAI0) < 1 {
							at := script.LocOf(master)
							n.AddMoveToDesire(script.Loc{X: at.X + right(20, -20), Y: at.Y, Z: at.Z}, 100000000)
							n.SetInt(script.SlotIntAI0, 1)
						}
					case flag == 1:
						if n.Int(script.SlotIntAI0) < 2 {
							n.AddMoveToDesire(script.Loc{X: right(45822, 45782), Y: 109981, Z: -1970}, 100000000)
							n.SetInt(script.SlotIntAI0, 2)
						}
					case n.Int(script.SlotIntAI0) < 3:
						n.AddMoveToDesire(script.Loc{X: right(44545, 44505), Y: 108867, Z: -2020}, 100000000)
						n.SetInt(script.SlotIntAI0, 3)
					}
				}
			}
			return base.Timer(s, e)
		},
	})}
}
