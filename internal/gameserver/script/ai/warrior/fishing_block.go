package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// FishingBlock is WarriorFishingBlock: the monster a fisher's catch turns
// into. It attacks the fisher it was spawned for, taunting them, and leaves
// at a 50 s check that finds it neither attacking nor casting. One spawned
// for nobody leaves at once.
func FishingBlock() script.Script {
	return script.Script{Dir: "ai/group", Behavior: true, NPCs: fishingBlockIDs(), Hooks: FishingBlockHooks()}
}

// fishingBlockIDs are WarriorFishingBlock's own NPCs.
func fishingBlockIDs() []int32 {
	return []int32{18319, 18320, 18321, 18322, 18323, 18324, 18325, 18326}
}

// fishingLine returns one of the three lines of set index the monster n
// says, picked at random, with name in it: each monster has nine lines from
// 1010400 on, three for its creation (index 0), three for a hit (3) and
// three for its death (6).
func fishingLine(s *script.Script, n *script.NPC, index int32, name string) string {
	return script.NpcString(1010400+index+((n.NpcID()-18319)*9)+int32(s.Rnd(3)), name)
}

// FishingBlockHooks are WarriorFishingBlock's hooks.
func FishingBlockHooks() script.Hooks {
	base := PhysicalSpecialHooks()
	return base.With(script.Hooks{
		OnTimer: func(_ *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3000") {
				if i := e.NPC.Intention(); i != script.IntentionAttack && i != script.IntentionCast {
					e.NPC.DeleteMe()
				}
			}
			return ""
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if s.Rnd(100) < 33 {
				e.NPC.Say(fishingLine(s, e.NPC, 3, e.Attacker.Name()))
			}
			base.Attacked(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if summoner := n.Summoner(); summoner == nil {
				n.DeleteMe()
			} else {
				n.AddAttackDesire(summoner, 2000)
				n.Say(fishingLine(s, n, 0, summoner.Name()))
				s.StartTimerAtFixedRate("3000", n, nil, 50000*time.Millisecond, 50000*time.Millisecond)
			}
			base.Created(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			e.NPC.Say(fishingLine(s, e.NPC, 6, e.Killer.Name()))
			base.MyDying(s, e)
		},
	})
}
