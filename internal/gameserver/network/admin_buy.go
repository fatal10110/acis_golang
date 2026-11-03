package network

import (
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminBuy answers //buy: the GM shop index page; //buy <id> opens the buy
// window of buylist id, any list, untaxed and without a merchant. An id
// that is not a number or names no list answers so.
func (l *GameClientLink) adminBuy(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		l.sendAdminFile(gm, "gmshops.htm")
		return
	}
	id, ok := parseInt32(args[0])
	if !ok {
		sendText(gm, "Invalid buylist id.")
		return
	}
	list, ok := l.merchant.List(int(id))
	if !ok {
		sendText(gm, "Invalid buylist id.")
		return
	}
	inv := gm.Inventory()
	if inv == nil {
		return
	}
	frame, err := serverpackets.FrameBuyList(list, l.merchant.Count, inv.Adena(), 0, l.merchant.Config().SiegeGuardsPriceRate, l.itemTemplates)
	if err != nil {
		l.log.Error().Err(err).Int32("buylist", id).Msg("build BuyList")
		return
	}
	gm.SendFrame(frame)
}
