package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

var shippedSkillTable = sync.OnceValue(func() *modelskill.Table {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "aCis_datapack", "data", "xml", "skills")
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	table, err := xmldata.LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		panic(err)
	}
	return table
})

// shippedSkill returns one shipped skill level, skipping the test when the
// datapack is not checked out next to the module.
func shippedSkill(t *testing.T, id modelskill.ID, level int) modelskill.Definition {
	t.Helper()
	table := shippedSkillTable()
	if table == nil {
		t.Skip("aCis_datapack not checked out near the module root")
	}
	def, ok := table.Get(id, level)
	if !ok {
		t.Fatalf("shipped skill %d level %d missing", id, level)
	}
	return def
}

// TestPlayerCastEvaluatesSkillCondition pins PlayableCast.canCast's
// skill.checkCondition for a player's own cast: Guts (139-1) carries
// <cond msgId="113" addName="1"><player hp="30"/></cond>. At full HP the
// cast is refused with S1_CANNOT_BE_USED naming the skill at level 1, with
// no ActionFailed and no cast broadcast; at 30% HP or less it starts.
func TestPlayerCastEvaluatesSkillCondition(t *testing.T) {
	t.Parallel()
	def := shippedSkill(t, 139, 1)
	srv := bootTargetConditionCaster(t, def)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(139, false, false))
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, 139, 1)
	assertNoActionFailedUntilQuiet(t, c, "Guts at full HP")
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("Guts at full HP started a cast")
	}

	maxHP := srv.PlayerMaxHP(t, objID)
	srv.DamagePlayerHP(t, objID, srv.PlayerCurrentHP(t, objID)-maxHP*3/10)
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(139, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "Guts at 30% HP")
}

// TestRechargeConditionReadsTargetKnownSkills pins Recharge (1013-1)'s
// <cond msgId="113" addName="1"><not><target active_skill_id="1013"/></not>
// </cond>: the target's known skills answer it (ConditionTargetActiveSkillId
// reads getSkill, an NPC's template skills), so a target whose template
// grants Recharge refuses the cast by name and one without it is not
// refused by the condition.
func TestRechargeConditionReadsTargetKnownSkills(t *testing.T) {
	t.Parallel()
	def := shippedSkill(t, 1013, 1)
	// The fixture caster's MP pool is not what this test is about.
	def.MPConsume, def.MPInitialConsume = 0, 0
	for _, tt := range []struct {
		name   string
		skills map[int]int
		reject bool
	}{
		{"target knows Recharge", map[int]int{1013: 1}, true},
		{"target does not know Recharge", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootTargetConditionCaster(t, def)
			c := srv.Client
			startInWorld(t, c)
			tmpl := conditionNPCTemplate("Monster", npc.RaceHumanoid)
			tmpl.Skills = tt.skills
			mob := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			targetHostile(t, c, mob.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(1013, false, false))
			frame := c.Read()
			if tt.reject {
				assertSystemMessageSkillFrame(t, frame, serverpackets.SystemMessageS1CannotBeUsed, 1013, 1)
				assertNoActionFailedUntilQuiet(t, c, tt.name)
				return
			}
			// Past the condition, the ONE target handler decides; this test
			// only pins that the condition itself no longer refuses.
			if frame[0] == serverpackets.OpcodeSystemMessage && wireReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageS1CannotBeUsed {
				t.Fatalf("%s: refused by the Recharge condition", tt.name)
			}
		})
	}
}
