package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/signspriest"
)

// sevenSignsBypass runs a Seven Signs priest's or Mammon NPC's
// "SevenSigns <n> ..." command for live at f: its messages, then its page,
// then the move it makes. A sign-up, a turn-in or a payout queues the save
// of live's sign-up row on live's persistence lane, behind the items the
// command took, so the reward a crash would otherwise restore, and the
// sign-up it would otherwise lose, are written right behind the change. It reports false when the command
// aborted, so that nothing more is sent.
func (l *GameClientLink) sevenSignsBypass(live *livePlayer, f *npc.Folk, command string, dawn bool) bool {
	if l.signsPriest == nil {
		l.log.Debug().Str("command", command).Msg("bypass: seven signs priest without a Seven Signs state")
		return true
	}
	classLevel, _ := player.ClassLevel(live.ClassID())
	reply := l.signsPriest.Command(signspriest.Talker{
		ObjectID:   live.ObjectID(),
		Inventory:  live.Inventory(),
		ClassLevel: classLevel,
		CastleClan: live.ClanCastleID() > 0,
		NextID:     l.nextObjectID,
	}, f.ObjectID(), dawn, command)
	if reply.Aborted {
		return false
	}
	if reply.Save {
		state, itemInstances, taken, signUp, log := l.sevenSigns, l.belongings.itemInstances, reply.Taken, reply.SignUp, l.log
		l.queueRowWrite(live.ObjectID(), "seven signs: save after the priests' dialog", func(ctx context.Context, objectID int32) error {
			// A new sign-up's row is inserted first, so the save below has
			// a row to update. Its failure leaves the sign-up in memory and
			// is only logged.
			if signUp != nil {
				if err := state.InsertPlayer(ctx, *signUp); err != nil {
					log.Error().Err(err).Int32("object_id", objectID).Msg("seven signs: insert sign-up")
				}
			}
			// The stacks the command took from are written next, and a
			// failure leaves the row to the next full save: the row never
			// lands ahead of the items that paid for it.
			if itemInstances != nil {
				if err := itemInstances.UpdateItems(ctx, taken); err != nil {
					return fmt.Errorf("write the items taken: %w", err)
				}
			}
			return state.SavePlayer(ctx, objectID)
		})
	}
	for _, n := range reply.Notices {
		sendSignsPriestNotice(live, n)
	}
	switch {
	case reply.HTML != "":
		sendFilledHTML(live, f.ObjectID(), reply.HTML, 0)
	case reply.Page != "":
		file := "data/html/seven_signs/" + reply.Page
		page := l.setPage(file)
		for i := 0; i+1 < len(reply.Fill); i += 2 {
			page = strings.ReplaceAll(page, reply.Fill[i], reply.Fill[i+1])
		}
		page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID())))
		l.sendFilePage(live, f.ObjectID(), file, page, 0)
	}
	if reply.Release {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	if reply.Depart {
		l.teleportLivePlayer(live, reply.Destination, 0)
	}
	return true
}

// sendSignsPriestNotice sends the message n stands for.
func sendSignsPriestNotice(live *livePlayer, n any) {
	switch n := n.(type) {
	case signspriest.SlotsFull:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
	case signspriest.NotEnoughAdena:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
	case signspriest.AdenaSpent:
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(n.Count)))
	case signspriest.NotEnoughItems:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
	case signspriest.ItemsSpent:
		sendDestroyedMessage(live, n.ItemID, n.Count)
	case signspriest.ItemPickedUp:
		if n.Count > 1 {
			live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageYouPickedUpS2S1, n.ItemID, int32(n.Count)))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageYouPickedUpS1, n.ItemID))
		}
	case signspriest.AdenaEarned:
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageEarnedS1Adena, int32(n.Count)))
	case signspriest.AncientAdenaEarned:
		live.SendFrame(serverpackets.FrameSystemMessageItemNameNumber(serverpackets.SystemMessageEarnedS2S1S, item.AncientAdenaID, int32(n.Count)))
	case signspriest.Joined:
		message := serverpackets.SystemMessageSevenSignsJoinedDusk
		if n.Cabal == sevensigns.Dawn {
			message = serverpackets.SystemMessageSevenSignsJoinedDawn
		}
		live.SendFrame(serverpackets.FrameSystemMessage(message))
	case signspriest.SealChosen:
		switch n.Seal {
		case sevensigns.Avarice:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFightForAvarice))
		case sevensigns.Gnosis:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFightForGnosis))
		case sevensigns.Strife:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFightForStrife))
		}
	case signspriest.ContribExceeded:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageContribScoreExceeded))
	case signspriest.ContribIncreased:
		live.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageContribScoreIncreasedS1, serverpackets.ItemNumberParam(int32(n.Score))))
	}
}
