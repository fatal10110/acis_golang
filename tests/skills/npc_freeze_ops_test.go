package skills

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The hostile NPC handle operations the freeze of the script-facing API
// adds (#3748): the cast cost checks the wizard behaviors make, the mute,
// attack and spoil reads, the quiet stance change and the 3D distances.

const (
	freezeCheapPhysical = modelskill.ID(9411)
	freezeCostly        = modelskill.ID(9412)
	freezeCheapMagic    = modelskill.ID(9413)
	freezeMonsterMP     = 40
)

// bootFreezeCaster boots one player with a monster standing 150 west of it
// that knows the three probe skills: a free physical one, a physical one
// costing more MP than the monster has, and a free magic one.
func bootFreezeCaster(t *testing.T) (*gameservertest.Server, *npc.Hostile, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	px, py, pz := srv.PlayerPosition(t, objID)
	home := location.Location{X: px - 150, Y: py, Z: pz}
	magic := probeDefinition(freezeCheapMagic, 900, 0)
	magic.Magic = true
	defs := &laterDefinitions{}
	defs.table.Store(modelskill.NewTable([]modelskill.Definition{
		probeDefinition(freezeCheapPhysical, 900, 0),
		probeDefinition(freezeCostly, 900, freezeMonsterMP+10),
		magic,
	}))
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.MPMax = freezeMonsterMP
	monster, _ := srv.SpawnMovingCastingHostileNPC(t, tmpl, home, home, defs)
	drainUntilQuiet(t, srv.Client)
	return srv, monster, objID
}

// TestScriptNPCCastCostAndMuteChecks pins CreatureCast.meetsHpMpConditions
// and meetsHpMpDisabledConditions, Creature.isPhysicalMuted and
// CreatureStatus.getMaxMp on a monster: a skill whose MP the monster lacks,
// no target and a skill the monster does not know all fail; a physical
// mute keeps only the physical skill from the disabled check, and the
// plain check ignores it.
func TestScriptNPCCastCostAndMuteChecks(t *testing.T) {
	t.Parallel()
	srv, monster, objID := bootFreezeCaster(t)
	n, p := script.NPCOf(monster), script.PlayerOf(worldCombatant(t, srv, objID))
	ref := func(id modelskill.ID) modelskill.Ref { return modelskill.Ref{ID: id, Level: 1} }

	onNPCQueue(t, monster, func() {
		if got := n.MaxMP(); got != freezeMonsterMP {
			t.Errorf("MaxMP() = %d, want %d", got, freezeMonsterMP)
		}
		if n.PhysicalMuted() {
			t.Error("PhysicalMuted() before any mute = true")
		}
		for _, tc := range []struct {
			target script.Creature
			id     modelskill.ID
			want   bool
		}{
			{p, freezeCheapPhysical, true},
			{n, freezeCheapMagic, true},
			{p, freezeCostly, false},
			{nil, freezeCheapPhysical, false},
			{p, modelskill.ID(9499), false},
		} {
			if got := n.MeetsHPMPConditions(tc.target, ref(tc.id)); got != tc.want {
				t.Errorf("MeetsHPMPConditions(%v, %d) = %v, want %v", tc.target, tc.id, got, tc.want)
			}
			if got := n.MeetsHPMPDisabledConditions(tc.target, ref(tc.id)); got != tc.want {
				t.Errorf("MeetsHPMPDisabledConditions(%v, %d) = %v, want %v", tc.target, tc.id, got, tc.want)
			}
		}
	})

	mute, err := effect.New(effect.Skill{ID: 1064, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "PhysicalMute", Time: 30})
	if err != nil {
		t.Fatalf("effect.New(PhysicalMute): %v", err)
	}
	mute.Effector, mute.Effected = monster, monster
	onNPCQueue(t, monster, func() { monster.EffectList().Add(mute) })

	onNPCQueue(t, monster, func() {
		if !n.PhysicalMuted() {
			t.Error("PhysicalMuted() under a physical mute = false")
		}
		if !n.MeetsHPMPConditions(p, ref(freezeCheapPhysical)) {
			t.Error("MeetsHPMPConditions of the physical skill under a physical mute = false")
		}
		if n.MeetsHPMPDisabledConditions(p, ref(freezeCheapPhysical)) {
			t.Error("MeetsHPMPDisabledConditions of the physical skill under a physical mute = true")
		}
		if !n.MeetsHPMPDisabledConditions(p, ref(freezeCheapMagic)) {
			t.Error("MeetsHPMPDisabledConditions of the magic skill under a physical mute = false")
		}
	})
}

