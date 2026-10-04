package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// lotteryAnnouncer tells every player online in state about the lottery.
type lotteryAnnouncer struct {
	state *world.State
}

// NewLotteryAnnouncer returns the announcer telling every player online in
// state about the lottery's rounds.
func NewLotteryAnnouncer(state *world.State) lottery.Announcer {
	return lotteryAnnouncer{state: state}
}

// OnSale announces round's tickets.
func (a lotteryAnnouncer) OnSale(round int32) {
	announceToOnline(a.state, "Lottery tickets are now available for Lucky Lottery #"+strconv.Itoa(int(round))+".", false)
}

// SalesClosed says ticket sales are suspended.
func (a lotteryAnnouncer) SalesClosed() {
	a.toAll(func() wire.Frame {
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageLotteryTicketSalesTempSuspended)
	})
}

// Drawn names round's jackpot and its first-place winners, or says there
// were none and the jackpot rolls over.
func (a lotteryAnnouncer) Drawn(round, prize, winners int32) {
	a.toAll(func() wire.Frame {
		if winners > 0 {
			return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageAmountForWinnerS1IsS2AdenaWeHaveS3PrizeWinner,
				serverpackets.NumberParam(round), serverpackets.NumberParam(prize), serverpackets.NumberParam(winners))
		}
		return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageAmountForLotteryS1IsS2AdenaNoWinner,
			serverpackets.NumberParam(round), serverpackets.NumberParam(prize))
	})
}

// toAll sends the frame build makes to every player online.
func (a lotteryAnnouncer) toAll(build func() wire.Frame) {
	if a.state == nil {
		return
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, p := range a.state.Players() {
			if listener, ok := p.(*livePlayer); ok {
				send(listener)
			}
		}
	})
}

// lotteryBypass answers a lottery seller's "Loto <n>" command: a page of
// the seller's, then ActionFailed. Pressing a number, opening the form and
// buying first need a running round selling tickets, and are otherwise
// refused with a notice and no page. A purchase whose form misses a number,
// or that the talker cannot pay, opens nothing; a paid one takes the price,
// adds it to the jackpot and hands over the ticket. A claim answers no page.
// It reports false for a negative n, which answers nothing at all.
func (l *GameClientLink) lotteryBypass(live *livePlayer, f *npc.Folk, command string) bool {
	lot := l.lottery
	val := lottery.Command(command)
	var page string
	switch {
	case val < 0:
		return false
	case val == lottery.CommandIntro:
		live.lottoPicks = lottery.Picks{}
		page = l.lotteryPage(f, lottery.PageIntro)
	case val <= lottery.CommandBuy:
		if !lotterySelling(live, lot.Status()) {
			return true
		}
		if val == lottery.CommandBuy {
			if !l.buyLotteryTicket(live) {
				return true
			}
			page = l.lotteryPage(f, lottery.PageJackpot)
			break
		}
		live.lottoPicks.Press(val)
		page = live.lottoPicks.Mark(l.lotteryPage(f, lottery.PageForm))
	case val == lottery.CommandJackpot:
		page = l.lotteryPage(f, lottery.PageJackpot)
	case val == lottery.CommandClaims:
		page = strings.ReplaceAll(l.lotteryPage(f, lottery.PageClaims), "%result%", lot.ClaimList(heldTickets(live)))
	case val == lottery.CommandInstructions:
		page = lot.Instructions(l.lotteryPage(f, lottery.PageInstructions))
	default:
		l.claimLotteryTicket(live, int32(val))
		return true
	}
	sendFilledHTML(live, f.ObjectID(), lot.Fill(page, f.ObjectID()), 0)
	live.SendFrame(serverpackets.FrameActionFailed())
	return true
}

// lotterySelling reports whether st sells tickets, telling live why not
// otherwise.
func lotterySelling(live *livePlayer, st lottery.Status) bool {
	switch {
	case !st.Started:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoLotteryTicketsCurrentSold))
	case !st.Selling:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoLotteryTicketsAvailable))
	default:
		return true
	}
	return false
}

// lotteryPage is seller f's lottery page n as the window takes it, before
// its placeholders are filled; a seller without the page shows the default
// one.
func (l *GameClientLink) lotteryPage(f *npc.Folk, n int) string {
	path := "data/html/default/" + strconv.Itoa(f.NpcID()) + "-" + strconv.Itoa(n) + ".htm"
	if _, ok := l.html.Get(path); !ok {
		path = "data/html/npcdefault.htm"
	}
	return l.setPage(path)
}

