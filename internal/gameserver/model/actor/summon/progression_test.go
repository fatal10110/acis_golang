package summon

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestPetAddExpAndSpLevelsAndRefreshesGrowthStats(t *testing.T) {
	growth := &npc.PetData{Levels: map[int]npc.PetLevelStats{
		1: {MaxExp: 0, MaxMeal: 10, PAtk: 10, MaxHP: 100, MaxMP: 50, SSCount: 1, SPSCount: 1},
		2: {MaxExp: 100, MaxMeal: 20, PAtk: 20, MaxHP: 200, MaxMP: 100, SSCount: 2, SPSCount: 2},
	}}
	pet := NewPet(PetConfig{
		ObjectID: 1, ControlItemID: 2, Level: 1, Exp: 90, SP: 3,
		Growth: growth,
		Stats:  CombatStats{PAtk: 10, MaxHP: 100, MaxMP: 50, SSCount: 1, SPSCount: 1},
	})
	updates := 0
	var earned int64
	pet.SetStatusUpdater(func() { updates++ })
	pet.SetExpNotifier(func(exp int64) { earned = exp })

	pet.AddExpAndSp(10, 7)

	_, state, ok := pet.PetState()
	if !ok || state.Level != 2 || state.Exp != 100 || state.SP != 10 {
		t.Fatalf("PetState() = %+v, %v; want level=2 exp=100 sp=10", state, ok)
	}
	if got := pet.stats.PAtk; got != 20 {
		t.Fatalf("base PAtk = %v, want 20", got)
	}
	if got := pet.stats.MaxHP; got != 200 {
		t.Fatalf("base MaxHP = %v, want 200", got)
	}
	if got := pet.SSCount(); got != 2 {
		t.Fatalf("SSCount() = %d, want 2", got)
	}
	if got, want := pet.HP(), pet.MaxHPValue(); got != want {
		t.Fatalf("HP() = %v, want refreshed max %v", got, want)
	}
	if updates != 1 {
		t.Fatalf("status updates = %d, want 1", updates)
	}
	if earned != 10 {
		t.Fatalf("earned exp = %d, want 10", earned)
	}
}

func TestPetGrowthRefreshIsSafeWithCombatStatReads(t *testing.T) {
	growth := &npc.PetData{Levels: map[int]npc.PetLevelStats{
		1: {MaxExp: 0, MaxMeal: 10, PAtk: 10, PDef: 10, MAtk: 10, MDef: 10, MaxHP: 100, MaxMP: 50, SSCount: 1, SPSCount: 1},
		2: {MaxExp: 1, MaxMeal: 20, PAtk: 20, PDef: 20, MAtk: 20, MDef: 20, MaxHP: 200, MaxMP: 100, SSCount: 2, SPSCount: 2},
	}}
	pet := NewPet(PetConfig{Level: 1, Growth: growth, Stats: CombatStats{AttackSpeed: 300}})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 1_000 {
			pet.AddExpAndSp(1, 0)
		}
	}()
	go func() {
		defer wg.Done()
		for range 1_000 {
			pet.PAtk()
			pet.PDef()
			pet.MAtk()
			pet.MDef()
			pet.MaxHPValue()
			pet.MaxMPValue()
			pet.SSCount()
			pet.SPSCount()
			pet.PhysicalAttackSpeed()
		}
	}()
	wg.Wait()
}

func TestPetCanReceiveKillRewardRejectsExpPastLevel81Limit(t *testing.T) {
	growth := &npc.PetData{Levels: map[int]npc.PetLevelStats{81: {MaxExp: 1_000}}}
	owner := &liveOwnerStub{id: 1}
	state := world.New()
	state.Spawn(owner, 0, 0, 0, 0)
	pet := NewPet(PetConfig{Owner: owner, Level: 81, Exp: 11_001, Growth: growth})
	state.Spawn(pet, 0, 0, 0, 0)

	if pet.CanReceiveKillReward(1500) {
		t.Fatal("CanReceiveKillReward() = true, want false above level-81 max exp plus grace")
	}
}