// TestScriptNPCAttackingNowAndStop pins CreatureAttack.isAttackingNow and
// stop (the quests that halt an NPC's attack): a monster whose swing at the
// player has started is attacking until the stop, which ends the attack at
// once; its hit never lands.
func TestScriptNPCAttackingNowAndStop(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	px, py, pz := srv.PlayerPosition(t, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, srv.Client)
	n := script.NPCOf(attacker.Hostile)
	victim := worldCombatant(t, srv, objID)
	hp := srv.PlayerCurrentHP(t, objID)

	var before, during, after bool
	onNPCQueue(t, attacker.Hostile, func() { before = n.AttackingNow() })
	attacker.StartAttack(victim)
	onNPCQueue(t, attacker.Hostile, func() {
		during = n.AttackingNow()
		n.StopAttack()
		after = n.AttackingNow()
	})
	if before || !during || after {
		t.Fatalf("AttackingNow: idle %v, swinging %v, stopped %v; want false, true, false", before, during, after)
	}
	srv.Advance(t, 2*time.Second)
	if got := srv.PlayerCurrentHP(t, objID); got != hp {
		t.Fatalf("player HP %v after the stopped swing, want %v", got, hp)
	}
}

// TestScriptNPCSpoilerStanceAndDistances pins SpoilState.isActualSpoiler
// (Monster.getSpoilState().isActualSpoiler), Creature.setRunning (the run
// stance with nothing shown), WorldObject.distance3D and isIn3DRadius
// (strictly within), and the creature reads Creature.getAttackType,
// isRiding and Summon.getControlItemId give a monster.
func TestScriptNPCSpoilerStanceAndDistances(t *testing.T) {
	t.Parallel()
	srv, monster, objID := bootFreezeCaster(t)
	n, p := script.NPCOf(monster), script.PlayerOf(worldCombatant(t, srv, objID))

	onNPCQueue(t, monster, func() {
		if n.ActualSpoiler(p) {
			t.Error("ActualSpoiler before any spoil = true")
		}
		if !monster.SpoilPool().Mark(objID) {
			t.Error("fixture: the monster is already spoiled")
			return
		}
		if !n.ActualSpoiler(p) || n.ActualSpoiler(nil) {
			t.Errorf("after the spoil: ActualSpoiler(player) %v, ActualSpoiler(nil) %v; want true, false", n.ActualSpoiler(p), n.ActualSpoiler(nil))
		}

		running := monster.Running()
		n.SetRunningQuietly(!running)
		if monster.Running() == running {
			t.Errorf("SetRunningQuietly(%v) left the stance at %v", !running, running)
		}

		nl, pl := script.LocOf(n), script.LocOf(p)
		dx, dy, dz := float64(nl.X-pl.X), float64(nl.Y-pl.Y), float64(nl.Z-pl.Z)
		want := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if got := n.Distance3D(p); got != want {
			t.Errorf("Distance3D = %v, want %v", got, want)
		}
		if !script.InRadius3D(n, p, int(want)+1) || script.InRadius3D(p, n, int(want)) {
			t.Errorf("InRadius3D at %v: radius %d %v, radius %d %v", want, int(want)+1, script.InRadius3D(n, p, int(want)+1), int(want), script.InRadius3D(p, n, int(want)))
		}
		if got := script.AttackType(n); got != monster.AttackType() || got == item.WeaponBow {
			t.Errorf("AttackType(monster) = %v, want its own %v", got, monster.AttackType())
		}
		if script.Riding(n) || script.PetControlItemID(n) != 0 {
			t.Errorf("monster: Riding %v, PetControlItemID %d; want false, 0", script.Riding(n), script.PetControlItemID(n))
		}
	})
	if got := srv.ReadQueued(t, srv.Client); len(got) != 0 {
		t.Fatalf("the quiet stance change sent %d frames, want none", len(got))
	}
}
