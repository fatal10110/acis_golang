package items

import (
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: ItemSkills.useItem casts the Petrification Scroll's skill 2239
// with the scroll's object id. PlayableCast.canCast (PlayableCast.java:81-96)
// needs the scroll present and at least itemConsumeCount (1) of item 8379.
// PlayableCast.doCast (PlayableCast.java:46-62) destroys the carrier without
// a message, CreatureCast.doCast (CreatureCast.java:148-161) announces the
// cast (MagicSkillUse, no USE_S1 for an item cast, the gauge past 410 ms),
// and PlayerCast.doCast (PlayerCast.java:177-185) then destroys the skill's
// own item with its message, ignoring a failed destroy
// (Player.destroyItemByItemId, Player.java:1966-1995).

const selfConsumingSkillID = 2239

// selfConsumingScrollSkills is a skill table in which skill 2239 consumes
// the scroll carrying it. It targets the caster so the datapack's zone
// clause and party target stay out of the way; the consume shape is the
// datapack's (aCis_datapack/data/xml/skills/2200-2299.xml:655-664).
func selfConsumingScrollSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{
			ID: selfConsumingSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, HitTime: 500, StaticReuse: true,
			ItemConsumeID: int(gameservertest.SelfConsumingScrollID), ItemConsumeCount: 1,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

// TestSelfConsumingScrollCasts uses the scroll with one unit and with two.
// Either way the cast starts: MagicSkillUse, then the gauge, then the own
// item's report. One unit is taken by the carrier, so the own item finds
// nothing left and reads NOT_ENOUGH_ITEMS; two units pay both and read
// S1_DISAPPEARED naming the scroll. Both stacks end empty.
func TestSelfConsumingScrollCasts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		count int32
		check func(*testing.T, []byte)
	}{
		{
			name:  "one unit",
			count: 1,
			check: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageNotEnoughItems)
			},
		},
		{
			name:  "two units",
			count: 2,
			check: func(t *testing.T, f []byte) {
				assertSystemMessageItem(t, f, serverpackets.SystemMessageS1Disappeared, gameservertest.SelfConsumingScrollID)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithSkills(selfConsumingScrollSkills(t)),
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1))
			c, objID := srv.Client, srv.SoleObjectID(t)
			scroll := srv.GiveItem(t, objID, gameservertest.SelfConsumingScrollID, tc.count)
			startInWorld(t, c)

			c.Send(encodeUseItem(scroll, false))
			assertMagicSkillUseSelf(t, c.Read(), objID, selfConsumingSkillID, 1, 500, 0)
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSetupGauge, "SetupGauge")
			tc.check(t, c.Read())
			if !srv.PlayerCastingNow(t, objID) {
				t.Fatal("scroll cast not in flight, want it started")
			}
			if inst := srv.PlayerInventory(t, objID).ItemByTemplateID(gameservertest.SelfConsumingScrollID); inst != nil {
				t.Fatalf("scrolls left = %d, want none", inst.CountValue())
			}

			launched := c.ReadWithTimeout(2 * time.Second)
			if launched == nil {
				t.Fatal("no MagicSkillLaunched after the scroll cast started")
			}
			assertFrameOpcode(t, launched, serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
			srv.InventoryUpdates.Tick()
			drainUntilQuiet(t, c)
			srv.FlushItems(t)
			for _, inst := range persistedItems(t, srv, objID) {
				if inst.TemplateID == gameservertest.SelfConsumingScrollID {
					t.Fatalf("persisted scroll count = %d, want the stack gone", inst.Count)
				}
			}
		})
	}
}
