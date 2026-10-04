package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/classmaster"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// classMasterBypass runs a class manager's own command for live: the menus
// of the three occupation changes, the change itself, noblesse and the
// skill grant. It reports false when the command stopped outright, so that
// nothing more is sent.
func (l *GameClientLink) classMasterBypass(live *livePlayer, f *npc.Folk, command string) bool {
	cmd, _ := classmaster.ParseCommand(command)
	pages := setPages{l.html}
	switch cmd.Kind {
	case classmaster.CommandMenu:
		page := l.classMaster.MenuPage(pages, f.NpcID(), f.ObjectID(), live.ClassID(), live.Level(), cmd.Tier)
		sendFilledHTML(live, f.ObjectID(), page, 0)
	case classmaster.CommandChangeClass:
		if cmd.Malformed {
			return false
		}
		changed, aborted := l.classMasterTransfer(live, cmd.ClassID)
		if aborted {
			return false
		}
		if changed {
			sendFilledHTML(live, f.ObjectID(), classmaster.ChangedPage(pages, f.NpcID(), cmd.ClassID), 0)
		}
	case classmaster.CommandBecomeNoble:
		noble := live.IsNoble()
		if !noble {
			l.makeNoble(live)
			live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
		}
		sendFilledHTML(live, f.ObjectID(), classmaster.NoblePage(pages, f.NpcID(), noble), 0)
	case classmaster.CommandLearnSkills:
		l.rewardLiveSkills(live)
	}
	return true
}

// classMasterTransfer changes live to classID for the configured price:
// the items the change takes are destroyed and the ones it hands out
// given, each named in chat, then the occupation changes and the base
// class with it. changed reports a change made; aborted a request that
// stopped outright, with nothing more to send.
func (l *GameClientLink) classMasterTransfer(live *livePlayer, classID int) (changed, aborted bool) {
	inv := live.Inventory()
	job, refusal := l.classMaster.CheckTransfer(classmaster.Talker{
		ClassID:       live.ClassID(),
		Level:         live.Level(),
		WeightPenalty: live.WeightPenalty(),
		ItemCount: func(itemID int32) int {
			if inv == nil {
				return 0
			}
			return inv.ItemCount(itemID, -1, true)
		},
	}, classID)
	switch refusal {
	case classmaster.Refused:
		return false, false
	case classmaster.Overweight:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInventoryLessThan80Percent))
		return false, false
	case classmaster.NotEnoughItems:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return false, false
	case classmaster.Unconfigured:
		return false, true
	}
	tmpl, ok := l.templates.Get(classID)
	if !ok {
		l.log.Error().Int32("object_id", live.ObjectID()).Int("class_id", classID).Msg("class master: no template loaded")
		return false, true
	}
	for _, it := range job.Required {
		if !payClassMasterItem(live, it) {
			return false, false
		}
	}
	for _, it := range job.Reward {
		live.AddCreatedItem(it.ID, it.Count, l.nextObjectID)
	}
	if l.changeOccupation(live, classID, tmpl) && !live.SubclassActive() {
		live.SetBaseClass(classID, tmpl)
	}
	live.RefreshHennaStats()
	live.SendFrame(serverpackets.FrameHennaInfo(live.HennaSnapshot()))
	l.broadcastCharacterInfo(live)
	return true, false
}

// payClassMasterItem destroys it out of live's inventory, naming what
// disappeared: adena reads as adena spent, and a talker holding too little
// is told so.
func payClassMasterItem(live *livePlayer, it classmaster.Item) bool {
	if it.ID == item.AdenaID {
		return reduceAdena(live, it.Count)
	}
	return destroyHeldItems(live, it.ID, it.Count)
}

// makeNoble grants live noblesse status: the noble skills, the skill list
// and UserInfo showing them, and the status stored at once.
func (l *GameClientLink) makeNoble(live *livePlayer) {
	live.SetNoble(true)
	l.giveNobleSkills(live.Character)
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	if l.roster == nil {
		return
	}
	l.queueRowWrite(live.ObjectID(), "store noblesse", func(ctx context.Context, ownerID int32) error {
		return l.roster.SaveNoble(ctx, ownerID, true)
	})
}