// buyLotteryTicket sells live the ticket its form holds for the running
// round: the price taken, the jackpot raised, the ticket added and named.
// It reports false, having sold nothing, when the form misses a number or
// live cannot pay.
func (l *GameClientLink) buyLotteryTicket(live *livePlayer) bool {
	numbers, ok := live.lottoPicks.Numbers()
	if !ok {
		return false
	}
	inv := live.Inventory()
	if inv == nil {
		return false
	}
	lot := l.lottery
	round := lot.Status().Round
	price := lot.Config().TicketPrice
	// A buyer who cannot pay is refused before any id is taken: the
	// allocator's cursor never moves back, so an id taken and released for
	// each refused attempt would still use up the id range.
	if int(price) > inv.Adena() {
		reduceAdena(live, int(price))
		return false
	}
	// The ticket's id is taken before the price, so an exhausted id
	// factory refuses the sale instead of charging for no ticket.
	id, err := l.nextObjectID()
	if err != nil {
		l.log.Error().Err(err).Msg("lottery: no object id for a ticket")
		return false
	}
	if !reduceAdena(live, int(price)) {
		l.releaseObjectID(id)
		return false
	}
	lot.IncreasePrize(price)
	if ticket := inv.AddNew(lottery.TicketID, 1, id); ticket != nil {
		ticket.SetCustomType1(int(round))
		inv.SetEnchantLevel(ticket, int(numbers.Low))
		ticket.SetCustomType2(int(numbers.High))
	} else {
		l.releaseObjectID(id)
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageEarnedItemS1, lottery.TicketID))
	return true
}

// claimLotteryTicket cashes in live's ticket objectID of a past round: the
// ticket is destroyed and named, then what it won, if anything, is paid.
// Anything else held under objectID, or nothing, is left alone silently.
func (l *GameClientLink) claimLotteryTicket(live *livePlayer, objectID int32) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	st := inst.Snapshot()
	if st.TemplateID != lottery.TicketID || int32(st.CustomType1) >= l.lottery.Status().Round {
		return
	}
	// A past round's drawing never changes, so checking before the destroy
	// reads what the reference reads after it; the payout's id is taken
	// before the destroy, so an exhausted id factory keeps the ticket.
	_, adena := l.lottery.Check(int32(st.CustomType1), lottery.Numbers{Low: int32(st.EnchantLevel), High: int32(st.CustomType2)})
	var id int32
	if adena > 0 {
		var err error
		if id, err = l.nextObjectID(); err != nil {
			l.log.Error().Err(err).Msg("lottery: no object id for a payout")
			return
		}
	}
	if !destroyHeldItem(live, inst, st.Count) {
		if adena > 0 {
			l.releaseObjectID(id)
		}
		return
	}
	if adena > 0 {
		live.AddRewardItem(item.AdenaID, int(adena), id)
	}
}

// releaseObjectID gives back id, taken for a ticket or payout that was then
// refused, so a refused attempt holds no object id; an allocator that
// cannot take ids back keeps it.
func (l *GameClientLink) releaseObjectID(id int32) {
	if r, ok := l.ids.(interface{ ReleaseID(int32) }); ok {
		r.ReleaseID(id)
	}
}

// destroyHeldItem destroys count units of inst out of live's inventory,
// naming what disappeared; one it cannot destroy reads as not enough items.
func destroyHeldItem(live *livePlayer, inst *item.Instance, count int) bool {
	st := inst.Snapshot()
	if live.Inventory().DestroyItem(inst, count) == nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return false
	}
	sendDestroyedMessage(live, st.TemplateID, count)
	return true
}

// heldTickets lists live's lottery tickets in inventory order.
func heldTickets(live *livePlayer) []lottery.HeldTicket {
	inv := live.Inventory()
	if inv == nil {
		return nil
	}
	var out []lottery.HeldTicket
	for _, inst := range inv.ItemsByTemplateID(lottery.TicketID) {
		st := inst.Snapshot()
		out = append(out, lottery.HeldTicket{
			ObjectID: st.ObjectID,
			Round:    int32(st.CustomType1),
			Numbers:  lottery.Numbers{Low: int32(st.EnchantLevel), High: int32(st.CustomType2)},
		})
	}
	return out
}
