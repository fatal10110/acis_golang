package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

var (
	_ interface {
		Dead() bool
		HP() float64
		ReduceHPByDOT(float64, any)
	} = (*Character)(nil)
	_ interface {
		Dead() bool
		MPValue() float64
		ReduceMP(float64) float64
	} = (*Character)(nil)
)

func TestDamageOverTimeEffectTargetsCharacterAndBroadcastsStatus(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	statusUpdates := 0
	c.SetStatusBroadcaster(func() { statusUpdates++ })

	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = c
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := c.HP(), 76.0; got != want {
		t.Fatalf("HP() = %v, want %v", got, want)
	}
	if statusUpdates != 1 {
		t.Fatalf("status updates = %d, want 1", statusUpdates)
	}
}

func TestManaDamageOverTimeEffectTargetsCharacter(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "ManaDamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = c
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := c.MPValue(), 26.0; got != want {
		t.Fatalf("MPValue() = %v, want %v", got, want)
	}
}
