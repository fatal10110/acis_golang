package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The views a game master picks in the character window (Alt+G).
const (
	gmViewStatus    = 1
	gmViewClan      = 2
	gmViewSkills    = 3
	gmViewQuests    = 4
	gmViewInventory = 5
	gmViewWarehouse = 6
)

// requestGMCommand shows gm the view req.Command picks of the online player
// named req.TargetName, or for the warehouse view of the clan of that name
// when no such player plays. Only a game master whose access level opens
// the character window is answered.
//
// Every refusal is silent, as in the reference: a player who is not such a
// game master, a name no online player (nor, for the warehouse, clan)
// carries, the clan view of a clanless player and a view number outside
// 1-6. The character window holds no pending client action to release.
func (l *GameClientLink) requestGMCommand(gm *livePlayer, req clientpackets.RequestGMCommand) {
	if al := gm.accessLevel(); !al.IsGM || !al.AllowAltG {
		return
	}
	target, ok := l.livePlayerByName(req.TargetName)
	if !ok {
		if req.Command != gmViewWarehouse {
			return
		}
		if cl, found := l.clanService().Table().ByName(req.TargetName); found {
			l.sendGMClanWarehouseView(gm, cl)
		}
		return
	}
	// A target that left meanwhile is shown nothing, as a name no online
	// player carries.
	onPlayer(gm, target, func() { l.sendGMView(gm, target, req.Command) })
}

// sendGMView sends gm the view command picks of target, on target's queue.
func (l *GameClientLink) sendGMView(gm, target *livePlayer, command int32) {
	name := target.CharacterName()
	switch command {
	case gmViewStatus:
		gm.SendFrame(serverpackets.FrameGMViewCharacterInfo(l.userInfoSnapshot(target)))
		gm.SendFrame(serverpackets.FrameGMHennaInfo(target.HennaSnapshot()))
	case gmViewClan:
		// A clanless player has no clan to show: silent, as above.
		if cl, ok := l.clanService().ClanOf(target.Character); ok {
			gm.SendFrame(l.frameGMViewPledgeInfo(cl, target))
		}
	case gmViewSkills:
		gm.SendFrame(serverpackets.FrameGMViewSkillInfo(name, skillListEntries(target.Character, l.skills)))
	case gmViewQuests:
		list := target.Quests().List()
		quests := make([]serverpackets.QuestListEntry, len(list))
		for i, e := range list {
			quests[i] = serverpackets.QuestListEntry{QuestID: e.QuestID, Flags: e.Flags}
		}
		gm.SendFrame(serverpackets.FrameGMViewQuestList(name, quests))
	case gmViewInventory:
		inv := target.Inventory()
		frame, err := serverpackets.FrameGMViewItemList(name, target.InventoryLimit(), target.inventoryItems(), inv.Templates())
		if !l.sendGMViewFrame(gm, frame, err, "GMViewItemList") {
			return
		}
		gm.SendFrame(serverpackets.FrameGMHennaInfo(target.HennaSnapshot()))
	case gmViewWarehouse:
		wh := target.storage.warehouse
		frame, err := serverpackets.FrameGMViewWarehouseWithdrawList(name, int32(wh.Adena()), wh.Items(), wh.Templates())
		l.sendGMViewFrame(gm, frame, err, "GMViewWarehouseWithdrawList")
	}
	// Any other view number shows nothing: silent, as above.
}

// sendGMClanWarehouseView sends gm the view of the warehouse of cl, under
// its leader's name. A warehouse that cannot be read is logged and the
// client released.
func (l *GameClientLink) sendGMClanWarehouseView(gm *livePlayer, cl *clan.Clan) {
	wh, err := l.clanWarehouse(cl.ID())
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", cl.ID()).Msg("gm view: restore clan warehouse")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	frame, err := serverpackets.FrameGMViewWarehouseWithdrawList(cl.SubunitLeaderName(clan.SubunitMain), int32(wh.Adena()), wh.Items(), wh.Templates())
	l.sendGMViewFrame(gm, frame, err, "GMViewWarehouseWithdrawList")
}

// sendGMViewFrame sends gm frame, or on err, a view too large to encode,
// logs it and releases the client. It reports whether frame was sent.
func (l *GameClientLink) sendGMViewFrame(gm *livePlayer, frame wire.Frame, err error, packet string) bool {
	if err != nil {
		l.log.Error().Err(err).Msg("build " + packet)
		gm.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	gm.SendFrame(frame)
	return true
}
