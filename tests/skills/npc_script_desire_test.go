package skills

import (
	"math"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Script cast desires (NpcAI.addCastDesire): a script asks a monster to
// cast a skill on a player with a weight; the monster's next think promotes
// the heaviest desire and casts. The checked form refuses a skill whose
// reuse or cost the monster cannot meet; the hold form refuses a target the
// monster could only reach by walking.

const (
	costlyProbe   = modelskill.ID(9201)
	shortProbe    = modelskill.ID(9202)
	longProbe     = modelskill.ID(9203)
	costlyProbeMP = 50
)

// TestScriptCastDesireLandsOnPlayer queues a checked, moving cast desire
// for a damage skill aimed at the player: the monster's next think casts
// it, observers see the monster cast at the player, and the player loses
// the skill's power in HP.
func TestScriptCastDesireLandsOnPlayer(t *testing.T) {
	t.Parallel()
	srv, caster, _ := bootRealDamageCaster(t, realDamageProbeSkill())
	c := srv.Client
	objID := srv.SoleObjectID(t)
	player := worldCombatant(t, srv, objID)
	before := srv.PlayerCurrentHP(t, objID)

	caster.AI().AddCastDesire(player, modelskill.Ref{ID: realDamageProbe, Level: 1}, 1000, true, true)
	thinkOnNPCQueue(t, caster)

	use := readUntil(t, c, serverpackets.OpcodeMagicSkillUse)
	r := wireReader(use[1:])
	if gotCaster, gotTarget, gotSkill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); gotCaster != caster.ObjectID() || gotTarget != objID || gotSkill != int32(realDamageProbe) {
		t.Fatalf("MagicSkillUse caster/target/skill = %d/%d/%d, want %d/%d/%d",
			gotCaster, gotTarget, gotSkill, caster.ObjectID(), objID, realDamageProbe)
	}
	want := before - realDamageProbeHP
	srv.AdvanceUntil(t, "the desired cast landing on the player", func() bool { return srv.PlayerCurrentHP(t, objID) == want })
}

// TestScriptCastDesireRefusals drives the real cast controller's gates: a
// checked desire for a skill whose MP the monster lacks is refused while
// the unchecked one is queued, and a hold desire is queued only when the
// player stands within the skill's range plus both collision radii.
func TestScriptCastDesireRefusals(t *testing.T) {
	t.Parallel()
	defs := &laterDefinitions{}
	srv, caster := bootLaterDefinitionsCaster(t, defs)
	objID := srv.SoleObjectID(t)
	player := worldCombatant(t, srv, objID)

	px, py, pz := srv.PlayerPosition(t, objID)
	nx, ny, nz := caster.Position()
	gap := location.Location{X: nx, Y: ny, Z: nz}.Distance2D(location.Location{X: px, Y: py, Z: pz})
	radii := caster.CollisionRadius() + player.CollisionRadius()
	// shortProbe's reach ends just short of the player; longProbe's covers it.
	shortRange := int(math.Floor(gap-radii)) - 1
	longRange := int(math.Ceil(gap))
	defs.table.Store(modelskill.NewTable([]modelskill.Definition{
		probeDefinition(costlyProbe, 150, costlyProbeMP),
		probeDefinition(shortProbe, shortRange, 0),
		probeDefinition(longProbe, longRange, 0),
	}))

	queued := func(id modelskill.ID) bool {
		for _, d := range caster.AI().Desires().Snapshot() {
			if d.Skill.ID == id {
				return true
			}
		}
		return false
	}
	ref := func(id modelskill.ID) modelskill.Ref { return modelskill.Ref{ID: id, Level: 1} }

	caster.AI().AddCastDesire(player, ref(costlyProbe), 10, true, true)
	if queued(costlyProbe) {
		t.Fatal("checked cast desire queued for a skill whose MP the monster lacks")
	}
	caster.AI().AddCastDesire(player, ref(costlyProbe), 10, false, true)
	if !queued(costlyProbe) {
		t.Fatal("unchecked cast desire refused for a skill whose MP the monster lacks")
	}

	caster.AI().AddCastDesire(player, ref(shortProbe), 10, true, false)
	if queued(shortProbe) {
		t.Fatalf("hold cast desire queued for a player %.1f away out of a %d reach", gap, shortRange)
	}
	caster.AI().AddCastDesire(player, ref(shortProbe), 10, true, true)
	if !queued(shortProbe) {
		t.Fatal("moving cast desire refused for an out-of-reach player")
	}
	caster.AI().AddCastDesire(player, ref(longProbe), 10, true, false)
	if !queued(longProbe) {
		t.Fatalf("hold cast desire refused for a player %.1f away within a %d range", gap, longRange)
	}
}

// laterDefinitions is a skill table filled once the test has measured the
// fixture's positions.
type laterDefinitions struct {
	table atomic.Pointer[modelskill.Table]
}

func (d *laterDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	table := d.table.Load()
	if table == nil {
		return modelskill.Definition{}, false
	}
	return table.Definition(ref)
}

// bootLaterDefinitionsCaster is bootRealDamageCaster over defs.
func bootLaterDefinitionsCaster(t *testing.T, defs *laterDefinitions) (*gameservertest.Server, *npc.Hostile) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	caster, _ := srv.SpawnCastingHostileNPC(t, realDamageCasterTemplate(100), defs)
	drainUntilQuiet(t, srv.Client)
	return srv, caster
}

func probeDefinition(id modelskill.ID, castRange, mp int) modelskill.Definition {
	return modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, SkillType: "REAL_DAMAGE",
		CastRange: castRange, MPConsume: mp, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		Power: 1,
	}
}
