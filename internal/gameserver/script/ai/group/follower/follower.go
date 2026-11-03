// Package follower is the NPCs that keep a companion at their side: a pet
// manager's pet, a golem crafter's golem, a saint's follower. Each master
// brings its follower into the world as it appears, and the follower then
// walks to a random point next to its master every five seconds.
package follower

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// moveTimer is the timer each follower moves on.
const moveTimer = "5001"

// moveWeight is the weight of a follower's walk to its master.
const moveWeight = 30

// moveOffset is how far, on each axis, from its master a follower walks to.
const moveOffset = 50

// pair is a master and the follower it brings.
type pair struct{ master, follower int32 }

// pairs are the masters and their followers.
func pairs() []pair {
	return []pair{
		{30731, 31202}, // Martin, Maximus
		{30827, 31203}, // Lundy, Moon Dancer
		{30828, 31204}, // Waters, Georgio
		{30829, 31205}, // Cooper, Katz
		{30830, 31206}, // Joey, Ten Ten
		{30831, 31207}, // Nelson, Sardinia
		{30869, 31208}, // Lemper, La Grange
		{31067, 31209}, // Rood, Misty Rain
		{31265, 31758}, // Annette, Rafi
		{31309, 31266}, // Woods, Kaiser
		{31592, 31593}, // Telson, Dorothy
		{31605, 31606}, // Kinsley, Alice de Catrina
		{31608, 31609}, // Belinda, Aurora
		{31614, 31629}, // Radyss, Kaleidos
		{31624, 31625}, // Donath, Yeti
		{31701, 31703}, // Follower 2, Follower a
		{31702, 31704}, // Follower 3, Follower b
		{31954, 31955}, // Saroyan, Ruby
		{32070, 32071}, // Adolph, Linda
	}
}

// masters are the NPCs that bring a follower.
func masters() []int32 {
	ps := pairs()
	ids := make([]int32, len(ps))
	for i, p := range ps {
		ids[i] = p.master
	}
	return ids
}

// followerOf returns the follower master brings.
func followerOf(master int32) int32 {
	ps := pairs()
	return ps[slices.IndexFunc(ps, func(p pair) bool { return p.master == master })].follower
}

// New returns the script. Its cell maps each follower's npc id to the
// master that brought it last.
func New() script.Script {
	masterOf := &script.MapCell[int32, *script.NPC]{}
	return script.Script{
		Dir:  "ai/group",
		Bind: script.Bindings{script.EventCreated: masters()},
		Hooks: script.Hooks{
			OnTimer: func(s *script.Script, e script.Timer) string { return onTimer(s, e, masterOf) },
			OnCreated: func(s *script.Script, e script.Created) {
				bringFollower(s, e.NPC, masterOf)

				var base script.Hooks
				base.Created(s, e)
			},
		},
	}
}

// onTimer asks the follower to walk to a random point next to its master,
// unless its AI sleeps.
func onTimer(s *script.Script, e script.Timer, masterOf *script.MapCell[int32, *script.NPC]) string {
	if strings.EqualFold(e.Name, moveTimer) {
		master, _ := masterOf.Get(e.NPC.NpcID())

		at := s.RandomOffset(script.LocOf(master), moveOffset)

		if !e.NPC.AISleeping() {
			e.NPC.AddMoveToDesire(at, moveWeight)
		}
	}
	return ""
}

// bringFollower brings master's follower into the world where master
// stands, running, remembers master as the follower's, and makes the
// follower move every five seconds, the first time five seconds from now.
// A follower that cannot be placed aborts the hook before anything is
// remembered or timed.
func bringFollower(s *script.Script, master *script.NPC, masterOf *script.MapCell[int32, *script.NPC]) {
	followerID := followerOf(master.NpcID())

	follower := s.Spawner().AddSpawn(followerID, master, false, 0)
	if follower == nil {
		panic("follower: the follower of a master cannot be placed")
	}
	follower.SetRunningQuietly(true)

	masterOf.Set(followerID, master)

	s.StartTimerAtFixedRate(moveTimer, follower, nil, 5000*time.Millisecond, 5000*time.Millisecond)
}
