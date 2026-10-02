package bbs

import (
	"strconv"
	"strings"
	"time"
)

// The memo board's pages are built in code, not read from page files.

// topicsPerPage is how many topics a page of the memo topic list shows.
const topicsPerPage = 12

// topicPagerStep is the topic count the topic list's page links divide
// by. The page count then rounds up unless it times 8 equals the number of
// clans, as the reference compares it with the clan count.
const topicPagerStep = 8

// The date layouts of the memo board: the topic list and the post edit
// form show a short date and time, US style, the time's AM/PM after a
// narrow no-break space; the post page shows the post's full date.
const (
	shortDateLayout = "1/2/06, 3:04\u202fPM"
	fullDateLayout  = "2006-01-02 15:04:05"
)

// ShortDate is a stored date in Unix milliseconds as the topic list and
// the post edit form show it, in local time.
func ShortDate(ms int64) string { return time.UnixMilli(ms).Local().Format(shortDateLayout) }

// FullDate is a stored date in Unix milliseconds as the post page shows
// it, in local time to the second.
func FullDate(ms int64) string { return time.UnixMilli(ms).Local().Format(fullDateLayout) }

// ForumMissing is the page a topic command naming no memo forum shows.
func ForumMissing(id int32) string {
	return "<html><body><br><br><center>The forum #" + itoa(id) + " doesn't exist.</center></body></html>"
}

// TopicMissing is the page a topic command naming no topic shows.
func TopicMissing(id int32) string {
	return "<html><body><br><br><center>The topic #" + itoa(id) + " doesn't exist.</center></body></html>"
}

// NamedForumMissing is the page a topic or post form naming no forum
// shows; name is the forum as the form gave it.
func NamedForumMissing(name string) string {
	return "<html><body><br><br><center>The forum named '" + name + "' doesn't exist.</center></body></html>"
}

// NamedTopicMissing is the page a topic or post form naming no topic shows.
func NamedTopicMissing(name string) string {
	return "<html><body><br><br><center>The topic named '" + name + "' doesn't exist.</center></body></html>"
}

// NamedPostMissing is the page a post form naming no post shows.
func NamedPostMissing(id int32) string {
	return "<html><body><br><br><center>The post named '" + itoa(id) + "' doesn't exist.</center></body></html>"
}

// The pages a post command naming no forum, topic or post shows, and the
// one a post of a forum other than a memo forum shows.
const (
	PostForumMissing = "<html><body><br><br><center>This forum doesn't exist.</center></body></html>"
	PostTopicMissing = "<html><body><br><br><center>This topic doesn't exist.</center></body></html>"
	PostMissing      = "<html><body><br><br><center>This post doesn't exist.</center></body></html>"
	PostOffLimits    = "<html><body><br><br><center>The forum is off-limits.</center></body></html>"
)

func itoa(n int32) string { return strconv.Itoa(int(n)) }

