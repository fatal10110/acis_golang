package pets

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/ketraorcsupport"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ketraFootman is a Ketra soldier KetraOrcSupport watches.
const ketraFootman = 21324

// TestKetraSoldierPetrifiesAnAllysSummon (KetraOrcSupport.onSeeSpell): a
// Ketra ally's wolf casts a skill of aggro points beside a Ketra soldier,
// which then wants to petrify the wolf, not its owner, and holds no hate
// of it. The player's own cast is alliance_support_see_spell.scenario.
func TestKetraSoldierPetrifiesAnAllysSummon(t *testing.T) {
	t.Parallel()
	strike := wolfStrike()
	strike.AggroPoints = 100
	petrify := modelskill.Definition{
		ID: 4515, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "PARALYZE", Offensive: true, CastRange: 1500, StaticHitTime: true, StaticReuse: true,
	}
	defs := modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		strike,
		petrify,
	})
	wolf := wolfTemplate()
	wolf.Skills = map[int]int{wolfStrikeSkill: 1}
	db := sqltest.SharedDB(t)
	path := "script.feature.KetraOrcSupport"
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithScriptSkills(strike, petrify),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{ketraFootman: script.KindHostile},
			[]script.Listing{{Path: path}}, script.Catalog{path: ketraorcsupport.New}),
	)
	ownerID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE characters SET varka_ketra_ally = 1 WHERE obj_Id = ?`, ownerID); err != nil {
		t.Fatal(err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	petActor, _ := h.spawnWolf(t)
	x, y, z := srv.PlayerPosition(t, ownerID)
	at := location.Location{X: x + 300, Y: y, Z: z}
	hostile := srv.SpawnHostileNPCAt(t, at)
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = ketraFootman, ketraFootman
	footman := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: x - 200, Y: y, Z: z})
	srv.InstallNPCCastSeam(t, footman, defs)
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
	drainFrames(t, h.client)

	startWolfStrike(t, h)
	var got actorai.Desire
	srv.AdvanceUntil(t, "the footman's petrify desire", func() bool {
		for _, d := range footman.AI().Desires().Snapshot() {
			if d.Kind == actorai.IntentionCast && d.Skill.ID == 4515 {
				got = d
				return true
			}
		}
		return false
	})
	if got.FinalTarget == nil || got.FinalTarget.ObjectID() != petActor.ObjectID() || got.Weight != 1000000 || !got.MoveToTarget {
		t.Fatalf("petrify desire = %+v, want on the wolf %d at 1000000, walking to it", got, petActor.ObjectID())
	}
	if hate := footman.AggroHate(petActor); hate != 0 {
		t.Fatalf("footman's hate of the wolf = %v, want 0", hate)
	}
}
