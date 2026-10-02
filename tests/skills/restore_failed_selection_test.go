package skills

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// brokenPassive is a passive skill whose stat func names no stat, so giving
// it to a character fails.
func brokenPassive(id modelskill.ID) modelskill.Definition {
	return modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationPassive,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "noSuchStat", Value: 1}},
	}
}

// TestFailedSelectionKeepsSavedSkillState pins #3222: a selection whose
// restore fails after reading the character attaches nothing, so nothing
// saves the skill state back at its end; the saved effect and reuse rows
// must still be there, and the next selection that succeeds restores both.
// The reference reads them only once the skills are given
// (Player.restore: giveSkills, then restoreEffects, Player.java:4138-4143).
func TestFailedSelectionKeepsSavedSkillState(t *testing.T) {
	t.Parallel()
	const brokenID = 9001
	cases := []struct {
		name string
		opts func(t *testing.T) []gameservertest.Option
		// breakIt makes the selection fail; mendIt undoes it.
		breakIt, mendIt string
	}{
		{
			name: "give skills",
			opts: func(t *testing.T) []gameservertest.Option {
				tmpl := gameservertest.ClassTemplate()
				tmpl.Skills = append(tmpl.Skills, player.SkillGrant{SkillID: brokenID, Level: 1, MinLevel: 6})
				return []gameservertest.Option{
					gameservertest.WithClassTemplate(tmpl),
					gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingBuffDef(), brokenPassive(brokenID)})),
				}
			},
			breakIt: "UPDATE characters SET level = 6 WHERE obj_Id = ?",
			mendIt:  "UPDATE characters SET level = 5 WHERE obj_Id = ?",
		},
		{
			name: "death penalty passive",
			opts: func(t *testing.T) []gameservertest.Option {
				return []gameservertest.Option{
					gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingBuffDef(), brokenPassive(5076)})),
				}
			},
			breakIt: "UPDATE characters SET death_penalty_level = 1 WHERE obj_Id = ?",
			mendIt:  "UPDATE characters SET death_penalty_level = 0 WHERE obj_Id = ?",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootSavedBuff(t, 100, tc.opts(t)...)
			ctx := context.Background()
			if _, err := srv.DB.ExecContext(ctx, tc.breakIt, objID); err != nil {
				t.Fatalf("break the selection: %v", err)
			}
			c := srv.Client
			c.Send(encodeRequestGameStart(0))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeServerClose, "ServerClose")
			c.ExpectClosed()
			srv.FlushPersistence(t)
			if count, restoreType := skillSaveRow(t, srv, objID, loadingBuffID, 1); count != 1 || restoreType != 0 {
				t.Fatalf("saved rows after the failed selection = %d (restore_type %d), want the buff row intact", count, restoreType)
			}

			if _, err := srv.DB.ExecContext(ctx, tc.mendIt, objID); err != nil {
				t.Fatalf("mend the selection: %v", err)
			}
			relogin := srv.DialClient(t, "player1", 1)
			selectOnly(t, relogin)
			relogin.Send(encodeEnterWorld())
			frames := readEnterWorldBurstWithRestoredBuff(t, relogin)
			if entries := readAbnormalStatusUpdateEntriesFromFrame(t, frames[3]); len(entries) != 1 || entries[0].SkillID != loadingBuffID {
				t.Fatalf("EnterWorld AbnormalStatusUpdate = %+v, want the restored buff", entries)
			}
			coolTimes := readSkillCoolTimeEntriesFromFrame(t, frames[len(frames)-2])
			if len(coolTimes) != 1 || coolTimes[0].SkillID != loadingBuffID || coolTimes[0].RemainingSeconds <= 0 {
				t.Fatalf("SkillCoolTime = %+v, want the buff's restored reuse timer", coolTimes)
			}
		})
	}
}
