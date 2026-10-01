package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// enchantListLevelHint is the level an enchant trainer names to a talker
// below it who has nothing to enchant.
const enchantListLevelHint = 74

// currentTrainer returns the civilian NPC live last selected when live can
// still interact with it: the NPC every skill learn and enchant request is
// made at.
func (l *GameClientLink) currentTrainer(live *livePlayer) (*npc.Folk, bool) {
	f := live.currentFolk.Load()
	if f == nil || !l.playerCanDoInteract(live, f) {
		return nil, false
	}
	return f, true
}

// showSkillList opens f's list of skills live can learn now. An NPC that
// does not train live's profession opens its no-skills page instead. An
// empty list names the level of the next skill, or that none is left, and
// closes the learn window. Every list ends with ActionFailed.
func (l *GameClientLink) showSkillList(live *livePlayer, f *npc.Folk) {
	if !f.CanTeach(live.ClassID()) {
		sendValidatedHTML(live, f.ObjectID(), f.NoSkillsPage(l.html), 0)
		return
	}
	if list, ok := l.acquireSkillList(live); ok {
		live.SendFrame(list)
	} else {
		if next := live.Template().RequiredLevelForNextSkillGrant(live.Level()); next > 0 {
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDoNotHaveFurtherSkillsToLearnS1, int32(next)))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		}
		live.SendFrame(serverpackets.FrameAcquireSkillDone())
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}

// showEnchantSkillList opens f's list of skills live can enchant now. An
// NPC that does not train live's profession opens its no-skills page, and
// a talker short of a third profession a page saying so. An empty list
// says nothing can be enchanted, names level 74 to a talker below it, and
// closes the learn window. Every list ends with ActionFailed.
func (l *GameClientLink) showEnchantSkillList(live *livePlayer, f *npc.Folk) {
	if !f.CanTeach(live.ClassID()) {
		sendValidatedHTML(live, f.ObjectID(), f.NoSkillsPage(l.html), 0)
		return
	}
	if tier, _ := player.ClassLevel(live.ClassID()); tier < skillstate.EnchantMinClassLevel {
		sendValidatedHTML(live, f.ObjectID(), npc.ThirdClassRequiredPage, 0)
		return
	}
	nodes := skillstate.EnchantSkillsFor(live.Character, l.skillTrees, l.skills)
	if len(nodes) == 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageThereIsNoSkillThatEnablesEnchant))
		if live.Level() < enchantListLevelHint {
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDoNotHaveFurtherSkillsToLearnS1, enchantListLevelHint))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		}
		live.SendFrame(serverpackets.FrameAcquireSkillDone())
	} else {
		entries := make([]serverpackets.EnchantSkillEntry, 0, len(nodes))
		for _, node := range nodes {
			entries = append(entries, serverpackets.EnchantSkillEntry{
				ID:     int32(node.ID),
				Level:  int32(node.Level),
				SPCost: int32(node.SP),
				XPCost: int64(node.Exp),
			})
		}
		live.SendFrame(serverpackets.FrameExEnchantSkillList(entries))
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}

// showFishSkillList opens the fishing skills live can learn now. An empty
// list names the level of the next fishing skill, or that none is left,
// and closes the learn window. Every list ends with ActionFailed.
func (l *GameClientLink) showFishSkillList(live *livePlayer) {
	if list, ok := l.fishingAcquireSkillList(live); ok {
		live.SendFrame(list)
	} else {
		if next := l.skillTrees.RequiredLevelForNextFishingSkill(live.Level(), live.HasDwarvenCraft()); next > 0 {
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDoNotHaveFurtherSkillsToLearnS1, int32(next)))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		}
		live.SendFrame(serverpackets.FrameAcquireSkillDone())
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}
