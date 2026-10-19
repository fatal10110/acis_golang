package pets

import (
	"context"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPetRegenTakesRegenMultipliers calls out a wounded wolf on a server
// whose players.properties sets HpRegenMultiplier 2 and MpRegenMultiplier
// 3. A pet is never raid related, so its npc template bases take them
// before the stat (CreatureStatus.getRegenHp/getRegenMp, which
// PetStatus.getRegenHp/getRegenMp start from).
//
// Oracle, with the bonuses of TestPetRegeneratesOnEachRegenTick: HP
// 2.0*2*1.58*0.99 = 6.2568; MP 0.9*3*1.22*0.99 = 3.26106.
func TestPetRegenTakesRegenMultipliers(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{regenWolfTemplate(), treeTemplate()})),
		gameservertest.WithRegenMultipliers(2, 3, 5),
	})
	saved := pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal}
	if err := h.srv.Pets.Save(context.Background(), h.collarID, saved); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)

	regenTick(t, h)
	if got, want := wolf.HP(), 100+6.2568; math.Abs(got-want) > 1e-9 {
		t.Fatalf("HP after a regen tick = %v, want %v", got, want)
	}
	if got, want := wolf.MPValue(), 10+3.26106; math.Abs(got-want) > 1e-9 {
		t.Fatalf("MP after a regen tick = %v, want %v", got, want)
	}
}
