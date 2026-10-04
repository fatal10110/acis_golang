package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// newKindHostile builds a hostile of the given instance kind and template
// aggro range, standing at the origin with no line-of-sight query attached.
func newKindHostile(t *testing.T, id int32, kind InstanceKind, aggroRange int) *Hostile {
	t.Helper()
	h, err := NewHostile(&Instance{ObjectID: id, Template: &Template{ID: int(id), Type: string(kind), AggroRange: aggroRange}, Kind: kind},
		newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// peaceTarget is a playable target whose peace-zone membership and karma
// the test sets.
type peaceTarget struct {
	attackabletest.Combatant
	world.Presence
	inPeace bool
	karma   int
}

func (p *peaceTarget) ObjectID() int32   { return 900 }
func (p *peaceTarget) Kind() actor.Kind  { return actor.KindPlayer }
func (p *peaceTarget) InPeaceZone() bool { return p.inPeace }
func (p *peaceTarget) Karma() int        { return p.karma }

// TestHostileAggressiveFollowsKind pins isAggressive per NPC class: Npc
// answers false (Guard, SiegeGuard), Monster answers aggroRange > 0 (and so
// do its Chest, RaidBoss, ... subclasses), FestivalMonster and
// FriendlyMonster always answer true.
func TestHostileAggressiveFollowsKind(t *testing.T) {
	for _, tc := range []struct {
		kind       InstanceKind
		aggroRange int
		want       bool
	}{
		{"Monster", 0, false},
		{"Monster", 300, true},
		{"RaidBoss", 500, true},
		{"Chest", 0, false},
		{"FestivalMonster", 0, true},
		{"FriendlyMonster", 0, true},
		{"Guard", 300, false},
		{"SiegeGuard", 300, false},
	} {
		h := newKindHostile(t, 1, tc.kind, tc.aggroRange)
		if got := h.Aggressive(); got != tc.want {
			t.Errorf("%s aggroRange=%d Aggressive() = %v, want %v", tc.kind, tc.aggroRange, got, tc.want)
		}
	}
}

// TestGuardAttackAggroMobGatesMonsterTargets pins the Guard branch of the
// auto-attack rule: a Guard picks an aggressive Monster-family NPC only when
// GuardAttackAggroMob is on, never a non-aggressive monster or a
// non-Monster NPC, and always a karma player in sight.
func TestGuardAttackAggroMobGatesMonsterTargets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       InstanceKind
		aggroRange int
		wantOn     bool
	}{
		{"aggressive monster", "Monster", 300, true},
		{"passive monster", "Monster", 0, false},
		{"festival monster", "FestivalMonster", 0, true},
		{"aggressive raid boss", "RaidBoss", 500, true},
		{"friendly monster", "FriendlyMonster", 300, false},
		{"other guard", "Guard", 300, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			guard := newKindHostile(t, 1, "Guard", 300)
			target := newKindHostile(t, 2, tc.kind, tc.aggroRange)
			if guard.AutoAttackTargetValid(target, 1000, false) {
				t.Errorf("default config: guard targets %s, want not", tc.name)
			}
			guard.SetAIConfig(AIConfig{MobAggroInPeaceZone: true, GuardAttackAggroMob: true})
			if got := guard.AutoAttackTargetValid(target, 1000, false); got != tc.wantOn {
				t.Errorf("GuardAttackAggroMob on: guard targets %s = %v, want %v", tc.name, got, tc.wantOn)
			}
		})
	}

	guard := newKindHostile(t, 1, "Guard", 300)
	if !guard.AutoAttackTargetValid(&peaceTarget{karma: 100}, 1000, false) {
		t.Error("guard skips a karma player in sight")
	}
	if guard.AutoAttackTargetValid(&peaceTarget{}, 1000, true) {
		t.Error("guard targets a karma-free player")
	}
}

// TestMobAggroInPeaceZoneGatesPeaceTargets pins the general auto-attack
// rule's peace gate: a target in a peace zone is excluded only when
// MobAggroInPeaceZone is off, whatever allowPeaceful says, and allowPeaceful
// stands in for the NPC's own aggressiveness.
func TestMobAggroInPeaceZoneGatesPeaceTargets(t *testing.T) {
	for _, tc := range []struct {
		name          string
		aggroRange    int
		inPeace       bool
		mobAggroPeace bool
		allowPeaceful bool
		want          bool
	}{
		{"default, aggressive, target in peace", 300, true, true, false, true},
		{"default, aggressive, target outside", 300, false, true, false, true},
		{"default, passive, target in peace", 0, true, true, false, false},
		{"default, passive, allowPeaceful, target in peace", 0, true, true, true, true},
		{"off, aggressive, target in peace", 300, true, false, false, false},
		{"off, aggressive, allowPeaceful, target in peace", 300, true, false, true, false},
		{"off, aggressive, target outside", 300, false, false, false, true},
		{"off, passive, allowPeaceful, target outside", 0, false, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mob := newKindHostile(t, 1, "Monster", tc.aggroRange)
			mob.SetAIConfig(AIConfig{MobAggroInPeaceZone: tc.mobAggroPeace})
			got := mob.AutoAttackTargetValid(&peaceTarget{inPeace: tc.inPeace}, 1000, tc.allowPeaceful)
			if got != tc.want {
				t.Fatalf("AutoAttackTargetValid = %v, want %v", got, tc.want)
			}
		})
	}
}
