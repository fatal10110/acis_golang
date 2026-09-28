package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// petVitals is the HP/MP pair a PetStatusUpdate reports.
type petVitals struct{ hp, mp int32 }

// readPetStatusVitals returns a PetStatusUpdate's current HP and MP.
func readPetStatusVitals(t *testing.T, frame []byte) petVitals {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePetStatusUpdate, "PetStatusUpdate")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // summon type
	r.ReadInt32() // object id
	r.ReadInt32() // x
	r.ReadInt32() // y
	r.ReadInt32() // z
	r.ReadString()
	r.ReadInt32() // current feed
	r.ReadInt32() // max feed
	hp := r.ReadInt32()
	r.ReadInt32() // max HP
	mp := r.ReadInt32()
	return petVitals{hp: hp, mp: mp}
}

func currentPetVitals(pet *summon.Actor) petVitals {
	return petVitals{hp: int32(pet.HP()), mp: int32(pet.MPValue())}
}

// TestPeriodicEffectTickRefreshesPetWindow lands each periodic HP/MP effect
// on a damaged, drained pet and sweeps it twice. Every tick that changes the
// pet's HP or MP sends its owner exactly one PetStatusUpdate carrying the new
// values and shows a watching player the pet's NpcInfo once, as the
// reference's summon status broadcast does on every HP or MP change.
func TestPeriodicEffectTickRefreshesPetWindow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		effect string
		debuff bool
		value  float64
		step   petVitals
	}{
		{name: "heal over time", effect: "HealOverTime", value: 10, step: petVitals{hp: 10}},
		{name: "damage over time", effect: "DamOverTime", debuff: true, value: 10, step: petVitals{hp: -10}},
		{name: "mana heal over time", effect: "ManaHealOverTime", value: 5, step: petVitals{mp: 5}},
		{name: "mana damage over time", effect: "ManaDamOverTime", debuff: true, value: 5, step: petVitals{mp: -5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t)
			pet, _ := h.spawnWolf(t)
			watcher := h.joinSecondPlayer(t, "Watcher")
			runOn(t, pet.Queue(), func() {
				pet.SetHP(pet.HP() - 50)
				pet.ReduceMP(20)
			})

			e, err := effect.New(effect.Skill{ID: 102, Level: 1, Debuff: tc.debuff},
				modelskill.EffectTemplate{Name: tc.effect, Value: tc.value, Count: 3, Time: 1})
			if err != nil {
				t.Fatalf("effect.New(%s): %v", tc.effect, err)
			}
			e.Effector, e.Effected = pet, pet
			runOn(t, pet.Queue(), func() { pet.EffectList().Add(e) })
			drainUntilQuiet(t, h.client)
			drainUntilQuiet(t, watcher.client)

			for tick := 1; tick <= 2; tick++ {
				before := currentPetVitals(pet)
				h.srv.Advance(t, 1100*time.Millisecond)
				h.srv.TickEffects()

				want := petVitals{hp: before.hp + tc.step.hp, mp: before.mp + tc.step.mp}
				if got := currentPetVitals(pet); got != want {
					t.Fatalf("tick %d: pet vitals = %+v, want %+v", tick, got, want)
				}
				var updates []petVitals
				for _, frame := range drainFrames(t, h.client) {
					if frame[0] == serverpackets.OpcodePetStatusUpdate {
						updates = append(updates, readPetStatusVitals(t, frame))
					}
				}
				if len(updates) != 1 || updates[0] != want {
					t.Fatalf("tick %d: owner PetStatusUpdates = %+v, want one carrying %+v", tick, updates, want)
				}
				if n := countNPCInfoFor(drainFrames(t, watcher.client), pet.ObjectID()); n != 1 {
					t.Fatalf("tick %d: watcher got %d pet NpcInfo, want 1", tick, n)
				}
			}
		})
	}
}

// TestFullPetHealOverTimeTickStaysSilent sweeps a heal-over-time on a pet
// already at full HP: the tick restores nothing, so, like the reference's
// zero-amount bypass, the owner's pet window is not refreshed.
func TestFullPetHealOverTimeTickStaysSilent(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	runOn(t, pet.Queue(), func() { pet.SetHP(pet.MaxHPValue()) })
	e, err := effect.New(effect.Skill{ID: 102, Level: 1}, modelskill.EffectTemplate{Name: "HealOverTime", Value: 10, Count: 3, Time: 1})
	if err != nil {
		t.Fatalf("effect.New: %v", err)
	}
	e.Effector, e.Effected = pet, pet
	runOn(t, pet.Queue(), func() { pet.EffectList().Add(e) })
	drainUntilQuiet(t, h.client)

	h.srv.Advance(t, 1100*time.Millisecond)
	h.srv.TickEffects()
	if hp := pet.HP(); hp != pet.MaxHPValue() {
		t.Fatalf("pet HP = %v after a full-HP heal tick, want max %v", hp, pet.MaxHPValue())
	}
	if n := countOpcode(drainFrames(t, h.client), serverpackets.OpcodePetStatusUpdate); n != 0 {
		t.Fatalf("full-HP heal tick sent %d PetStatusUpdate, want 0", n)
	}
}
