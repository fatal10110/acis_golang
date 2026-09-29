package pets

import (
	"context"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// regenWolfTemplate is the wolf with the npc template regeneration a pet
// regenerates by: 2.0 HP and 0.9 MP a tick before its CON, MEN and level
// bonuses.
func regenWolfTemplate() *npc.Template {
	wolf := wolfTemplate()
	wolf.HPRegen, wolf.MPRegen = 2.0, 0.9
	return wolf
}

// bootSavedWolf boots the owner with a pets row already saved for the collar,
// so the next call-out restores the wolf from it.
func bootSavedWolf(t *testing.T, saved pet.State) *petWorld {
	t.Helper()
	return bootSavedWolfOf(t, regenWolfTemplate(), saved)
}

// bootSavedWolfOf is bootSavedWolf with the wolf built from wolf.
func bootSavedWolfOf(t *testing.T, wolf *npc.Template, saved pet.State) *petWorld {
	t.Helper()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
	})
	if err := h.srv.Pets.Save(context.Background(), h.collarID, saved); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	return h
}

// regenTick runs one production HP/MP regeneration sweep and waits for it.
func regenTick(t *testing.T, h *petWorld) {
	t.Helper()
	task.NewNPCRegen(h.srv.State).Tick()
	h.srv.Settle(t)
}

// TestPetRegeneratesOnEachRegenTick calls out a wounded wolf. Each regen tick
// gives it its HP and MP regeneration, and its owner the refreshed status.
//
// Oracle, from the reference formulas: CON 43 gives a 1.58 bonus
// (floor(1.03^(43-27.632)*100+.5)/100), MEN 20 a 1.22 bonus
// (floor(1.01^(20+0.06)*100+.5)/100), and level 10 a 0.99 level mod
// ((100-11+10)/100). HP: 2.0*1.58*0.99 = 3.1284; MP: 0.9*1.22*0.99 = 1.08702.
func TestPetRegeneratesOnEachRegenTick(t *testing.T) {
	t.Parallel()
	h := bootSavedWolf(t, pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal})
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)

	regenTick(t, h)
	if got, want := wolf.HP(), 100+3.1284; math.Abs(got-want) > 1e-9 {
		t.Fatalf("HP after a regen tick = %v, want %v", got, want)
	}
	if got, want := wolf.MPValue(), 10+1.08702; math.Abs(got-want) > 1e-9 {
		t.Fatalf("MP after a regen tick = %v, want %v", got, want)
	}
	if !hasOpcode(drainFrames(t, h.client), serverpackets.OpcodePetStatusUpdate) {
		t.Fatal("owner got no PetStatusUpdate for the regenerated wolf")
	}
}

// TestPetRegenGivesAtLeastOnePerTick calls out a wounded wolf whose npc
// template regenerates less than a point a tick. Each regen tick still gives
// it one HP and one MP, the reference's floor.
//
// Oracle, with the bonuses above: HP 0.3*1.58*0.99 = 0.46926 and MP
// 0.5*1.22*0.99 = 0.6039, both raised to 1.
func TestPetRegenGivesAtLeastOnePerTick(t *testing.T) {
	t.Parallel()
	slow := wolfTemplate()
	slow.HPRegen, slow.MPRegen = 0.3, 0.5
	h := bootSavedWolfOf(t, slow, pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal})
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	if hp, mp := wolf.HPRegenRate(), wolf.MPRegenRate(); hp >= 1 || mp >= 1 {
		t.Fatalf("regen rates = %v HP, %v MP, want both below 1", hp, mp)
	}

	regenTick(t, h)
	if got := wolf.HP(); got != 101 {
		t.Fatalf("HP after a regen tick = %v, want 101", got)
	}
	if got := wolf.MPValue(); got != 11 {
		t.Fatalf("MP after a regen tick = %v, want 11", got)
	}
}

// TestPetSavedDeadRestoresDeadWithoutRegen restores a wolf whose row was saved
// below half a hit point, as a corpse lost to a restart leaves it. The wolf
// comes back dead: it neither regenerates nor takes a heal, and the owner
// cannot send it back.
func TestPetSavedDeadRestoresDeadWithoutRegen(t *testing.T) {
	t.Parallel()
	h := bootSavedWolf(t, pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 0.4, CurMP: 10, Fed: wolfMaxMeal})
	wolf, _ := h.spawnWolf(t)
	if !wolf.Dead() {
		t.Fatal("wolf saved at 0.4 HP restored alive, want it dead")
	}
	drainUntilQuiet(t, h.client)

	regenTick(t, h)
	regenTick(t, h)
	if got := wolf.MPValue(); got != 10 {
		t.Fatalf("restored dead wolf MP after regen ticks = %v, want 10 kept", got)
	}
	if hasOpcode(drainFrames(t, h.client), serverpackets.OpcodePetStatusUpdate) {
		t.Fatal("restored dead wolf's status was republished by a regen tick")
	}
	if healed := wolf.AddHP(50); healed != 0 {
		t.Fatalf("restored dead wolf healed %v, want no heal", healed)
	}
	wolf.Unsummon()
	if got, ok := h.srv.State.Summon(h.ownerID); !ok || got.ObjectID() != wolf.ObjectID() {
		t.Fatal("restored dead wolf was unsummoned, want its corpse kept")
	}
}
