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
		ReduceHPByDOT(float64, effect.Participant, bool)
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
// closed-PR review: a mana-DOT tick must broadcast a status update carrying
// MP, matching PlayerStatus.broadcastStatusUpdate()'s unconditional CUR_MP
// inclusion (EffectManaDamOverTime.java:35 -> CreatureStatus.reduceMp/setMp,
// CreatureStatus.java:338-355, 274-306 -> the Player override at
// PlayerStatus.java:408-416, which sends CUR_HP+CUR_MP+CUR_CP on every
// call, unlike the generic HP-only, threshold-gated broadcast the base
// Creature/Npc path uses). The generic statusBroadcaster hook is HP-only on
// the wire (network/targeting.go's targetHPAttributes), so this must go
// through the separate MP-carrying broadcaster, not the HP one.
func TestManaDamageOverTimeEffectTargetsCharacter(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	mpStatusUpdates := 0
	c.SetMPStatusBroadcaster(func() { mpStatusUpdates++ })
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
	if mpStatusUpdates != 1 {
		t.Fatalf("MP status updates = %d, want 1", mpStatusUpdates)
	}
}
