package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// communityBoard is the board's settings and mail, fixed at boot.
type communityBoard struct {
	cfg  bbs.Config
	mail *bbs.Mailbox
	// serverNews shows the server news page at login.
	serverNews bool
}

// boardSession is a player's community board state: the friends and
// blocked characters picked on the friends board, and the mail list page
// last shown. Owned by the player's queue.
type boardSession struct {
	friends bbs.Selection
	blocks  bbs.Selection
	// mailPosition is the mail list page last shown; a mail command that
	// finds no mail goes back to it.
	mailPosition int
}

// Every board command and form the board answers runs on the player's
// queue. A command whose arguments do not parse (a missing token, a number
// that does not read) shows nothing: the board link that sent it leaves no
// client action pending.

// requestShowBoard opens the board on its home command.
func (l *GameClientLink) requestShowBoard(live *livePlayer) {
	l.boardCommand(live, l.board.cfg.Home)
}

// boardCommand runs a board link's command.
func (l *GameClientLink) boardCommand(live *livePlayer, command string) {
	if !l.board.cfg.Enabled {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCommunityBoardOffline))
		return
	}
	switch {
	case strings.HasPrefix(command, "_bbshome"):
		l.boardHome(live, command)
	case strings.HasPrefix(command, "_bbsgetfav"):
		l.boardUnported(live, command, "favorites board (#3201)")
	case strings.HasPrefix(command, "_bbsloc"):
		l.boardUnported(live, command, "region board (#3202)")
	case strings.HasPrefix(command, "_bbsclan"):
		l.boardClan(live, command)
	case strings.HasPrefix(command, "_bbsmemo"):
		l.boardUnported(live, command, "memo board (#3201)")
	case strings.HasPrefix(command, "_bbsmail"), command == "_maillist_0_1_0_":
		l.boardMail(live, command)
	case strings.HasPrefix(command, "_friend"), strings.HasPrefix(command, "_block"):
		l.boardFriends(live, command)
	case strings.HasPrefix(command, "_bbstopics"), strings.HasPrefix(command, "_bbsposts"):
		l.boardUnported(live, command, "memo board (#3201)")
	default:
		l.sendBoard(live, bbs.NotImplemented(command))
	}
}

// requestBBSWrite submits a board form.
func (l *GameClientLink) requestBBSWrite(live *livePlayer, req clientpackets.RequestBBSWrite) {
	if !l.board.cfg.Enabled {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCommunityBoardOffline))
		return
	}
	switch req.URL {
	case "Topic", "Post":
		l.boardUnported(live, req.URL, "memo board (#3201)")
	case "_bbsclan":
		l.boardClanWrite(live, req.Args)
	case "Mail":
		l.boardMailWrite(live, req.Args)
	case "_friend":
		l.boardFriendsWrite(live, req.Args)
	default:
		// The region board takes no form either: its form answers as an
		// unknown one, naming its first argument.
		name := req.URL
		if req.URL == "_bbsloc" {
			name = req.Args[0]
		}
		l.sendBoard(live, bbs.NotImplemented(name))
	}
}

// boardUnported answers a board command whose board is not ported yet with
// the page an unknown command shows.
// ponytail: the favorites, memo and region boards need the forum model
// (#3201) and the castle registry (#3202); until then their commands show
// the unknown-command page and log the gap.
func (l *GameClientLink) boardUnported(live *livePlayer, command, board string) {
	l.log.Debug().Str("command", command).Str("board", board).Msg("community board: board not modeled")
	l.sendBoard(live, bbs.NotImplemented(command))
}

// boardHome shows the home board: its index, or the page the command
// names after a ';'.
func (l *GameClientLink) boardHome(live *livePlayer, command string) {
	switch {
	case command == "_bbshome":
		l.sendBoardFile(live, bbs.TopFolder+bbs.TopIndex)
	case strings.HasPrefix(command, "_bbshome;"):
		tokens := bbs.Tokens(command, ";")
		if len(tokens) < 2 {
			return
		}
		l.sendBoardFile(live, bbs.TopFolder+tokens[1])
	default:
		l.sendBoard(live, bbs.NotImplemented(command))
	}
}

// boardPage returns the board page name, under the board's page folder;
// false, logged, when there is no such page.
func (l *GameClientLink) boardPage(name string) (string, bool) {
	page, ok := l.html.Get(bbs.PageFolder + name)
	if !ok {
		l.log.Debug().Str("page", name).Msg("community board: page missing")
	}
	return page, ok
}

// sendBoardFile shows the board page name; a missing page shows nothing.
func (l *GameClientLink) sendBoardFile(live *livePlayer, name string) {
	if page, ok := l.boardPage(name); ok {
		l.sendBoard(live, page)
	}
}

// sendBoard shows html in the board window, in up to three parts. A page
// too long for three parts shows nothing.
func (l *GameClientLink) sendBoard(live *livePlayer, html string) {
	for _, frame := range serverpackets.FramesShowBoardPage(html) {
		live.SendFrame(frame)
	}
}

// sendBoardEdit shows html as a page with an edit form, then fills the form
// with text, title and date. A page too long for the form window is left
// out; the form is filled either way.
func (l *GameClientLink) sendBoardEdit(live *livePlayer, html, text, title, date string) {
	if frame, ok := serverpackets.FrameShowBoardEdit(html); ok {
		live.SendFrame(frame)
	}
	live.SendFrame(serverpackets.FrameShowBoardFields(bbs.EditFields(live.Name, live.ObjectID(), live.AccountName(), text, title, date)))
}

// sendLoginBoardPages sends, at login, the new-mail notice when live has
// unread mail and the board is on, then its clan's notice when the board
// is on and the clan shows one, or else the server news when they are
// shown.
func (l *GameClientLink) sendLoginBoardPages(client *Client, live *livePlayer) {
	if l.board.cfg.Enabled && l.board.mail != nil && l.board.mail.HasUnread(live.ObjectID()) {
		client.Session.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNewMail))
		client.Session.SendFrame(serverpackets.FramePlaySound(serverpackets.SoundNewMail))
		client.Session.SendFrame(serverpackets.FrameExMailArrived())
	}
	if cl, ok := l.clanService().ClanOf(live.Character); l.board.cfg.Enabled && ok {
		if notice, shown := cl.Notice(); shown {
			page := l.loginPage("data/html/clan_notice.htm")
			page = strings.ReplaceAll(page, "%clan_name%", cl.Name())
			page = strings.ReplaceAll(page, "%notice_text%", bbs.NoticeText(notice))
			l.sendLoginPage(client, live, page)
			return
		}
	}
	if l.board.serverNews {
		l.sendLoginPage(client, live, l.loginPage("data/html/servnews.htm"))
	}
}

// loginPage returns the page file, or the missing-page notice naming it.
func (l *GameClientLink) loginPage(file string) string {
	if page, ok := l.html.Get(file); ok {
		return page
	}
	return "<html><body>My html is missing:<br>" + file + "</body></html>"
}

// sendLoginPage opens page in an HTML window during the login burst and
// makes its links the ones live may send back.
func (l *GameClientLink) sendLoginPage(client *Client, live *livePlayer, page string) {
	live.bypasses.record(serverpackets.NpcHtmlBody(page))
	client.Session.SendFrame(serverpackets.FrameNpcHtmlMessage(0, page, 0))
}
