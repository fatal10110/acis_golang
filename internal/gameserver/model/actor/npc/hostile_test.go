package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestHostileUsesWorldVisibilityAndTemplateAttackRange(t *testing.T) {
	state := world.New()
	target := &hostileTarget{id: 200}
	state.Spawn(target, 100, 100, 0, 0)
	move := &hostileMove{}
	strike := &hostileAttack{canAttack: true}

	hostile := newTestHostile(t, move, strike)
	state.Spawn(hostile, 120, 100, 0, 0)

	hostile.AddDamageHate(target, 0, 100)
	hostile.Think()

	if strike.target != target {
		t.Fatalf("attack target = %v, want known target", strike.target)
	}
	if move.followTarget != target || move.followRange != 80 {
		t.Fatalf("follow = (%v, %d), want (%v, 80)", move.followTarget, move.followRange, target)
	}
}

func TestHostileIgnoresUnknownTarget(t *testing.T) {
	state := world.New()
	target := &hostileTarget{id: 200}
	state.Spawn(target, world.MaxX, world.MaxY, 0, 0)
	move := &hostileMove{}
	strike := &hostileAttack{canAttack: true}

	hostile := newTestHostile(t, move, strike)
	state.Spawn(hostile, world.MinX, world.MinY, 0, 0)

	hostile.AddDamageHate(target, 0, 100)
	hostile.Think()

	if strike.target != nil {
		t.Fatalf("attack target = %v, want none for unknown target", strike.target)
	}
	if move.followTarget != nil {
		t.Fatalf("follow target = %v, want none for unknown target", move.followTarget)
	}
}

func TestHostileRunsFromAITask(t *testing.T) {
	state := world.New()
	target := &hostileTarget{id: 200}
	state.Spawn(target, 100, 100, 0, 0)
	strike := &hostileAttack{canAttack: true}
	hostile := newTestHostile(t, &hostileMove{}, strike)
	state.Spawn(hostile, 120, 100, 0, 0)
	hostile.AddDamageHate(target, 0, 100)

	brains := task.NewAI()
	brains.Add(hostile)
	brains.Tick()

	if strike.target != target {
		t.Fatalf("attack target = %v, want target after AI task tick", strike.target)
	}
}

func TestNewHostileRejectsInvalidDependencies(t *testing.T) {
	inst := &Instance{ObjectID: 101, Template: &Template{ID: 9001, Type: "Monster"}}
	move := &hostileMove{}
	strike := &hostileAttack{}

	tests := []struct {
		name   string
		inst   *Instance
		move   ai.MoveController
		strike ai.AttackController
	}{
		{name: "nil instance", move: move, strike: strike},
		{name: "nil template", inst: &Instance{ObjectID: 101}, move: move, strike: strike},
		{name: "nil move", inst: inst, strike: strike},
		{name: "nil attack", inst: inst, move: move},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewHostile(tc.inst, tc.move, tc.strike); err == nil {
				t.Fatal("NewHostile() error = nil")
			}
		})
	}
}

func TestNewHostileRejectsNonAttackableKind(t *testing.T) {
	inst := &Instance{
		ObjectID: 101,
		Template: &Template{ID: 9001, Type: "Folk"},
		Kind:     "Folk",
	}

	if _, err := NewHostile(inst, &hostileMove{}, &hostileAttack{}); err == nil {
		t.Fatal("NewHostile() error = nil")
	}
}

func newTestHostile(t *testing.T, move ai.MoveController, strike ai.AttackController) *Hostile {
	t.Helper()
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{
			ID:              9001,
			Type:            "Monster",
			BaseAttackRange: 80,
		},
		Kind: "Monster",
	}, move, strike)
	if err != nil {
		t.Fatal(err)
	}
	return hostile
}

type hostileTarget struct {
	world.Presence
	id int32
}

func (t *hostileTarget) ObjectID() int32  { return t.id }
func (t *hostileTarget) SiegeGuard() bool { return false }
func (t *hostileTarget) AlikeDead() bool  { return false }

type hostileMove struct {
	followTarget attackable.Combatant
	followRange  int
}

func (m *hostileMove) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) bool {
	m.followTarget = target
	m.followRange = attackRange
	return false
}

func (m *hostileMove) Stop() {}

type hostileAttack struct {
	canAttack bool
	target    attackable.Combatant
}

func (a *hostileAttack) BowCoolingDown() bool { return false }
func (a *hostileAttack) AttackingNow() bool   { return false }
func (a *hostileAttack) CanAttack(attackable.Combatant) bool {
	return a.canAttack
}
func (a *hostileAttack) DoAttack(target attackable.Combatant) {
	a.target = target
}
