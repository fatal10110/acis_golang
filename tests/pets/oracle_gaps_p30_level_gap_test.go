package pets

import (
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootGappedWolf brings in a level 1 owner whose wolf, saved at petLevel,
// knows the strike skill, summons it and targets the fixture monster.
func bootGappedWolf(t *testing.T, petLevel int) (*petWorld, *summon.Actor, *npc.Hostile) {
	t.Helper()
	wolf := wolfTemplate()
	wolf.Skills = map[int]int{wolfStrikeSkill: 1}
	wolf.Pet.Levels[petLevel] = wolfLevelStats(wolfLevelExp)
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		wolfStrike(),
	}), gamesql.NewCharacterSkillStore(db))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skills),
	})
	if err := h.srv.Pets.Save(petCtx(), h.collarID, pet.State{
		Level: petLevel, Exp: wolfLevelExp, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: wolfMaxMeal,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	actor, _ := h.spawnWolf(t)
	if got := actor.Level(); got != petLevel {
		t.Fatalf("wolf level = %d, want %d", got, petLevel)
	}
	hostile := h.srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	return h, actor, hostile
}

// TestPetOutlevelingItsOwnerRefusesCommands pins RequestActionUse's level
// gate for a pet (a servitor has none): a pet more than 20 levels above its
// level 1 owner refuses the attack command with PET_TOO_HIGH_TO_CONTROL and
// never swings, and refuses a commanded special skill silently, with no cast
// and no message; a pet exactly 20 above obeys both.
func TestPetOutlevelingItsOwnerRefusesCommands(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		petLevel int
		obeys    bool
	}{
		{"gap of 20", 21, true},
		{"gap of 21", 22, false},
	} {
		t.Run(tt.name+"/attack", func(t *testing.T) {
			t.Parallel()
			h, wolf, _ := bootGappedWolf(t, tt.petLevel)
			h.client.Send(encodeRequestActionUse(petAttackAction, false))
			frames := advanceCollecting(t, h, 2*time.Second)
			if got := hasSystemMessage(frames, serverpackets.SystemMessagePetTooHighToControl); got == tt.obeys {
				t.Fatalf("PET_TOO_HIGH_TO_CONTROL sent = %v, want %v", got, !tt.obeys)
			}
			if swung := petAttacks(frames, wolf) > 0 || wolf.IsAttackingNow(); swung != tt.obeys {
				t.Fatalf("pet swung = %v, want %v", swung, tt.obeys)
			}
		})
		t.Run(tt.name+"/skill", func(t *testing.T) {
			t.Parallel()
			h, _, _ := bootGappedWolf(t, tt.petLevel)
			h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
			frames := advanceCollecting(t, h, 2*time.Second)
			if hasSystemMessage(frames, serverpackets.SystemMessagePetTooHighToControl) {
				t.Fatal("a refused special skill told the owner PET_TOO_HIGH_TO_CONTROL, want silence")
			}
			_, cast := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse)
			if cast != tt.obeys {
				t.Fatalf("pet cast = %v, want %v (frames %x)", cast, tt.obeys, frameOpcodes(frames))
			}
			if _, failed := firstOpcode(frames, serverpackets.OpcodeActionFailed); !failed {
				t.Fatal("the commanded skill released no ActionFailed")
			}
		})
	}
}
