package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: PlayerStatus.addLevel (PlayerStatus.java:625-648) runs
// giveSkills — whose SkillList ends the level's grant — then the clan and
// party refreshes, refreshWeightPenalty, refreshExpertisePenalty and only
// then UserInfo. refreshExpertisePenalty (Player.java:1146-1190) sends its
// own SkillList and EtcStatusUpdate when the penalty state flips.

// TestLevelUpGradePenaltyFollowsLevelSkillList: a level-19 player without
// Expertise wears the D-grade sword and so carries the weapon grade
// penalty. Reaching level 20 grants Expertise for free, which lifts the
// penalty: the client gets the level's SkillList first, then the penalty's
// SkillList and an EtcStatusUpdate without the penalty, then UserInfo.
func TestLevelUpGradePenaltyFollowsLevelSkillList(t *testing.T) {
	t.Parallel()
	const expertise = 239
	tmpl := gameservertest.ClassTemplate()
	tmpl.Skills = append(tmpl.Skills, player.SkillGrant{SkillID: expertise, Level: 1, MinLevel: 20, Cost: 0})
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 19, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(tmpl),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(sword, false))
	if etc := etcStatusFrames(collectUntilQuiet(t, c)); len(etc) != 1 || !etcStatusGradePenalty(t, etc[0]) {
		t.Fatalf("equip EtcStatusUpdate frames = %d, want one showing the grade penalty", len(etc))
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)

	srv.AddPlayerLevel(t, objID, 1)
	frames := collectUntilQuiet(t, c)

	var lists []int
	etc, userInfo := -1, -1
	for i, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSkillList:
			lists = append(lists, i)
		case serverpackets.OpcodeEtcStatusUpdate:
			if etc >= 0 {
				t.Fatalf("level-up frames %x: more than one EtcStatusUpdate", testsupport.FrameOpcodes(frames))
			}
			etc = i
		case serverpackets.OpcodeUserInfo:
			if userInfo < 0 {
				userInfo = i
			}
		}
	}
	if len(lists) != 2 || etc < 0 || userInfo < 0 {
		t.Fatalf("level-up frames %x: SkillLists at %v, EtcStatusUpdate at %d, UserInfo at %d; want two SkillLists, one EtcStatusUpdate and a UserInfo",
			testsupport.FrameOpcodes(frames), lists, etc, userInfo)
	}
	if !(lists[0] < lists[1] && lists[1] < etc && etc < userInfo) {
		t.Fatalf("level-up frames %x: SkillLists at %v, EtcStatusUpdate at %d, UserInfo at %d; want level SkillList, penalty SkillList, EtcStatusUpdate, UserInfo",
			testsupport.FrameOpcodes(frames), lists, etc, userInfo)
	}
	if etcStatusGradePenalty(t, frames[etc]) {
		t.Fatal("EtcStatusUpdate after gaining Expertise still shows the grade penalty")
	}
}
