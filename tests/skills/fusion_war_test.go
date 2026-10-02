package skills

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	warFusionSkillID int32 = 3626
	warFusionForceID int32 = 3627
)

// socialClansAtWar seeds Caster and Mate in Knights and Stranger leading
// Rivals, the two clans at war with each other both ways.
func socialClansAtWar(t *testing.T) gameservertest.Option {
	knights, rivals := strconv.Itoa(int(socialKnightsID)), strconv.Itoa(int(socialRivalsID))
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		for _, stmt := range []string{
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
				SELECT ` + knights + `, 'Knights', 5, obj_Id FROM characters WHERE char_name = 'Caster'`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
				VALUES (` + rivals + `, 'Rivals', 5, ` + strconv.Itoa(int(socialStrangerID)) + `)`,
			`UPDATE characters SET clanid = ` + knights + `, power_grade = 6 WHERE char_name IN ('Caster', 'Mate')`,
			`UPDATE characters SET clanid = ` + rivals + `, power_grade = 6 WHERE char_name = 'Stranger'`,
			`INSERT INTO clan_wars (clan1, clan2) VALUES ('` + knights + `', '` + rivals + `'), ('` + rivals + `', '` + knights + `')`,
		} {
			if _, err := db.ExecContext(context.Background(), stmt); err != nil {
				t.Fatalf("seed clans at war: %v", err)
			}
		}
	})
}

// TestCtrlFusionLandsOnWarEnemy casts an offensive FUSION skill with CTRL
// at an unflagged player of a clan at mutual war with the caster's: the
// policy lets it through on CTRL (Playable.java:436-437), and the launch
// takes the target as the cast committed to it without judging it again
// (PlayerCast.doFusionCast, PlayerCast.java:50-89, startFusionSkill on the
// target), so the force effect lands on the enemy before the caster's
// MagicSkillUse reaches it.
func TestCtrlFusionLandsOnWarEnemy(t *testing.T) {
	t.Parallel()
	fusion := modelskill.Definition{
		ID: modelskill.ID(warFusionSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, CastRange: 900, HitTime: 15_000, SkillType: "FUSION",
		TriggeredID: int(warFusionForceID), TriggeredLevel: 1,
	}
	force := modelskill.Definition{
		ID: modelskill.ID(warFusionForceID), Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
	}
	s := bootSocialSceneKnowing(t, fusion, []modelskill.Definition{force}, socialClansAtWar(t))
	s.selectPlayer(t, s.strangerID)

	s.caster.Send(encodeRequestMagicSkillUse(warFusionSkillID, true, false))
	// The cast starts rather than being refused as an invalid target.
	for frame := s.caster.Read(); frame[0] != serverpackets.OpcodeMagicSkillUse; frame = s.caster.Read() {
		if frame[0] == serverpackets.OpcodeSystemMessage || frame[0] == serverpackets.OpcodeActionFailed {
			t.Fatalf("the CTRL fusion cast on a war enemy was refused with frame %#x", frame[0])
		}
	}

	var sawForce bool
	for {
		frame := s.stranger.Read()
		if frame[0] == serverpackets.OpcodeAbnormalStatusUpdate {
			for _, e := range readAbnormalStatusUpdateEntriesFromFrame(t, frame) {
				sawForce = sawForce || e.SkillID == warFusionForceID
			}
			continue
		}
		if frame[0] == serverpackets.OpcodeMagicSkillUse {
			break
		}
	}
	if !sawForce {
		t.Fatalf("the war enemy never received the force effect %d", warFusionForceID)
	}
}
