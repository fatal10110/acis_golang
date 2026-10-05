package network

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// unknownWriter names a mail's sender no character has.
const unknownWriter = "Unknown"

// boardMail runs a mail board command: _bbsmail (or the tab's
// _maillist_0_1_0_) lists the inbox; _bbsmail;<folder>[;page[;searchType;
// search]] lists a folder, the folder named in lower case; _bbsmail;crea
// opens the write form; _bbsmail;<view|reply|del|store>;<id> acts on a
// mail of the player's. Any other action, or a mail id the player has no
// mail under, goes back to the list page last shown; the folder list's own
// page links name the folder in upper case and so land there too.
func (l *GameClientLink) boardMail(live *livePlayer, command string) {
	if l.board.mail == nil {
		return
	}
	if command == "_bbsmail" || command == "_maillist_0_1_0_" {
		l.showMailList(live, bbs.MailQuery{Page: 1, Folder: bbs.Inbox})
		return
	}
	tokens := bbs.Tokens(command, ";")
	if len(tokens) < 2 {
		return
	}
	action, args := tokens[1], tokens[2:]
	switch action {
	case "inbox", "sentbox", "archive", "temparchive":
		folder, _ := bbs.ParseFolder(action)
		q := bbs.MailQuery{Page: 1, Folder: folder}
		if len(args) > 0 {
			page, err := commons.ParseInt(args[0], 32)
			if err != nil {
				return
			}
			q.Page = int(page)
		}
		if len(args) > 1 {
			q.SearchType = args[1]
		}
		if len(args) > 2 {
			q.Search = args[2]
		}
		l.showMailList(live, q)
		return
	case "crea":
		l.sendBoardFile(live, bbs.MailWritePage)
		return
	}
	id := int64(-1)
	if len(args) > 0 {
		var err error
		if id, err = commons.ParseInt(args[0], 32); err != nil {
			return
		}
	}
	mail, ok := l.board.mail.Mail(live.ObjectID(), int32(id))
	if !ok {
		l.showLastMailList(live)
		return
	}
	switch action {
	case "view":
		// A mail whose page cannot be shown stays unread.
		if l.showMail(live, mail) && mail.Unread {
			l.board.mail.MarkRead(live.ObjectID(), mail.ID)
		}
	case "reply":
		l.showMailReply(live, mail)
	case "del":
		l.board.mail.Delete(live.ObjectID(), mail.ID)
		l.showLastMailList(live)
	case "store":
		l.board.mail.Move(live.ObjectID(), mail.ID, bbs.Archive)
		l.showMailList(live, bbs.MailQuery{Page: 1, Folder: bbs.Archive})
	}
}

// boardMailWrite submits a mail board form: Send (recipients, subject and
// message in the third to fifth arguments) or Search;<folder> (search
// type and text in the fourth and fifth).
func (l *GameClientLink) boardMailWrite(live *livePlayer, args [5]string) {
	if l.board.mail == nil {
		return
	}
	switch {
	case args[0] == "Send":
		l.sendMail(live, args[2], args[3], args[4])
		l.showMailList(live, bbs.MailQuery{Page: 1, Folder: bbs.SentBox})
	case strings.HasPrefix(args[0], "Search"):
		tokens := bbs.Tokens(args[0], ";")
		if len(tokens) < 2 {
			return
		}
		folder, ok := bbs.ParseFolder(tokens[1])
		if !ok {
			return
		}
		l.showMailList(live, bbs.MailQuery{Page: 1, Folder: folder, SearchType: args[3], Search: args[4]})
	default:
		l.sendBoard(live, bbs.NotImplemented(args[0]))
	}
}

// mailWriters names the senders of box: a sender no character has reads
// unknownWriter.
func (l *GameClientLink) mailWriters(box []bbs.Mail) func(int32) string {
	ids := make([]int32, 0, len(box))
	for _, mail := range box {
		ids = append(ids, mail.SenderID)
	}
	names := l.characterNames(ids)
	return func(id int32) string {
		if name, ok := names[id]; ok {
			return name
		}
		return unknownWriter
	}
}

// showMailList shows the folder page q asks for and remembers it as the
// player's last one.
func (l *GameClientLink) showMailList(live *livePlayer, q bbs.MailQuery) {
	page, ok := l.boardPage(bbs.MailListPage)
	if !ok {
		return
	}
	box := l.board.mail.Box(live.ObjectID())
	html, shown := bbs.RenderMailList(page, box, q, l.mailWriters(box))
	live.board.mailPosition = shown
	l.sendBoard(live, html)
}

