package network

import (
	"fmt"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Etc-item handler names of the items whose use only opens a client window
// or plays a die throw. None of them consumes the item; the Record of Seven
// Signs (sevenSignsRecordsHandler) is one of them too.
const (
	booksHandler        = "Books"
	calculatorsHandler  = "Calculators"
	mapsHandler         = "Maps"
	specialXMasHandler  = "SpecialXMas"
	rollingDicesHandler = "RollingDices"
)

// diceLandingOffset is how far in front of the roller a thrown die lands.
const diceLandingOffset = 30

// useWindowItem answers UseItem on a book, calculator, map, Christmas seal,
// die or Record of Seven Signs, and reports whether tmpl is one of them.
// rollDice consumes the client's dice reuse gate and reports whether a throw
// may go ahead; it is asked only when a die is thrown.
func (l *GameClientLink) useWindowItem(live *livePlayer, tmpl *item.Template, rollDice func() bool) bool {
	if tmpl.EtcItem == nil {
		return false
	}
	switch tmpl.EtcItem.Handler {
	case booksHandler:
		l.openBook(live, tmpl.ID)
	case calculatorsHandler:
		live.SendFrame(serverpackets.FrameShowCalculator(tmpl.ID))
	case mapsHandler:
		l.showMiniMap(live, tmpl.ID)
	case specialXMasHandler:
		live.SendFrame(serverpackets.FrameShowXMasSeal(tmpl.ID))
	case rollingDicesHandler:
		l.throwDie(live, tmpl.ID, rollDice)
	case sevenSignsRecordsHandler:
		l.useSevenSignsRecords(live)
	default:
		return false
	}
	return true
}

// openBook shows the help page of book bookID, its links becoming the
// player's valid bypasses, then releases the click. A book without a page
// shows the missing-page notice naming the path.
func (l *GameClientLink) openBook(live *livePlayer, bookID int32) {
	file := "data/html/help/" + strconv.Itoa(int(bookID)) + ".htm"
	page, ok := l.html.Get(file)
	if !ok {
		page = fmt.Sprintf("<html><body>My html is missing:<br>%s</body></html>", file)
	}
	sendValidatedHTML(live, 0, page, bookID)
	live.SendFrame(serverpackets.FrameActionFailed())
}

// showMiniMap opens map mapID drawn for the current Seven Signs period.
func (l *GameClientLink) showMiniMap(live *livePlayer, mapID int32) {
	var period int32
	if l.sevenSigns != nil {
		period = int32(l.sevenSigns.CurrentPeriod())
	}
	live.SendFrame(serverpackets.FrameShowMiniMap(mapID, period))
}

// throwDie rolls die dieID for live: the throw and the rolled number reach
// live and every player that knows it. A throw inside the reuse delay tells
// live alone to wait and shows nothing to anyone else.
func (l *GameClientLink) throwDie(live *livePlayer, dieID int32, allowed func() bool) {
	if allowed != nil && !allowed() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotThrowDiceAtThisTime))
		return
	}
	number := int32(rnd.GetRange(1, 6))
	landing := location.OrientedLocation{Location: live.CurrentLocation(), Heading: live.Heading()}.Ahead(diceLandingOffset)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameDice(live.ObjectID(), dieID, number, landing)
	})
	name := live.Name
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageS1RolledS2, name, number)
	})
}
