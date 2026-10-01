package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestHotSpringNectarKillsUndyingFolk drives the shipped skill at its real
// target kind, a civilian NPC: RealDamage.useSkill calls doDie(caster)
// once the loss empties the HP, which kills even an undying template (the
// 1 HP floor lives only in CreatureStatus.reduceHp). Observers see it
// fall.
func TestHotSpringNectarKillsUndyingFolk(t *testing.T) {
	t.Parallel()
	srv, caster, cast := bootRealDamageCaster(t, shippedHotSpringNectar(t))
	tmpl := gameservertest.FolkTemplate("Folk", 35588)
	if !tmpl.Undying {
		t.Fatal("fixture civilian NPC is not undying")
	}
	victim := srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: 110, Y: 20, Z: 30})
	drainUntilQuiet(t, srv.Client)

	cast(victim, hotSpringNectar)
	srv.AdvanceUntil(t, "the civilian NPC dying to Hot Spring Nectar", victim.Dead)
	if hp := victim.HP(); hp != 0 {
		t.Fatalf("civilian NPC HP after lethal real damage = %v, want 0", hp)
	}
	readDieOf(t, srv.Client, victim.ObjectID())
	if caster.Dead() {
		t.Fatal("the caster died")
	}
}
