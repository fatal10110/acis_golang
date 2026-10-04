package classmaster

import (
	"math"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// Pages resolves a datapack page by its data/html path.
type Pages interface {
	Get(path string) (string, bool)
}

// noChangeLevel is changeLevel's answer for a class with no occupation
// change left.
const noChangeLevel = math.MaxInt32

// lastClassID is the highest profession id the menus look through.
const lastClassID = 118

// Pages of a class manager, by the val of its <npcId>-<val>.htm.
const (
	pageNoChange = 1
	pageMenu     = 2
	pageComeBack = 3
	pageChanged  = 4
	pageNoble    = 5
	pageNowNoble = 6
)

// changeLevel is the level a class of tier (0 for a base class) must reach
// before its next occupation change: 20, 40 and 76, noChangeLevel past the
// third.
func changeLevel(tier int) int {
	switch tier {
	case 0:
		return 20
	case 1:
		return 40
	case 2:
		return 76
	}
	return noChangeLevel
}

// canTransfer reports whether a class of current may change to next: next
// must be one of current's own next occupations, or, with AllowEntireTree,
// any later occupation of its line.
func (c Config) canTransfer(current, next int) bool {
	parent, ok := player.ClassParent(next)
	if !ok {
		return false
	}
	if parent == current {
		return true
	}
	return c.AllowEntireTree && next != current && player.ClassEqualsOrChildOf(next, current)
}

// pagePath is the datapack path of a class manager's page: <npcId>.htm for
// val 0, <npcId>-<val>.htm otherwise.
func pagePath(npcID, val int) string {
	name := strconv.Itoa(npcID)
	if val != 0 {
		name += "-" + strconv.Itoa(val)
	}
	return "data/html/mods/classmaster/" + name + ".htm"
}

// page reads page val of the class manager npcID; a missing page reads as
// a "My html is missing" notice naming it.
func page(pages Pages, npcID, val int) string {
	path := pagePath(npcID, val)
	if html, ok := pages.Get(path); ok {
		return html
	}
	return "<html><body>My html is missing:<br>" + path + "</body></html>"
}

// MenuPage is the page the class manager npcID shows a talker of classID
// at level asking for occupation change tier: the classes it may change
// to, or why it may not. A tier not offered explains which change the
// manager does offer next, in plain text. The page's links name the
// manager by objectID, and it lists the items tier takes.
func (c Config) MenuPage(pages Pages, npcID int, objectID int32, classID, level, tier int) string {
	current, _ := player.ClassLevel(classID)
	var html string
	switch {
	case !c.Allowed(tier):
		html = "<html><body>" + c.closedText(current) + "</body></html>"
	case current >= tier:
		html = page(pages, npcID, pageNoChange)
	default:
		minLevel := changeLevel(current)
		switch {
		case level >= minLevel || c.AllowEntireTree:
			var menu strings.Builder
			for id := 0; id <= lastClassID; id++ {
				if t, ok := player.ClassLevel(id); !ok || t != tier || !c.canTransfer(classID, id) {
					continue
				}
				menu.WriteString(`<a action="bypass -h npc_%objectId%_change_class ` + strconv.Itoa(id) + `">` + player.ClassName(id) + `</a><br>`)
			}
			if menu.Len() > 0 {
				html = page(pages, npcID, pageMenu)
				html = strings.ReplaceAll(html, "%name%", player.ClassName(classID))
				html = strings.ReplaceAll(html, "%menu%", menu.String())
			} else {
				html = page(pages, npcID, pageComeBack)
				html = strings.ReplaceAll(html, "%level%", strconv.Itoa(changeLevel(tier-1)))
			}
		case minLevel < noChangeLevel:
			html = page(pages, npcID, pageComeBack)
			html = strings.ReplaceAll(html, "%level%", strconv.Itoa(minLevel))
		default:
			html = page(pages, npcID, pageNoChange)
		}
	}
	html = strings.ReplaceAll(html, "%objectId%", strconv.Itoa(int(objectID)))
	return strings.ReplaceAll(html, "%req_items%", c.requiredItemsHTML(tier))
}

// closedText tells a talker whose class is of tier current which change
// the manager offers next, when the one it asked for is not offered.
func (c Config) closedText(current int) string {
	const noChange = "I can't change your occupation.<br>"
	switch current {
	case 0:
		switch {
		case c.Allowed(1):
			return "Come back here when you reached level 20 to change your class.<br>"
		case c.Allowed(2):
			return "Come back after your first occupation change.<br>"
		case c.Allowed(3):
			return "Come back after your second occupation change.<br>"
		}
		return noChange
	case 1:
		switch {
		case c.Allowed(2):
			return "Come back here when you reached level 40 to change your class.<br>"
		case c.Allowed(3):
			return "Come back after your second occupation change.<br>"
		}
		return noChange
	case 2:
		if c.Allowed(3) {
			return "Come back here when you reached level 76 to change your class.<br>"
		}
		return noChange
	case 3:
		return "There is no class change available for you anymore.<br>"
	}
	return ""
}

// requiredItemsHTML lists the items occupation change tier takes as table
// rows, or a single "none" row.
func (c Config) requiredItemsHTML(tier int) string {
	job, _ := c.Job(tier)
	if len(job.Required) == 0 {
		return "<tr><td>none</td></tr>"
	}
	var b strings.Builder
	for _, it := range job.Required {
		b.WriteString(`<tr><td><font color="LEVEL">` + strconv.Itoa(it.Count) + `</font></td><td>&#` + strconv.Itoa(int(it.ID)) + `</td></tr>`)
	}
	return b.String()
}

// ChangedPage is the page confirming the change to classID.
func ChangedPage(pages Pages, npcID, classID int) string {
	return strings.ReplaceAll(page(pages, npcID, pageChanged), "%name%", player.ClassName(classID))
}

// NoblePage is the page answering a request for noblesse: that the talker
// already holds it, or that it now does.
func NoblePage(pages Pages, npcID int, alreadyNoble bool) string {
	if alreadyNoble {
		return page(pages, npcID, pageNoble)
	}
	return page(pages, npcID, pageNowNoble)
}
