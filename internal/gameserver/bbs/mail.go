// Package bbs owns the community board: the mailboxes of the mail board,
// the rules for sending, reading, filing and deleting mail, and the pages
// the home, mail, clan and friends boards show. It decides and renders; the
// network layer resolves characters and clans for it and turns its pages
// and outcomes into packets.
package bbs

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"
)

// Folder is the box of a mailbox a mail is filed in.
type Folder int

// The mail folders, in the order a stored mail position counts them.
const (
	Inbox Folder = iota
	SentBox
	Archive
	TempArchive
)

var folders = [...]struct{ name, description, link string }{
	Inbox:       {"INBOX", "Inbox", `<a action="bypass _bbsmail">Inbox</a>`},
	SentBox:     {"SENTBOX", "Sent Box", `<a action="bypass _bbsmail;sentbox">Sent Box</a>`},
	Archive:     {"ARCHIVE", "Mail Archive", `<a action="bypass _bbsmail;archive">Mail Archive</a>`},
	TempArchive: {"TEMPARCHIVE", "Temporary Mail Archive", `<a action="bypass _bbsmail;temp_archive">Temporary Mail Archive</a>`},
}

// String is the folder's name as the page links carry it: upper case.
func (f Folder) String() string { return folders[f].name }

// Column is the folder as the location column stores it: lower case.
func (f Folder) Column() string { return strings.ToLower(folders[f].name) }

// ParseFolder reads a folder name in any case.
func ParseFolder(name string) (Folder, bool) {
	upper := strings.ToUpper(name)
	for f := range folders {
		if folders[f].name == upper {
			return Folder(f), true
		}
	}
	return 0, false
}

// Mail is one mail as a mailbox files it.
type Mail struct {
	ID         int32
	ReceiverID int32
	SenderID   int32
	Folder     Folder
	Recipients string
	Subject    string
	Message    string
	Sent       time.Time
	Unread     bool
}

// SentText is the mail's send time as the pages show it, in local time to
// the minute.
func (m Mail) SentText() string { return m.Sent.Local().Format("2006-01-02 15:04") }

// MailStore writes the bbs_mail rows.
type MailStore interface {
	InsertMail(ctx context.Context, m Mail) error
	DeleteMail(ctx context.Context, id int32) error
	MarkMailRead(ctx context.Context, id int32) error
	MoveMail(ctx context.Context, id int32, f Folder) error
}

// Writer runs a store write later, on ownerID's lane, so the writes for
// one owner keep their order and none blocks the caller.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// The mail limits.
const (
	// dailySendLimit is how many mails a sender's sent box may have taken
	// in the last day before sending is refused.
	dailySendLimit = 10
	// maxRecipients is the longest recipient list a sender that is not a
	// game master may address.
	maxRecipients = 5
	// inboxCapacity is how many mails an inbox holds before mail to it is
	// refused.
	inboxCapacity = 100
	// subjectLength is the longest subject kept, in UTF-16 code units.
	subjectLength = 128
	// mailWriteTimeout bounds one bbs_mail write.
	mailWriteTimeout = 2 * time.Second
)

// The widths of the bbs_mail text columns, in characters. A mail whose
// recipient list or message is wider cannot be stored.
const (
	recipientsColumnWidth = 200
	subjectColumnWidth    = 128
	messageColumnWidth    = 3000
)

// noSubject stands in for an empty subject.
const noSubject = "(no subject)"

// Mailbox holds every character's mail. mu guards boxes, lastID,
// unstoredSends and every filed mail: a sender's queue files mail into
// other characters' boxes while their own queues read them.
type Mailbox struct {
	mu sync.Mutex
	// boxes is each owner's mail in ascending id order.
	boxes  map[int32][]*Mail
	lastID int32
	// unstoredSends is, per sender, when each delivered mail too wide to
	// store went out. Such a mail files no sent-box copy, so these times
	// stand in for the copies in the daily limit.
	unstoredSends map[int32][]time.Time

	store  MailStore
	writes Writer
	log    zerolog.Logger
}

// NewMailbox returns an empty mailbox writing through store on writes.
func NewMailbox(store MailStore, writes Writer, log zerolog.Logger) *Mailbox {
	return &Mailbox{boxes: map[int32][]*Mail{}, unstoredSends: map[int32][]time.Time{}, store: store, writes: writes, log: log}
}

