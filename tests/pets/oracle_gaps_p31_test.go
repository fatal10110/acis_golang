package pets

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestAssassinServitorBuffCritsFromBehindOnly lays the shipped Assassin
// Servitor buff (1348-1: a critical-rate multiplier gated on the attacker
// standing behind its target) on a pet and has it swing at a monster. The
// pet's critical roll equals its unbuffed rate, so a swing crits exactly when
// the gated term applies: from behind it does, from the front it does not.
// The fixture monster faces east (heading 0).
func TestAssassinServitorBuffCritsFromBehindOnly(t *testing.T) {
	t.Parallel()
	table, err := xmldata.LoadSkillDefinitions(datapack.Path(t, "data", "xml", "skills"), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	def, ok := table.Get(1348, 1)
	if !ok {
		t.Fatal("shipped skill 1348-1 missing")
	}
	for _, tc := range []struct {
		name     string
		dx       int
		wantCrit bool
	}{
		{"from behind", 60, true},
		{"from the front", -60, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wolf := wolfTemplate()
			wolf.CritRate = 4
			h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
			})
			pet, _ := h.spawnWolf(t)
			px, py, pz := pet.Position()
			at := location.Location{X: px + tc.dx, Y: py, Z: pz}
			hostile := h.srv.SpawnHostileNPCAt(t, at)
			drainUntilQuiet(t, h.client)

			runOn(t, pet.Queue(), func() {
				unbuffed := int(pet.Against(nil, nil).CriticalRate())
				effect.Apply(pet, pet, effect.SkillFromDefinition(*def), def.Effects)
				calls := 0
				pet.SetRollSource(func(int) int {
					calls++
					if calls%2 == 0 {
						return unbuffed
					}
					return 0
				})
			})
			h.client.Send(encodeAction(hostile.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
			drainFrames(t, h.client)
			h.client.Send(encodeRequestActionUse(petAttackAction, false))

			hit, _ := summonSwingFeedback(t, h.srv, h.client, pet.ObjectID(), time.Second)
			if hit.Flags&attack.HitMiss != 0 {
				t.Fatalf("pet Attack flags = %#x, want a landed hit", hit.Flags)
			}
			if got := hit.Flags&attack.HitCritical != 0; got != tc.wantCrit {
				t.Fatalf("critical = %v (flags %#x), want %v", got, hit.Flags, tc.wantCrit)
			}
		})
	}
}
