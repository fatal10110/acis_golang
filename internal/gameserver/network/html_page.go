package network

import (
	"html"
	"strings"

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

// sendFilePage is sendFilledHTML for page, set from the datapack page
// file: while //show html is on, a game master is then told the file's
// name in the general chat, under the name HTML, right after the window. A
// page built rather than set from a file has no file and is never named.
func (l *GameClientLink) sendFilePage(live *livePlayer, objectID int32, file, page string, itemID int32) {
	sendFilledHTML(live, objectID, page, itemID)
	l.namePageFile(live, file)
}

// namePageFile tells live the name of file, the page file of the window it
// was just sent, while //show html is on and live is a game master.
func (l *GameClientLink) namePageFile(live *livePlayer, file string) {
	if file != "" && l.showPageFiles.Load() && live.IsGM() {
		live.SendFrame(serverpackets.FrameCreatureSay(0, serverpackets.SayTypeAll, "HTML", pageFileName(file)))
	}
}

// sendValidatedFile is sendValidatedHTML for html, set from the datapack
// page file, named as sendFilePage names it.
func (l *GameClientLink) sendValidatedFile(live *livePlayer, objectID int32, file, html string, itemID int32) {
	l.sendFilePage(live, objectID, file, serverpackets.NpcHtmlBody(html), itemID)
}

// pageFileName is file as //show html names it: what follows its first
// "html/", the whole name without one.
func pageFileName(file string) string {
	if _, after, ok := strings.Cut(file, "html/"); ok {
		return after
	}
	return file
}

// setPage is the datapack page file as an HTML window takes it when the
// page is set, before its placeholders are filled: the page with the page
// limit applied, or the missing-page notice naming file.
func (l *GameClientLink) setPage(file string) string {
	page, ok := l.html.Get(file)
	if !ok {
		page = missingPage(file)
	}
	return serverpackets.NpcHtmlBody(page)
}

// missingPage is the notice a window shows for a page file the cache does
// not hold. The name is escaped, since it may carry text a player typed:
// the notice shows it as text and offers no link of its own.
func missingPage(file string) string {
	return "<html><body>My html is missing:<br>" + html.EscapeString(file) + "</body></html>"
}

// setPages reads the datapack pages an NPC dialog fills in the way an HTML
// window takes them when set: each page found has the page limit applied
// as it is loaded, before the NPC fills its placeholders. It remembers the
// file last asked for, the one the dialog's page is set from.
type setPages struct {
	html *datacache.HTML
	file string
}

// pages is a fresh reader of the pages one NPC dialog opens.
func (l *GameClientLink) pages() *setPages { return &setPages{html: l.html} }

// Get returns the page at path with the page limit applied.
func (p *setPages) Get(path string) (string, bool) {
	p.file = path
	page, ok := p.html.Get(path)
	if !ok {
		return "", false
	}
	return serverpackets.NpcHtmlBody(page), true
}
