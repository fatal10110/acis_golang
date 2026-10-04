package schemebuffer

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// pageLimit is the buffs one page of the scheme editor lists.
const pageLimit = 6

// commands are the scheme buffer's own dialog commands, matched as the
// start of the command's first word. Every other command is a plain NPC
// one.
var commands = []string{"menu", "cleanup", "heal", "support", "givebuffs", "editschemes", "skill", "createscheme", "deletescheme"}

// Command reports whether command is one the scheme buffer answers itself.
// A command with no word at all is too: it stops the dialog.
func Command(command string) bool {
	words := fields(command)
	if len(words) == 0 {
		return true
	}
	for _, c := range commands {
		if strings.HasPrefix(words[0], c) {
			return true
		}
	}
	return false
}

// Action is what a command does to the talker before its page.
type Action int

const (
	// ActionNone does nothing.
	ActionNone Action = iota
	// ActionCleanup ends every effect not lasting through death on the
	// talker, then on the talker's summon.
	ActionCleanup
	// ActionHeal fills the talker's CP, HP and MP, then the HP and MP of the
	// talker's summon.
	ActionHeal
	// ActionGive takes Cost adena from the talker, unless it is 0, then
	// lands Buffs on GiveTo.
	ActionGive
)

// GiveTarget is who a scheme's buffs go to.
type GiveTarget int

const (
	// GiveSelf buffs the talker.
	GiveSelf GiveTarget = iota
	// GivePet buffs the talker's summon.
	GivePet
	// GiveNobody names a target that is neither: the talker is told they
	// have no pet.
	GiveNobody
)

// NoPetMessage is the notice for a scheme given to a missing summon.
const NoPetMessage = "You don't have a pet."

// Talker is what a dialog command reads of the player sending it.
type Talker struct {
	ObjectID     int32
	MaxBuffCount int
}

// Reply is the scheme buffer's answer to one dialog command, carried out
// in field order: Action, then Message, then Page.
type Reply struct {
	// Aborted is a command too malformed to answer: nothing at all is
	// sent, not even the dialog's closing ActionFailed.
	Aborted bool
	Action  Action
	// GiveTo, Cost and Buffs are ActionGive's target, fee and buffs, the
	// latter in scheme order, each at the level the buffer gives it; a
	// buff whose skill level is not loaded is left out.
	GiveTo GiveTarget
	Cost   int32
	Buffs  []skill.Definition
	// Message is a plain-text notice, sent when set.
	Message string
	// Page is the window opened, with no NPC object id, when set.
	Page string
}

// Bypass answers command, the part of a dialog link after the NPC's object
// id, sent to the scheme buffer npcID (objectID) by talker. page loads a
// data/html page as an HTML window takes it when set.
func (m *Manager) Bypass(page func(path string) string, npcID int, objectID int32, talker Talker, command string) Reply {
	words := fields(command)
	if len(words) == 0 {
		return Reply{Aborted: true}
	}
	d := dialog{m: m, page: page, npcID: npcID, objectID: objectID, talker: talker}
	args := words[1:]
	m.mu.Lock()
	defer m.mu.Unlock()
	switch c := words[0]; {
	case strings.HasPrefix(c, "menu"):
		return Reply{Page: d.menu()}
	case strings.HasPrefix(c, "cleanup"):
		return Reply{Action: ActionCleanup, Page: d.menu()}
	case strings.HasPrefix(c, "heal"):
		return Reply{Action: ActionHeal, Page: d.menu()}
	case strings.HasPrefix(c, "support"):
		return Reply{Page: d.schemesPage()}
	case strings.HasPrefix(c, "givebuffs"):
		return d.give(args)
	case strings.HasPrefix(c, "editschemes"):
		if len(args) < 3 {
			return Reply{Aborted: true}
		}
		pageNo, err := commons.ParseInt(args[2], 32)
		if err != nil {
			return Reply{Aborted: true}
		}
		html, ok := d.editPage(args[0], args[1], int(pageNo))
		return Reply{Aborted: !ok, Page: html}
	case strings.HasPrefix(c, "skill"):
		return d.selectSkill(c, args)
	case strings.HasPrefix(c, "createscheme"):
		return d.create(args)
	default: // deletescheme
		return d.remove(args)
	}
}

// dialog is one command being answered, under the Manager's lock.
type dialog struct {
	m        *Manager
	page     func(path string) string
	npcID    int
	objectID int32
	talker   Talker
}

