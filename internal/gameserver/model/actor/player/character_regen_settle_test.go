package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// spawnedRegenCharacter returns a character in the world on an idle queue,
// short of HP so its regeneration phase is armed.
func spawnedRegenCharacter(t *testing.T) (*Character, *world.State) {
	t.Helper()
	c := attachIdleLive(t, liveCharacter(1, combatTemplate(), combatItems()))
	state := world.New()
	c.world = state
	state.Spawn(c, 0, 0, 0, 0)
	c.SetResourceValues(Resources{MaxHP: 100, CurrentHP: 10, MaxMP: 30, CurrentMP: 30, MaxCP: 50, CurrentCP: 50})
	if !c.Regen().Active() {
		t.Fatal("regeneration idle after the drop")
	}
	return c, state
}

// TestRegenPhaseKeptOffGridMidTeleport pins that taking a short character
// off the grid, as a teleport does until Appearing, is no reason to stop its
// regeneration; leaving the world is.
func TestRegenPhaseKeptOffGridMidTeleport(t *testing.T) {
	c, state := spawnedRegenCharacter(t)
	state.Leave(c)
	if c.Visible() {
		t.Fatal("character still on the grid after Leave")
	}
	c.SettleRegen()
	if !c.Regen().Active() {
		t.Fatal("a settle off the grid disarmed the regeneration of a short character")
	}
	state.Rejoin(c)
	state.Despawn(c)
	c.SettleRegen()
	if c.Regen().Active() {
		t.Fatal("regeneration still armed after the character left the world")
	}
}

// TestRefillResourcesStopsRegen pins the level-up refill's stop of the
// regeneration task: back at full, the next drop starts a fresh phase.
func TestRefillResourcesStopsRegen(t *testing.T) {
	c, _ := spawnedRegenCharacter(t)
	c.refillResources(100, 30, 50)
	if res := c.ResourceValues(); res.CurrentHP != res.MaxHP {
		t.Fatalf("HP after the refill = %v, want max %v", res.CurrentHP, res.MaxHP)
	}
	if c.Regen().Active() {
		t.Fatal("regeneration still armed after the refill to full")
	}
}

// TestClampResourcesStopsRegenAtFull pins that a class change's clamp which
// leaves the character full stops its regeneration task.
func TestClampResourcesStopsRegenAtFull(t *testing.T) {
	c, _ := spawnedRegenCharacter(t)
	c.AttachStatFuncs([]effect.Mod{
		{Stat: stat.MaxHP, Op: effect.OpSet, Value: 5},
		{Stat: stat.MaxMP, Op: effect.OpSet, Value: 5},
		{Stat: stat.MaxCP, Op: effect.OpSet, Value: 5},
	})
	if !c.Regen().Active() {
		t.Fatal("lowering the maxima settled the phase; the clamp is not exercised")
	}
	c.ClampResources()
	if res := c.ResourceValues(); res.CurrentHP != res.MaxHP || res.CurrentMP != res.MaxMP || res.CurrentCP != res.MaxCP {
		t.Fatalf("resources after the clamp = %+v, want full", res)
	}
	if c.Regen().Active() {
		t.Fatal("regeneration still armed after the clamp left the character full")
	}
}
