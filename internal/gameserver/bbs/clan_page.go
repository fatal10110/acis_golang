package bbs

import (
	"strconv"
	"strings"
)

// The clan board's pages, under the board's page folder.
const (
	ClanListPage       = "clan/clanlist.htm"
	ClanHomePage       = "clan/clanhome.htm"
	ClanHomeLeaderPage = "clan/clanhome-leader.htm"
	ClanHomeMemberPage = "clan/clanhome-member.htm"
	ClanNoticePage     = "clan/clanhome-notice.htm"
	ClanManagementPage = "clan/clanhome-management.htm"
	ClanMailPage       = "clan/clanhome-mail.htm"
)

// clansPerPage is the clan list's page size as its page links count it.
const clansPerPage = 8

// clanListStep is the clan list's paging step: page n lists the clans from
// index 7(n-1) up to index 7(n+1).
const clanListStep = 7

// ClanCard is a clan as the clan board shows it.
type ClanCard struct {
	ID           int32
	Name         string
	LeaderName   string
	Level        int
	Members      int
	AllyID       int32
	AllyName     string
	Introduction string
}

// RenderClanList fills page, the clan list, with clans from index on;
// ownClan is the viewer's clan id, 0 for none.
func RenderClanList(page string, clans []ClanCard, index int, ownClan int32) string {
	homebar := ""
	if ownClan != 0 {
		homebar = `<table width=610 bgcolor=A7A19A><tr><td width=5></td><td width=605><a action="bypass _bbsclan;home;` + strconv.Itoa(int(ownClan)) + `">[GO TO MY CLAN]</a></td></tr></table>`
	}
	page = strings.ReplaceAll(page, "%homebar%", homebar)
	index = max(index, 1)

	var b strings.Builder
	for i, cl := range clans {
		if i > (index+1)*clanListStep {
			break
		}
		if i < (index-1)*clanListStep {
			continue
		}
		b.WriteString(`<table width=610><tr><td width=5></td><td width=150 align=center><a action="bypass _bbsclan;home;`)
		b.WriteString(strconv.Itoa(int(cl.ID)))
		b.WriteString(`">`)
		b.WriteString(cl.Name)
		b.WriteString(`</a></td><td width=150 align=center>`)
		b.WriteString(cl.LeaderName)
		b.WriteString(`</td><td width=100 align=center>`)
		b.WriteString(strconv.Itoa(cl.Level))
		b.WriteString(`</td><td width=200 align=center>`)
		b.WriteString(strconv.Itoa(cl.Members))
		b.WriteString(`</td><td width=5></td></tr></table><br1><img src="L2UI.Squaregray" width=605 height=1><br1>`)
	}
	b.WriteString("<table><tr>")
	if index == 1 {
		b.WriteString(`<td><button action="" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16></td>`)
	} else {
		b.WriteString(`<td><button action="_bbsclan;clan;` + strconv.Itoa(index-1) + `" back="l2ui_ch3.prev1_down" fore="l2ui_ch3.prev1" width=16 height=16 ></td>`)
	}
	pages := (len(clans) + clansPerPage - 1) / clansPerPage
	for n := 1; n <= pages; n++ {
		if n == index {
			b.WriteString("<td> " + strconv.Itoa(n) + " </td>")
		} else {
			b.WriteString(`<td><a action="bypass _bbsclan;clan;` + strconv.Itoa(n) + `"> ` + strconv.Itoa(n) + " </a></td>")
		}
	}
	if index == pages {
		b.WriteString(`<td><button action="" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16></td>`)
	} else {
		b.WriteString(`<td><button action="bypass _bbsclan;clan;` + strconv.Itoa(index+1) + `" back="l2ui_ch3.next1_down" fore="l2ui_ch3.next1" width=16 height=16 ></td>`)
	}
	b.WriteString("</tr></table>")
	return strings.ReplaceAll(page, "%clanlist%", b.String())
}

// RenderClanHome fills page, one of the clan home pages, with cl.
func RenderClanHome(page string, cl ClanCard) string {
	ally := ""
	if cl.AllyID > 0 {
		ally = cl.AllyName
	}
	page = strings.ReplaceAll(page, "%clanid%", strconv.Itoa(int(cl.ID)))
	page = strings.ReplaceAll(page, "%clanIntro%", cl.Introduction)
	page = strings.ReplaceAll(page, "%clanName%", cl.Name)
	page = strings.ReplaceAll(page, "%clanLvL%", strconv.Itoa(cl.Level))
	page = strings.ReplaceAll(page, "%clanMembers%", strconv.Itoa(cl.Members))
	page = strings.ReplaceAll(page, "%clanLeader%", cl.LeaderName)
	return strings.ReplaceAll(page, "%allyName%", ally)
}

// RenderClanNotice fills page, the clan notice settings, for clanID whose
// notice is enabled or not.
func RenderClanNotice(page string, clanID int32, enabled bool) string {
	page = strings.ReplaceAll(page, "%clanid%", strconv.Itoa(int(clanID)))
	page = strings.ReplaceAll(page, "%enabled%", "["+strconv.FormatBool(enabled)+"]")
	return strings.ReplaceAll(page, "%flag%", strconv.FormatBool(!enabled))
}

// RenderClanManagement fills page, the clan management form, for clanID;
// access describes the access of the clan's announcement and bulletin
// boards.
func RenderClanManagement(page string, clanID int32, access string) string {
	page = strings.ReplaceAll(page, "%clanid%", strconv.Itoa(int(clanID)))
	page = strings.ReplaceAll(page, "%curAnnNonPer%", access)
	page = strings.ReplaceAll(page, "%curAnnMemPer%", access)
	page = strings.ReplaceAll(page, "%curCbbNonPer%", access)
	return strings.ReplaceAll(page, "%curCbbMemPer%", access)
}

// RenderClanMail fills page, the clan mail form, for the clan clanID named
// name.
func RenderClanMail(page string, clanID int32, name string) string {
	page = strings.ReplaceAll(page, "%clanid%", strconv.Itoa(int(clanID)))
	return strings.ReplaceAll(page, "%clanName%", name)
}
