package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// petFramesBeforeDie counts the owner's PetInfo and PetStatusUpdate frames
// ahead of the pet's Die.
func petFramesBeforeDie(t *testing.T, frames [][]byte, petID int32) (infos, statuses int) {
	t.Helper()
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeDie:
			if wire.NewReader(f[1:]).ReadInt32() == petID {
				return infos, statuses
			}
		case serverpackets.OpcodePetInfo:
			infos++
		case serverpackets.OpcodePetStatusUpdate:
			statuses++
		}
	}
	t.Fatal("owner never saw the pet's Die")
	return 0, 0
}

// TestPetDeathStripRefreshesPetWindowOnce kills a pet without a blessing.
// Its death strip ends every effect with exit(true), so a run-speed buff's
// removal sends no per-effect refresh (Creature.removeStatsByOwner,
// Creature.java:1198-1204), and Summon.stopAllEffectsExceptThoseThatLastThroughDeath
// then sends the owner one PetInfo (sendPetInfosToOwner, Summon.java:385-389,
// 598-604) whether or not anything was stripped. The buff's removal adds
// only the icon update's frame.
func TestPetDeathStripRefreshesPetWindowOnce(t *testing.T) {
	t.Parallel()
	var plainStatuses int
	for _, tt := range []struct {
		name  string
		buff  bool
		infos int
	}{
		{name: "no effects", infos: 1},
		{name: "run-speed buff", buff: true, infos: 1 + iconUpdatePetInfo},
	} {
		// Sequential: the buffed case compares against the plain one.
		t.Run(tt.name, func(t *testing.T) { plainStatuses = killStrippedPet(t, tt.buff, tt.infos, plainStatuses) })
	}
}

// killStrippedPet kills a fresh pet, with a run-speed buff when buff is set,
// and checks the owner's PetInfo count before its Die against infos and,
// for the buffed pet, its PetStatusUpdate count against plainStatuses. It
// returns the PetStatusUpdate count.
func killStrippedPet(t *testing.T, buff bool, infos, plainStatuses int) int {
	t.Helper()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	if buff {
		e, err := effect.New(effect.Skill{ID: 1204, Level: 1}, modelskill.EffectTemplate{
			Name: "Buff", Time: 1800, Count: 1, Icon: true, StackType: "speed_up", StackOrder: 1,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "runSpd", Value: 1.33}},
		})
		if err != nil {
			t.Fatalf("effect.New: %v", err)
		}
		e.Effector, e.Effected = pet, pet
		runOn(t, pet.Queue(), func() { pet.EffectList().Add(e) })
		drainUntilQuiet(t, h.client)
	}

	killPetKeepingFrames(t, h, pet)
	gotInfos, statuses := petFramesBeforeDie(t, drainFrames(t, h.client), pet.ObjectID())
	if gotInfos != infos {
		t.Fatalf("owner PetInfo frames before the pet's Die = %d, want %d", gotInfos, infos)
	}
	if buff && statuses != plainStatuses {
		t.Fatalf("owner PetStatusUpdate frames before the pet's Die = %d, want %d as without the buff", statuses, plainStatuses)
	}
	if held := pet.EffectList().All(); len(held) != 0 {
		t.Fatalf("pet still holds %d effects after death, want none", len(held))
	}
	return statuses
}
