package bbs

import (
	"strconv"
	"strings"
)

// The mail board's pages, under the board's page folder.
const (
	MailListPage  = "mail/mail.htm"
	MailShowPage  = "mail/mail-show.htm"
	MailWritePage = "mail/mail-write.htm"
	MailReplyPage = "mail/mail-reply.htm"
)

// mailsPerPage is how many mails one page of a folder lists.
const mailsPerPage = 10

// subjectListLength is the longest subject the folder list shows whole, in
// UTF-16 code units.
const subjectListLength = 30

// MailQuery is one listing of a folder: the page asked for and an optional
// search, by writer or (SearchType "title", in any case) by subject.
type MailQuery struct {
	Page       int
	Folder     Folder
	SearchType string
	Search     string
}

func (q MailQuery) searching() bool { return q.SearchType != "" && q.Search != "" }

// matches reports whether mail answers q's search; writer names a sender.
func (q MailQuery) matches(mail Mail, writer func(int32) string) bool {
	search := strings.ToLower(q.Search)
	if strings.EqualFold(q.SearchType, "title") {
		return strings.Contains(strings.ToLower(mail.Subject), search)
	}
	return strings.Contains(strings.ToLower(writer(mail.SenderID)), search)
}

// count is how many of box's mails in folder answer q's search, or all of
// them without one.
func (q MailQuery) count(box []Mail, folder Folder, writer func(int32) string) int {
	n := 0
	for _, mail := range box {
		if mail.Folder == folder && (!q.searching() || q.matches(mail, writer)) {
			n++
		}
	}
	return n
}

// mailPages is how many pages list count mails; an empty folder still has
// one.
func mailPages(count int) int {
	if count < 1 {
		return 1
	}
	return (count + mailsPerPage - 1) / mailsPerPage
}

// RenderMailList fills page, the folder list page, with the page of box q
// asks for, and returns it with the page number shown: q's page held to
// the folder's pages. writer names a mail's sender.
func RenderMailList(page string, box []Mail, q MailQuery, writer func(int32) string) (string, int) {
	empty := MailQuery{}
	pages := mailPages(q.count(box, q.Folder, writer))
	shown := min(q.Page, pages)
	shown = max(shown, 1)

	page = strings.ReplaceAll(page, "%inbox%", strconv.Itoa(empty.count(box, Inbox, writer)))
	page = strings.ReplaceAll(page, "%sentbox%", strconv.Itoa(empty.count(box, SentBox, writer)))
	page = strings.ReplaceAll(page, "%archive%", strconv.Itoa(empty.count(box, Archive, writer)))
	page = strings.ReplaceAll(page, "%temparchive%", strconv.Itoa(empty.count(box, TempArchive, writer)))
	page = strings.ReplaceAll(page, "%type%", folders[q.Folder].description)
	page = strings.ReplaceAll(page, "%htype%", q.Folder.Column())

	// The first page lists indexes 0 to 9, page n indexes 10(n-1) to 10n-1.
	last := shown*mailsPerPage - 1
	if shown == 1 {
		last = mailsPerPage - 1
	}
	first := last - (mailsPerPage - 1)
	var rows strings.Builder
	index := 0
	for _, mail := range box {
		if mail.Folder != q.Folder || (q.searching() && !q.matches(mail, writer)) {
			continue
		}
		if index < first {
			index++
			continue
		}
		if index > last {
			break
		}
		rows.WriteString(`<table width=610><tr><td width=5></td><td width=150>`)
		rows.WriteString(writer(mail.SenderID))
		rows.WriteString(`</td><td width=300><a action="bypass _bbsmail;view;`)
		rows.WriteString(strconv.Itoa(int(mail.ID)))
		rows.WriteString(`">`)
		if mail.Unread {
			rows.WriteString(`<font color="LEVEL">`)
		}
		rows.WriteString(ellipsize(mail.Subject, subjectListLength))
		if mail.Unread {
			rows.WriteString(`</font>`)
		}
		rows.WriteString(`</a></td><td width=150>`)
		rows.WriteString(mail.SentText())
		rows.WriteString(`</td><td width=5></td></tr></table><img src="L2UI.Squaregray" width=610 height=1>`)
		index++
	}
	page = strings.ReplaceAll(page, "%maillist%", rows.String())
	return strings.ReplaceAll(page, "%maillistlength%", mailPager(q, shown, pages)), shown
}