// Restore files rows, the stored mail, once at boot before any player
// connects. A new mail's id follows the highest stored one.
func (m *Mailbox) Restore(rows []Mail) {
	rows = slices.Clone(rows)
	slices.SortFunc(rows, func(a, b Mail) int { return cmp.Compare(a.ID, b.ID) })
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range rows {
		mail := rows[i]
		m.boxes[mail.ReceiverID] = append(m.boxes[mail.ReceiverID], &mail)
		m.lastID = max(m.lastID, mail.ID)
	}
}

// HasUnread reports whether ownerID has a mail it has not opened.
func (m *Mailbox) HasUnread(ownerID int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mail := range m.boxes[ownerID] {
		if mail.Unread {
			return true
		}
	}
	return false
}

// Box returns ownerID's mail in ascending id order.
func (m *Mailbox) Box(ownerID int32) []Mail {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Mail, len(m.boxes[ownerID]))
	for i, mail := range m.boxes[ownerID] {
		out[i] = *mail
	}
	return out
}

// Mail returns the mail id of ownerID's box.
func (m *Mailbox) Mail(ownerID, id int32) (Mail, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mail := m.findLocked(ownerID, id); mail != nil {
		return *mail, true
	}
	return Mail{}, false
}

func (m *Mailbox) findLocked(ownerID, id int32) *Mail {
	for _, mail := range m.boxes[ownerID] {
		if mail.ID == id {
			return mail
		}
	}
	return nil
}

// MarkRead marks the mail id of ownerID's box opened.
func (m *Mailbox) MarkRead(ownerID, id int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mail := m.findLocked(ownerID, id); mail != nil {
		mail.Unread = false
	}
	m.write(id, "mark mail read", func(ctx context.Context, st MailStore) error { return st.MarkMailRead(ctx, id) })
}

// Move files the mail id of ownerID's box in f.
func (m *Mailbox) Move(ownerID, id int32, f Folder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mail := m.findLocked(ownerID, id); mail != nil {
		mail.Folder = f
	}
	m.write(id, "move mail", func(ctx context.Context, st MailStore) error { return st.MoveMail(ctx, id, f) })
}

// Delete takes the mail id out of ownerID's box.
func (m *Mailbox) Delete(ownerID, id int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.boxes[ownerID] = slices.DeleteFunc(m.boxes[ownerID], func(mail *Mail) bool { return mail.ID == id })
	m.write(id, "delete mail", func(ctx context.Context, st MailStore) error { return st.DeleteMail(ctx, id) })
}

// countLocked is how many of ownerID's mails are filed in f.
func (m *Mailbox) countLocked(ownerID int32, f Folder) int {
	n := 0
	for _, mail := range m.boxes[ownerID] {
		if mail.Folder == f {
			n++
		}
	}
	return n
}

// write queues fn on the lane of the mail id, so the writes of one row
// land in the order they were made. Every caller holds mu; Enqueue only
// appends to the lane.
func (m *Mailbox) write(id int32, what string, fn func(context.Context, MailStore) error) {
	if m.store == nil {
		return
	}
	store, log := m.store, m.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), mailWriteTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32("mail_id", id).Msg("bbs: " + what)
		}
	}
	if m.writes == nil {
		job()
		return
	}
	if !m.writes.Enqueue(id, job) {
		log.Error().Int32("mail_id", id).Msg("bbs: " + what + ": write dropped")
	}
}

// RecipientNames returns the names a recipient list addresses: the list,
// with its surrounding blanks trimmed, split on ';'. Trailing empty names
// are dropped; any other empty name stays, and finds no character.
func RecipientNames(list string) []string {
	return splitList(strings.TrimFunc(list, func(r rune) bool { return r <= ' ' }), ";")
}

// SendRefusal is why a mail was refused before any recipient was tried.
type SendRefusal int

// The send refusals.
const (
	SendAllowed SendRefusal = iota
	// SendDailyLimit: the sender's sent box took dailySendLimit mails in
	// the last day.
	SendDailyLimit
	// SendTooManyRecipients: a sender that is not a game master addressed
	// more than maxRecipients names.
	SendTooManyRecipients
)

// CheckSend returns why senderID may not send a mail to names at now. A
// delivered mail too wide to store counts against the daily limit as its
// sent-box copy would have.
func (m *Mailbox) CheckSend(senderID int32, gm bool, names []string, now time.Time) SendRefusal {
	since := now.Add(-24 * time.Hour)
	m.mu.Lock()
	sent := 0
	for _, mail := range m.boxes[senderID] {
		if mail.Folder == SentBox && mail.Sent.After(since) {
			sent++
		}
	}
	sends := slices.DeleteFunc(m.unstoredSends[senderID], func(at time.Time) bool { return !at.After(since) })
	if len(sends) == 0 {
		delete(m.unstoredSends, senderID)
	} else {
		m.unstoredSends[senderID] = sends
	}
	sent += len(sends)
	m.mu.Unlock()
	switch {
	case sent >= dailySendLimit:
		return SendDailyLimit
	case len(names) > maxRecipients && !gm:
		return SendTooManyRecipients
	}
	return SendAllowed
}