// path is the buffer's chat page val: <npcId>.htm for 0, else
// <npcId>-<val>.htm.
func (d dialog) path(val int) string {
	name := strconv.Itoa(d.npcID)
	if val != 0 {
		name += "-" + strconv.Itoa(val)
	}
	return "data/html/mods/buffer/" + name + ".htm"
}

func (d dialog) fillObjectID(html string) string {
	return strings.ReplaceAll(html, "%objectId%", strconv.Itoa(int(d.objectID)))
}

// menu is the buffer's first page.
func (d dialog) menu() string {
	return d.fillObjectID(d.page(d.path(0)))
}

// give answers givebuffs <scheme> <cost> [pet]: the scheme's buffs on the
// talker, or on the summon for a third word "pet" in any case; any other
// third word names nobody.
func (d dialog) give(args []string) Reply {
	if len(args) < 2 {
		return Reply{Aborted: true}
	}
	cost, err := commons.ParseInt(args[1], 32)
	if err != nil {
		return Reply{Aborted: true}
	}
	reply := Reply{Action: ActionGive, GiveTo: GiveSelf, Cost: int32(cost)}
	if len(args) > 2 {
		reply.GiveTo = GiveNobody
		if equalIgnoreCase(args[2], "pet") {
			reply.GiveTo = GivePet
		}
	}
	if s, ok := d.m.lookup(d.talker.ObjectID, args[0]); ok {
		for _, id := range s.skills {
			buff, ok := d.m.buff(id)
			if !ok || d.m.skills == nil {
				continue
			}
			if def, ok := d.m.skills.Definition(buff.Skill); ok {
				reply.Buffs = append(reply.Buffs, def)
			}
		}
	}
	return reply
}

// selectSkill answers skillselect and skillunselect <group> <scheme>
// <skillId> <page>: the skill is added to the scheme while it holds fewer
// buffs than the talker can carry, or taken out of it, then the editor
// shows that page again. Adding to a scheme the talker does not keep
// stops the dialog, unless the scheme is named "none".
func (d dialog) selectSkill(command string, args []string) Reply {
	if len(args) < 4 {
		return Reply{Aborted: true}
	}
	group, name := args[0], args[1]
	id, err := commons.ParseInt(args[2], 32)
	if err != nil {
		return Reply{Aborted: true}
	}
	pageNo, err := commons.ParseInt(args[3], 32)
	if err != nil {
		return Reply{Aborted: true}
	}
	var reply Reply
	s, kept := d.m.lookup(d.talker.ObjectID, name)
	switch {
	case strings.HasPrefix(command, "skillselect") && !equalIgnoreCase(name, "none"):
		count := 0
		if kept {
			count = len(s.skills)
		}
		if count >= d.talker.MaxBuffCount {
			reply.Message = "This scheme has reached the maximum amount of buffs."
			break
		}
		if !kept {
			return Reply{Aborted: true}
		}
		s.skills = append(s.skills, int32(id))
	case strings.HasPrefix(command, "skillunselect") && kept:
		for i, sk := range s.skills {
			if sk == int32(id) {
				s.skills = append(s.skills[:i:i], s.skills[i+1:]...)
				break
			}
		}
	}
	html, ok := d.editPage(group, name, int(pageNo))
	if !ok {
		return Reply{Aborted: true}
	}
	reply.Page = html
	return reply
}

// create answers createscheme <name>: a name of at most 14 characters the
// talker does not keep yet, while under the maximum, becomes an empty
// scheme, trimmed, and the schemes page opens. Every refusal is a notice
// alone.
func (d dialog) create(args []string) Reply {
	const nameNotice = "Scheme's name must contain up to 14 chars. Spaces are trimmed."
	if len(args) == 0 {
		return Reply{Message: nameNotice}
	}
	name := args[0]
	if utf16Len(name) > 14 {
		return Reply{Message: nameNotice}
	}
	if o := d.m.owners[d.talker.ObjectID]; o != nil {
		if len(o.schemes) == d.m.cfg.MaxSchemes {
			return Reply{Message: "Maximum schemes amount is already reached."}
		}
		if _, ok := o.find(name); ok {
			return Reply{Message: "The scheme name already exists."}
		}
	}
	d.m.setLocked(d.talker.ObjectID, strings.TrimFunc(name, javaSpace), nil)
	return Reply{Page: d.schemesPage()}
}

