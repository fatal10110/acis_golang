package pets

import (
	"fmt"
	"slices"
	"testing"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

const hotSpringNectar = modelskill.ID(4989)

// TestHotSpringNectarKillsInvulnerablePet has a monster cast the shipped
// "Npc - Hot Spring Nectar" (4989, REAL_DAMAGE, power 100000) on an
// invulnerable pet. REAL_DAMAGE is not a hit (aCis RealDamage.useSkill
// calls doDie on the target once HP - power <= 0, with no invulnerability
// check on the way), so the pet dies: the owner sees it fall and reads the
// pet death message, and no damage message, since no hit landed.
func TestHotSpringNectarKillsInvulnerablePet(t *testing.T) {
	t.Parallel()
	table, err := xmldata.LoadSkillDefinitions(datapack.Path(t, "data", "xml", "skills"), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	nectar, ok := table.Get(hotSpringNectar, 1)
	if !ok || nectar.SkillType != "REAL_DAMAGE" || nectar.Power != 100000 {
		t.Fatalf("shipped 4989 = %+v (found %v), want REAL_DAMAGE of power 100000", nectar, ok)
	}

	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	caster, aiCtl := h.srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{nectar}))
	runOnPetQueue(t, pet, func() { pet.SetInvul(true) })
	drainUntilQuiet(t, h.client)

	if !caster.Queue().Post(func() { aiCtl.Cast(pet, modelskill.Ref{ID: hotSpringNectar, Level: 1}) }) {
		t.Fatal("post npc cast: queue closed")
	}
	h.srv.AdvanceUntil(t, "the pet dying to Hot Spring Nectar", pet.Dead)
	if hp := pet.HP(); hp != 0 {
		t.Fatalf("pet HP after lethal real damage = %v, want 0", hp)
	}

	petID := pet.ObjectID()
	tags := deathTags(t, drainFrames(t, h.client), petID, h.ownerID)
	if len(tags) == 0 || tags[0] != fmt.Sprintf("Die %d", petID) {
		t.Fatalf("owner death frames = %q, want the pet's Die first", tags)
	}
	if !slices.Contains(tags, fmt.Sprintf("SystemMessage %d", serverpackets.SystemMessageResurrectPetWithin20Minutes)) {
		t.Fatalf("owner death frames = %q, want the pet death message", tags)
	}
	if slices.Contains(tags, fmt.Sprintf("SystemMessage %d", serverpackets.SystemMessagePetReceivedS2DamageByS1)) {
		t.Fatalf("owner death frames = %q, want no damage message", tags)
	}
}
