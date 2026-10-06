package network

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func (l *GameClientLink) requestLinkHTML(live *livePlayer, req clientpackets.RequestLinkHTML) {
	if live == nil || l.html == nil {
		return
	}
	if strings.Contains(req.Link, "..") || !strings.Contains(req.Link, ".htm") {
		return
	}
	html, ok := l.html.Get(req.Link)
	if !ok {
		html = fmt.Sprintf("<html><body>My html is missing:<br>%s</body></html>", req.Link)
	}
	sendValidatedHTML(live, 0, html, 0)
}

// sendPlayerHelp opens a help page. Its links are not validated, so the
// bypass whitelist is left as it was.
func (l *GameClientLink) sendPlayerHelp(live *livePlayer, requestedPath string) {
	if strings.Contains(requestedPath, "..") {
		return
	}
	fields := strings.FieldsFunc(requestedPath, helpPathSeparator)
	if len(fields) == 0 {
		return
	}

	// The page name is the first word's text before any '#', and the item
	// id the text between its first and second '#'. Empty pieces at the end
	// count for nothing, so a word of only '#' names no page at all.
	parts := strings.Split(fields[0], "#")
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return
	}
	file := "data/html/help/" + parts[0]
	itemID := int32(0)
	if len(parts) > 1 {
		id, err := commons.ParseInt(parts[1], 32)
		if err != nil {
			return
		}
		itemID = int32(id)
	}
	// The last page of Lidia's diary marks the diary read in her quest; a
	// quest state that holds no number there aborts the request.
	if l.journals != nil && !l.journals.ReadHelpPage(live.Character, parts[0], itemID) {
		return
	}

	html, ok := l.html.Get(file)
	if !ok {
		html = fmt.Sprintf("<html><body>My html is missing:<br>%s</body></html>", file)
	}
	live.SendFrame(serverpackets.FrameNpcHtmlMessage(0, serverpackets.NpcHtmlBody(html), itemID))
}

// helpPathSeparator splits a help bypass into words: only the ASCII space,
// tab, newline, carriage return and form feed separate them.
func helpPathSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}
