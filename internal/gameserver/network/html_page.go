package network

import (
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// sendFilledHTML opens an HTML window on live showing page exactly as
// given and makes its bypass links the ones live may send back. page was
// limited when it was loaded or built, by setPage, setPages or
// NpcHtmlBody; filling its placeholders afterwards may grow it past the
// limit, and it is still sent whole.
func sendFilledHTML(live *livePlayer, objectID int32, page string, itemID int32) {
	live.bypasses.record(page)
	live.SendFrame(serverpackets.FrameNpcHtmlMessage(objectID, page, itemID))
}

// setPage is the datapack page file as an HTML window takes it when the
// page is set, before its placeholders are filled: the page with the page
// limit applied, or the missing-page notice naming file.
func (l *GameClientLink) setPage(file string) string {
	page, ok := l.html.Get(file)
	if !ok {
		page = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	return serverpackets.NpcHtmlBody(page)
}

// setPages reads the datapack pages an NPC dialog fills in the way an HTML
// window takes them when set: each page found has the page limit applied
// as it is loaded, before the NPC fills its placeholders.
type setPages struct{ html *datacache.HTML }

// Get returns the page at path with the page limit applied.
func (p setPages) Get(path string) (string, bool) {
	page, ok := p.html.Get(path)
	if !ok {
		return "", false
	}
	return serverpackets.NpcHtmlBody(page), true
}
