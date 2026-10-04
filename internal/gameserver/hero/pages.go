package hero

import (
	"slices"
	"strconv"
	"strings"
)

// The entries a diary page and a fight history page show.
const (
	diaryPerPage int32 = 10
	fightPerPage int32 = 20
)

// pageButton is the paging button of a hero page: Prev to an older page,
// Next to a newer one.
func pageButton(label, command string, heroClass, page int32) string {
	return `<button value="` + label + `" action="bypass ` + command + `?class=` + strconv.Itoa(int(heroClass)) +
		`&page=` + strconv.Itoa(int(page)) + `" width=60 height=25 back="L2UI_ct1.button_df" fore="L2UI_ct1.button_df">`
}

// DiaryPage fills template, the diary page, with page page of the diary
// as objectID, the hero heroName of class heroClass, shows it: newest
// first, ten to a page, under the hero's message. ok is false when the
// page is not sent: objectID has no diary page or no message, the page
// starts before the first entry, or it shows an entry without its text.
// The page numbers are 32-bit, wrapping as the reference's do.
func (m *Manager) DiaryPage(template, heroName string, heroClass, objectID, page int32) (string, bool) {
	m.mu.Lock()
	_, ok := m.records.diaryHeroes[objectID]
	message, hasMessage := m.records.messages[objectID]
	entries := slices.Clone(m.records.diary)
	m.mu.Unlock()
	if !ok || !hasMessage {
		return "", false
	}
	html, ok := replacePlaceholder(template, "%heroname%", heroName)
	if !ok {
		return "", false
	}
	if html, ok = replacePlaceholder(html, "%message%", message); !ok {
		return "", false
	}
	if len(entries) == 0 {
		return replacePlaceholders(html, "%list%", "", "%buttprev%", "", "%buttnext%", "")
	}
	slices.Reverse(entries)
	var (
		sb      strings.Builder
		color   = true
		counter int32
		breakAt int32
	)
	for i := (page - 1) * diaryPerPage; i < int32(len(entries)); i++ {
		if i < 0 {
			return "", false
		}
		breakAt = i
		e := entries[i]
		if !e.hasAction {
			return "", false
		}
		sb.WriteString("<tr><td>")
		if color {
			sb.WriteString(`<table width=270 bgcolor="131210">`)
		} else {
			sb.WriteString("<table width=270>")
		}
		sb.WriteString(`<tr><td width=270><font color="LEVEL">` + e.date + ":xx</font></td></tr><tr><td width=270>" + e.action +
			"</td></tr><tr><td>&nbsp;</td></tr></table></td></tr>")
		color = !color
		counter++
		if counter >= diaryPerPage {
			break
		}
	}
	prev, next := "", ""
	if breakAt < int32(len(entries))-1 {
		prev = pageButton("Prev", "_diary", heroClass, page+1)
	}
	if page > 1 {
		next = pageButton("Next", "_diary", heroClass, page-1)
	}
	return replacePlaceholders(html, "%buttprev%", prev, "%buttnext%", next, "%list%", sb.String())
}

// FightsPage fills template, the fight history page, with page page of the
// fights as objectID, the hero heroName of class heroClass, shows them:
// oldest first, twenty to a page, under its victories, draws and losses.
// ok is false when the page is not sent: objectID has no fight history,
// the page starts before the first fight, or it shows a fight without its
// result.
func (m *Manager) FightsPage(template, heroName string, heroClass, objectID, page int32) (string, bool) {
	m.mu.Lock()
	counts, ok := m.records.fightCounts[objectID]
	fights := slices.Clone(m.records.fights)
	m.mu.Unlock()
	if !ok {
		return "", false
	}
	html, ok := replacePlaceholder(template, "%heroname%", heroName)
	if !ok {
		return "", false
	}
	if len(fights) == 0 {
		counts = fightCounts{}
		if html, ok = replacePlaceholders(html, "%list%", "", "%buttprev%", "", "%buttnext%", ""); !ok {
			return "", false
		}
	} else {
		var (
			sb      strings.Builder
			color   = true
			counter int32
			breakAt int32
		)
		for i := (page - 1) * fightPerPage; i < int32(len(fights)); i++ {
			if i < 0 {
				return "", false
			}
			breakAt = i
			f := fights[i]
			if !f.hasResult {
				return "", false
			}
			sb.WriteString("<tr><td>")
			if color {
				sb.WriteString(`<table width=270 bgcolor="131210">`)
			} else {
				sb.WriteString(`<table width=270><tr><td width=220><font color="LEVEL">`)
			}
			sb.WriteString(f.start + "</font>&nbsp;&nbsp;" + f.result + "</td><td width=50 align=right>")
			if f.classed > 0 {
				sb.WriteString(`<font color="FFFF99">cls</font>`)
			} else {
				sb.WriteString(`<font color="999999">non-cls<font>`)
			}
			sb.WriteString("</td></tr><tr><td width=220>vs " + f.opponent + " (" + f.opponentClass + ")</td><td width=50 align=right>(" +
				f.time + ")</td></tr><tr><td colspan=2>&nbsp;</td></tr></table></td></tr>")
			color = !color
			counter++
			if counter >= fightPerPage {
				break
			}
		}
		prev, next := "", ""
		if breakAt < int32(len(fights))-1 {
			prev = pageButton("Prev", "_match", heroClass, page+1)
		}
		if page > 1 {
			next = pageButton("Next", "_match", heroClass, page-1)
		}
		if html, ok = replacePlaceholders(html, "%buttprev%", prev, "%buttnext%", next, "%list%", sb.String()); !ok {
			return "", false
		}
	}
	html = strings.ReplaceAll(html, "%win%", strconv.Itoa(counts.victories))
	html = strings.ReplaceAll(html, "%draw%", strconv.Itoa(counts.draws))
	return strings.ReplaceAll(html, "%loos%", strconv.Itoa(counts.losses)), true
}

// replacePlaceholders replaces each placeholder of pairs, in order, with
// the value after it, as replacePlaceholder does.
func replacePlaceholders(html string, pairs ...string) (string, bool) {
	for i := 0; i+1 < len(pairs); i += 2 {
		var ok bool
		if html, ok = replacePlaceholder(html, pairs[i], pairs[i+1]); !ok {
			return "", false
		}
	}
	return html, true
}

// replacePlaceholder replaces every placeholder in html with value the
// way the reference's page fill does: its '$' are escaped, then the value
// is read as a regular-expression replacement, so a backslash takes the
// next character as it is and "$0" stands for the placeholder itself. ok
// is false for a value that replacement refuses: one ending in an
// unpaired backslash, or naming any other group. The reference sends no
// page then.
func replacePlaceholder(html, placeholder, value string) (string, bool) {
	escaped := strings.ReplaceAll(value, "$", `\$`)
	var b strings.Builder
	for i := 0; i < len(escaped); i++ {
		switch c := escaped[i]; c {
		case '\\':
			i++
			if i == len(escaped) {
				return "", false
			}
			b.WriteByte(escaped[i])
		case '$':
			// The placeholder holds no group: only group 0, the match
			// itself, is there, and every further 0 still names it.
			i++
			if i == len(escaped) || escaped[i] != '0' {
				return "", false
			}
			for i+1 < len(escaped) && escaped[i+1] == '0' {
				i++
			}
			b.WriteString(placeholder)
		default:
			b.WriteByte(c)
		}
	}
	return strings.ReplaceAll(html, placeholder, b.String()), true
}