// remove answers deletescheme <name>: the talker's scheme of that name, in
// any case, is dropped, then the schemes page opens. A command naming no
// scheme is told so first.
func (d dialog) remove(args []string) Reply {
	var reply Reply
	if len(args) == 0 {
		reply.Message = "This scheme name is invalid."
	} else if o := d.m.owners[d.talker.ObjectID]; o != nil {
		if i, ok := o.find(args[0]); ok {
			o.schemes = append(o.schemes[:i:i], o.schemes[i+1:]...)
		}
	}
	reply.Page = d.schemesPage()
	return reply
}

// schemesPage is page 1: the talker's schemes with their fee and links.
func (d dialog) schemesPage() string {
	var b strings.Builder
	var schemes []*scheme
	if o := d.m.owners[d.talker.ObjectID]; o != nil {
		schemes = o.schemes
	}
	if len(schemes) == 0 {
		b.WriteString(`<font color="LEVEL">You haven't defined any scheme.</font>`)
	}
	maxBuffs := strconv.Itoa(d.talker.MaxBuffCount)
	for _, s := range schemes {
		cost := d.m.fee(s.skills)
		costText := strconv.Itoa(int(cost))
		b.WriteString(`<font color="LEVEL">` + s.name + ` [` + strconv.Itoa(len(s.skills)) + ` / ` + maxBuffs + `]`)
		if cost > 0 {
			b.WriteString(" - cost: " + formatNumber(int64(cost)))
		}
		b.WriteString(`</font><br1>`)
		b.WriteString(`<a action="bypass npc_%objectId%_givebuffs ` + s.name + ` ` + costText + `">Use on Me</a>&nbsp;|&nbsp;`)
		b.WriteString(`<a action="bypass npc_%objectId%_givebuffs ` + s.name + ` ` + costText + ` pet">Use on Pet</a>&nbsp;|&nbsp;`)
		b.WriteString(`<a action="bypass npc_%objectId%_editschemes Buffs ` + s.name + ` 1">Edit</a>&nbsp;|&nbsp;`)
		b.WriteString(`<a action="bypass npc_%objectId%_deletescheme ` + s.name + `">Delete</a><br>`)
	}
	html := d.page(d.path(1))
	html = strings.ReplaceAll(html, "%schemes%", commons.HTMLValue(b.String()))
	html = strings.ReplaceAll(html, "%max_schemes%", strconv.Itoa(d.m.cfg.MaxSchemes))
	return d.fillObjectID(html)
}

// editPage is page 2, the editor of scheme name showing page pageNo of the
// buffs of group. ok is false when the page cannot be built: a page number
// below 1, or a listed buff with no level 1 skill.
func (d dialog) editPage(group, name string, pageNo int) (string, bool) {
	count := 0
	if s, ok := d.m.lookup(d.talker.ObjectID, name); ok {
		count = len(s.skills)
	}
	types := d.typesFrame(group, name)
	list, ok := d.skillList(group, name, pageNo)
	if !ok {
		return "", false
	}
	html := d.page(d.path(2))
	html = strings.ReplaceAll(html, "%schemename%", commons.HTMLValue(name))
	html = strings.ReplaceAll(html, "%count%", strconv.Itoa(count)+" / "+strconv.Itoa(d.talker.MaxBuffCount))
	html = strings.ReplaceAll(html, "%typesframe%", commons.HTMLValue(types))
	html = strings.ReplaceAll(html, "%skilllistframe%", commons.HTMLValue(list))
	return d.fillObjectID(html), true
}