// mailPager is the folder list's page links: previous, up to 21 page
// numbers around the page shown, next. Every link names the folder in
// upper case.
func mailPager(q MailQuery, shown, pages int) string {
	search := ""
	if q.searching() {
		search = ";" + q.SearchType + ";" + q.Search
	}
	link := func(n int) string {
		return "bypass _bbsmail;" + q.Folder.String() + ";" + strconv.Itoa(n) + search
	}
	var b strings.Builder
	number := func(n int) {
		if n == shown {
			b.WriteString("<td> " + strconv.Itoa(n) + " </td>")
			return
		}
		b.WriteString(`<td><a action="` + link(n) + `"> ` + strconv.Itoa(n) + " </a></td>")
	}
	prev := shown
	if shown != 1 {
		prev = shown - 1
	}
	b.WriteString(`<td><table><tr><td></td></tr><tr><td><button action="` + link(prev) + `" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16></td></tr></table></td>`)
	switch {
	case pages <= 21:
		for n := 1; n <= pages; n++ {
			number(n)
		}
	case shown <= 11:
		for n := 1; n <= 10+shown; n++ {
			number(n)
		}
	case pages-shown > 10:
		for n := shown - 10; n <= shown+10; n++ {
			number(n)
		}
	default:
		for n := shown - 10; n <= pages; n++ {
			number(n)
		}
	}
	next := shown
	if shown != pages {
		next = shown + 1
	}
	b.WriteString(`<td><table><tr><td></td></tr><tr><td><button action="` + link(next) + `" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16 ></td></tr></table></td>`)
	return b.String()
}

// escapeMarkup turns the markup characters <, > and " of s into entities.
func escapeMarkup(s string) string {
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return strings.ReplaceAll(s, `"`, "&quot;")
}

// RenderMailView fills page, the mail page, with mail; writer names its
// sender. The message's line breaks become <br> before the markup is
// escaped, so they show as text, as the line breaks stored at send do.
func RenderMailView(page string, mail Mail, writer string) string {
	page = strings.ReplaceAll(page, "%maillink%", folders[mail.Folder].link+"&nbsp;&gt;&nbsp;"+mail.Subject)
	page = strings.ReplaceAll(page, "%writer%", writer)
	page = strings.ReplaceAll(page, "%sentDate%", mail.SentText())
	page = strings.ReplaceAll(page, "%receiver%", mail.Recipients)
	page = strings.ReplaceAll(page, "%delDate%", "Unknown")
	page = strings.ReplaceAll(page, "%title%", escapeMarkup(mail.Subject))
	page = strings.ReplaceAll(page, "%mes%", escapeMarkup(strings.ReplaceAll(mail.Message, "\r\n", "<br>")))
	return strings.ReplaceAll(page, "%mailId%", strconv.Itoa(int(mail.ID)))
}

// RenderMailReply fills page, the reply form, for a reply to mail;
// recipients is the list the reply goes to.
func RenderMailReply(page string, mail Mail, recipients string) string {
	link := folders[mail.Folder].link + `&nbsp;&gt;&nbsp;<a action="bypass _bbsmail;view;` + strconv.Itoa(int(mail.ID)) + `">` + mail.Subject + "</a>&nbsp;&gt;&nbsp;"
	page = strings.ReplaceAll(page, "%maillink%", link)
	page = strings.ReplaceAll(page, "%recipients%", recipients)
	return strings.ReplaceAll(page, "%mailId%", strconv.Itoa(int(mail.ID)))
}
