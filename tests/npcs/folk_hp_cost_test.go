package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestFolkHPCostKeepsWalkStance pins NpcCast.onMagicHitTimer's
// reduceHp(hpConsume, actor, true): a skill's HP cost bypasses
// Npc.reduceCurrentHp, so a walking civilian NPC pays it and stays in its
// walk stance, observers seeing no ChangeMoveType and no NpcInfo.
func TestFolkHPCostKeepsWalkStance(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	tmpl := folkTemplate("Folk", 13031)
	tmpl.CanMove = true
	f := w.spawnFolk(t, tmpl, 40)
	f.SetRunStance(false)
	drainUntilQuiet(t, w.c)
	if f.Running() {
		t.Fatal("civilian NPC still running, want walk stance")
	}

	hp := f.HP()
	f.ConsumeHP(10)
	if got := f.HP(); got != hp-10 {
		t.Fatalf("HP after a 10 HP cost = %v, want %v", got, hp-10)
	}
	if f.Running() {
		t.Fatal("HP cost put the NPC in its run stance, want walk")
	}
	for _, frame := range drainFrames(t, w.c) {
		if objectFrame(frame, serverpackets.OpcodeChangeMoveType, f.ObjectID()) || objectFrame(frame, serverpackets.OpcodeNPCInfo, f.ObjectID()) {
			t.Fatalf("HP cost showed frame %#x, want no stance change", frame[0])
		}
	}
}

// TestInvulnerableFolkPaysHPCost pins CreatureStatus.reduceHp's invul gate:
// an invulnerable civilian NPC still pays its own skill's HP cost, never
// below its undying floor, while another creature's hit takes nothing.
func TestInvulnerableFolkPaysHPCost(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	f := w.spawnFolk(t, folkTemplate("Folk", 13031), 40)
	f.SetInvul(true)

	hp := f.HP()
	f.ConsumeHP(10)
	if got := f.HP(); got != hp-10 {
		t.Fatalf("invulnerable NPC HP after a 10 HP cost = %v, want %v", got, hp-10)
	}

	attacker := w.onlineCharacter(t)
	if f.TakeDamage(10, attacker) || f.HP() != hp-10 {
		t.Fatalf("invulnerable NPC HP after a hit = %v, want %v untouched", f.HP(), hp-10)
	}

	f.ConsumeHP(hp * 2)
	if f.Dead() || f.HP() != 1 {
		t.Fatalf("undying invulnerable NPC after a lethal HP cost: dead %v, HP %v; want alive at 1", f.Dead(), f.HP())
	}
}
