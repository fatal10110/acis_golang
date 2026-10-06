package script

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// recordingBrain records each desire request as one line.
type recordingBrain struct{ calls []string }

func (b *recordingBrain) record(format string, args ...any) {
	b.calls = append(b.calls, fmt.Sprintf(format, args...))
}

func (b *recordingBrain) AddAttackDesire(target attackable.Combatant, weight float64) {
	b.record("attack %v %v", id(target), weight)
}

func (b *recordingBrain) AddAttackDesireHold(target attackable.Combatant, weight float64) {
	b.record("attack-hold %v %v", id(target), weight)
}

func (b *recordingBrain) AddAttackDesireDamage(target attackable.Combatant, damage int, weight float64) {
	b.record("attack-damage %v %d %v", id(target), damage, weight)
}

func (b *recordingBrain) AddCastDesire(target attackable.Combatant, ref skill.Ref, weight float64, checkConditions, moveToTarget bool) {
	b.record("cast %v %v %v check=%v move=%v", id(target), ref, weight, checkConditions, moveToTarget)
}

func (b *recordingBrain) AddFollowDesire(target attackable.Combatant, weight float64) {
	b.record("follow %v %v", id(target), weight)
}

func (b *recordingBrain) AddWanderDesire(timer int, weight float64) {
	b.record("wander %d %v", timer, weight)
}

func (b *recordingBrain) AddDoNothingDesire(timer int, weight float64) {
	b.record("nothing %d %v", timer, weight)
}

func (b *recordingBrain) AddFleeDesire(target attackable.Combatant, distance int, weight float64) {
	b.record("flee %v %d %v", id(target), distance, weight)
}

func (b *recordingBrain) AddSocialDesire(socialID, timer int, weight float64) {
	b.record("social %d %d %v", socialID, timer, weight)
}

// combatant is a creature the world tracks under objectID.
type combatant struct {
	attackabletest.Combatant
	world.Presence
	objectID int32
}

func (c *combatant) ObjectID() int32 { return c.objectID }

func id(c attackable.Combatant) any {
	if c == nil {
		return "nil"
	}
	return c.ObjectID()
}

// TestNPCDesireRequests pins what each NPC handle desire asks of the NPC's
// AI: the target resolved to the creature the world tracks, and the hold
// and condition-check flags of each variant.
func TestNPCDesireRequests(t *testing.T) {
	b := &recordingBrain{}
	n := &NPC{self: &combatant{objectID: 1}, brain: b}
	p := &Player{self: &combatant{objectID: 2}}
	ref := skill.Ref{ID: 4107, Level: 1}

	n.AddAttackDesire(p, 2000)
	n.AddAttackDesireHold(p, 50)
	n.AddAttackDesireDamage(p, 1, 200)
	n.AddCastDesire(p, ref, 1000000)
	n.AddCastDesireUnchecked(n, ref, 10000)
	n.AddCastDesireHold(p, ref, 30)
	n.AddFollowDesire(n, 5)
	n.AddWanderDesire(5, 5)
	n.AddDoNothingDesire(40, 30)
	n.AddFleeDesire(p, 500, 10000)
	n.AddSocialDesire(3, 7000, 1000)
	n.AddAttackDesire(nil, 1)
	n.AddFollowDesire((*Player)(nil), 1)
	n.AddCastDesire(&Player{}, ref, 1)

	want := []string{
		"attack 2 2000",
		"attack-hold 2 50",
		"attack-damage 2 1 200",
		"cast 2 {4107 1} 1e+06 check=true move=true",
		"cast 1 {4107 1} 10000 check=false move=true",
		"cast 2 {4107 1} 30 check=true move=false",
		"follow 1 5",
		"wander 5 5",
		"nothing 40 30",
		"flee 2 500 10000",
		"social 3 7000 1000",
		"attack nil 1",
		"follow nil 1",
		"cast nil {4107 1} 1 check=true move=true",
	}
	if len(b.calls) != len(want) {
		t.Fatalf("calls = %q, want %q", b.calls, want)
	}
	for i := range want {
		if b.calls[i] != want[i] {
			t.Fatalf("call %d = %q, want %q", i, b.calls[i], want[i])
		}
	}
}
