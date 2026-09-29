package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: PlayerCast.doCast (PlayerCast.java:177-185) destroys the skill's
// consume item after CreatureCast.doCast has sent MagicSkillUse, USE_S1 and
// the gauge, through Player.destroyItemByItemId(id, count, true)
// (Player.java:1966-1995). Its message names what went: adena through
// reduceAdena as S1_DISAPPEARED_ADENA (672, a number), a shadow item as
// S1S_REMAINING_MANA_IS_NOW_0 (1982), more than one unit as S2_S1_DISAPPEARED
// (301, item name then item number), one unit as S1_DISAPPEARED (302).

// systemMessageParam is one SystemMessage parameter as it is on the wire.
type systemMessageParam struct {
	typ, value int32
}

// assertSystemMessageParams asserts frame is SystemMessage messageID carrying
// exactly params.
func assertSystemMessageParams(t *testing.T, frame []byte, messageID int, params ...systemMessageParam) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if got := r.ReadInt32(); got != int32(len(params)) {
		t.Fatalf("system message %d param count = %d, want %d", messageID, got, len(params))
	}
	for i, want := range params {
		if got := (systemMessageParam{r.ReadInt32(), r.ReadInt32()}); got != want {
			t.Fatalf("system message %d param %d = %+v, want %+v", messageID, i, got, want)
		}
	}
}

// TestCastConsumeItemMessageFollowsCastStart casts a skill-bar skill paying
// each kind of consume item. The caster reads MagicSkillUse, USE_S1 and the
// gauge, then the message naming what its item cost, then the launch; the
// stack is down by the cost.
func TestCastConsumeItemMessageFollowsCastStart(t *testing.T) {
	t.Parallel()
	const shadowSwordID = 7884
	for _, tc := range []struct {
		name          string
		itemID, count int32
		held          int32
		messageID     int
		params        []systemMessageParam
	}{
		{
			name: "one unit", itemID: 20, count: 1, held: 5,
			messageID: serverpackets.SystemMessageS1Disappeared,
			params:    []systemMessageParam{{serverpackets.SystemMessageParamItemName, 20}},
		},
		{
			name: "several units", itemID: 20, count: 3, held: 5,
			messageID: serverpackets.SystemMessageS2S1Disappeared,
			params: []systemMessageParam{
				{serverpackets.SystemMessageParamItemName, 20},
				{serverpackets.SystemMessageParamItemNumber, 3},
			},
		},
		{
			name: "adena", itemID: item.AdenaID, count: 2, held: 5,
			messageID: serverpackets.SystemMessageS1DisappearedAdena,
			params:    []systemMessageParam{{serverpackets.SystemMessageParamNumber, 2}},
		},
		{
			name: "shadow item", itemID: shadowSwordID, count: 1, held: 1,
			messageID: serverpackets.SystemMessageRemainingManaIsNow0,
			params:    []systemMessageParam{{serverpackets.SystemMessageParamItemName, shadowSwordID}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
					ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					SkillType: "DUMMY", HitTime: 500, StaticHitTime: true, StaticReuse: true,
					ItemConsumeID: int(tc.itemID), ItemConsumeCount: int(tc.count),
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, 5, 1)
			srv.GiveItem(t, objID, tc.itemID, tc.held)
			startInWorld(t, c)
			inv := srv.PlayerInventory(t, objID)
			before := inv.ItemCount(tc.itemID, -1, true)

			c.Send(encodeRequestMagicSkillUse(5, false, false))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, 5, 1)
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSetupGauge, "SetupGauge")
			assertSystemMessageParams(t, c.Read(), tc.messageID, tc.params...)
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
			if got, want := inv.ItemCount(tc.itemID, -1, true), before-int(tc.count); got != want {
				t.Fatalf("item %d count = %d after the cast, want %d", tc.itemID, got, want)
			}
			drainUntilQuiet(t, c)
		})
	}
}
