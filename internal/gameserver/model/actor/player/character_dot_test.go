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
	before := c.HP()

	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = c
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := c.HP(), before-4; got != want {
		t.Fatalf("HP() = %v, want %v", got, want)
	}
	if statusUpdates != 1 {
		t.Fatalf("status updates = %d, want 1", statusUpdates)
	}
}

// TestManaDamageOverTimeEffectTargetsCharacter pins Finding 2 of the #1088
// closed-PR review: a mana-DOT tick must broadcast a status update, matching
// EffectManaDamOverTime.onActionTime -> Status.reduceMp -> setMp ->
// broadcastStatusUpdate() (EffectManaDamOverTime.java:35,
// CreatureStatus.java:338-355, 274-306). Both HP-DOT paths and the mana
// heal tick already broadcast; the mana-damage tick was the sole silent
// one.
func TestManaDamageOverTimeEffectTargetsCharacter(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	statusUpdates := 0
	c.SetStatusBroadcaster(func() { statusUpdates++ })
	before := c.MPValue()
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "ManaDamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = c
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := c.MPValue(), before-4; got != want {
		t.Fatalf("MPValue() = %v, want %v", got, want)
	}
	if statusUpdates != 1 {
		t.Fatalf("status updates = %d, want 1", statusUpdates)
	}
}