// RenderMemoTopics is page index of memo forum f's topic list, newest
// first; clans is the number of clans, which the page links' count reads
// (see topicPagerStep).
func RenderMemoTopics(f ForumView, index int32, clans int) string {
	id := itoa(f.ID)
	var b strings.Builder
	b.WriteString(`<html><body><br><br><table border=0 width=610><tr><td width=10></td><td width=600 align=left><a action="bypass _bbshome">HOME</a>&nbsp;>&nbsp;<a action="bypass _bbsmemo">Memo Form</a></td></tr></table><img src="L2UI.squareblank" width="1" height="10"><center><table border=0 cellspacing=0 cellpadding=2 bgcolor=888888 width=610><tr><td FIXWIDTH=5></td><td FIXWIDTH=415 align=center>&$413;</td><td FIXWIDTH=120 align=center></td><td FIXWIDTH=70 align=center>&$418;</td></tr></table>`)
	byID := make(map[int32]Topic, len(f.Topics))
	for _, t := range f.Topics {
		byID[t.ID] = t
	}
	// The list walks the topic ids down from the highest one held, page
	// index taking the 12 topics after the 12(index-1) newest.
	for i, j := int32(0), f.LastTopicID+1; i < topicsPerPage*index; j-- {
		if j < 0 {
			break
		}
		t, ok := byID[j]
		if !ok {
			continue
		}
		if i >= topicsPerPage*(index-1) {
			b.WriteString(`<table border=0 cellspacing=0 cellpadding=5 WIDTH=610><tr><td FIXWIDTH=5></td><td FIXWIDTH=415><a action="bypass _bbsposts;read;`)
			b.WriteString(id + ";" + itoa(t.ID) + `">` + t.Name)
			b.WriteString(`</a></td><td FIXWIDTH=120 align=center></td><td FIXWIDTH=70 align=center>`)
			b.WriteString(ShortDate(t.Date))
			b.WriteString(`</td></tr></table><img src="L2UI.Squaregray" width="610" height="1">`)
		}
		i++
	}
	b.WriteString(`<br><table width=610 cellspace=0 cellpadding=0><tr><td width=50><button value="&$422;" action="bypass _bbsmemo" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2"></td><td width=510 align=center><table border=0><tr>`)
	if index == 1 {
		b.WriteString(`<td><button action="" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16 ></td>`)
	} else {
		b.WriteString(`<td><button action="bypass _bbstopics;read;` + id + ";" + itoa(index-1) + `" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16 ></td>`)
	}
	pages := int32(len(f.Topics) / topicPagerStep)
	if int(pages)*topicPagerStep != clans {
		pages++
	}
	for n := int32(1); n <= pages; n++ {
		if n == index {
			b.WriteString("<td> " + itoa(n) + " </td>")
		} else {
			b.WriteString(`<td><a action="bypass _bbstopics;read;` + id + ";" + itoa(n) + `"> ` + itoa(n) + " </a></td>")
		}
	}
	if index == pages {
		b.WriteString(`<td><button action="" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16 ></td>`)
	} else {
		b.WriteString(`<td><button action="bypass _bbstopics;read;` + id + ";" + itoa(index+1) + `" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16 ></td>`)
	}
	b.WriteString(`</tr></table></td><td align=right><button value = "&$421;" action="bypass _bbstopics;crea;` + id)
	b.WriteString(`" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2" ></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=10></td></tr><tr><td></td><td align=center><table border=0><tr><td></td><td><edit var = "Search" width=130 height=11></td><td><button value="&$420;" action="Write 5 -2 0 Search _ _" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2"></td></tr></table></td></tr></table><br><br><br></center></body></html>`)
	return b.String()
}

// RenderMemoNewTopic is the new topic form of memo forum forumID.
func RenderMemoNewTopic(forumID int32) string {
	id := itoa(forumID)
	return `<html><body><br><br><table border=0 width=610><tr><td width=10></td><td width=600 align=left><a action="bypass _bbshome">HOME</a>&nbsp;>&nbsp;<a action="bypass _bbsmemo">Memo Form</a></td></tr></table><img src="L2UI.squareblank" width="1" height="10"><center><table border=0 cellspacing=0 cellpadding=0><tr><td width=610><img src="sek.cbui355" width="610" height="1"><br1><img src="sek.cbui355" width="610" height="1"></td></tr></table><table fixwidth=610 border=0 cellspacing=0 cellpadding=0><tr><td><img src="l2ui.mini_logo" width=5 height=20></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=1></td><td align=center FIXWIDTH=60 height=29>&$413;</td><td FIXWIDTH=540><edit var = "Title" width=540 height=13></td><td><img src="l2ui.mini_logo" width=5 height=1></td></tr></table><table fixwidth=610 border=0 cellspacing=0 cellpadding=0><tr><td><img src="l2ui.mini_logo" width=5 height=10></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=1></td><td align=center FIXWIDTH=60 height=29 valign=top>&$427;</td><td align=center FIXWIDTH=540><MultiEdit var ="Content" width=535 height=313></td><td><img src="l2ui.mini_logo" width=5 height=1></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=10></td></tr></table><table fixwidth=610 border=0 cellspacing=0 cellpadding=0><tr><td><img src="l2ui.mini_logo" width=5 height=10></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=1></td><td align=center FIXWIDTH=60 height=29>&nbsp;</td><td align=center FIXWIDTH=70><button value="&$140;" action="Write Topic crea ` +
		id +
		` Title Content Title" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2" ></td><td align=center FIXWIDTH=70><button value = "&$141;" action="bypass _bbsmemo" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2"> </td><td align=center FIXWIDTH=400>&nbsp;</td><td><img src="l2ui.mini_logo" width=5 height=1></td></tr></table></center></body></html>`
}

