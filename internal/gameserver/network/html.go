package network

import (
	"fmt"
	"strconv"
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

func (l *GameClientLink) requestBypassToServer(live *livePlayer, req clientpackets.RequestBypassToServer) {
	if live == nil || l.html == nil || req.Command == "" {
		return
	}
	const playerHelp = "player_help "
	if !strings.HasPrefix(req.Command, playerHelp) {
		return
	}
	l.sendPlayerHelp(live, strings.TrimPrefix(req.Command, playerHelp))
}

func (l *GameClientLink) sendPlayerHelp(live *livePlayer, requestedPath string) {
	if strings.Contains(requestedPath, "..") {
		return
	}
	fields := strings.Fields(requestedPath)
	if len(fields) == 0 {
		return
	}

	parts := strings.SplitN(fields[0], "#", 2)
	file := "data/html/help/" + parts[0]
	itemID := int32(0)
	if len(parts) == 2 {
		id, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return
		}
		itemID = int32(id)
	}

	html, ok := l.html.Get(file)
	if !ok {
		html = fmt.Sprintf("<html><body>My html is missing:<br>%s</body></html>", file)
	}
	live.SendFrame(serverpackets.FrameNpcHtmlMessage(0, html, itemID))
}
