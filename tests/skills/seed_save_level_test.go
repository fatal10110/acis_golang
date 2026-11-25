package skills

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestGrownSeedSavesAtSkillLevelAndRestoresAtPowerOne casts a single-level
// seed skill twice, so its one live effect has grown to power 2, and logs
// out. The save names the effect by the skill that cast it (level 1), not by
// its grown power, so the row is written; the power itself is not saved, and
// the relog rebuilds the seed from its skill at power 1.
func TestGrownSeedSavesAtSkillLevelAndRestoresAtPowerOne(t *testing.T) {
	t.Parallel()
	seed := modelskill.Definition{
		ID: seedOfFireID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		StaticHitTime: true, SkillType: "SEED",
		// Count 1 is what the skill reader defaults an effect's count to.
		Effects: []modelskill.EffectTemplate{{Name: "Seed", Count: 1, Time: 60, Icon: true}},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{seed})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, seedOfFireID, 1)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	power := func() (p, effects int) {
		onPlayerQueue(t, srv, objID, func(pc *player.Character) {
			p, effects = pc.SeedPower(seedOfFireID), len(pc.EffectList().All())
		})
		return p, effects
	}
	for cast := 1; cast <= 2; cast++ {
		if !castStartsToEnd(t, srv, c, objID, seedOfFireID) {
			t.Fatalf("seed cast %d was refused", cast)
		}
	}
	if p, n := power(); p != 2 || n != 1 {
		t.Fatalf("after two seed casts: power %d with %d effects, want power 2 with one effect", p, n)
	}

	logout(t, srv, c)

	type savedRow struct{ level, count, curTime, restoreType int }
	var rows []savedRow
	q, err := srv.DB.QueryContext(context.Background(),
		"SELECT skill_level, effect_count, effect_cur_time, restore_type FROM character_skills_save WHERE char_obj_id = ? AND skill_id = ?",
		objID, seedOfFireID)
	if err != nil {
		t.Fatal(err)
	}
	for q.Next() {
		var r savedRow
		if err := q.Scan(&r.level, &r.count, &r.curTime, &r.restoreType); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	if err := q.Err(); err != nil {
		t.Fatal(err)
	}
	_ = q.Close()
	// The seed's one pending tick and the seconds it has run, well inside
	// its 60-second period.
	if len(rows) != 1 || rows[0].level != 1 || rows[0].count != 1 ||
		rows[0].curTime < 0 || rows[0].curTime >= 60 || rows[0].restoreType != 0 {
		t.Fatalf("character_skills_save rows for the grown seed = %+v, want one level-1 effect row (count 1, cur time in [0,60))", rows)
	}

	relogin := srv.DialClient(t, "player1", 1)
	relogin.Send(encodeRequestGameStart(0))
	if reply := relogin.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := relogin.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	relogin.Send(encodeEnterWorld())
	readEnterWorldBurstWithRestoredBuff(t, relogin)
	drainUntilQuiet(t, relogin)

	if p, n := power(); p != 1 || n != 1 {
		t.Fatalf("after relog: seed power %d with %d effects, want power 1 with one effect", p, n)
	}
}