// RenderMemoPost is the page of memo topic t, showing its post p. The
// post's text shows as written: its angle brackets escaped and its line
// breaks kept.
func RenderMemoPost(t Topic, p Post) string {
	mes := strings.ReplaceAll(p.Text, ">", "&gt;")
	mes = strings.ReplaceAll(mes, "<", "&lt;")
	mes = strings.ReplaceAll(mes, "\n", "<br1>")
	forumID, topicID, date := itoa(t.ForumID), itoa(t.ID), FullDate(p.Date)
	return `<html><body><br><br><table border=0 width=610><tr><td width=10></td><td width=600 align=left><a action="bypass _bbshome">HOME</a>&nbsp;>&nbsp;<a action="bypass _bbsmemo">Memo Form</a></td></tr></table><img src="L2UI.squareblank" width="1" height="10"><center><table border=0 cellspacing=0 cellpadding=0 bgcolor=333333><tr><td height=10></td></tr><tr><td fixWIDTH=55 align=right valign=top>&$413; : &nbsp;</td><td fixWIDTH=380 valign=top>` +
		t.Name +
		`</td><td fixwidth=5></td><td fixwidth=50></td><td fixWIDTH=120></td></tr><tr><td height=10></td></tr><tr><td align=right><font color="AAAAAA" >&$417; : &nbsp;</font></td><td><font color="AAAAAA">` +
		t.OwnerName +
		`</font></td><td></td><td><font color="AAAAAA">&$418; :</font></td><td><font color="AAAAAA">` +
		date +
		`</font></td></tr><tr><td height=10></td></tr></table><br><table border=0 cellspacing=0 cellpadding=0><tr><td fixwidth=5></td><td FIXWIDTH=600 align=left>` +
		mes +
		`</td><td fixqqwidth=5></td></tr></table><br><img src="L2UI.squareblank" width="1" height="5"><img src="L2UI.squaregray" width="610" height="1"><img src="L2UI.squareblank" width="1" height="5"><table border=0 cellspacing=0 cellpadding=0 FIXWIDTH=610><tr><td width=50><button value="&$422;" action="bypass _bbsmemo" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2"></td><td width=560 align=right><table border=0 cellspacing=0><tr><td FIXWIDTH=300></td><td><button value = "&$424;" action="bypass _bbsposts;edit;` +
		forumID +
		`;` +
		topicID +
		`;0" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2" ></td>&nbsp;<td><button value = "&$425;" action="bypass _bbstopics;del;` +
		forumID +
		`;` +
		topicID +
		`" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2" ></td>&nbsp;<td><button value = "&$421;" action="bypass _bbstopics;crea;` +
		forumID +
		`" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2" ></td>&nbsp;</tr></table></td></tr></table><br><br><br></center></body></html>`
}

// RenderPostEdit is the edit form of topic t of forum forumID; its text
// field is filled apart (see EditFields).
func RenderPostEdit(forumID int32, t Topic) string {
	name, topicID := t.Name, itoa(t.ID)
	return `<html><body><br><br><table border=0 width=610><tr><td width=10></td><td width=600 align=left><a action="bypass _bbshome">HOME</a>&nbsp;>&nbsp;<a action="bypass _bbsmemo">Memo Form</a></td></tr></table><img src="L2UI.squareblank" width="1" height="10"><center><table border=0 cellspacing=0 cellpadding=0><tr><td width=610><img src="sek.cbui355" width="610" height="1"><br1><img src="sek.cbui355" width="610" height="1"></td></tr></table><table fixwidth=610 border=0 cellspacing=0 cellpadding=0><tr><td><img src="l2ui.mini_logo" width=5 height=20></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=1></td><td align=center FIXWIDTH=60 height=29>&$413;</td><td FIXWIDTH=540>` +
		name +
		`</td><td><img src="l2ui.mini_logo" width=5 height=1></td></tr></table><table fixwidth=610 border=0 cellspacing=0 cellpadding=0><tr><td><img src="l2ui.mini_logo" width=5 height=10></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=1></td><td align=center FIXWIDTH=60 height=29 valign=top>&$427;</td><td align=center FIXWIDTH=540><MultiEdit var ="Content" width=535 height=313></td><td><img src="l2ui.mini_logo" width=5 height=1></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=10></td></tr></table><table fixwidth=610 border=0 cellspacing=0 cellpadding=0><tr><td><img src="l2ui.mini_logo" width=5 height=10></td></tr><tr><td><img src="l2ui.mini_logo" width=5 height=1></td><td align=center FIXWIDTH=60 height=29>&nbsp;</td><td align=center FIXWIDTH=70><button value="&$140;" action="Write Post ` +
		itoa(forumID) +
		`;` +
		topicID +
		`;0 _ Content Content Content" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2" ></td><td align=center FIXWIDTH=70><button value = "&$141;" action="bypass _bbsmemo" back="l2ui_ch3.smallbutton2_down" width=65 height=20 fore="l2ui_ch3.smallbutton2"> </td><td align=center FIXWIDTH=400>&nbsp;</td><td><img src="l2ui.mini_logo" width=5 height=1></td></tr></table></center></body></html>`
}