// typesFrame lists the buff groups four to a row, each linking to its
// first editor page but the group shown.
func (d dialog) typesFrame(group, name string) string {
	var b strings.Builder
	b.WriteString("<table>")
	count := 0
	var categories []string
	if d.m.buffs != nil {
		categories = d.m.buffs.Categories()
	}
	for _, c := range categories {
		if count == 0 {
			b.WriteString("<tr>")
		}
		if equalIgnoreCase(group, c) {
			b.WriteString(`<td width=65>` + c + `</td>`)
		} else {
			b.WriteString(`<td width=65><a action="bypass npc_%objectId%_editschemes ` + c + ` ` + name + ` 1">` + c + `</a></td>`)
		}
		count++
		if count == 4 {
			b.WriteString("</tr>")
			count = 0
		}
	}
	if !strings.HasSuffix(b.String(), "</tr>") {
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

// skillList lists page pageNo of group's buffs, a page past the last
// showing the last, each with a button adding it to scheme name or taking
// it out, then the page footer.
func (d dialog) skillList(group, name string, pageNo int) (string, bool) {
	var buffs []skill.BufferSkill
	if d.m.buffs != nil {
		for _, b := range d.m.buffs.Entries() {
			if equalIgnoreCase(b.Category, group) {
				buffs = append(buffs, b)
			}
		}
	}
	if len(buffs) == 0 {
		return "That group doesn't contain any skills.", true
	}
	pages := len(buffs) / pageLimit
	if len(buffs)%pageLimit != 0 {
		pages++
	}
	if pageNo > pages {
		pageNo = pages
	}
	if pageNo < 1 {
		return "", false
	}
	buffs = buffs[(pageNo-1)*pageLimit : min(pageNo*pageLimit, len(buffs))]
	var selected []int32
	if s, ok := d.m.lookup(d.talker.ObjectID, name); ok {
		selected = s.skills
	}
	pageText := strconv.Itoa(pageNo)
	var b strings.Builder
	for row, buff := range buffs {
		id := int32(buff.Skill.ID)
		def, ok := d.m.skillName(id)
		if !ok {
			return "", false
		}
		if row%2 == 0 {
			b.WriteString(`<table width="280" bgcolor="000000"><tr>`)
		} else {
			b.WriteString(`<table width="280"><tr>`)
		}
		command, back, fore := "skillselect", "L2UI_CH3.mapbutton_zoomin2", "L2UI_CH3.mapbutton_zoomin1"
		if contains(selected, id) {
			command, back, fore = "skillunselect", "L2UI_CH3.mapbutton_zoomout2", "L2UI_CH3.mapbutton_zoomout1"
		}
		b.WriteString(`<td height=40 width=40><img src="` + icon(id) + `" width=32 height=32></td><td width=190>` + def +
			`<br1><font color="B09878">` + buff.Description + `</font></td><td><button action="bypass npc_%objectId%_` + command + ` ` +
			group + ` ` + name + ` ` + strconv.Itoa(int(id)) + ` ` + pageText + `" width=32 height=32 back="` + back + `" fore="` + fore + `"></td>`)
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	for i := pageLimit; i > len(buffs); i-- {
		b.WriteString("<img height=41>")
	}
	link := `<a action="bypass npc_` + strconv.Itoa(int(d.objectID)) + `_editschemes ` + group + ` ` + name + ` `
	b.WriteString(`<br><img src="L2UI.SquareGray" width=280 height=1><table width="100%" bgcolor=000000><tr>`)
	if pageNo > 1 {
		b.WriteString(`<td align=left width=70>` + link + strconv.Itoa(pageNo-1) + `">Previous</a></td>`)
	} else {
		b.WriteString(`<td align=left width=70>Previous</td>`)
	}
	b.WriteString(`<td align=center width=100>Page ` + pageText + `</td>`)
	if pageNo < pages {
		b.WriteString(`<td align=right width=70>` + link + strconv.Itoa(pageNo+1) + `">Next</a></td>`)
	} else {
		b.WriteString(`<td align=right width=70>Next</td>`)
	}
	b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	return b.String(), true
}

// skillName is the name of skill id at level 1.
func (m *Manager) skillName(id int32) (string, bool) {
	if m.skills == nil {
		return "", false
	}
	def, ok := m.skills.Get(skill.ID(id), 1)
	return def.Name, ok
}

// fee is what giving ids costs: the static cost per buff when one is set,
// else the sum of the buffs' prices, in 32-bit arithmetic.
func (m *Manager) fee(ids []int32) int32 {
	if m.cfg.StaticCost > 0 {
		return int32(len(ids)) * int32(m.cfg.StaticCost)
	}
	var fee int32
	for _, id := range ids {
		if buff, ok := m.buff(id); ok {
			fee += int32(buff.Price)
		}
	}
	return fee
}

// icon is a skill's icon name: its id padded to at least three digits for
// ids below 1000.
func icon(id int32) string {
	switch {
	case id < 100:
		return "icon.skill00" + strconv.Itoa(int(id))
	case id < 1000:
		return "icon.skill0" + strconv.Itoa(int(id))
	default:
		return "icon.skill" + strconv.Itoa(int(id))
	}
}

func contains(ids []int32, id int32) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// formatNumber writes n with a comma between each group of three digits.
func formatNumber(n int64) string {
	s := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}

// fields splits command into its words, separated by spaces.
func fields(command string) []string {
	return strings.FieldsFunc(command, func(r rune) bool { return r == ' ' })
}

// equalIgnoreCase reports whether a and b match ignoring case, unit by
// UTF-16 unit.
func equalIgnoreCase(a, b string) bool { return compareIgnoreCase(a, b) == 0 }

// javaSpace is the set trimmed around a scheme name: every character up to
// and including the space.
func javaSpace(r rune) bool { return r <= ' ' }
