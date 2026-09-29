package pets

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	ownerHealSkill     = 1011
	ownerManaHealSkill = 1013
	ownerHealPower     = 30
	ownerManaHealPower = 12
)

// bootOwnerHealer brings the owner in knowing a static heal and a mana heal,
// both single-target and free, calls out the wolf and targets it.
func bootOwnerHealer(t *testing.T) (*petWorld, *summon.Actor) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
		{
			ID: ownerHealSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "HEAL_STATIC", Power: ownerHealPower, CastRange: 600,
			StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 0,
		},
		{
			ID: ownerManaHealSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "MANAHEAL", Power: ownerManaHealPower, CastRange: 600,
			StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 0,
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t, gameservertest.WithSkills(skills))
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	for _, id := range []int{ownerHealSkill, ownerManaHealSkill} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	pet, _ := h.spawnWolf(t)
	x, y, z := pet.Position()
	h.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, h.client)
	return h, pet
}

// TestOwnerHealOnPetRefreshesPetWindow has the owner heal, then mana heal,
// its wounded, drained wolf. The player's hit first republishes the pet's
// status before the restore lands (CreatureCast.onMagicHitTimer,
// CreatureCast.java:274-288 -> Summon.updateAndBroadcastStatus): one
// PetStatusUpdate with the old values to the owner and one NpcInfo to a
// watching player. The restore then sends its own pair, the PetStatusUpdate
// carrying the new values, as the reference's summon status broadcast does
// on every HP or MP change.
func TestOwnerHealOnPetRefreshesPetWindow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		skill int32
		step  petVitals
	}{
		{name: "heal", skill: ownerHealSkill, step: petVitals{hp: ownerHealPower}},
		{name: "mana heal", skill: ownerManaHealSkill, step: petVitals{mp: ownerManaHealPower}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, pet := bootOwnerHealer(t)
			watcher := h.joinSecondPlayer(t, "Watcher")
			runOn(t, pet.Queue(), func() {
				pet.SetHP(pet.HP() - 100)
				pet.ReduceMP(50)
			})
			drainUntilQuiet(t, h.client)
			drainUntilQuiet(t, watcher.client)

			before := currentPetVitals(pet)
			want := petVitals{hp: before.hp + tc.step.hp, mp: before.mp + tc.step.mp}
			h.client.Send(encodeRequestMagicSkillUse(tc.skill))
			h.srv.AdvanceUntil(t, "the restore landing on the pet", func() bool { return currentPetVitals(pet) == want })

			var updates []petVitals
			for _, frame := range drainFrames(t, h.client) {
				if frame[0] == serverpackets.OpcodePetStatusUpdate {
					updates = append(updates, readPetStatusVitals(t, frame))
				}
			}
			if len(updates) != 2 || updates[0] != before || updates[1] != want {
				t.Fatalf("owner PetStatusUpdates = %+v, want the pre-skill %+v then %+v", updates, before, want)
			}
			if n := countNPCInfoFor(drainFrames(t, watcher.client), pet.ObjectID()); n != 2 {
				t.Fatalf("watcher got %d pet NpcInfo, want 2 (pre-skill refresh, then the restore)", n)
			}
		})
	}
}

// TestPetCastMPCostRefreshesPetWindow has the wolf pay its strike's MP cost:
// the payment sends the owner one PetStatusUpdate carrying the reduced MP.
func TestPetCastMPCostRefreshesPetWindow(t *testing.T) {
	t.Parallel()
	const mpCost = 9
	strike := wolfStrike()
	strike.MPConsume = mpCost
	strike.Power = 1
	h, petActor, _ := bootWolfStrikerWith(t, strike)
	before := currentPetVitals(petActor)
	want := petVitals{hp: before.hp, mp: before.mp - mpCost}

	startWolfStrike(t, h)
	h.srv.AdvanceUntil(t, "the pet paying its strike's MP cost", func() bool { return currentPetVitals(petActor) == want })

	var updates []petVitals
	for _, frame := range drainFrames(t, h.client) {
		if frame[0] == serverpackets.OpcodePetStatusUpdate {
			updates = append(updates, readPetStatusVitals(t, frame))
		}
	}
	if len(updates) != 1 || updates[0] != want {
		t.Fatalf("owner PetStatusUpdates = %+v, want one carrying %+v", updates, want)
	}
}
