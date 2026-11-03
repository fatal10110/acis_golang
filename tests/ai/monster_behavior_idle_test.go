package ai

import (
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// TestMonsterBehaviorIdles ports MonsterBehavior's no-desire hook. With
// nothing to do, a monster that moves to attack (MovingAttack 1, the
// default) follows its live master with weight 5 when it is a following
// private (Party_Type 1), and otherwise wanders, trying a walk every 5 s,
// with weight 5: a lone monster, a party leader (Party_Type 2) and a
// private whose master link is gone. One that holds its ground
// (MovingAttack 0) wants neither, private or not.
func TestMonsterBehaviorIdles(t *testing.T) {
	t.Parallel()
	holdPrivate := warriorTemplate(behaviorPrivateID, npc.AIParams{"Party_Type": "1", "Party_Loyalty": "3", "MovingAttack": "0"})
	for _, row := range []struct {
		name string
		// private, when set, is the leader's private template; the
		// scenario then checks the private, else a lone warrior.
		private *npc.Template
		// masterless drops the private's master link before the AI runs.
		masterless bool
		follow     bool
		wander     bool
	}{
		{name: "lone wanders", wander: true},
		{name: "lone holds ground"},
		{name: "private follows its master", private: behaviorPrivate(), follow: true},
		{name: "masterless private wanders", private: behaviorPrivate(), masterless: true, wander: true},
		{name: "private holds ground", private: holdPrivate},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			var w *warriorWorld
			var h, leader *npc.Hostile
			if row.private == nil {
				var ai npc.AIParams
				if !row.wander {
					ai = npc.AIParams{"MovingAttack": "0"}
				}
				w = bootBehaviors(t, warriorTemplate(behaviorWarriorID, ai))
				h = w.spawn(t, behaviorWarriorID, 100, 0)
			} else {
				w = bootBehaviors(t, behaviorLeader(), row.private)
				leader = w.spawn(t, behaviorLeaderID, 100, 0)
				h = privatesOf(t, leader, 1)[0]
				if row.masterless {
					w.onQueue(t, h, func() {
						h.SetMaster(nil)
						h.AI().Desires().Clear()
					})
				}
			}

			w.ticks(t, 2)
			d, ok := desire(h, actorai.IntentionFollow)
			if ok != row.follow {
				t.Fatalf("follow desire = %+v (%v), want one: %v", d, ok, row.follow)
			}
			if ok && (d.Weight != 5 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != leader.ObjectID()) {
				t.Fatalf("follow desire = %+v, want the private's master with weight 5", d)
			}
			d, ok = desire(h, actorai.IntentionWander)
			if ok != row.wander {
				t.Fatalf("wander desire = %+v (%v), want one: %v", d, ok, row.wander)
			}
			if ok && (d.Timer != 5 || d.Weight != 5) {
				t.Fatalf("wander desire = timer %v, weight %v; want 5, 5", d.Timer, d.Weight)
			}
			if leader != nil {
				if d, ok := desire(leader, actorai.IntentionWander); !ok || d.Timer != 5 || d.Weight != 5 {
					t.Fatalf("party leader wander desire = %+v (%v), want timer 5, weight 5", d, ok)
				}
			}
		})
	}
}
