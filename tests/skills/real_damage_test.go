package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// REAL_DAMAGE oracle (aCis RealDamage.useSkill): for each target that is a
// live Creature, hpLeft = HP - power; hpLeft <= 0 calls doDie(caster),
// anything else calls setHp(hpLeft, true). There is no invulnerability,
// damage-permission or CP check on the way, and CreatureCast.callSkill
// hands the targets to the handler without one either. The shipped user is
// "Npc - Hot Spring Nectar" (4989, power 100000, target ONE), which the
// clan-hall watering-game manager casts on another NPC.

const (
	hotSpringNectar   = modelskill.ID(4989)
	realDamageProbe   = modelskill.ID(9106)
	realDamageProbeHP = 25
)

// shippedHotSpringNectar returns the shipped datapack definition of skill
// 4989 and pins the fields this suite's expectations rest on.
func shippedHotSpringNectar(t *testing.T) modelskill.Definition {
	t.Helper()
	for _, def := range shippedSkillDefinitions(t) {
		if def.ID != hotSpringNectar || def.Level != 1 {
			continue
		}
		if def.SkillType != "REAL_DAMAGE" || def.Power != 100000 || def.Target != modelskill.TargetOne || !def.Offensive {
			t.Fatalf("shipped 4989 = %+v, want an offensive ONE REAL_DAMAGE of power 100000", def)
		}
		return def
	}
	t.Fatal("shipped datapack has no skill 4989 level 1")
	return modelskill.Definition{}
}

// realDamageProbeSkill is a REAL_DAMAGE skill weak enough to leave its
// target alive.
func realDamageProbeSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: realDamageProbe, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, SkillType: "REAL_DAMAGE",
		CastRange: 150, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		Power: realDamageProbeHP,
	}
}

func realDamageCasterTemplate(id int) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

// bootRealDamageCaster brings a player in next to a monster wired with the
// production AI-cast seam that knows defs.
func bootRealDamageCaster(t *testing.T, defs ...modelskill.Definition) (*gameservertest.Server, *npc.Hostile, func(target attackable.Combatant, id modelskill.ID)) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	caster, aiCtl := srv.SpawnCastingHostileNPC(t, realDamageCasterTemplate(100), modelskill.NewTable(defs))
	drainUntilQuiet(t, srv.Client)
	cast := func(target attackable.Combatant, id modelskill.ID) {
		t.Helper()
		if !caster.Queue().Post(func() { aiCtl.Cast(target, modelskill.Ref{ID: id, Level: 1}) }) {
			t.Fatal("post npc cast: queue closed")
		}
	}
	return srv, caster, cast
}

// readDieOf reads frames until the Die broadcast for objID.
func readDieOf(t *testing.T, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	for range 100 {
		frame := readUntil(t, c, serverpackets.OpcodeDie)
		if wireReader(frame[1:]).ReadInt32() == objID {
			return
		}
	}
	t.Fatalf("no Die broadcast for %d", objID)
}

// TestHotSpringNectarKillsNPC drives the shipped skill the way its one
// caster uses it: a monster casts it on another NPC, whose 1000 HP the
// 100000 power empties, so the target dies to the caster and observers see
// it fall.
func TestHotSpringNectarKillsNPC(t *testing.T) {
	t.Parallel()
	srv, caster, cast := bootRealDamageCaster(t, shippedHotSpringNectar(t))
	victim := srv.SpawnHostileNPCTemplateAt(t, realDamageCasterTemplate(101), location.Location{X: 110, Y: 20, Z: 30})
	drainUntilQuiet(t, srv.Client)

	cast(victim, hotSpringNectar)
	srv.AdvanceUntil(t, "the NPC dying to Hot Spring Nectar", victim.Dead)
	if hp := victim.HP(); hp != 0 {
		t.Fatalf("NPC HP after lethal real damage = %v, want 0", hp)
	}
	readDieOf(t, srv.Client, victim.ObjectID())
	if caster.Dead() {
		t.Fatal("the caster died")
	}
}

// TestHotSpringNectarKillsInvulnerablePlayerThroughCP pins that the loss
// is not a hit: an invulnerable player with full CP still dies to it, and
// the death reaches the player's own client.
func TestHotSpringNectarKillsInvulnerablePlayerThroughCP(t *testing.T) {
	t.Parallel()
	srv, _, cast := bootRealDamageCaster(t, shippedHotSpringNectar(t))
	objID := srv.SoleObjectID(t)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.SetCP(pc.MaxCPValue())
		pc.Live.SetInvul(true)
	})
	drainUntilQuiet(t, srv.Client)

	cast(worldCombatant(t, srv, objID), hotSpringNectar)
	srv.AdvanceUntil(t, "the player dying to Hot Spring Nectar", func() bool { return srv.PlayerDead(t, objID) })
	if hp := srv.PlayerCurrentHP(t, objID); hp != 0 {
		t.Fatalf("player HP after lethal real damage = %d, want 0", hp)
	}
	readDieOf(t, srv.Client, objID)
}

// TestRealDamageTakesExactHPAndSparesCP covers the survivable branch on a
// player and an NPC: HP drops by exactly the power, the player's CP is
// untouched, and the player's own full status goes out. A loss that lands
// exactly on zero HP kills.
func TestRealDamageTakesExactHPAndSparesCP(t *testing.T) {
	t.Parallel()
	srv, _, cast := bootRealDamageCaster(t, realDamageProbeSkill())
	c, objID := srv.Client, srv.SoleObjectID(t)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetCP(pc.MaxCPValue()) })
	drainUntilQuiet(t, c)
	hp, cp := srv.PlayerCurrentHP(t, objID), srv.PlayerCurrentCP(t, objID)
	if hp <= realDamageProbeHP {
		t.Fatalf("setup: player HP %d leaves no room for a survivable hit", hp)
	}

	cast(worldCombatant(t, srv, objID), realDamageProbe)
	srv.AdvanceUntil(t, "the player losing HP", func() bool { return srv.PlayerCurrentHP(t, objID) < hp })
	if got := srv.PlayerCurrentHP(t, objID); got != hp-realDamageProbeHP {
		t.Fatalf("player HP = %d, want %d", got, hp-realDamageProbeHP)
	}
	if got := srv.PlayerCurrentCP(t, objID); got != cp {
		t.Fatalf("player CP = %d, want untouched %d", got, cp)
	}
	assertCasterStatus(t, srv, readUntil(t, c, serverpackets.OpcodeStatusUpdate), objID, hp-realDamageProbeHP, srv.PlayerCurrentMP(t, objID))
	if srv.PlayerDead(t, objID) {
		t.Fatal("survivable real damage killed the player")
	}

	srv.Advance(t, time.Second)
	victim := srv.SpawnHostileNPCTemplateAt(t, realDamageCasterTemplate(101), location.Location{X: 110, Y: 20, Z: 30})
	drainUntilQuiet(t, c)
	cast(victim, realDamageProbe)
	full := float64(victim.MaxHP())
	srv.AdvanceUntil(t, "the NPC losing HP", func() bool { return victim.HP() < full })
	if got := victim.HP(); got != full-realDamageProbeHP {
		t.Fatalf("NPC HP = %v, want %v", got, full-realDamageProbeHP)
	}

	// The next cast leaves exactly zero HP, which the handler treats as
	// lethal.
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetHP(realDamageProbeHP) })
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	cast(worldCombatant(t, srv, objID), realDamageProbe)
	srv.AdvanceUntil(t, "the player dying at exactly zero HP", func() bool { return srv.PlayerDead(t, objID) })
	readDieOf(t, c, objID)
}