// Recipient is one name of a recipient list as the sender's lookup found
// it.
type Recipient struct {
	Name string
	// ID is the character's object id, 0 when no character has the name.
	ID          int32
	AccessLevel int
	// Online reports whether the character is in the world; BlockingAll and
	// BlocksSender are only known then.
	Online       bool
	BlockingAll  bool
	BlocksSender bool
}

// DeliveryResult is what happened to the mail for one recipient.
type DeliveryResult int

// The delivery results.
const (
	Delivered DeliveryResult = iota
	// DeliveryInvalidTarget: no character has the name, or it names the
	// sender.
	DeliveryInvalidTarget
	// DeliveryToGM: the recipient is a game master.
	DeliveryToGM
	// DeliveryBlockingAll: the recipient blocks everything.
	DeliveryBlockingAll
	// DeliveryBlocked: the recipient blocks the sender.
	DeliveryBlocked
	// DeliveryInboxFull: the recipient's inbox is full.
	DeliveryInboxFull
)

// Delivery is the mail's outcome for one recipient, in list order.
type Delivery struct {
	Recipient Recipient
	Result    DeliveryResult
}

// Sender is the character sending a mail.
type Sender struct {
	ID int32
	GM bool
}

// Send files a mail from sender for every recipient that takes it, then a
// copy in the sender's sent box when at least one did, and reports each
// recipient's outcome and whether the copy was filed. A game master skips
// every check but the invalid target. Only a mail whose fields fit their
// columns is stored; one that does not reaches its recipients for the run
// but files no sent-box copy. It still counts against the sender's daily
// limit, which the reference reaches only by storing a truncated row.
func (m *Mailbox) Send(sender Sender, recipients []Recipient, list, subject, message string, now time.Time) ([]Delivery, bool) {
	subject = trimOr(subject, subjectLength, noSubject)
	message = strings.ReplaceAll(message, "\n", "<br1>")
	stored := utf8.RuneCountInString(list) <= recipientsColumnWidth &&
		utf8.RuneCountInString(subject) <= subjectColumnWidth &&
		utf8.RuneCountInString(message) <= messageColumnWidth

	m.mu.Lock()
	defer m.mu.Unlock()
	deliveries := make([]Delivery, 0, len(recipients))
	delivered := false
	for _, r := range recipients {
		result := m.deliveryLocked(sender, r)
		deliveries = append(deliveries, Delivery{Recipient: r, Result: result})
		if result != Delivered {
			continue
		}
		delivered = true
		m.fileLocked(Mail{
			ReceiverID: r.ID, SenderID: sender.ID, Folder: Inbox,
			Recipients: list, Subject: subject, Message: message, Sent: now, Unread: true,
		}, stored)
	}
	if !delivered {
		return deliveries, false
	}
	if !stored {
		m.unstoredSends[sender.ID] = append(m.unstoredSends[sender.ID], now)
		return deliveries, false
	}
	m.fileLocked(Mail{
		ReceiverID: sender.ID, SenderID: sender.ID, Folder: SentBox,
		Recipients: list, Subject: subject, Message: message, Sent: now,
	}, true)
	return deliveries, true
}

func (m *Mailbox) deliveryLocked(sender Sender, r Recipient) DeliveryResult {
	switch {
	case r.ID <= 0 || r.ID == sender.ID:
		return DeliveryInvalidTarget
	case sender.GM:
		return Delivered
	case r.AccessLevel > 0:
		return DeliveryToGM
	case r.Online && r.BlockingAll:
		return DeliveryBlockingAll
	case r.Online && r.BlocksSender:
		return DeliveryBlocked
	case m.countLocked(r.ID, Inbox) >= inboxCapacity:
		return DeliveryInboxFull
	}
	return Delivered
}

// fileLocked gives mail the next id, files it in its receiver's box and,
// when stored, queues its row.
func (m *Mailbox) fileLocked(mail Mail, stored bool) {
	m.lastID++
	mail.ID = m.lastID
	m.boxes[mail.ReceiverID] = append(m.boxes[mail.ReceiverID], &mail)
	if stored {
		row := mail
		m.write(row.ID, "insert mail", func(ctx context.Context, st MailStore) error { return st.InsertMail(ctx, row) })
	}
}