// showLastMailList shows the inbox page the player was last shown.
func (l *GameClientLink) showLastMailList(live *livePlayer) {
	const foldersPerPosition = 1000
	position := live.board.mailPosition
	folder := bbs.Folder(position / foldersPerPosition)
	if folder < bbs.Inbox || folder > bbs.TempArchive {
		return
	}
	l.showMailList(live, bbs.MailQuery{Page: position % foldersPerPosition, Folder: folder})
}

// showMail shows one mail; false when its page is missing.
func (l *GameClientLink) showMail(live *livePlayer, mail bbs.Mail) bool {
	page, ok := l.boardPage(bbs.MailShowPage)
	if !ok {
		return false
	}
	l.sendBoard(live, bbs.RenderMailView(page, mail, l.mailWriters([]bbs.Mail{mail})(mail.SenderID)))
	return true
}

// showMailReply opens the reply form to mail: a mail the player sent is
// answered to its recipients, any other to its sender.
func (l *GameClientLink) showMailReply(live *livePlayer, mail bbs.Mail) {
	page, ok := l.boardPage(bbs.MailReplyPage)
	if !ok {
		return
	}
	recipients := mail.Recipients
	if mail.SenderID != live.ObjectID() {
		recipients = l.mailWriters([]bbs.Mail{mail})(mail.SenderID)
	}
	l.sendBoardEdit(live, bbs.RenderMailReply(page, mail, recipients), " ", "Re: "+mail.Subject, "0")
}

// sendMail sends a mail from live to the recipient list. A refusal before
// any recipient is tried answers with its message alone; then each
// recipient that does not take the mail answers live with its own message,
// each online one that does is told of the new mail, and live is told the
// mail went out once a copy reached its sent box.
func (l *GameClientLink) sendMail(live *livePlayer, list, subject, message string) {
	names := bbs.RecipientNames(list)
	sender := bbs.Sender{ID: live.ObjectID(), GM: live.accessLevel().IsGM}
	now := time.Now()
	switch l.board.mail.CheckSend(sender.ID, sender.GM, names, now) {
	case bbs.SendDailyLimit:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreMessagesToday))
		return
	case bbs.SendTooManyRecipients:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyFiveRecipients))
		return
	}
	recipients := make([]bbs.Recipient, len(names))
	for i, name := range names {
		recipients[i] = l.mailRecipient(live, name)
	}
	deliveries, sent := l.board.mail.Send(sender, recipients, list, subject, message, now)
	for _, d := range deliveries {
		l.announceDelivery(live, d)
	}
	if sent {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSentMail))
	}
}

// mailRecipient looks name up as a recipient of a mail from live.
func (l *GameClientLink) mailRecipient(live *livePlayer, name string) bbs.Recipient {
	r := bbs.Recipient{Name: name}
	ref, found := l.findCharacter(name)
	if !found {
		return r
	}
	r.ID, r.AccessLevel = ref.ID, ref.AccessLevel
	if target, online := l.livePlayerByID(ref.ID); online {
		r.Online = true
		r.AccessLevel = target.accessLevel().Level
		r.BlockingAll = target.BlockingAll()
		r.BlocksSender = l.relations.IsBlocked(ref.ID, live.ObjectID())
	}
	return r
}

// announceDelivery tells live, and the recipient when it is online, how the
// mail fared for one recipient.
func (l *GameClientLink) announceDelivery(live *livePlayer, d bbs.Delivery) {
	switch d.Result {
	case bbs.DeliveryInvalidTarget:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	case bbs.DeliveryToGM:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageCannotMailGMS1, d.Recipient.Name))
	case bbs.DeliveryBlockingAll:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1BlockedEverything, d.Recipient.Name))
	case bbs.DeliveryBlocked:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1BlockedYouCannotMail, d.Recipient.Name))
	case bbs.DeliveryInboxFull:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMessageNotSent))
		if target, online := l.livePlayerByID(d.Recipient.ID); online {
			target.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMailboxFull))
		}
	case bbs.Delivered:
		if target, online := l.livePlayerByID(d.Recipient.ID); online {
			target.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNewMail))
			target.SendFrame(serverpackets.FramePlaySound(serverpackets.SoundNewMail))
			target.SendFrame(serverpackets.FrameExMailArrived())
		}
	}
}
