package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// wolfPrevLevelExp is where the growth row below the wolf's spawn level
// starts, for a death penalty that takes a level away.
const wolfPrevLevelExp = int64(200)

// penaltyWolfTemplate is the wolf with the growth row below its spawn level.
func penaltyWolfTemplate() *npc.Template {
	wolf := wolfTemplate()
	wolf.Pet.Levels[wolfLevel-1] = wolfLevelStats(wolfPrevLevelExp)
	return wolf
}

// TestPetDeathPenalty kills level-10 wolves. Level 10 spans 500 experience
// (500 to 1000), and a pet loses 6.5-0.07*10 = 5.8 percent of its level's
// span on death: 29 experience.
func TestPetDeathPenalty(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		exp       int64
		wantExp   int64
		wantLevel int
	}{
		{"keeps its level", 700, 671, wolfLevel},
		{"drops a level", 510, 481, wolfLevel - 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{penaltyWolfTemplate(), treeTemplate()})),
			})
			if err := h.srv.Pets.Save(petCtx(), h.collarID, pet.State{
				Level: wolfLevel, Exp: tt.exp, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: wolfMaxMeal,
			}); err != nil {
				t.Fatalf("seed pets row: %v", err)
			}
			wolf, _ := h.spawnWolf(t)
			drainUntilQuiet(t, h.client)

			obj, _ := h.srv.State.Player(h.ownerID)
			wolf.ReduceHP(wolf.HP()+1, obj.(attackable.Combatant), modelskill.Definition{})
			h.srv.AdvanceUntil(t, "wolf dead", wolf.Dead)
			h.srv.Settle(t)
			h.srv.InventoryUpdates.Tick()
			frames := drainFrames(t, h.client)

			if got := wolf.Exp(); got != tt.wantExp {
				t.Fatalf("dead wolf exp = %d, want %d", got, tt.wantExp)
			}
			if got := wolf.Level(); got != tt.wantLevel {
				t.Fatalf("dead wolf level = %d, want %d", got, tt.wantLevel)
			}
			if tt.wantLevel == wolfLevel {
				if got := h.liveCollarEnchant(t); got != wolfLevel {
					t.Fatalf("collar enchant = %d, want %d kept", got, wolfLevel)
				}
				return
			}
			// The lost level reaches the owner's pet window, then the
			// collar's enchant.
			petInfo, collar := -1, -1
			for i, frame := range frames {
				switch {
				case petInfo < 0 && sawPetInfo([][]byte{frame}, wolf.ObjectID()):
					petInfo = i
				case collar < 0 && frame[0] == serverpackets.OpcodeInventoryUpdate:
					for _, e := range readCollarUpdates(t, frame) {
						if e.objID == h.collarID && int(e.enchant) == tt.wantLevel {
							collar = i
						}
					}
				}
			}
			if petInfo < 0 || collar < 0 || petInfo > collar {
				t.Fatalf("PetInfo at %d, collar InventoryUpdate at %d, want both, PetInfo first", petInfo, collar)
			}
			if got := h.persistedCollarEnchant(t); got != tt.wantLevel {
				t.Fatalf("persisted collar enchant = %d, want %d", got, tt.wantLevel)
			}
		})
	}
}
