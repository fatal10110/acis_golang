package effect_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// betraySummon is a minimal summon stub recording TryToAttack/TryToFollow
// calls; there is no lightweight real summon type to construct here, and
// it has no logic (beyond event recording) that could drift from a real
// implementation, so it stays a test double per docs/agents/test-strategy.md.
type betraySummon struct {
	owner  attackable.Combatant
	events []string
}

func (s *betraySummon) ObjectID() int32 { return 0 }

func (s *betraySummon) Dead() bool { return false }

func (s *betraySummon) OwnerCombatant() attackable.Combatant { return s.owner }

func (s *betraySummon) TryToAttack(target any) {
	s.events = append(s.events, "attack:"+effectObjectID(target))
}

func (s *betraySummon) TryToFollow(target any) {
	s.events = append(s.events, "follow:"+effectObjectID(target))
}

func effectObjectID(target any) string {
	o, ok := target.(interface{ ObjectID() int32 })
	if !ok || o == nil {
		return "nil"
	}
	return strconv.FormatInt(int64(o.ObjectID()), 10)
}

// TestBetrayEffectAttacksSummonOwnerAndFollowsOnExit uses a real
// *player.Character as the summon owner instead of a hand-rolled fake:
// *player.Character already implements attackable.Combatant.
func TestBetrayEffectAttacksSummonOwnerAndFollowsOnExit(t *testing.T) {
	owner := &player.Character{ID: 42}
	summon := &betraySummon{owner: owner}
	e, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "Betray"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = summon

	if !e.OnStart(e) {
		t.Fatal("betray effect start rejected a summon with an owner")
	}
	if want := []string{"attack:42"}; !reflect.DeepEqual(summon.events, want) {
		t.Fatalf("events after OnStart = %#v, want %#v", summon.events, want)
	}

	e.OnExit(e)
	if want := []string{"attack:42", "follow:42"}; !reflect.DeepEqual(summon.events, want) {
		t.Fatalf("events after OnExit = %#v, want %#v", summon.events, want)
	}
}

// TestRandomizeHateEffectRejectsATargetWithNoThreatTable uses a real
// *player.Character instead of a hand-rolled fake: it genuinely lacks a
// RandomizeHate method, same as the fake did, but proves the rejection
// against production's own actor type rather than a stand-in.
func TestRandomizeHateEffectRejectsATargetWithNoThreatTable(t *testing.T) {
	e, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "RandomizeHate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = &player.Character{ID: 1}

	if e.OnStart(e) {
		t.Fatal("randomize-hate effect started against a target with no RandomizeHate method")
	}
}
