package network

import (
	"fmt"
	"strings"

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
	live.SendFrame(serverpackets.FrameNpcHtmlMessage(0, html, 0))
}
