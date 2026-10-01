package ai

import (
	"testing"

	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// ---- playable attack gate ----

func TestRefusesPlayableTarget(t *testing.T) {
	blessed := func(g *gateFake) *gateFake { g.blessed = true; return g }
	cursed := func(g *gateFake) *gateFake { g.cursed = true; return g }
	inPvP := func(g *gateFake) *gateFake { g.pvp = true; return g }
	npc := func(g *gateFake) *gateFake { g.kind = modelactor.KindNPC; return g }

	tests := []struct {
		name     string
		attacker attackable.Combatant
		target   attackable.Combatant
		want     bool
	}{
		{"karma attacker 10 levels above a blessed target", gatePlayerFake(1, 30, 500), blessed(gatePlayerFake(2, 20, 0)), true},
		{"karma attacker 9 levels above a blessed target", gatePlayerFake(1, 29, 500), blessed(gatePlayerFake(2, 20, 0)), false},
		{"karma-free attacker far above a blessed target", gatePlayerFake(1, 40, 0), blessed(gatePlayerFake(2, 20, 0)), false},
		{"blessed target inside a PvP zone, attacker outside", gatePlayerFake(1, 30, 500), inPvP(blessed(gatePlayerFake(2, 20, 0))), false},
		{"attacker inside a PvP zone, blessed target outside", inPvP(gatePlayerFake(1, 30, 500)), blessed(gatePlayerFake(2, 20, 0)), true},
		{"blessed attacker, karma target 10 levels above", blessed(gatePlayerFake(1, 20, 0)), gatePlayerFake(2, 30, 500), true},
		{"blessed attacker, karma target 9 levels above", blessed(gatePlayerFake(1, 20, 0)), gatePlayerFake(2, 29, 500), false},
		{"blessed attacker, karma-free target far above", blessed(gatePlayerFake(1, 20, 0)), gatePlayerFake(2, 40, 0), false},
		{"blessed attacker, karma target above inside a PvP zone", blessed(gatePlayerFake(1, 20, 0)), inPvP(gatePlayerFake(2, 30, 500)), false},
		{"level 20 attacker, cursed-weapon target", gatePlayerFake(1, 20, 0), cursed(gatePlayerFake(2, 40, 0)), true},
		{"level 21 attacker, cursed-weapon target", gatePlayerFake(1, 21, 0), cursed(gatePlayerFake(2, 40, 0)), false},
		{"cursed-weapon attacker, level 20 target", cursed(gatePlayerFake(1, 40, 0)), gatePlayerFake(2, 20, 0), true},
		{"cursed-weapon attacker, level 21 target", cursed(gatePlayerFake(1, 40, 0)), gatePlayerFake(2, 21, 0), false},
		{"cursed-weapon attacker, level 20 target inside a PvP zone", cursed(gatePlayerFake(1, 40, 0)), inPvP(gatePlayerFake(2, 20, 0)), true},
		{"non-playable target", cursed(gatePlayerFake(1, 40, 500)), npc(blessed(gatePlayerFake(2, 1, 0))), false},
		{"summon of a blessed player", gatePlayerFake(1, 30, 500), gateSummonFake(3, blessed(gatePlayerFake(2, 20, 0))), true},
		{"summon of a blessed player inside a PvP zone", gatePlayerFake(1, 30, 500), inPvP(gateSummonFake(3, blessed(gatePlayerFake(2, 20, 0)))), false},
		{"summon of a blessed player whose owner alone is in a PvP zone", gatePlayerFake(1, 30, 500), gateSummonFake(3, inPvP(blessed(gatePlayerFake(2, 20, 0)))), true},
		{"summon of a karma player at a blessed target", gateSummonFake(3, gatePlayerFake(1, 30, 500)), blessed(gatePlayerFake(2, 20, 0)), true},
		{"attacker with no acting player", actor(1), blessed(gatePlayerFake(2, 1, 0)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := refusesPlayableTarget(tc.attacker, tc.target); got != tc.want {
				t.Fatalf("refusesPlayableTarget() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- keep attacking after a swing ----

func TestCanKeepAttacking(t *testing.T) {
	inPvP := func(g *gateFake) *gateFake { g.pvp = true; return g }
	betrayedSummon := func(g *gateFake) *gateFake { g.betrayed = true; return g }
	npc := &gateFake{id: 9, kind: modelactor.KindNPC}

	tests := []struct {
		name     string
		attacker attackable.Combatant
		target   attackable.Combatant
		want     bool
	}{
		{"no target", gatePlayerFake(1, 40, 0), nil, false},
		{"non-playable target", gatePlayerFake(1, 40, 0), npc, true},
		{"unflagged player", gatePlayerFake(1, 40, 0), gatePlayerFake(2, 40, 0), false},
		{"karma player", gatePlayerFake(1, 40, 0), gatePlayerFake(2, 40, 500), true},
		{"summon of a karma player", gatePlayerFake(1, 40, 0), gateSummonFake(3, gatePlayerFake(2, 40, 500)), true},
		{"summon of an unflagged player", gatePlayerFake(1, 40, 0), gateSummonFake(3, gatePlayerFake(2, 40, 0)), false},
		{"both inside a PvP zone", inPvP(gatePlayerFake(1, 40, 0)), inPvP(gatePlayerFake(2, 40, 0)), true},
		{"only the target inside a PvP zone", gatePlayerFake(1, 40, 0), inPvP(gatePlayerFake(2, 40, 0)), false},
		{"only the attacker inside a PvP zone", inPvP(gatePlayerFake(1, 40, 0)), gatePlayerFake(2, 40, 0), false},
		{"summon in a PvP zone at a player in one", inPvP(gateSummonFake(3, gatePlayerFake(1, 40, 0))), inPvP(gatePlayerFake(2, 40, 0)), true},
		{"betrayed summon at its owner", betrayedSummon(gateSummonFake(3, gatePlayerFake(1, 40, 0))), gatePlayerFake(1, 40, 0), true},
		{"loyal summon at an unflagged player", gateSummonFake(3, gatePlayerFake(1, 40, 0)), gatePlayerFake(2, 40, 0), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := canKeepAttacking(tc.attacker, tc.target); got != tc.want {
				t.Fatalf("canKeepAttacking() = %v, want %v", got, tc.want)
			}
		})
	}
}
